package api

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// fakeDirectory stands in for an LDAP server: auth.SwapLDAPAuthenticate routes
// every Guard's directory bind here for the duration of a test.
type fakeDirectory struct {
	mu    sync.Mutex
	users map[string]struct{ password, role string }
	binds int
}

func installFakeDirectory(t *testing.T, users map[string][2]string) *fakeDirectory {
	t.Helper()
	d := &fakeDirectory{users: map[string]struct{ password, role string }{}}
	for name, pr := range users {
		d.users[name] = struct{ password, role string }{pr[0], pr[1]}
	}
	restore := auth.SwapLDAPAuthenticate(func(_ auth.LDAPConfig, u, p string) (string, bool, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.binds++
		e, ok := d.users[strings.ToLower(u)]
		if !ok || e.password != p || p == "" {
			return "", false, nil
		}
		return e.role, true, nil
	})
	t.Cleanup(restore)
	return d
}

type ldapEnv struct {
	*idEnv
	ext *platform.ExternalMFAStore
	dir *fakeDirectory
}

func newLDAPEnv(t *testing.T, policy auth.MFAPolicy, dirUsers map[string][2]string) *ldapEnv {
	t.Helper()
	dir := installFakeDirectory(t, dirUsers)
	// These tests write many audit entries to the process-wide ring; drop them
	// afterwards so later tests that read the newest N timeline entries (e.g.
	// TestTimelineMergesSources, default limit 200) are not crowded out.
	t.Cleanup(func() {
		platform.SetAuditRingSize(1)
		platform.SetAuditRingSize(5000) // auditRingSizeDefault
	})
	hub := platform.NewUserHub(nil)
	store := &memRevStore{}
	ext := platform.NewMemoryExternalMFAStore()
	h := NewWithUsers(platform.NewDemoProvider(), "demo", idAdminUser, idAdminPass, "", nil, hub,
		AuthOptions{SessionSecret: "test-secret", RevocationStore: store, RequireMFA: policy,
			ExternalMFAStore: ext, LDAPEnabled: true, LDAPURL: "ldap://directory.invalid"})
	return &ldapEnv{idEnv: &idEnv{h: h, users: hub, store: store}, ext: ext, dir: dir}
}

// enrol is idEnv.enrol followed by a short wait. Enabling revokes the user's
// sessions with a millisecond cut-off, and the fake directory answers
// instantly (a local account's bcrypt check takes far longer), so a sign-in in
// the same millisecond would count as issued before the cut-off.
func (e *ldapEnv) enrol(t *testing.T, user, pass string) (string, []string) {
	t.Helper()
	secret, recovery := e.idEnv.enrol(t, user, pass)
	time.Sleep(3 * time.Millisecond)
	return secret, recovery
}

