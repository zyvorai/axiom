package api

import (
	"bytes"
	"compress/flate"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SP-initiated SAML request tracking.
//
// GET /auth/saml/login mints a random AuthnRequest ID and records it twice:
// in a signed, short-lived cookie (binds the response to the browser that
// started the sign-in) and in an in-memory, single-use set on the Server
// (makes a captured response unusable a second time, even from the same
// browser). The ACS requires the response's InResponseTo to match both and
// consumes the entry. The set is per process: with several replicas the
// login and the ACS POST must reach the same replica (session affinity).

const (
	samlReqCookie   = "axiom_saml_req"
	samlReqTTL      = 5 * time.Minute
	samlReqStoreCap = 4096
	samlACSPath     = "/api/v1/auth/saml/acs"
)

// samlNow is the clock for request tracking; tests move it forward.
var samlNow = time.Now

// samlRequestStore holds outstanding AuthnRequest IDs -> expiry. The zero
// value is ready to use. When shared is set (AttachSharedSAMLStore), add/consume
// also merge through that store so replicas share outstanding IDs.
type samlRequestStore struct {
	mu     sync.Mutex
	ids    map[string]time.Time
	shared interface {
		Load() ([]byte, error)
		Save([]byte) error
	}
}

// add records id, sweeping expired entries first. When the store is still at
// capacity (e.g. a flood of unauthenticated /saml/login calls) the entries
// closest to expiry are evicted, so memory stays bounded.
func (st *samlRequestStore) add(id string, exp time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.ids == nil {
		st.ids = make(map[string]time.Time)
	}
	now := samlNow()
	for k, e := range st.ids {
		if !now.Before(e) {
			delete(st.ids, k)
		}
	}
	for len(st.ids) >= samlReqStoreCap {
		var oldest string
		var oldestExp time.Time
		for k, e := range st.ids {
			if oldest == "" || e.Before(oldestExp) {
				oldest, oldestExp = k, e
			}
		}
		delete(st.ids, oldest)
	}
	st.ids[id] = exp
	st.persistSharedLocked()
}

// consume removes id and reports whether it was outstanding and unexpired.
func (st *samlRequestStore) consume(id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.ids == nil {
		st.ids = make(map[string]time.Time)
	}
	st.loadSharedLocked()
	exp, ok := st.ids[id]
	if !ok {
		return false
	}
	delete(st.ids, id)
	ok = samlNow().Before(exp)
	if ok {
		st.persistSharedLocked()
	}
	return ok
}

func (st *samlRequestStore) loadSharedLocked() {
	if st.shared == nil {
		return
	}
	raw, err := st.shared.Load()
	if err != nil || len(raw) == 0 {
		return
	}
	var snap samlSharedSnap
	if json.Unmarshal(raw, &snap) != nil || snap.IDs == nil {
		return
	}
	now := samlNow()
	for id, unix := range snap.IDs {
		exp := time.Unix(unix, 0)
		if !now.Before(exp) {
			continue
		}
		if cur, has := st.ids[id]; !has || exp.After(cur) {
			st.ids[id] = exp
		}
	}
}

func (st *samlRequestStore) persistSharedLocked() {
	if st.shared == nil {
		return
	}
	snap := samlSharedSnap{IDs: map[string]int64{}}
	now := samlNow()
	for id, exp := range st.ids {
		if now.Before(exp) {
			snap.IDs[id] = exp.Unix()
		}
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return
	}
	_ = st.shared.Save(raw)
}

// newSAMLRequestID returns a random ID that is a valid xs:ID (an NCName must
// not start with a digit, hence the underscore).
func newSAMLRequestID() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	return "_" + hex.EncodeToString(b)
}

