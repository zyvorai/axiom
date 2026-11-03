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
	"io"
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

const (
	testIdPSSO        = "https://idp.example.test/sso"
	testSessionSecret = "test-session-secret"
	// httptest requests have Host example.com and no TLS, so publicBase is
	// http://example.com.
	testACSURL = "http://example.com" + samlACSPath
)

func postSAMLWithCookie(h http.Handler, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/saml/acs", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		if c != nil {
			r.AddCookie(c)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// samlLoginStart runs GET /api/v1/auth/saml/login and returns the ID of the
// AuthnRequest it sent to the IdP plus the axiom_saml_req cookie it set.
func samlLoginStart(t *testing.T, h http.Handler) (string, *http.Cookie) {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/saml/login?next=/pods", nil))
	if w.Code != http.StatusFound {
		t.Fatalf("saml/login: want 302, got %d: %s", w.Code, w.Body.String())
	}
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if got := loc.Scheme + "://" + loc.Host + loc.Path; got != testIdPSSO {
		t.Fatalf("saml/login redirected to %q, want the IdP SSO URL", got)
	}
	if rs := loc.Query().Get("RelayState"); rs != "/pods" {
		t.Fatalf("RelayState = %q, want /pods", rs)
	}
	deflated, err := base64.StdEncoding.DecodeString(loc.Query().Get("SAMLRequest"))
	if err != nil {
		t.Fatalf("SAMLRequest is not base64: %v", err)
	}
	reqXML, err := io.ReadAll(flate.NewReader(bytes.NewReader(deflated)))
	if err != nil {
		t.Fatalf("SAMLRequest is not DEFLATE-compressed: %v", err)
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(reqXML); err != nil {
		t.Fatalf("SAMLRequest is not XML: %v", err)
	}
	root := doc.Root()
	if root.Tag != "AuthnRequest" {
		t.Fatalf("SAMLRequest root = %q, want AuthnRequest", root.Tag)
	}
	id := root.SelectAttrValue("ID", "")
	if len(id) < 2 || id[0] != '_' {
		t.Fatalf("AuthnRequest ID %q is not an NCName of the form _<hex>", id)
	}
	if acs := root.SelectAttrValue("AssertionConsumerServiceURL", ""); acs != testACSURL {
		t.Fatalf("AssertionConsumerServiceURL = %q, want %q", acs, testACSURL)
	}
	if dest := root.SelectAttrValue("Destination", ""); dest != testIdPSSO {
		t.Fatalf("Destination = %q, want %q", dest, testIdPSSO)
	}
	var c *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == samlReqCookie {
			c = ck
		}
	}
	if c == nil {
		t.Fatal("saml/login did not set the axiom_saml_req cookie")
	}
	return id, c
}

// withInResponseTo adds InResponseTo to the root samlp:Response of an
// (unsigned) samlXML document.
func withInResponseTo(doc, id string) string {
	return strings.Replace(doc, "<samlp:Response ", fmt.Sprintf(`<samlp:Response InResponseTo="%s" `, id), 1)
}

// bearerSC is a bearer SubjectConfirmation; empty inResponseTo / recipient
// leave the attribute out.
func bearerSC(inResponseTo, recipient string, notOnOrAfter time.Time) string {
	return subjectConfirmation(samlBearerMethod, inResponseTo, recipient, notOnOrAfter)
}

func subjectConfirmation(method, inResponseTo, recipient string, notOnOrAfter time.Time) string {
	attrs := ""
	if inResponseTo != "" {
		attrs += fmt.Sprintf(` InResponseTo="%s"`, inResponseTo)
	}
	if recipient != "" {
		attrs += fmt.Sprintf(` Recipient="%s"`, recipient)
	}
	attrs += fmt.Sprintf(` NotOnOrAfter="%s"`, notOnOrAfter.UTC().Format(time.RFC3339))
	return fmt.Sprintf(`<saml:SubjectConfirmation Method="%s"><saml:SubjectConfirmationData%s/></saml:SubjectConfirmation>`, method, attrs)
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
		SAMLLogin:         testIdPSSO,
		SAMLEntityID:      "https://sp.example.test",
		SAMLAllowUnsigned: true,
		SessionSecret:     secret,
	})
	id, reqCookie := samlLoginStart(t, h)
	w := postSAMLWithCookie(h, url.Values{"SAMLResponse": {b64XML(withInResponseTo(samlXML("alice", "alice@example.test", nil, nil), id))}}, reqCookie)
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

