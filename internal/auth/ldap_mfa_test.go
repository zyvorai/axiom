package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMFA is an MFAProvider keyed by username that accepts a fixed code once.
type fakeMFA struct {
	mu       sync.Mutex
	enrolled map[string]bool
	code     string
	used     map[string]bool
	lookups  int
}

func (f *fakeMFA) MFAEnrolled(u string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups++
	return f.enrolled[u]
}

func (f *fakeMFA) VerifySecondFactor(u, code string, _ time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.enrolled[u] || code != f.code || f.used[u+code] {
		return false
	}
	if f.used == nil {
		f.used = map[string]bool{}
	}
	f.used[u+code] = true
	return true
}

// fakeLookupUsers is a UserAuthenticator + UserLookup with fixed accounts.
type fakeLookupUsers struct{ fakeUserAuthenticator }

func (f *fakeLookupUsers) Lookup(u string) (string, []string, bool) {
	x, ok := f.users[u]
	return x.role, x.namespaces, ok
}

// swapDirectory installs a fake directory bind for the test.
func swapDirectory(t *testing.T, users map[string]string, role string) *int {
	t.Helper()
	binds := 0
	restore := SwapLDAPAuthenticate(func(_ LDAPConfig, u, p string) (string, bool, error) {
		binds++
		if pw, ok := users[u]; ok && pw == p && p != "" {
			return role, true, nil
		}
		return "", false, nil
	})
	t.Cleanup(restore)
	return &binds
}

func basicHeader(u, p string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(u+":"+p))
}

func ldapGuard(ext MFAProvider, policy MFAPolicy) Guard {
	return Guard{Username: "demo", Password: "demo", SessionSecret: "s3cret",
		LDAP: LDAPConfig{Enabled: true, URL: "ldap://directory.invalid"}, ExternalMFA: ext, RequireMFA: policy}
}

func serveGuard(g Guard) (http.Handler, *Identity) {
	var got Identity
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = IdentityFromContext(r.Context())
		w.WriteHeader(204)
	}))
	return h, &got
}

func doLogin(h http.Handler, user, pass, code string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]string{"username": user, "password": pass, "code": code})
	r := httptest.NewRequest("POST", LoginPath, strings.NewReader(string(b)))
	r.RemoteAddr = "198.51.100.7:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func doGet(h http.Handler, path string, set func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	r.RemoteAddr = "198.51.100.7:1"
	set(r)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestGuardLDAPWithSecondFactor(t *testing.T) {
	swapDirectory(t, map[string]string{"alice": "dir-pw"}, "admin")
	ext := &fakeMFA{enrolled: map[string]bool{"alice": true}, code: "123456"}
	g := ldapGuard(ext, MFAPolicyOff)
	h, got := serveGuard(g)

	// A wrong directory password is a plain failure and never looks at MFA.
	w := doGet(h, "/api/v1/overview", func(r *http.Request) { r.Header.Set("Authorization", basicHeader("alice", "nope")) })
	if w.Code != 401 || w.Header().Get(MFAHeader) != "" || ext.lookups != 0 {
		t.Fatalf("wrong password: %d %q lookups=%d", w.Code, w.Header().Get(MFAHeader), ext.lookups)
	}
	// Right password, enrolled: Basic alone is refused with the MFA header.
	w = doGet(h, "/api/v1/overview", func(r *http.Request) { r.Header.Set("Authorization", basicHeader("alice", "dir-pw")) })
	if w.Code != 401 || w.Header().Get(MFAHeader) != "required" {
		t.Fatalf("basic with mfa: %d %q", w.Code, w.Header().Get(MFAHeader))
	}
	// Login with the code: an LDAP session with the bind's role.
	lw := doLogin(h, "alice", "dir-pw", "123456")
	if lw.Code != 200 {
		t.Fatalf("login: %d %s", lw.Code, lw.Body.String())
	}
	var resp struct{ Token, Role string }
	_ = json.Unmarshal(lw.Body.Bytes(), &resp)
	sess, ok := ParseSession("s3cret", strings.TrimPrefix(resp.Token, "axiom."))
	if !ok || sess.Issuer != LDAPSessionIssuer || sess.Role != "admin" || resp.Role != "admin" {
		t.Fatalf("session %+v ok=%v resp=%+v", sess, ok, resp)
	}
	w = doGet(h, "/api/v1/overview", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+resp.Token) })
	if w.Code != 204 || !got.ViaLDAP() || got.User != "alice" || got.Role != "admin" {
		t.Fatalf("ldap session: %d %+v", w.Code, *got)
	}
	// Replay refused; a wrong code is a failure.
	if w := doLogin(h, "alice", "dir-pw", "123456"); w.Code != 401 {
		t.Fatalf("replay: %d", w.Code)
	}
	// Revoking the user ends the LDAP session.
	g.Revocations = NewRevocations(nil)
	h2, _ := serveGuard(g)
	time.Sleep(2 * time.Millisecond)
	g.Revocations.RevokeUser("alice")
	w = doGet(h2, "/api/v1/overview", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+resp.Token) })
	if w.Code != 401 {
		t.Fatalf("revoked ldap session: %d", w.Code)
	}
}

