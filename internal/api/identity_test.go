package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

const (
	idAdminUser = "root"
	idAdminPass = "root-password"
)

type idEnv struct {
	h     http.Handler
	users *platform.UserHub
	store *memRevStore
}

// memRevStore is an in-memory auth.RevocationStore that counts saves.
type memRevStore struct {
	mu    sync.Mutex
	data  []byte
	saves int
}

func (m *memRevStore) Load() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data, nil
}
func (m *memRevStore) Save(b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data, m.saves = append([]byte(nil), b...), m.saves+1
	return nil
}

func newIDEnv(t *testing.T, opts auth.Guard, totp string) *idEnv {
	t.Helper()
	hub := platform.NewUserHub(nil)
	store := &memRevStore{}
	h := NewWithUsers(platform.NewDemoProvider(), "demo", idAdminUser, idAdminPass, "", nil, hub,
		AuthOptions{SessionSecret: "test-secret", RevocationStore: store, TOTPSecret: totp})
	return &idEnv{h: h, users: hub, store: store}
}

func basic(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

type reqOpt func(*http.Request)

func withBasic(u, p string) reqOpt { return func(r *http.Request) { r.Header.Set("Authorization", basic(u, p)) } }
func withCookie(c string) reqOpt   { return func(r *http.Request) { r.Header.Set("Cookie", "axiom_session="+c) } }
func withBearer(tok string) reqOpt {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
}

func (e *idEnv) do(method, path, body string, opts ...reqOpt) *httptest.ResponseRecorder {
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	} else {
		rd = strings.NewReader("")
	}
	r := httptest.NewRequest(method, path, rd)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	r.RemoteAddr = "192.0.2.10:1234"
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func sessionCookie(w *httptest.ResponseRecorder) string {
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.SessionCookie && c.Value != "" {
			return c.Value
		}
	}
	return ""
}

func (e *idEnv) addUser(t *testing.T, name, pass, role string, ns ...string) {
	t.Helper()
	if _, err := e.users.Create(context.Background(), name, pass, role, ns); err != nil {
		t.Fatal(err)
	}
}

func jsonBody(v any) string { b, _ := json.Marshal(v); return string(b) }

func decodeInto(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("bad JSON %q: %v", w.Body.String(), err)
	}
}

// enrol walks the whole enrolment through the API and returns the secret and
// the recovery codes.
func (e *idEnv) enrol(t *testing.T, user, pass string) (secret string, recovery []string) {
	t.Helper()
	w := e.do("POST", "/api/v1/auth/mfa/setup", jsonBody(map[string]string{"password": pass}), withBasic(user, pass))
	if w.Code != 200 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	var s struct{ Secret, URI string }
	decodeInto(t, w, &s)
	secret = auth.NormalizeTOTPSecret(s.Secret)
	if !strings.HasPrefix(s.URI, "otpauth://totp/") || !strings.Contains(s.URI, "secret="+secret) {
		t.Fatalf("bad provisioning URI %q for secret %q", s.URI, secret)
	}
	code, _ := auth.TOTPCodeAt(secret, time.Now())
	w = e.do("POST", "/api/v1/auth/mfa/enable", jsonBody(map[string]string{"code": code}), withBasic(user, pass))
	if w.Code != 200 {
		t.Fatalf("enable: %d %s", w.Code, w.Body.String())
	}
	var en struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	decodeInto(t, w, &en)
	if len(en.RecoveryCodes) != 10 {
		t.Fatalf("want 10 recovery codes, got %v", en.RecoveryCodes)
	}
	return secret, en.RecoveryCodes
}

func (e *idEnv) login(user, pass, code string) *httptest.ResponseRecorder {
	b := map[string]string{"username": user, "password": pass}
	if code != "" {
		b["code"] = code
	}
	return e.do("POST", auth.LoginPath, jsonBody(b))
}

// A fresh code for a step the account has not used yet. Enrolment consumed the
// current step, so move one step forward (within the accepted drift).
func nextCode(secret string) string {
	c, _ := auth.TOTPCodeAt(secret, time.Now().Add(30*time.Second))
	return c
}