func TestLDAPMFAEnrolLoginAndDisable(t *testing.T) {
	e := newLDAPEnv(t, auth.MFAPolicyOff, map[string][2]string{"dora": {"dora-dir-pw", "operator"}})

	// Before enrolment: a directory account signs in with its password alone.
	if w := e.do("GET", "/api/v1/session", "", withBasic("dora", "dora-dir-pw")); w.Code != 200 || !strings.Contains(w.Body.String(), `"auth":"ldap"`) {
		t.Fatalf("basic before mfa: %d %s", w.Code, w.Body.String())
	}
	var st mfaStatusView
	w := e.do("GET", "/api/v1/auth/mfa", "", withBasic("dora", "dora-dir-pw"))
	decodeInto(t, w, &st)
	if !st.Available || st.Enabled || st.Managed != "" {
		t.Fatalf("mfa status for an LDAP caller: %+v", st)
	}

	// Setup re-verifies the directory password.
	w = e.do("POST", "/api/v1/auth/mfa/setup", jsonBody(map[string]string{"password": "wrong"}), withBasic("dora", "dora-dir-pw"))
	if w.Code != 403 {
		t.Fatalf("setup with the wrong directory password: %d %s", w.Code, w.Body.String())
	}
	secret, recovery := e.enrol(t, "dora", "dora-dir-pw")

	// Basic alone is now refused, with the MFA header (not a wrong-password 401).
	w = e.do("GET", "/api/v1/session", "", withBasic("dora", "dora-dir-pw"))
	if w.Code != 401 || w.Header().Get(auth.MFAHeader) != "required" {
		t.Fatalf("basic after mfa: %d %q", w.Code, w.Header().Get(auth.MFAHeader))
	}
	// A wrong directory password reveals nothing about MFA.
	w = e.do("GET", "/api/v1/session", "", withBasic("dora", "nope"))
	if w.Code != 401 || w.Header().Get(auth.MFAHeader) != "" {
		t.Fatalf("wrong password after mfa: %d %q", w.Code, w.Header().Get(auth.MFAHeader))
	}
	if w := e.login("dora", "nope", nextCode(secret)); w.Code != 401 || strings.Contains(w.Body.String(), `"mfa"`) {
		t.Fatalf("login with wrong password: %d %s", w.Code, w.Body.String())
	}
	// The login endpoint asks for the code, then issues an LDAP session.
	if w := e.login("dora", "dora-dir-pw", ""); w.Code != 401 || w.Header().Get(auth.MFAHeader) != "required" {
		t.Fatalf("login without code: %d", w.Code)
	}
	code := nextCode(secret)
	lw := e.login("dora", "dora-dir-pw", code)
	cookie := sessionCookie(lw)
	if lw.Code != 200 || cookie == "" || !strings.Contains(lw.Body.String(), `"role":"operator"`) {
		t.Fatalf("login with code: %d %s", lw.Code, lw.Body.String())
	}
	sess, ok := auth.ParseSession("test-secret", cookie)
	if !ok || sess.Issuer != auth.LDAPSessionIssuer || sess.Role != "operator" {
		t.Fatalf("session: %+v ok=%v", sess, ok)
	}
	w = e.do("GET", "/api/v1/session", "", withCookie(cookie))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"auth":"ldap"`) {
		t.Fatalf("ldap session: %d %s", w.Code, w.Body.String())
	}
	// The same code again is a replay.
	if w := e.login("dora", "dora-dir-pw", code); w.Code != 401 {
		t.Fatalf("replayed code: %d", w.Code)
	}
	// A recovery code works exactly once.
	if w := e.login("dora", "dora-dir-pw", recovery[0]); w.Code != 200 {
		t.Fatalf("recovery code: %d %s", w.Code, w.Body.String())
	}
	if w := e.login("dora", "dora-dir-pw", recovery[0]); w.Code != 401 {
		t.Fatalf("recovery code reused: %d", w.Code)
	}
	w = e.do("GET", "/api/v1/auth/mfa", "", withCookie(cookie))
	decodeInto(t, w, &st)
	if !st.Available || !st.Enabled || st.RecoveryCodes != 9 {
		t.Fatalf("mfa status after enrolling: %+v", st)
	}

	// Disable needs the directory password and a code.
	bad := jsonBody(map[string]string{"password": "wrong", "code": recovery[1]})
	if w := e.do("POST", "/api/v1/auth/mfa/disable", bad, withCookie(cookie)); w.Code != 403 {
		t.Fatalf("disable with wrong password: %d", w.Code)
	}
	good := jsonBody(map[string]string{"password": "dora-dir-pw", "code": recovery[2]})
	if w := e.do("POST", "/api/v1/auth/mfa/disable", good, withCookie(cookie)); w.Code != 200 {
		t.Fatalf("disable: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(cookie)); w.Code != 401 {
		t.Fatalf("session survived disabling MFA: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/session", "", withBasic("dora", "dora-dir-pw")); w.Code != 200 {
		t.Fatalf("basic after disable: %d", w.Code)
	}
	if e.ext.MFAEnrolled("dora") || len(e.ext.List()) != 0 {
		t.Fatal("disabling left a record behind")
	}
}

func TestLDAPMFAWrongCodeIsThrottled(t *testing.T) {
	e := newLDAPEnv(t, auth.MFAPolicyOff, map[string][2]string{"eli": {"eli-dir-pw", "viewer"}})
	e.enrol(t, "eli", "eli-dir-pw")
	var last int
	for i := 0; i < 12; i++ {
		last = e.login("eli", "eli-dir-pw", "000000").Code
	}
	if last != 429 {
		t.Fatalf("repeated wrong codes were not rate limited: %d", last)
	}
}

func TestLDAPMFAAdminResetAndList(t *testing.T) {
	e := newLDAPEnv(t, auth.MFAPolicyOff, map[string][2]string{
		"fay": {"fay-dir-pw", "operator"}, "gus": {"gus-dir-pw", "operator"},
	})
	secret, _ := e.enrol(t, "fay", "fay-dir-pw")
	cookie := sessionCookie(e.login("fay", "fay-dir-pw", nextCode(secret)))
	if w := e.do("GET", "/api/v1/session", "", withCookie(cookie)); w.Code != 200 {
		t.Fatalf("fay session: %d", w.Code)
	}

	// Only an admin may list, and the list carries no secret material.
	if w := e.do("GET", "/api/v1/users/external-mfa", "", withBasic("gus", "gus-dir-pw")); w.Code != 403 {
		t.Fatalf("operator list: %d", w.Code)
	}
	w := e.do("GET", "/api/v1/users/external-mfa", "", withBasic(idAdminUser, idAdminPass))
	if w.Code != 200 {
		t.Fatalf("admin list: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	var list []struct {
		Username  string `json:"username"`
		Enabled   bool   `json:"enabled"`
		UpdatedAt string `json:"updatedAt"`
	}
	decodeInto(t, w, &list)
	if len(list) != 1 || list[0].Username != "fay" || !list[0].Enabled || list[0].UpdatedAt == "" {
		t.Fatalf("list: %s", body)
	}
	for _, leak := range []string{secret, "secret", "pending", "recovery", "lastStep"} {
		if strings.Contains(body, leak) {
			t.Fatalf("list leaks %q: %s", leak, body)
		}
	}

	// An operator cannot reset; an admin can, and fay's sessions end.
	if w := e.do("DELETE", "/api/v1/users/fay/mfa", "", withBasic("gus", "gus-dir-pw")); w.Code != 403 {
		t.Fatalf("operator reset: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/users/fay/mfa", "", withBasic(idAdminUser, idAdminPass)); w.Code != 200 {
		t.Fatalf("admin reset: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/session", "", withCookie(cookie)); w.Code != 401 {
		t.Fatalf("fay's session survived the reset: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/session", "", withBasic("fay", "fay-dir-pw")); w.Code != 200 {
		t.Fatalf("fay with password after reset: %d", w.Code)
	}
	// Neither a local account nor a directory record: 404.
	if w := e.do("DELETE", "/api/v1/users/fay/mfa", "", withBasic(idAdminUser, idAdminPass)); w.Code != 404 {
		t.Fatalf("reset with nothing to reset: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/users/nobody/mfa", "", withBasic(idAdminUser, idAdminPass)); w.Code != 404 {
		t.Fatalf("reset unknown user: %d", w.Code)
	}
}

func TestLDAPMFARequirePolicy(t *testing.T) {
	e := newLDAPEnv(t, auth.MFAPolicyOperator, map[string][2]string{
		"hal": {"hal-dir-pw", "operator"}, "ivy": {"ivy-dir-pw", "viewer"},
	})
	// Covered (operator) and not enrolled: held at enrolment.
	if w := e.do("GET", "/api/v1/overview", "", withBasic("hal", "hal-dir-pw")); !isEnrolmentBlock(w.Code, w.Body.String()) {
		t.Fatalf("unenrolled covered LDAP user: %d %s", w.Code, w.Body.String())
	}
	if req, en := e.sessionMFA(t, withBasic("hal", "hal-dir-pw")); !req || en {
		t.Fatalf("session mfa before: required=%v enrolled=%v", req, en)
	}
	// Signing in through the login endpoint still works and is held too.
	held := sessionCookie(e.login("hal", "hal-dir-pw", ""))
	if w := e.do("GET", "/api/v1/overview", "", withCookie(held)); !isEnrolmentBlock(w.Code, w.Body.String()) {
		t.Fatalf("unenrolled LDAP session: %d %s", w.Code, w.Body.String())
	}
	// Not covered (viewer): unaffected.
	if w := e.do("GET", "/api/v1/overview", "", withBasic("ivy", "ivy-dir-pw")); w.Code != 200 {
		t.Fatalf("uncovered LDAP user: %d", w.Code)
	}
	// Enrolling releases the hold.
	secret, _ := e.enrol(t, "hal", "hal-dir-pw")
	cookie := sessionCookie(e.login("hal", "hal-dir-pw", nextCode(secret)))
	if w := e.do("GET", "/api/v1/overview", "", withCookie(cookie)); w.Code != 200 {
		t.Fatalf("after enrolling: %d %s", w.Code, w.Body.String())
	}
	if req, en := e.sessionMFA(t, withCookie(cookie)); !req || !en {
		t.Fatalf("session mfa after: required=%v enrolled=%v", req, en)
	}
	// An SSO session of the same name is not an LDAP identity: unaffected.
	tok, err := auth.SignSession("test-secret", auth.Session{User: "ivy", Role: "operator", Issuer: "https://idp.example"})
	if err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/api/v1/overview", "", withCookie(tok)); w.Code != 200 {
		t.Fatalf("SSO session: %d %s", w.Code, w.Body.String())
	}
	if req, _ := e.sessionMFA(t, withCookie(tok)); req {
		t.Fatal("SSO session reported as required")
	}
	var st mfaStatusView
	decodeInto(t, e.do("GET", "/api/v1/auth/mfa", "", withCookie(tok)), &st)
	if st.Available {
		t.Fatalf("SSO caller offered directory MFA: %+v", st)
	}
	if w := e.do("POST", "/api/v1/auth/mfa/setup", jsonBody(map[string]string{"password": "ivy-dir-pw"}), withCookie(tok)); w.Code != 400 {
		t.Fatalf("SSO caller setup: %d %s", w.Code, w.Body.String())
	}
}

// A local account of the same name takes precedence over the directory one:
// its own second factor applies, and the directory store is not used.
func TestLDAPMFALocalAccountTakesPrecedence(t *testing.T) {
	e := newLDAPEnv(t, auth.MFAPolicyOff, map[string][2]string{"jo": {"jo-dir-pw", "operator"}})
	e.addUser(t, "jo", "jo-local-pw", "viewer")
	secret, _ := e.enrol(t, "jo", "jo-local-pw")
	if e.ext.MFAEnrolled("jo") || len(e.ext.List()) != 0 {
		t.Fatal("enrolment of the local account went to the directory store")
	}
	if !e.users.MFAEnrolled("jo") {
		t.Fatal("local account not enrolled")
	}
	// The directory password does not bypass the local account's second factor.
	w := e.do("GET", "/api/v1/session", "", withBasic("jo", "jo-dir-pw"))
	if w.Code != 401 || w.Header().Get(auth.MFAHeader) != "required" {
		t.Fatalf("directory password for a name with a local second factor: %d %q", w.Code, w.Header().Get(auth.MFAHeader))
	}
	if w := e.login("jo", "jo-local-pw", nextCode(secret)); w.Code != 200 {
		t.Fatalf("local login: %d %s", w.Code, w.Body.String())
	}
	// Admin reset resets the local account.
	if w := e.do("DELETE", "/api/v1/users/jo/mfa", "", withBasic(idAdminUser, idAdminPass)); w.Code != 200 {
		t.Fatalf("reset: %d", w.Code)
	}
	if e.users.MFAEnrolled("jo") {
		t.Fatal("local account still enrolled after reset")
	}
}

func TestLDAPMFASecretsNeverInJSON(t *testing.T) {
	e := newLDAPEnv(t, auth.MFAPolicyOff, map[string][2]string{"kim": {"kim-dir-pw", "admin"}})
	secret, recovery := e.enrol(t, "kim", "kim-dir-pw")
	cookie := sessionCookie(e.login("kim", "kim-dir-pw", nextCode(secret)))
	for _, path := range []string{"/api/v1/session", "/api/v1/auth/mfa", "/api/v1/users", "/api/v1/users/external-mfa"} {
		w := e.do("GET", path, "", withCookie(cookie))
		if w.Code != 200 {
			t.Fatalf("GET %s: %d %s", path, w.Code, w.Body.String())
		}
		b := w.Body.String()
		if strings.Contains(b, secret) || strings.Contains(b, auth.HashRecoveryCode(recovery[1])) || strings.Contains(b, recovery[1]) {
			t.Fatalf("GET %s leaks secret material: %s", path, b)
		}
	}
}
