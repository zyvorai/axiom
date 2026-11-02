package api

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
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

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
	"github.com/russellhaering/goxmldsig/etreeutils"

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

func TestSAMLACSRejectsGETBinding(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID: "https://sp.example.test", SAMLAllowUnsigned: true, SessionSecret: "test-session-secret",
	})
	w := getSAML(h, url.Values{"SAMLResponse": {b64XML(samlXML("bob", "", nil, nil))}})
	if w.Code == http.StatusFound {
		t.Fatalf("a SAMLResponse in the URL must not mint a session, got %d", w.Code)
	}
}

// --- real XML-DSig verification ---

type idp struct {
	key *rsa.PrivateKey
	der []byte
	pem string
}

func newIdP(t *testing.T, cn string) idp {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return idp{key: key, der: der, pem: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))}
}

const spEntity = "https://sp.example.test"

// signedSAML builds a Response whose Assertion is enveloped-signed by signer.
func signedSAML(t *testing.T, signer idp, nameID, audience string, notBefore, notOnOrAfter time.Time) string {
	t.Helper()
	xml := fmt.Sprintf(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion" ID="_resp1"><saml:Assertion ID="_assert1"><saml:Subject><saml:NameID>%s</saml:NameID></saml:Subject><saml:Conditions NotBefore="%s" NotOnOrAfter="%s"><saml:AudienceRestriction><saml:Audience>%s</saml:Audience></saml:AudienceRestriction></saml:Conditions></saml:Assertion></samlp:Response>`,
		nameID, notBefore.UTC().Format(time.RFC3339), notOnOrAfter.UTC().Format(time.RFC3339), audience)
	doc := etree.NewDocument()
	if err := doc.ReadFromString(xml); err != nil {
		t.Fatal(err)
	}
	assertion := doc.Root().FindElement("./Assertion")
	// Sign a self-contained copy: exclusive c14n must see the namespaces the
	// Assertion inherits from the Response, exactly as a real IdP produces it.
	nsctx, err := etreeutils.NSBuildParentContext(assertion)
	if err != nil {
		t.Fatal(err)
	}
	detached, err := etreeutils.NSDetatch(nsctx, assertion)
	if err != nil {
		t.Fatal(err)
	}
	sctx := dsig.NewDefaultSigningContext(dsig.TLSCertKeyStore(tls.Certificate{Certificate: [][]byte{signer.der}, PrivateKey: signer.key}))
	sctx.SetSignatureMethod(dsig.RSASHA256SignatureMethod)
	signed, err := sctx.SignEnveloped(detached)
	if err != nil {
		t.Fatal(err)
	}
	doc.Root().RemoveChild(assertion)
	doc.Root().AddChild(signed)
	out, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func samlHandler(pemCert string) http.Handler {
	return New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLEntityID: spEntity, SAMLIDPCert: pemCert, SessionSecret: "test-session-secret",
	})
}

func TestSAMLACSSignedAssertionAccepted(t *testing.T) {
	i := newIdP(t, "idp.example.test")
	x := signedSAML(t, i, "carol", spEntity, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	w := postSAML(samlHandler(i.pem), url.Values{"SAMLResponse": {b64XML(x)}})
	if w.Code != http.StatusFound {
		t.Fatalf("a correctly signed, in-window, right-audience assertion must be accepted, got %d: %s", w.Code, w.Body.String())
	}
}

// The forgery documented in GAPS.md: the IdP certificate is public, so an
// attacker embeds it in an UNSIGNED assertion. Pinning alone accepted this.
func TestSAMLACSRejectsUnsignedAssertionCarryingThePinnedCert(t *testing.T) {
	i := newIdP(t, "idp.example.test")
	forged := samlXML("admin", "", []string{"admins"}, i.der)
	w := postSAML(samlHandler(i.pem), url.Values{"SAMLResponse": {b64XML(forged)}})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("forged unsigned assertion must be rejected with 401, got %d: %s", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("no session cookie may be issued for a forged assertion")
	}
}

func TestSAMLACSRejectsAssertionSignedByAnotherKey(t *testing.T) {
	good, attacker := newIdP(t, "idp.example.test"), newIdP(t, "attacker.example.test")
	x := signedSAML(t, attacker, "mallory", spEntity, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	w := postSAML(samlHandler(good.pem), url.Values{"SAMLResponse": {b64XML(x)}})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("assertion signed by an unpinned key must be rejected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSAMLACSRejectsTamperedSignedAssertion(t *testing.T) {
	i := newIdP(t, "idp.example.test")
	x := signedSAML(t, i, "carol", spEntity, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	tampered := strings.Replace(x, ">carol<", ">admin<", 1)
	w := postSAML(samlHandler(i.pem), url.Values{"SAMLResponse": {b64XML(tampered)}})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("modifying a signed assertion must invalidate it, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSAMLACSRejectsWrongAudienceAndExpired(t *testing.T) {
	i := newIdP(t, "idp.example.test")
	cases := map[string]string{
		"wrong audience": signedSAML(t, i, "carol", "https://other-sp.example", time.Now().Add(-time.Minute), time.Now().Add(time.Hour)),
		"expired":        signedSAML(t, i, "carol", spEntity, time.Now().Add(-3*time.Hour), time.Now().Add(-time.Hour)),
		"not yet valid":  signedSAML(t, i, "carol", spEntity, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour)),
	}
	for name, x := range cases {
		w := postSAML(samlHandler(i.pem), url.Values{"SAMLResponse": {b64XML(x)}})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: want 401, got %d: %s", name, w.Code, w.Body.String())
		}
	}
}

// Signature wrapping: a genuinely signed assertion for "carol" plus an extra,
// unsigned assertion for "admin" placed first. The identity must come from the
// signed one only.
func TestSAMLACSSignatureWrappingUsesOnlySignedContent(t *testing.T) {
	i := newIdP(t, "idp.example.test")
	x := signedSAML(t, i, "carol", spEntity, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	evil := `<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"><saml:Subject><saml:NameID>admin</saml:NameID></saml:Subject></saml:Assertion>`
	wrapped := strings.Replace(x, `ID="_resp1">`, `ID="_resp1">`+evil, 1)
	signedBytes, err := verifySAMLSignature([]byte(wrapped), i.pem)
	if err == nil {
		user, _, _, perr := parseSAMLAssertion(signedBytes)
		if perr != nil || user != "carol" {
			t.Fatalf("identity must come from the signed assertion, got %q (%v)", user, perr)
		}
	}
	// (a rejection of the wrapped document is also an acceptable outcome)
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
		name                         string
		groups                       []string
		adminGroup, viewerGroup, def string
		want                         string
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