func TestMFAEnrolmentAndLogin(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "alice", "alice-password", "operator")

	// Before enrolment: Basic works and so does the login endpoint.
	if w := e.do("GET", "/api/v1/session", "", withBasic("alice", "alice-password")); w.Code != 200 {
		t.Fatalf("basic before mfa: %d", w.Code)
	}
	secret, recovery := e.enrol(t, "alice", "alice-password")

	// Enabling revoked sessions and, more importantly, Basic alone is now refused.
	w := e.do("GET", "/api/v1/session", "", withBasic("alice", "alice-password"))
	if w.Code != 401 || w.Header().Get(auth.MFAHeader) != "required" {
		t.Fatalf("basic after mfa should be 401 with %s header, got %d %v", auth.MFAHeader, w.Code, w.Header())
	}

	// Password only: asks for the code, does not count as a failed guess.
	w = e.login("alice", "alice-password", "")
	if w.Code != 401 || w.Header().Get(auth.MFAHeader) != "required" || !strings.Contains(w.Body.String(), `"mfa":true`) {
		t.Fatalf("password-only login: %d %s", w.Code, w.Body.String())
	}
	// Wrong code, wrong password: both 401 and no session.
	if w = e.login("alice", "alice-password", "000000"); w.Code != 401 || sessionCookie(w) != "" {
		t.Fatalf("wrong code: %d", w.Code)
	}
	if w = e.login("alice", "nope", "000000"); w.Code != 401 || sessionCookie(w) != "" {
		t.Fatalf("wrong password: %d", w.Code)
	}

	// Right code: a session, usable by cookie and by bearer token.
	code := nextCode(secret)
	w = e.login("alice", "alice-password", code)
	if w.Code != 200 {
		t.Fatalf("login with code: %d %s", w.Code, w.Body.String())
	}
	cookie := sessionCookie(w)
	var lr struct {
		Token string `json:"token"`
		Role  string `json:"role"`
		MFA   bool   `json:"mfa"`
	}
	decodeInto(t, w, &lr)
	if cookie == "" || !strings.HasPrefix(lr.Token, "axiom.") || lr.Role != "operator" || !lr.MFA {
		t.Fatalf("bad login response: cookie=%q %+v", cookie, lr)
	}
	if w = e.do("GET", "/api/v1/session", "", withCookie(cookie)); w.Code != 200 || !strings.Contains(w.Body.String(), `"alice"`) {
		t.Fatalf("session by cookie: %d %s", w.Code, w.Body.String())
	}
	if w = e.do("GET", "/api/v1/session", "", withBearer(lr.Token)); w.Code != 200 {
		t.Fatalf("session by bearer token: %d", w.Code)
	}

	// The same code cannot be used twice.
	if w = e.login("alice", "alice-password", code); w.Code != 401 {
		t.Fatalf("replayed TOTP code was accepted: %d", w.Code)
	}

	// A recovery code works exactly once.
	if w = e.login("alice", "alice-password", recovery[0]); w.Code != 200 {
		t.Fatalf("recovery code: %d %s", w.Code, w.Body.String())
	}
	if w = e.login("alice", "alice-password", recovery[0]); w.Code != 401 {
		t.Fatalf("recovery code was accepted twice: %d", w.Code)
	}
	var st mfaStatusView
	w = e.do("GET", "/api/v1/auth/mfa", "", withCookie(cookie))
	decodeInto(t, w, &st)
	if !st.Available || !st.Enabled || st.RecoveryCodes != 9 {
		t.Fatalf("status after using one recovery code: %+v", st)
	}
}

