package api

import (
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// newIDEnvPolicy is newIDEnv with AXIOM_REQUIRE_MFA set.
func newIDEnvPolicy(t *testing.T, policy auth.MFAPolicy, totp string) *idEnv {
	t.Helper()
	hub := platform.NewUserHub(nil)
	store := &memRevStore{}
	h := NewWithUsers(platform.NewDemoProvider(), "demo", idAdminUser, idAdminPass, "", nil, hub,
		AuthOptions{SessionSecret: "test-secret", RevocationStore: store, TOTPSecret: totp, RequireMFA: policy})
	return &idEnv{h: h, users: hub, store: store}
}

type sessionMFA struct {
	MFA struct {
		Required bool `json:"required"`
		Enrolled bool `json:"enrolled"`
	} `json:"mfa"`
}

func (e *idEnv) sessionMFA(t *testing.T, opt reqOpt) (required, enrolled bool) {
	t.Helper()
	w := e.do("GET", "/api/v1/session", "", opt)
	if w.Code != 200 {
		t.Fatalf("session: %d %s", w.Code, w.Body.String())
	}
	var s sessionMFA
	decodeInto(t, w, &s)
	return s.MFA.Required, s.MFA.Enrolled
}

func isEnrolmentBlock(code int, body string) bool {
	return code == 403 && strings.Contains(body, `"mfaEnrollmentRequired":true`) &&
		strings.Contains(body, "two-factor sign-in is required for your role: set it up under Account security")
}

func TestRequireMFAPolicyThroughAPI(t *testing.T) {
	covers := map[auth.MFAPolicy]map[string]bool{
		auth.MFAPolicyOff:      {},
		auth.MFAPolicyAdmin:    {"admin": true},
		auth.MFAPolicyOperator: {"admin": true, "operator": true},
		auth.MFAPolicyAll:      {"admin": true, "operator": true, "viewer": true},
	}
	for policy, covered := range covers {
		e := newIDEnvPolicy(t, policy, "")
		for _, role := range []string{"admin", "operator", "viewer"} {
			plain, enrolled := "p-"+role, "e-"+role
			e.addUser(t, plain, plain+"-password", role)
			e.addUser(t, enrolled, enrolled+"-password", role)

			// Not enrolled: Basic still authenticates, but a covered role is
			// confined to enrolment.
			w := e.do("GET", "/api/v1/overview", "", withBasic(plain, plain+"-password"))
			if covered[role] {
				if !isEnrolmentBlock(w.Code, w.Body.String()) {
					t.Errorf("policy=%s %s unenrolled: %d %s", policy, role, w.Code, w.Body.String())
				}
			} else if w.Code != 200 {
				t.Errorf("policy=%s %s unenrolled (uncovered): %d", policy, role, w.Code)
			}
			req, en := e.sessionMFA(t, withBasic(plain, plain+"-password"))
			if req != covered[role] || en {
				t.Errorf("policy=%s %s unenrolled session mfa: required=%v enrolled=%v", policy, role, req, en)
			}

			// Enrolled (enrolment itself must get through the block).
			secret, _ := e.enrol(t, enrolled, enrolled+"-password")
			lw := e.login(enrolled, enrolled+"-password", nextCode(secret))
			cookie := sessionCookie(lw)
			if lw.Code != 200 || cookie == "" {
				t.Fatalf("policy=%s %s login: %d %s", policy, role, lw.Code, lw.Body.String())
			}
			if w := e.do("GET", "/api/v1/overview", "", withCookie(cookie)); w.Code != 200 {
				t.Errorf("policy=%s %s enrolled: %d %s", policy, role, w.Code, w.Body.String())
			}
			req, en = e.sessionMFA(t, withCookie(cookie))
			if req != covered[role] || !en {
				t.Errorf("policy=%s %s enrolled session mfa: required=%v enrolled=%v", policy, role, req, en)
			}
		}
	}
}

func TestRequireMFAAllowListThroughAPI(t *testing.T) {
	e := newIDEnvPolicy(t, auth.MFAPolicyAll, "")
	e.addUser(t, "olga", "olga-password", "operator")
	cookie := sessionCookie(e.login("olga", "olga-password", ""))
	if cookie == "" {
		t.Fatal("a not-yet-enrolled covered account must still be able to sign in")
	}
	for _, path := range []string{"/api/v1/session", "/api/v1/auth/mfa", "/api/v1/auth/providers", "/api/v1/health", "/api/v1/metrics"} {
		if w := e.do("GET", path, "", withCookie(cookie)); w.Code != 200 {
			t.Errorf("GET %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/api/v1/overview", "/api/v1/pods", "/api/v1/fleet", "/api/v1/cost/budgets"} {
		if w := e.do("GET", path, "", withCookie(cookie)); !isEnrolmentBlock(w.Code, w.Body.String()) {
			t.Errorf("GET %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := e.do("POST", "/api/v1/ws-ticket", "", withCookie(cookie)); !isEnrolmentBlock(w.Code, w.Body.String()) {
		t.Errorf("ws-ticket: %d", w.Code)
	}
	// Setup with the wrong password reaches the handler (403 from it, not the block).
	w := e.do("POST", "/api/v1/auth/mfa/setup", jsonBody(map[string]string{"password": "wrong"}), withCookie(cookie))
	if w.Code != 403 || strings.Contains(w.Body.String(), "mfaEnrollmentRequired") {
		t.Errorf("setup reached the handler: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/v1/auth/logout-all", "", withCookie(cookie)); w.Code != 200 {
		t.Errorf("logout-all: %d %s", w.Code, w.Body.String())
	}
	cookie = sessionCookie(e.login("olga", "olga-password", ""))
	if w := e.do("POST", "/api/v1/auth/logout", "", withCookie(cookie)); w.Code != 200 {
		t.Errorf("logout: %d", w.Code)
	}
}

// Enrolling lifts the block on the very next request (after the sign-in
// with a code that enabling requires), and disabling brings it back.
func TestRequireMFAEnrolThenDisable(t *testing.T) {
	e := newIDEnvPolicy(t, auth.MFAPolicyOperator, "")
	e.addUser(t, "pete", "pete-password", "operator")
	if w := e.do("GET", "/api/v1/overview", "", withBasic("pete", "pete-password")); w.Code != 403 {
		t.Fatalf("before enrolling: %d", w.Code)
	}
	secret, recovery := e.enrol(t, "pete", "pete-password")
	cookie := sessionCookie(e.login("pete", "pete-password", nextCode(secret)))
	if w := e.do("GET", "/api/v1/overview", "", withCookie(cookie)); w.Code != 200 {
		t.Fatalf("after enrolling: %d %s", w.Code, w.Body.String())
	}
	body := jsonBody(map[string]string{"password": "pete-password", "code": recovery[0]})
	if w := e.do("POST", "/api/v1/auth/mfa/disable", body, withCookie(cookie)); w.Code != 200 {
		t.Fatalf("disable is allowed: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/overview", "", withBasic("pete", "pete-password")); !isEnrolmentBlock(w.Code, w.Body.String()) {
		t.Fatalf("after disabling: %d %s", w.Code, w.Body.String())
	}
}

func TestRequireMFAEnvAdminThroughAPI(t *testing.T) {
	// Break glass: no AXIOM_TOTP_SECRET, so the env admin is exempt.
	e := newIDEnvPolicy(t, auth.MFAPolicyAll, "")
	if w := e.do("GET", "/api/v1/users", "", withBasic(idAdminUser, idAdminPass)); w.Code != 200 {
		t.Fatalf("exempt env admin: %d %s", w.Code, w.Body.String())
	}
	if req, en := e.sessionMFA(t, withBasic(idAdminUser, idAdminPass)); req || en {
		t.Fatalf("exempt env admin session mfa: required=%v enrolled=%v", req, en)
	}
	// With AXIOM_TOTP_SECRET it signs in with a code and is satisfied.
	secret := auth.GenerateTOTPSecret()
	e = newIDEnvPolicy(t, auth.MFAPolicyAdmin, secret)
	code, _ := auth.TOTPCodeAt(secret, time.Now())
	cookie := sessionCookie(e.login(idAdminUser, idAdminPass, code))
	if w := e.do("GET", "/api/v1/users", "", withCookie(cookie)); w.Code != 200 {
		t.Fatalf("env admin with TOTP: %d", w.Code)
	}
	if req, en := e.sessionMFA(t, withCookie(cookie)); !req || !en {
		t.Fatalf("env admin with TOTP session mfa: required=%v enrolled=%v", req, en)
	}
}

func TestRequireMFASSOAndAPIKeyUnaffected(t *testing.T) {
	hub := platform.NewUserHub(nil)
	h := NewWithUsers(platform.NewDemoProvider(), "demo", idAdminUser, idAdminPass, "", nil, hub,
		AuthOptions{SessionSecret: "test-secret", RevocationStore: &memRevStore{}, RequireMFA: auth.MFAPolicyAll, APIKey: "k3y"})
	e := &idEnv{h: h, users: hub}
	// A local account of the same name exists and has not enrolled.
	e.addUser(t, "quinn", "quinn-password", "admin")
	tok, err := auth.SignSession("test-secret", auth.Session{User: "quinn", Role: "admin", Issuer: "https://idp.example"})
	if err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/api/v1/overview", "", withCookie(tok)); w.Code != 200 {
		t.Fatalf("SSO session: %d %s", w.Code, w.Body.String())
	}
	if req, en := e.sessionMFA(t, withCookie(tok)); req || en {
		t.Fatalf("SSO session mfa: required=%v enrolled=%v", req, en)
	}
	if w := e.do("GET", "/api/v1/users", "", withBearer("k3y")); w.Code != 200 {
		t.Fatalf("API key: %d", w.Code)
	}
	if req, _ := e.sessionMFA(t, withBearer("k3y")); req {
		t.Fatal("API key reported as required")
	}
}

// A namespace-restricted viewer under "all" can still enrol (the MFA routes
// are self-service) and then use their namespace — and only it.
func TestRequireMFARestrictedViewer(t *testing.T) {
	e := newIDEnvPolicy(t, auth.MFAPolicyAll, "")
	e.addUser(t, "rita", "rita-password", "viewer", "payments")
	if w := e.do("GET", "/api/v1/pods?namespace=payments", "", withBasic("rita", "rita-password")); !isEnrolmentBlock(w.Code, w.Body.String()) {
		t.Fatalf("before enrolling: %d %s", w.Code, w.Body.String())
	}
	secret, _ := e.enrol(t, "rita", "rita-password")
	cookie := sessionCookie(e.login("rita", "rita-password", nextCode(secret)))
	if w := e.do("GET", "/api/v1/pods?namespace=payments", "", withCookie(cookie)); w.Code != 200 {
		t.Fatalf("own namespace after enrolling: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/pods?namespace=commerce", "", withCookie(cookie)); w.Code != 403 || strings.Contains(w.Body.String(), "mfaEnrollmentRequired") {
		t.Fatalf("other namespace after enrolling: %d %s", w.Code, w.Body.String())
	}
}