func TestGuardLDAPWrongCodeCountsAsFailure(t *testing.T) {
	swapDirectory(t, map[string]string{"bob": "dir-pw"}, "operator")
	ext := &fakeMFA{enrolled: map[string]bool{"bob": true}, code: "654321"}
	h, _ := serveGuard(ldapGuard(ext, MFAPolicyOff))
	last := 0
	for i := 0; i < 12; i++ {
		last = doLogin(h, "bob", "dir-pw", "000000").Code
	}
	if last != 429 {
		t.Fatalf("wrong codes not throttled: %d", last)
	}
}

func TestGuardLDAPWithoutSecondFactorAndPolicy(t *testing.T) {
	swapDirectory(t, map[string]string{"carl": "dir-pw"}, "operator")
	ext := &fakeMFA{enrolled: map[string]bool{}}
	h, got := serveGuard(ldapGuard(ext, MFAPolicyOperator))
	basicCarl := func(r *http.Request) { r.Header.Set("Authorization", basicHeader("carl", "dir-pw")) }
	w := doGet(h, "/api/v1/overview", basicCarl)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "mfaEnrollmentRequired") {
		t.Fatalf("unenrolled covered LDAP user: %d %s", w.Code, w.Body.String())
	}
	if w := doGet(h, "/api/v1/auth/mfa", basicCarl); w.Code != 204 || !got.ViaLDAP() {
		t.Fatalf("enrolment path: %d", w.Code)
	}
	ext.enrolled["carl"] = true
	if w := doGet(h, "/api/v1/overview", basicCarl); w.Code != 401 || w.Header().Get(MFAHeader) != "required" {
		t.Fatalf("after enrolling, basic: %d", w.Code)
	}

	// Without an external store the policy does not apply (it could not be met).
	h, _ = serveGuard(ldapGuard(nil, MFAPolicyAll))
	if w := doGet(h, "/api/v1/overview", basicCarl); w.Code != 204 {
		t.Fatalf("no external store: %d", w.Code)
	}
}

func TestGuardLDAPLocalAccountTakesPrecedence(t *testing.T) {
	swapDirectory(t, map[string]string{"dee": "dir-pw"}, "admin")
	users := &fakeLookupUsers{fakeUserAuthenticator{users: map[string]struct {
		password   string
		role       string
		namespaces []string
	}{"dee": {password: "local-pw", role: "viewer"}}}}
	local := &fakeMFA{enrolled: map[string]bool{"dee": true}, code: "111111"}
	ext := &fakeMFA{enrolled: map[string]bool{}, code: "222222"}
	g := ldapGuard(ext, MFAPolicyOff)
	g.Users, g.MFA = users, local
	h, _ := serveGuard(g)
	// The directory password for a name with a local second factor still
	// needs the local factor; the directory store's code is not accepted.
	if w := doGet(h, "/api/v1/overview", func(r *http.Request) { r.Header.Set("Authorization", basicHeader("dee", "dir-pw")) }); w.Code != 401 || w.Header().Get(MFAHeader) != "required" {
		t.Fatalf("directory password: %d %q", w.Code, w.Header().Get(MFAHeader))
	}
	if w := doLogin(h, "dee", "dir-pw", "222222"); w.Code != 401 {
		t.Fatalf("external code accepted for a local name: %d", w.Code)
	}
	if w := doLogin(h, "dee", "local-pw", "111111"); w.Code != 200 {
		t.Fatalf("local login: %d %s", w.Code, w.Body.String())
	}
}

func TestVerifyPasswordChecksTheDirectory(t *testing.T) {
	binds := swapDirectory(t, map[string]string{"eve": "dir-pw"}, "operator")
	g := ldapGuard(nil, MFAPolicyOff)
	if !g.VerifyPassword("eve", "dir-pw") || g.VerifyPassword("eve", "wrong") {
		t.Fatal("VerifyPassword does not check the directory password")
	}
	if *binds != 2 {
		t.Fatalf("binds=%d", *binds)
	}
}