func TestMFAEnrolmentGuards(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "viewer1", "viewer-password", "viewer")

	// Setup needs the current password, even for an authenticated caller.
	if w := e.do("POST", "/api/v1/auth/mfa/setup", jsonBody(map[string]string{"password": "wrong"}), withBasic("viewer1", "viewer-password")); w.Code != 403 {
		t.Fatalf("setup with wrong password: %d", w.Code)
	}
	// Enabling before setup, and with a bad code, fails.
	if w := e.do("POST", "/api/v1/auth/mfa/enable", jsonBody(map[string]string{"code": "123456"}), withBasic("viewer1", "viewer-password")); w.Code != 400 {
		t.Fatalf("enable without setup: %d", w.Code)
	}
	// A read-only viewer may still manage their own second factor.
	w := e.do("POST", "/api/v1/auth/mfa/setup", jsonBody(map[string]string{"password": "viewer-password"}), withBasic("viewer1", "viewer-password"))
	if w.Code != 200 {
		t.Fatalf("viewer setup: %d %s", w.Code, w.Body.String())
	}
	if w = e.do("POST", "/api/v1/auth/mfa/enable", jsonBody(map[string]string{"code": "000000"}), withBasic("viewer1", "viewer-password")); w.Code != 400 {
		t.Fatalf("enable with bad code: %d", w.Code)
	}
	// The env account is configured through the environment, not the API.
	if w = e.do("POST", "/api/v1/auth/mfa/setup", jsonBody(map[string]string{"password": idAdminPass}), withBasic(idAdminUser, idAdminPass)); w.Code != 400 || !strings.Contains(w.Body.String(), "AXIOM_TOTP_SECRET") {
		t.Fatalf("env account setup: %d %s", w.Code, w.Body.String())
	}
	// Anonymous callers get nothing.
	if w = e.do("GET", "/api/v1/auth/mfa", ""); w.Code != 401 {
		t.Fatalf("anonymous status: %d", w.Code)
	}
}