// signedSAML builds a Response whose Assertion is enveloped-signed by signer,
// with no SubjectConfirmation and no InResponseTo.
func signedSAML(t *testing.T, signer idp, nameID, audience string, notBefore, notOnOrAfter time.Time) string {
	t.Helper()
	return signedSAMLWith(t, signer, nameID, audience, notBefore, notOnOrAfter, "", "")
}

// signedSAMLWith is signedSAML plus a raw SubjectConfirmation element placed
// inside the (signed) Subject, and an InResponseTo on the (unsigned, when
// only the Assertion is signed) outer Response.
func signedSAMLWith(t *testing.T, signer idp, nameID, audience string, notBefore, notOnOrAfter time.Time, subjectConfirmation, outerInResponseTo string) string {
	t.Helper()
	outer := ""
	if outerInResponseTo != "" {
		outer = fmt.Sprintf(` InResponseTo="%s"`, outerInResponseTo)
	}
	xml := fmt.Sprintf(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion" ID="_resp1"%s><saml:Assertion ID="_assert1"><saml:Subject><saml:NameID>%s</saml:NameID>%s</saml:Subject><saml:Conditions NotBefore="%s" NotOnOrAfter="%s"><saml:AudienceRestriction><saml:Audience>%s</saml:Audience></saml:AudienceRestriction></saml:Conditions></saml:Assertion></samlp:Response>`,
		outer, nameID, subjectConfirmation, notBefore.UTC().Format(time.RFC3339), notOnOrAfter.UTC().Format(time.RFC3339), audience)
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
	return samlHandlerIdPInit(pemCert, false)
}

func samlHandlerIdPInit(pemCert string, allowIdPInitiated bool) http.Handler {
	return New(platform.NewDemoProvider(), "demo", "demo", "demo", "", AuthOptions{
		SAMLLogin: testIdPSSO, SAMLEntityID: spEntity, SAMLIDPCert: pemCert, SessionSecret: testSessionSecret,
		SAMLAllowIdPInitiated: allowIdPInitiated,
	})
}

// signedForLogin is a well-formed SP-initiated response to AuthnRequest id:
// bearer SubjectConfirmation addressed to this ACS, InResponseTo id on both
// the SubjectConfirmationData and the Response.
func signedForLogin(t *testing.T, signer idp, nameID, audience string, notBefore, notOnOrAfter time.Time, id string) string {
	t.Helper()
	return signedSAMLWith(t, signer, nameID, audience, notBefore, notOnOrAfter, bearerSC(id, testACSURL, time.Now().Add(5*time.Minute)), id)
}

// acsAfterLogin runs GET /saml/login, builds the response for the issued
// AuthnRequest ID and POSTs it to the ACS with the request cookie.
func acsAfterLogin(t *testing.T, h http.Handler, build func(id string) string) *httptest.ResponseRecorder {
	t.Helper()
	id, c := samlLoginStart(t, h)
	return postSAMLWithCookie(h, url.Values{"SAMLResponse": {b64XML(build(id))}}, c)
}

func TestSAMLACSSignedAssertionAccepted(t *testing.T) {
	i := newIdP(t, "idp.example.test")
	w := acsAfterLogin(t, samlHandler(i.pem), func(id string) string {
		return signedForLogin(t, i, "carol", spEntity, time.Now().Add(-time.Minute), time.Now().Add(time.Hour), id)
	})
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
	for _, c := range w.Result().Cookies() {
		// The only cookie allowed is the cleared axiom_saml_req.
		if c.Name != samlReqCookie || c.MaxAge >= 0 {
			t.Fatalf("no session cookie may be issued for a forged assertion, got %+v", c)
		}
	}
}

