package api

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// postSAML sends the SAMLResponse form value as a real IdP POST (application
// form-urlencoded body) would — httptest.NewRequest alone won't populate
// r.PostForm without an explicit Content-Type, since net/http only parses
// the body as a form when the header says so.
func postSAML(h http.Handler, form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/saml/acs", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func getSAML(h http.Handler, form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/saml/acs?"+form.Encode(), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// samlXML builds a minimal SAMLResponse document with a Subject NameID,
// optional email/groups attributes and an optional embedded X509Certificate
// (as verifySAMLIdPCert expects to find one for cert-pinning), matching the
// shape parseSAMLAssertion / verifySAMLIdPCert scan for in saml.go.
func samlXML(nameID, email string, groups []string, certDER []byte) string {
	var attrs strings.Builder
	if email != "" {
		fmt.Fprintf(&attrs, `<saml:Attribute Name="email"><saml:AttributeValue>%s</saml:AttributeValue></saml:Attribute>`, email)
	}
	for _, g := range groups {
		fmt.Fprintf(&attrs, `<saml:Attribute Name="groups"><saml:AttributeValue>%s</saml:AttributeValue></saml:Attribute>`, g)
	}
	sig := ""
	if len(certDER) > 0 {
		sig = fmt.Sprintf(`<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:KeyInfo><ds:X509Data><ds:X509Certificate>%s</ds:X509Certificate></ds:X509Data></ds:KeyInfo></ds:Signature>`,
			base64.StdEncoding.EncodeToString(certDER))
	}
	return fmt.Sprintf(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">
  <saml:Assertion>
    <saml:Subject><saml:NameID>%s</saml:NameID></saml:Subject>
    <saml:AttributeStatement>%s</saml:AttributeStatement>
    %s
  </saml:Assertion>
</samlp:Response>`, nameID, attrs.String(), sig)
}

func b64XML(xmlDoc string) string {
	return base64.StdEncoding.EncodeToString([]byte(xmlDoc))
}

// selfSignedCert generates a throwaway self-signed certificate for
// exercising verifySAMLIdPCert's pinning check. Returned as both PEM (what
// AXIOM_SAML_IDP_CERT / SAMLIDPCert holds) and raw DER (what gets embedded,
// base64-encoded, as the assertion's X509Certificate).
func selfSignedCert(t *testing.T, cn string) (pemStr string, der []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return string(pemBytes), der
}

func TestSAMLACSNotConfiguredReturnsNotImplemented(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := postSAML(h, url.Values{"SAMLResponse": {b64XML(samlXML("alice", "", nil, nil))}})
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 when SAML is not configured, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSAMLACSMissingSAMLResponse(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID: "https://sp.example.test", SAMLAllowUnsigned: true,
	})
	w := postSAML(h, url.Values{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing SAMLResponse, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "missing SAMLResponse") {
		t.Fatalf("expected missing SAMLResponse error, got: %s", w.Body.String())
	}
}

func TestSAMLACSInvalidEncodingRejected(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID: "https://sp.example.test", SAMLAllowUnsigned: true,
	})
	w := postSAML(h, url.Values{"SAMLResponse": {"not-valid-base64!!!"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for undecodable SAMLResponse, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid SAMLResponse encoding") {
		t.Fatalf("expected encoding error, got: %s", w.Body.String())
	}
}

func TestSAMLACSUnparsableAssertionRejected(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID: "https://sp.example.test", SAMLAllowUnsigned: true,
	})
	// Valid base64, but the decoded payload is neither XML nor a compressed
	// document that inflates to XML, so parseSAMLAssertion must fail cleanly.
	w := postSAML(h, url.Values{"SAMLResponse": {base64.StdEncoding.EncodeToString([]byte("definitely not xml"))}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unparsable assertion, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSAMLACSRejectsWithoutCertOrAllowUnsigned(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID: "https://sp.example.test",
		// No SAMLIDPCert and SAMLAllowUnsigned left false (the safe default).
	})
	w := postSAML(h, url.Values{"SAMLResponse": {b64XML(samlXML("alice", "alice@example.test", nil, nil))}})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when no cert is pinned and unsigned assertions aren't allowed, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "SAML IdP certificate required") {
		t.Fatalf("expected certificate-required error, got: %s", w.Body.String())
	}
}

func TestSAMLACSAllowUnsignedMintsSession(t *testing.T) {
	const secret = "test-session-secret"
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID:      "https://sp.example.test",
		SAMLAllowUnsigned: true,
		SessionSecret:     secret,
	})
	w := postSAML(h, url.Values{"SAMLResponse": {b64XML(samlXML("alice", "alice@example.test", nil, nil))}})
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect after a successful ACS, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/?sso=1" {
		t.Fatalf("expected redirect to /?sso=1, got %q", loc)
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.SessionCookie {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatalf("expected a session cookie to be set")
	}
	sess, ok := auth.ParseSession(secret, cookie.Value)
	if !ok {
		t.Fatalf("session cookie did not parse with the configured secret")
	}
	if sess.User != "alice" {
		t.Fatalf("expected user alice, got %q", sess.User)
	}
	if sess.Email != "alice@example.test" {
		t.Fatalf("expected email alice@example.test, got %q", sess.Email)
	}
	// No admin/viewer group mapping configured -> normalizeDefaultRole("") -> admin.
	if sess.Role != "admin" {
		t.Fatalf("expected default role admin, got %q", sess.Role)
	}
}

func TestSAMLACSGetBindingSupported(t *testing.T) {
	const secret = "test-session-secret"
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID:      "https://sp.example.test",
		SAMLAllowUnsigned: true,
		SessionSecret:     secret,
	})
	w := getSAML(h, url.Values{"SAMLResponse": {b64XML(samlXML("bob", "", nil, nil))}})
	if w.Code != http.StatusFound {
		t.Fatalf("expected the GET/HTTP-Redirect binding to also succeed, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSAMLACSNoUserFallsBackToSAMLUser(t *testing.T) {
	const secret = "test-session-secret"
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID:      "https://sp.example.test",
		SAMLAllowUnsigned: true,
		SessionSecret:     secret,
	})
	// Empty NameID and no email/uid attribute at all.
	w := postSAML(h, url.Values{"SAMLResponse": {b64XML(samlXML("", "", nil, nil))}})
	if w.Code != http.StatusFound {
		t.Fatalf("expected success even with no NameID, got %d: %s", w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.SessionCookie {
			sess, ok := auth.ParseSession(secret, c.Value)
			if !ok || sess.User != "saml-user" {
				t.Fatalf("expected fallback user 'saml-user', got %+v (ok=%v)", sess, ok)
			}
			return
		}
	}
	t.Fatalf("expected a session cookie")
}

// samlXMLBareCert embeds the certificate as a bare, unprefixed
// <X509Certificate> element directly under <Assertion> rather than inside
// the standard XML-DSig <ds:Signature><ds:KeyInfo><ds:X509Data> nesting.
// See TestSAMLACSCertPinningBrokenForStandardXMLDSigNesting below for why:
// with the realistic nesting, extractXMLTag anchors on the wrong ancestor
// tag and corrupts the extracted value. This shape isolates and exercises
// the actual sha256 comparison in verifySAMLIdPCert on its own.
func samlXMLBareCert(nameID string, certDER []byte) string {
	return fmt.Sprintf(`<Response><Assertion><Subject><NameID>%s</NameID></Subject><X509Certificate>%s</X509Certificate></Assertion></Response>`,
		nameID, base64.StdEncoding.EncodeToString(certDER))
}

func TestSAMLACSCertPinningAcceptsMatchingCertWhenExtractable(t *testing.T) {
	pemCert, der := selfSignedCert(t, "idp.example.test")
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID: "https://sp.example.test",
		SAMLIDPCert:  pemCert,
	})
	w := postSAML(h, url.Values{"SAMLResponse": {b64XML(samlXMLBareCert("carol", der))}})
	if w.Code != http.StatusFound {
		t.Fatalf("expected success when the embedded cert matches the pinned IdP cert, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSAMLACSCertPinningRejectsMismatchedCert(t *testing.T) {
	pinnedPEM, _ := selfSignedCert(t, "idp.example.test")
	_, otherDER := selfSignedCert(t, "attacker.example.test")
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID: "https://sp.example.test",
		SAMLIDPCert:  pinnedPEM,
	})
	w := postSAML(h, url.Values{"SAMLResponse": {b64XML(samlXMLBareCert("mallory", otherDER))}})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when the embedded cert does not match the pinned one, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "does not match") {
		t.Fatalf("expected a certificate mismatch error, got: %s", w.Body.String())
	}
}

// BUG (found while writing this test, not fixed per instructions):
// verifySAMLIdPCert extracts the embedded certificate via extractXMLTag,
// which has a real matching bug for namespaced tags: rather than requiring
// a match to start exactly at the target tag's own '<', it scans a 64-byte
// lookahead window from *every* preceding '<' in the whole document for the
// substring ":X509Certificate>" and stops at the first hit, even if that
// '<' belongs to an ancestor element. In the standard XML-DSig nesting that
// essentially every real IdP uses to sign a SAMLResponse —
// <ds:Signature><ds:KeyInfo><ds:X509Data><ds:X509Certificate>...— the '<'
// that opens <ds:KeyInfo> (or <ds:X509Data>) is within that 64-byte window
// of ":X509Certificate>", so extraction anchors on the wrong tag's '>' and
// the "certificate" it reads back is corrupted with a literal
// "<ds:X509Data><ds:X509Certificate>" prefix glued onto the real base64.
// That fails to base64-decode, so verifySAMLIdPCert rejects it as "bad
// X509Certificate encoding" — meaning AXIOM_SAML_IDP_CERT pinning as
// currently implemented will reject realistically-shaped, correctly signed
// SAMLResponses even when the embedded certificate exactly matches the
// pinned one. See TestExtractXMLTagNamespacedTagNearAncestorIsCorrupted
// for a minimal reproduction of the underlying extractXMLTag bug.
func TestSAMLACSCertPinningWorksForStandardXMLDSigNesting(t *testing.T) {
	pemCert, der := selfSignedCert(t, "idp.example.test")
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID: "https://sp.example.test",
		SAMLIDPCert:  pemCert,
	})
	w := postSAML(h, url.Values{"SAMLResponse": {b64XML(samlXML("carol", "carol@example.test", nil, der))}})
	if w.Code != http.StatusFound {
		t.Fatalf("expected a matching cert wrapped in standard XML-DSig nesting (<ds:Signature><ds:KeyInfo><ds:X509Data><ds:X509Certificate>) to verify and redirect, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSAMLACSCertPinningRejectsAssertionWithNoCert(t *testing.T) {
	pinnedPEM, _ := selfSignedCert(t, "idp.example.test")
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID: "https://sp.example.test",
		SAMLIDPCert:  pinnedPEM,
	})
	w := postSAML(h, url.Values{"SAMLResponse": {b64XML(samlXML("eve", "", nil, nil))}})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when a cert is pinned but the assertion carries none, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no X509Certificate") {
		t.Fatalf("expected a missing-certificate error, got: %s", w.Body.String())
	}
}

// --- unit tests for the pure helper functions in saml.go ---

func TestDecodeSAMLResponseStdBase64XML(t *testing.T) {
	xmlDoc := "<Response><Assertion/></Response>"
	got, err := decodeSAMLResponse(base64.StdEncoding.EncodeToString([]byte(xmlDoc)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != xmlDoc {
		t.Fatalf("expected %q, got %q", xmlDoc, got)
	}
}

func TestDecodeSAMLResponseRawBase64XML(t *testing.T) {
	xmlDoc := "<Response><Assertion/></Response>"
	got, err := decodeSAMLResponse(base64.RawStdEncoding.EncodeToString([]byte(xmlDoc)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != xmlDoc {
		t.Fatalf("expected %q, got %q", xmlDoc, got)
	}
}

func TestDecodeSAMLResponseInvalidBase64Errors(t *testing.T) {
	if _, err := decodeSAMLResponse("!!!not base64 at all!!!"); err == nil {
		t.Fatalf("expected an error for undecodable input")
	}
}

func TestDecodeSAMLResponseDeflateCompressed(t *testing.T) {
	xmlDoc := "<Response><Assertion/></Response>"
	var buf bytes.Buffer
	fw, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("new flate writer: %v", err)
	}
	if _, err := fw.Write([]byte(xmlDoc)); err != nil {
		t.Fatalf("flate write: %v", err)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("flate close: %v", err)
	}
	got, err := decodeSAMLResponse(base64.StdEncoding.EncodeToString(buf.Bytes()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != xmlDoc {
		t.Fatalf("expected inflated %q, got %q", xmlDoc, got)
	}
}

func TestParseSAMLAssertionExtractsSubjectAndAttributes(t *testing.T) {
	xmlDoc := samlXML("dave", "dave@example.test", []string{"platform-admins", "engineers"}, nil)
	user, email, groups, err := parseSAMLAssertion([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != "dave" {
		t.Fatalf("expected user dave, got %q", user)
	}
	if email != "dave@example.test" {
		t.Fatalf("expected email dave@example.test, got %q", email)
	}
	if len(groups) != 2 || groups[0] != "platform-admins" || groups[1] != "engineers" {
		t.Fatalf("expected both groups, got %v", groups)
	}
}

func TestParseSAMLAssertionFallsBackToEmailAsUser(t *testing.T) {
	xmlDoc := samlXML("", "noname@example.test", nil, nil)
	user, email, _, err := parseSAMLAssertion([]byte(xmlDoc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != email {
		t.Fatalf("expected user to fall back to email, got user=%q email=%q", user, email)
	}
}

func TestParseSAMLAssertionNoAssertionErrors(t *testing.T) {
	if _, _, _, err := parseSAMLAssertion([]byte(`<Response></Response>`)); err == nil {
		t.Fatalf("expected an error when the document has no Assertion")
	}
}

func TestResolveSAMLRole(t *testing.T) {
	cases := []struct {
		name                        string
		groups                      []string
		adminGroup, viewerGroup, def string
		want                        string
	}{
		{"no mapping configured defaults per defaultRole", []string{"anything"}, "", "", "viewer", "viewer"},
		{"no mapping configured falls back to admin by default", []string{"anything"}, "", "", "", "admin"},
		{"admin group match", []string{"engineers", "platform-admins"}, "platform-admins", "platform-viewers", "", "admin"},
		{"viewer group match", []string{"platform-viewers"}, "platform-admins", "platform-viewers", "", "viewer"},
		{"operator group fallback", []string{"axiom-operator"}, "platform-admins", "platform-viewers", "", "operator"},
		{"unmatched group defaults to viewer", []string{"unrelated"}, "platform-admins", "platform-viewers", "", "viewer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveSAMLRole(c.groups, c.adminGroup, c.viewerGroup, c.def)
			if got != c.want {
				t.Fatalf("resolveSAMLRole(%v, %q, %q, %q) = %q, want %q", c.groups, c.adminGroup, c.viewerGroup, c.def, got, c.want)
			}
		})
	}
}

// BUG (documented, not fixed — see the long comment on
// TestSAMLACSCertPinningBrokenForStandardXMLDSigNesting for the full
// picture): when the tag being searched for is namespace-prefixed and a
// short-enough ancestor tag precedes it within 64 bytes, extractXMLTag
// anchors on the ancestor's '<' instead of the target tag's own '<' and
// returns corrupted content — here the literal opening tag text
// "<ds:X509Certificate>" ends up prepended to the real value instead of
// being stripped.
// Regression test for a real bug: extractXMLTag used to search for the
// target tag name as a substring within a fixed 64-byte lookahead window
// from every preceding '<', which let a short enclosing ancestor tag (e.g.
// <ds:KeyInfo> or <ds:X509Data>) "reach through" to a nested target tag
// close enough behind it, anchoring on the ancestor's '<' instead of the
// target's and corrupting the extracted content with the ancestor's own
// literal markup. Fixed to match the tag by its exact name token instead.
func TestExtractXMLTagNamespacedTagNearAncestorIsClean(t *testing.T) {
	doc := `<ds:Signature xmlns:ds="x"><ds:X509Certificate>abc123</ds:X509Certificate></ds:Signature>`
	if got := extractXMLTag([]byte(doc), "X509Certificate"); got != "abc123" {
		t.Fatalf("expected clean content abc123, got %q", got)
	}
}

// Distance from the enclosing ancestor tag must not matter now that
// extractXMLTag matches by exact tag-name token rather than a fixed-size
// lookahead window.
func TestExtractXMLTagNamespacedTagFarFromAncestorWorks(t *testing.T) {
	doc := `<foo:Signature xmlns:foo="urn:example:padding:to:push:this:well:past:sixty:four:characters:of:distance"><foo:X509Certificate>abc123</foo:X509Certificate></foo:Signature>`
	if got := extractXMLTag([]byte(doc), "X509Certificate"); got != "abc123" {
		t.Fatalf("expected clean content abc123, got %q", got)
	}
}

func TestExtractXMLTagMissingReturnsEmpty(t *testing.T) {
	if got := extractXMLTag([]byte(`<Response></Response>`), "X509Certificate"); got != "" {
		t.Fatalf("expected empty string for a missing tag, got %q", got)
	}
}