func TestMFADisableAndAdminReset(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "bob", "bob-password", "operator")
	e.addUser(t, "carol", "carol-password", "operator")
	secretB, recoveryB := e.enrol(t, "bob", "bob-password")
	e.enrol(t, "carol", "carol-password")

	// Disable requires the password AND a valid code.
	cookie := sessionCookie(e.login("bob", "bob-password", nextCode(secretB)))
	if cookie == "" {
		t.Fatal("bob could not sign in")
	}
	body := jsonBody(map[string]string{"password": "bob-password", "code": "000000"})
	if w := e.do("POST", "/api/v1/auth/mfa/disable", body, withCookie(cookie)); w.Code != 403 {
		t.Fatalf("disable with a bad code: %d", w.Code)
	}
	// The current step was used by the login, so disable with a recovery code.
	body = jsonBody(map[string]string{"password": "bob-password", "code": recoveryB[0]})
	if w := e.do("POST", "/api/v1/auth/mfa/disable", body, withCookie(cookie)); w.Code != 200 {
		t.Fatalf("disable: %d", w.Code)
	}
	// Basic works again, and bob's old session is gone.
	if w := e.do("GET", "/api/v1/session", "", withBasic("bob", "bob-password")); w.Code != 200 {
		t.Fatalf("basic after disable: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(cookie)); w.Code != 401 {
		t.Fatalf("session survived disabling MFA: %d", w.Code)
	}

	// An administrator can reset someone else's; an operator cannot.
	if w := e.do("DELETE", "/api/v1/users/carol/mfa", "", withBasic("bob", "bob-password")); w.Code != 403 {
		t.Fatalf("operator reset: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/users/carol/mfa", "", withBasic(idAdminUser, idAdminPass)); w.Code != 200 {
		t.Fatalf("admin reset: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/session", "", withBasic("carol", "carol-password")); w.Code != 200 {
		t.Fatalf("carol after reset: %d", w.Code)
	}
	// The users list reports who has MFA.
	var list []platform.UserView
	decodeInto(t, e.do("GET", "/api/v1/users", "", withBasic(idAdminUser, idAdminPass)), &list)
	for _, u := range list {
		if u.MFAEnabled {
			t.Errorf("%s still reports MFA enabled", u.Username)
		}
	}
	// The secret and recovery hashes never appear in the API.
	raw := e.do("GET", "/api/v1/users", "", withBasic(idAdminUser, idAdminPass)).Body.String()
	for _, banned := range []string{"mfaSecret", "mfaPending", "mfaRecovery", "passwordHash", "mfaLastStep"} {
		if strings.Contains(raw, banned) {
			t.Errorf("user list leaks %q", banned)
		}
	}
}

func TestEnvAccountTOTP(t *testing.T) {
	secret := auth.GenerateTOTPSecret()
	e := newIDEnv(t, auth.Guard{}, secret)

	if w := e.do("GET", "/api/v1/session", "", withBasic(idAdminUser, idAdminPass)); w.Code != 401 || w.Header().Get(auth.MFAHeader) != "required" {
		t.Fatalf("env account basic with TOTP configured: %d", w.Code)
	}
	code, _ := auth.TOTPCodeAt(secret, time.Now())
	w := e.login(idAdminUser, idAdminPass, code)
	if w.Code != 200 {
		t.Fatalf("env login: %d %s", w.Code, w.Body.String())
	}
	cookie := sessionCookie(w)
	if w = e.do("GET", "/api/v1/users", "", withCookie(cookie)); w.Code != 200 {
		t.Fatalf("env session is not admin: %d", w.Code)
	}
	if w = e.login(idAdminUser, idAdminPass, code); w.Code != 401 {
		t.Fatalf("env TOTP replay accepted: %d", w.Code)
	}
	var st mfaStatusView
	decodeInto(t, e.do("GET", "/api/v1/auth/mfa", "", withCookie(cookie)), &st)
	if st.Available || !st.Enabled || st.Managed != "AXIOM_TOTP_SECRET" {
		t.Fatalf("env status: %+v", st)
	}
	// API keys are a separate credential and are unaffected.
}

func TestMalformedEnvTOTPIsIgnored(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "not base32!")
	if w := e.do("GET", "/api/v1/session", "", withBasic(idAdminUser, idAdminPass)); w.Code != 200 {
		t.Fatalf("a malformed AXIOM_TOTP_SECRET locked everyone out: %d", w.Code)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "dave", "dave-password", "operator")
	w1 := e.login("dave", "dave-password", "")
	w2 := e.login("dave", "dave-password", "")
	c1, c2 := sessionCookie(w1), sessionCookie(w2)
	if c1 == "" || c2 == "" || c1 == c2 {
		t.Fatalf("expected two distinct sessions: %q %q", c1, c2)
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(c1)); w.Code != 200 {
		t.Fatalf("session 1: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/auth/logout", "", withCookie(c1)); w.Code != 200 {
		t.Fatalf("logout: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(c1)); w.Code != 401 {
		t.Fatalf("logged-out session still works: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(c2)); w.Code != 200 {
		t.Fatalf("logout ended the wrong session: %d", w.Code)
	}
	// Sign out everywhere ends the rest, but a new login afterwards works.
	if w := e.do("POST", "/api/v1/auth/logout-all", "", withCookie(c2)); w.Code != 200 {
		t.Fatalf("logout-all: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(c2)); w.Code != 401 {
		t.Fatalf("session survived logout-all: %d", w.Code)
	}
	time.Sleep(2 * time.Millisecond)
	if c3 := sessionCookie(e.login("dave", "dave-password", "")); c3 == "" {
		t.Fatal("could not sign in after logout-all")
	} else if w := e.do("GET", "/api/v1/session", "", withCookie(c3)); w.Code != 200 {
		t.Fatalf("fresh session after logout-all: %d", w.Code)
	}
}

func TestAccountChangesEndSessions(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "erin", "erin-password", "operator")
	cookie := sessionCookie(e.login("erin", "erin-password", ""))

	// A role change applies at once (sessions are re-resolved against the account)
	// and revokes the session so the person re-authenticates.
	if _, err := e.users.UpdateRole(context.Background(), "erin", "viewer"); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(cookie)); w.Code != 401 {
		t.Fatalf("session survived a role change: %d", w.Code)
	}
	time.Sleep(2 * time.Millisecond)

	cookie = sessionCookie(e.login("erin", "erin-password", ""))
	if _, err := e.users.ResetPassword(context.Background(), "erin", "a-new-password"); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(cookie)); w.Code != 401 {
		t.Fatalf("session survived a password reset: %d", w.Code)
	}
	time.Sleep(2 * time.Millisecond)

	cookie = sessionCookie(e.login("erin", "a-new-password", ""))
	if err := e.users.Delete(context.Background(), "erin"); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(cookie)); w.Code != 401 {
		t.Fatalf("session survived deleting the account: %d", w.Code)
	}
}

func TestAdminRevokeSessions(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "frank", "frank-password", "operator")
	cookie := sessionCookie(e.login("frank", "frank-password", ""))
	if w := e.do("POST", "/api/v1/users/frank/revoke-sessions", "", withBasic("frank", "frank-password")); w.Code != 403 {
		t.Fatalf("an operator revoked sessions: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/users/frank/revoke-sessions", "", withBasic(idAdminUser, idAdminPass)); w.Code != 200 {
		t.Fatalf("admin revoke: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(cookie)); w.Code != 401 {
		t.Fatalf("session survived admin revocation: %d", w.Code)
	}
}

// A namespace-restricted viewer who signs in with a session must stay
// restricted: the session must not widen their access.
func TestSessionKeepsNamespaceRestriction(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "gina", "gina-password", "viewer", "payments")
	cookie := sessionCookie(e.login("gina", "gina-password", ""))
	if w := e.do("GET", "/api/v1/pods?namespace=payments", "", withCookie(cookie)); w.Code != 200 {
		t.Fatalf("own namespace: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/pods?namespace=commerce", "", withCookie(cookie)); w.Code != 403 {
		t.Fatalf("other namespace via session: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/pods?namespace=all", "", withCookie(cookie)); w.Code != 403 {
		t.Fatalf("all namespaces via session: %d", w.Code)
	}
	// Narrowing the restriction takes effect on the very next request.
	if _, err := e.users.UpdateNamespaces(context.Background(), "gina", []string{"commerce"}); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(cookie)); w.Code != 401 {
		t.Fatalf("old session after namespace change: %d", w.Code)
	}
}

func TestLoginEndpointIsThrottled(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "hank", "hank-password", "operator")
	var last int
	for i := 0; i < 12; i++ {
		last = e.login("hank", "wrong", "").Code
	}
	if last != 429 {
		t.Fatalf("repeated bad logins were not rate limited: last status %d", last)
	}
	// Even the right password is refused while locked out.
	if w := e.login("hank", "hank-password", ""); w.Code != 429 {
		t.Fatalf("locked-out account could still sign in: %d", w.Code)
	}
}

func TestLoginRejectsMalformedRequests(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "")
	if w := e.do("POST", auth.LoginPath, "not json"); w.Code != 400 {
		t.Fatalf("garbage body: %d", w.Code)
	}
	if w := e.do("POST", auth.LoginPath, jsonBody(map[string]string{"username": "", "password": ""})); w.Code != 401 {
		t.Fatalf("empty credentials: %d", w.Code)
	}
	big := jsonBody(map[string]string{"username": "x", "password": strings.Repeat("a", 10000)})
	if w := e.do("POST", auth.LoginPath, big); w.Code != 400 {
		t.Fatalf("oversized body: %d", w.Code)
	}
	// GET is not a login.
	if w := e.do("GET", auth.LoginPath, ""); w.Code != 401 && w.Code != 404 && w.Code != 405 {
		t.Fatalf("GET login: %d", w.Code)
	}
	// The env account without MFA can sign in through the endpoint too.
	if w := e.login(idAdminUser, idAdminPass, ""); w.Code != 200 {
		t.Fatalf("env account login: %d %s", w.Code, w.Body.String())
	}
}

func TestRevocationsSurviveRestart(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "iris", "iris-password", "operator")
	cookie := sessionCookie(e.login("iris", "iris-password", ""))
	if w := e.do("POST", "/api/v1/auth/logout", "", withCookie(cookie)); w.Code != 200 {
		t.Fatalf("logout: %d", w.Code)
	}
	if e.store.saves == 0 {
		t.Fatal("the revocation was not persisted")
	}
	// A new server process with the same secret and store still refuses it.
	hub := platform.NewUserHub(nil)
	if _, err := hub.Create(context.Background(), "iris", "iris-password", "operator", nil); err != nil {
		t.Fatal(err)
	}
	h2 := NewWithUsers(platform.NewDemoProvider(), "demo", idAdminUser, idAdminPass, "", nil, hub,
		AuthOptions{SessionSecret: "test-secret", RevocationStore: e.store})
	r := httptest.NewRequest("GET", "/api/v1/session", nil)
	r.Header.Set("Cookie", "axiom_session="+cookie)
	w := httptest.NewRecorder()
	h2.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("a revoked session came back after a restart: %d", w.Code)
	}
}