func TestSAMLACSRejectsAssertionSignedByAnotherKey(t *testing.T) {
	good, attacker := newIdP(t, "idp.example.test"), newIdP(t, "attacker.example.test")
	w := acsAfterLogin(t, samlHandler(good.pem), func(id string) string {
		return signedForLogin(t, attacker, "mallory", spEntity, time.Now().Add(-time.Minute), time.Now().Add(time.Hour), id)
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("assertion signed by an unpinned key must be rejected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSAMLACSRejectsTamperedSignedAssertion(t *testing.T) {
	i := newIdP(t, "idp.example.test")
	w := acsAfterLogin(t, samlHandler(i.pem), func(id string) string {
		x := signedForLogin(t, i, "carol", spEntity, time.Now().Add(-time.Minute), time.Now().Add(time.Hour), id)
		return strings.Replace(x, ">carol<", ">admin<", 1)
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("modifying a signed assertion must invalidate it, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSAMLACSRejectsWrongAudienceAndExpired(t *testing.T) {
	i := newIdP(t, "idp.example.test")
	type window struct {
		audience        string
		notBefore, nooa time.Time
	}
	cases := map[string]window{
		"wrong audience": {"https://other-sp.example", time.Now().Add(-time.Minute), time.Now().Add(time.Hour)},
		"expired":        {spEntity, time.Now().Add(-3 * time.Hour), time.Now().Add(-time.Hour)},
		"not yet valid":  {spEntity, time.Now().Add(time.Hour), time.Now().Add(2 * time.Hour)},
	}
	for name, c := range cases {
		w := acsAfterLogin(t, samlHandler(i.pem), func(id string) string {
			return signedForLogin(t, i, "carol", c.audience, c.notBefore, c.nooa, id)
		})
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: want 401, got %d: %s", name, w.Code, w.Body.String())
		}
	}
}

// InResponseTo request tracking and SubjectConfirmationData checks.
func TestSAMLACSInResponseToAndSubjectConfirmation(t *testing.T) {
	i := newIdP(t, "idp.example.test")
	nb, nooa := time.Now().Add(-time.Minute), time.Now().Add(time.Hour)
	soon := time.Now().Add(5 * time.Minute)
	good := func(id string) string { return signedForLogin(t, i, "carol", spEntity, nb, nooa, id) }
	keep := func(id string, c *http.Cookie) *http.Cookie { return c }

	cases := []struct {
		name     string
		allowIdP bool
		build    func(id string) string
		// cookie picks the axiom_saml_req cookie sent with the ACS POST.
		cookie func(id string, c *http.Cookie) *http.Cookie
		// prep runs between login and the ACS POST.
		prep     func(t *testing.T, h http.Handler, form url.Values, c *http.Cookie)
		want     int
		wantBody string
	}{
		{name: "valid SP-initiated flow", build: good, cookie: keep, want: http.StatusFound},
		{name: "valid with InResponseTo only on SubjectConfirmationData", cookie: keep, want: http.StatusFound,
			build: func(id string) string {
				return signedSAMLWith(t, i, "carol", spEntity, nb, nooa, bearerSC(id, testACSURL, soon), "")
			}},
		{name: "wrong InResponseTo", cookie: keep, want: http.StatusUnauthorized, wantBody: "does not match the sign-in",
			build: func(string) string { return good("_0123456789abcdef") }},
		{name: "replay of a consumed response", build: good, cookie: keep, want: http.StatusUnauthorized, wantBody: "already used",
			prep: func(t *testing.T, h http.Handler, form url.Values, c *http.Cookie) {
				if w := postSAMLWithCookie(h, form, c); w.Code != http.StatusFound {
					t.Fatalf("first use must succeed, got %d: %s", w.Code, w.Body.String())
				}
			}},
		{name: "missing request cookie", build: good, want: http.StatusUnauthorized, wantBody: "missing axiom_saml_req",
			cookie: func(string, *http.Cookie) *http.Cookie { return nil }},
		{name: "cookie signed with another secret", build: good, want: http.StatusUnauthorized, wantBody: "cookie is invalid",
			cookie: func(id string, c *http.Cookie) *http.Cookie {
				return &http.Cookie{Name: samlReqCookie, Value: signSAMLRequestCookie("another-secret", id, soon)}
			}},
		{name: "tampered cookie ID", build: good, want: http.StatusUnauthorized, wantBody: "cookie is invalid",
			cookie: func(id string, c *http.Cookie) *http.Cookie {
				// Swap in another ID but keep the original expiry and MAC.
				return &http.Cookie{Name: samlReqCookie, Value: "_deadbeef" + c.Value[len(id):]}
			}},
		{name: "expired request (cookie and outstanding entry)", build: good, cookie: keep, want: http.StatusUnauthorized, wantBody: "expired",
			prep: func(t *testing.T, _ http.Handler, _ url.Values, _ *http.Cookie) {
				advanceSAMLClock(t, samlReqTTL+time.Minute)
			}},
		{name: "expired outstanding entry, cookie still valid", build: good, want: http.StatusUnauthorized, wantBody: "unknown, expired or already used",
			cookie: func(id string, c *http.Cookie) *http.Cookie {
				return &http.Cookie{Name: samlReqCookie, Value: signSAMLRequestCookie(testSessionSecret, id, time.Now().Add(time.Hour))}
			},
			prep: func(t *testing.T, _ http.Handler, _ url.Values, _ *http.Cookie) {
				advanceSAMLClock(t, samlReqTTL+time.Minute)
			}},
		{name: "IdP-initiated rejected by default", cookie: keep, want: http.StatusUnauthorized, wantBody: "AXIOM_SAML_ALLOW_IDP_INITIATED",
			build: func(string) string {
				return signedSAMLWith(t, i, "carol", spEntity, nb, nooa, bearerSC("", testACSURL, soon), "")
			}},
		{name: "IdP-initiated accepted with the flag", allowIdP: true, want: http.StatusFound,
			cookie: func(string, *http.Cookie) *http.Cookie { return nil },
			build: func(string) string {
				return signedSAMLWith(t, i, "carol", spEntity, nb, nooa, bearerSC("", testACSURL, soon), "")
			}},
		{name: "IdP-initiated with the flag still checks Recipient", allowIdP: true, want: http.StatusUnauthorized, wantBody: "Recipient",
			cookie: func(string, *http.Cookie) *http.Cookie { return nil },
			build: func(string) string {
				return signedSAMLWith(t, i, "carol", spEntity, nb, nooa, bearerSC("", "https://other-sp.example/acs", soon), "")
			}},
		{name: "InResponseTo only on the unsigned outer Response is not trusted", cookie: keep, want: http.StatusUnauthorized, wantBody: "unsolicited",
			build: func(id string) string { return signedSAMLWith(t, i, "carol", spEntity, nb, nooa, "", id) }},
		{name: "outer Response InResponseTo contradicts the signed one", cookie: keep, want: http.StatusUnauthorized, wantBody: "does not match the signed assertion",
			build: func(id string) string {
				return signedSAMLWith(t, i, "carol", spEntity, nb, nooa, bearerSC(id, testACSURL, soon), "_0123456789abcdef")
			}},
		{name: "wrong Recipient", cookie: keep, want: http.StatusUnauthorized, wantBody: "Recipient",
			build: func(id string) string {
				return signedSAMLWith(t, i, "carol", spEntity, nb, nooa, bearerSC(id, "https://other-sp.example/acs", soon), id)
			}},
		{name: "expired SubjectConfirmationData NotOnOrAfter", cookie: keep, want: http.StatusUnauthorized, wantBody: "SubjectConfirmationData has expired",
			build: func(id string) string {
				return signedSAMLWith(t, i, "carol", spEntity, nb, nooa, bearerSC(id, testACSURL, time.Now().Add(-10*time.Minute)), id)
			}},
		{name: "non-bearer SubjectConfirmation Method", cookie: keep, want: http.StatusUnauthorized, wantBody: "only bearer",
			build: func(id string) string {
				sc := subjectConfirmation("urn:oasis:names:tc:SAML:2.0:cm:holder-of-key", id, testACSURL, soon)
				return signedSAMLWith(t, i, "carol", spEntity, nb, nooa, sc, id)
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := samlHandlerIdPInit(i.pem, tc.allowIdP)
			id, loginCookie := samlLoginStart(t, h)
			form := url.Values{"SAMLResponse": {b64XML(tc.build(id))}}
			c := tc.cookie(id, loginCookie)
			if tc.prep != nil {
				tc.prep(t, h, form, c)
			}
			w := postSAMLWithCookie(h, form, c)
			if w.Code != tc.want {
				t.Fatalf("want %d, got %d: %s", tc.want, w.Code, w.Body.String())
			}
			if tc.wantBody != "" && !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Fatalf("want body containing %q, got: %s", tc.wantBody, w.Body.String())
			}
			cleared, session := false, false
			for _, ck := range w.Result().Cookies() {
				switch ck.Name {
				case samlReqCookie:
					cleared = ck.MaxAge < 0
				case auth.SessionCookie:
					session = ck.Value != ""
				}
			}
			if !cleared {
				t.Error("every ACS attempt must clear the axiom_saml_req cookie")
			}
			if session != (tc.want == http.StatusFound) {
				t.Errorf("session cookie issued = %v, want %v", session, tc.want == http.StatusFound)
			}
		})
	}
}

// advanceSAMLClock moves the request-tracking clock forward for one test.
func advanceSAMLClock(t *testing.T, d time.Duration) {
	t.Helper()
	prev := samlNow
	samlNow = func() time.Time { return time.Now().Add(d) }
	t.Cleanup(func() { samlNow = prev })
}

func TestSAMLLoginRequestCookie(t *testing.T) {
	h := samlHandler("")
	_, c := samlLoginStart(t, h)
	if !c.HttpOnly || c.MaxAge != int(samlReqTTL/time.Second) || c.Path != "/api/v1/auth/saml" {
		t.Fatalf("unexpected axiom_saml_req cookie attributes: %+v", c)
	}
	if c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("plain-HTTP request cookie should be SameSite=Lax without Secure: %+v", c)
	}

	// Over HTTPS the cookie must survive the IdP's cross-site POST to the ACS.
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/saml/login", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	for _, ck := range w.Result().Cookies() {
		if ck.Name == samlReqCookie {
			if !ck.Secure || ck.SameSite != http.SameSiteNoneMode {
				t.Fatalf("HTTPS request cookie should be SameSite=None; Secure: %+v", ck)
			}
			return
		}
	}
	t.Fatal("no axiom_saml_req cookie over HTTPS")
}

func TestSAMLLoginIDsAreUnique(t *testing.T) {
	h := samlHandler("")
	a, _ := samlLoginStart(t, h)
	b, _ := samlLoginStart(t, h)
	if a == b {
		t.Fatalf("two logins produced the same AuthnRequest ID %q", a)
	}
}

func TestSAMLRequestStoreIsBoundedAndSingleUse(t *testing.T) {
	var st samlRequestStore
	exp := time.Now().Add(samlReqTTL)
	for n := 0; n < samlReqStoreCap+50; n++ {
		st.add(fmt.Sprintf("_%d", n), exp.Add(time.Duration(n)*time.Millisecond))
	}
	if len(st.ids) > samlReqStoreCap {
		t.Fatalf("store grew to %d entries, cap is %d", len(st.ids), samlReqStoreCap)
	}
	if st.consume("_0") {
		t.Fatal("the entry closest to expiry should have been evicted at capacity")
	}
	last := fmt.Sprintf("_%d", samlReqStoreCap+49)
	if !st.consume(last) {
		t.Fatal("newest entry should be outstanding")
	}
	if st.consume(last) {
		t.Fatal("an entry must be usable only once")
	}
	// Expired entries are swept on insert.
	st.add("_stale", time.Now().Add(-time.Second))
	st.add("_fresh", exp)
	if _, ok := st.ids["_stale"]; ok {
		t.Fatal("expired entries should be swept on insert")
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
		SAMLLogin:         testIdPSSO,
		SAMLEntityID:      "https://sp.example.test",
		SAMLAllowUnsigned: true,
		SessionSecret:     secret,
	})
	id, reqCookie := samlLoginStart(t, h)
	// Empty NameID and no email/uid attribute at all.
	w := postSAMLWithCookie(h, url.Values{"SAMLResponse": {b64XML(withInResponseTo(samlXML("", "", nil, nil), id))}}, reqCookie)
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