func samlReqMAC(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	// Domain-separate from session tokens signed with the same secret.
	_, _ = mac.Write([]byte("axiom-saml-req|" + payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// signSAMLRequestCookie returns "<id>.<unix exp>.<hmac>".
func signSAMLRequestCookie(secret, id string, exp time.Time) string {
	payload := id + "." + strconv.FormatInt(exp.Unix(), 10)
	return payload + "." + samlReqMAC(secret, payload)
}

var (
	errSAMLReqMissing = errors.New("no SAML sign-in in progress in this browser (missing " + samlReqCookie + " cookie)")
	errSAMLReqBadSig  = errors.New("SAML sign-in cookie is invalid")
	errSAMLReqExpired = errors.New("SAML sign-in cookie has expired; start the sign-in again")
)

// parseSAMLRequestCookie verifies the cookie and returns the request ID.
func parseSAMLRequestCookie(secret, v string) (string, error) {
	if secret == "" || v == "" {
		return "", errSAMLReqMissing
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 || parts[0] == "" {
		return "", errSAMLReqBadSig
	}
	want := samlReqMAC(secret, parts[0]+"."+parts[1])
	if subtle.ConstantTimeCompare([]byte(want), []byte(parts[2])) != 1 {
		return "", errSAMLReqBadSig
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", errSAMLReqBadSig
	}
	if !samlNow().Before(time.Unix(exp, 0)) {
		return "", errSAMLReqExpired
	}
	return parts[0], nil
}

// samlReqCookieTemplate: the IdP returns the user with a cross-site POST to
// the ACS, and browsers withhold SameSite=Lax cookies from cross-site POSTs.
// Over HTTPS the cookie is therefore SameSite=None (it is only a binding
// token; on its own it grants nothing). Plain HTTP cannot use None (browsers
// require Secure), so it falls back to Lax, which works when the IdP is
// same-site (e.g. a lab IdP on localhost).
func samlReqCookieTemplate(secure bool) *http.Cookie {
	c := &http.Cookie{Name: samlReqCookie, Path: "/api/v1/auth/saml", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode}
	if secure {
		c.SameSite = http.SameSiteNoneMode
	}
	return c
}

func setSAMLRequestCookie(w http.ResponseWriter, secure bool, value string) {
	c := samlReqCookieTemplate(secure)
	c.Value = value
	c.MaxAge = int(samlReqTTL / time.Second)
	http.SetCookie(w, c)
}

func clearSAMLRequestCookie(w http.ResponseWriter, secure bool) {
	c := samlReqCookieTemplate(secure)
	c.MaxAge = -1
	http.SetCookie(w, c)
}

// samlACSURL is the Assertion Consumer Service URL Axiom advertises in its
// AuthnRequest and expects as SubjectConfirmationData Recipient.
func (s *Server) samlACSURL(r *http.Request) string {
	return s.publicBase(r) + samlACSPath
}

// buildAuthnRequest returns the HTTP-Redirect-binding encoding (raw DEFLATE,
// then base64) of an unsigned AuthnRequest.
func buildAuthnRequest(id, destination, acsURL, issuer string, now time.Time) (string, error) {
	esc := func(v string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(v))
		return b.String()
	}
	var doc strings.Builder
	doc.WriteString(`<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"`)
	doc.WriteString(` ID="` + esc(id) + `" Version="2.0" IssueInstant="` + now.UTC().Format(time.RFC3339) + `"`)
	doc.WriteString(` Destination="` + esc(destination) + `" AssertionConsumerServiceURL="` + esc(acsURL) + `"`)
	doc.WriteString(` ProtocolBinding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST">`)
	if issuer != "" {
		doc.WriteString(`<saml:Issuer>` + esc(issuer) + `</saml:Issuer>`)
	}
	doc.WriteString(`<samlp:NameIDPolicy AllowCreate="true"/></samlp:AuthnRequest>`)

	var buf bytes.Buffer
	fw, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write([]byte(doc.String())); err != nil {
		return "", err
	}
	if err := fw.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// samlLogin starts SP-initiated SSO: it redirects to AXIOM_SAML_LOGIN_URL
// with a fresh AuthnRequest (HTTP-Redirect binding) whose ID the ACS will
// require as InResponseTo.
func (s *Server) samlLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth.SAMLLogin == "" {
		fail(w, http.StatusNotImplemented, errSSO("SAML is not configured. Set AXIOM_SAML_LOGIN_URL to your IdP SSO URL."))
		return
	}
	target, err := url.Parse(s.auth.SAMLLogin)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	dest := *target
	dest.RawQuery, dest.Fragment = "", ""

	id := newSAMLRequestID()
	now := samlNow()
	exp := now.Add(samlReqTTL)
	samlReq, err := buildAuthnRequest(id, dest.String(), s.samlACSURL(r), s.auth.SAMLEntityID, now)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}

	q := target.Query()
	q.Set("SAMLRequest", samlReq)
	if next := r.URL.Query().Get("next"); next != "" && q.Get("RelayState") == "" {
		q.Set("RelayState", next)
	}
	target.RawQuery = q.Encode()

	s.samlReqs.add(id, exp)
	setSAMLRequestCookie(w, s.requestSecure(r), signSAMLRequestCookie(s.sessionSecret(), id, exp))
	http.Redirect(w, r, target.String(), http.StatusFound)
}
