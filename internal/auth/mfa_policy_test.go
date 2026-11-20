package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// policyUsers is a local user store for the require-MFA tests: it
// authenticates, looks accounts up for sessions and says who has enrolled.
type policyUsers struct {
	roles    map[string]string
	ns       map[string][]string
	enrolled map[string]bool
}

func (p *policyUsers) Authenticate(u, pw string) (string, []string, bool) {
	role, ok := p.roles[u]
	if !ok || pw != "pw" {
		return "", nil, false
	}
	return role, p.ns[u], true
}
func (p *policyUsers) Lookup(u string) (string, []string, bool) {
	role, ok := p.roles[u]
	return role, p.ns[u], ok
}
func (p *policyUsers) MFAEnrolled(u string) bool { return p.enrolled[u] }
func (p *policyUsers) VerifySecondFactor(u, code string, _ time.Time) bool {
	return p.enrolled[u] && code == "123456"
}

func newPolicyUsers() *policyUsers {
	return &policyUsers{
		roles: map[string]string{
			"adm": "admin", "op": "operator", "view": "viewer", "rview": "viewer",
			"adm-mfa": "admin", "op-mfa": "operator", "view-mfa": "viewer",
		},
		ns:       map[string][]string{"rview": {"team-a"}},
		enrolled: map[string]bool{"adm-mfa": true, "op-mfa": true, "view-mfa": true},
	}
}

func policyMFAGuard(p MFAPolicy, users *policyUsers, envTOTP *EnvTOTP) (Guard, http.Handler) {
	g := Guard{Username: "root", Password: "rootpw", APIKey: "the-api-key", SessionSecret: "s3cret",
		Users: users, MFA: users, RequireMFA: p, EnvTOTP: envTOTP}
	return g, g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
}

func mfaBasic(u, p string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(u+":"+p))
}

func sessionToken(t *testing.T, s Session) string {
	t.Helper()
	tok, err := SignSession("s3cret", s)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func serveMFA(h http.Handler, method, path, authz string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = "192.0.2.1:1"
	if authz != "" {
		r.Header.Set("Authorization", authz)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestParseMFAPolicy(t *testing.T) {
	for in, want := range map[string]MFAPolicy{"": MFAPolicyOff, "off": MFAPolicyOff, " Admin ": MFAPolicyAdmin, "operator": MFAPolicyOperator, "ALL": MFAPolicyAll} {
		got, ok := ParseMFAPolicy(in)
		if !ok || got != want {
			t.Errorf("ParseMFAPolicy(%q) = %v,%v want %v", in, got, ok, want)
		}
		if ok && in != "" && !strings.EqualFold(strings.TrimSpace(in), got.String()) {
			t.Errorf("String() round trip for %q: %q", in, got.String())
		}
	}
	for _, bad := range []string{"yes", "admins", "viewer", "1"} {
		if _, ok := ParseMFAPolicy(bad); ok {
			t.Errorf("ParseMFAPolicy(%q) accepted", bad)
		}
	}
}

// Each policy value × each role × enrolled or not, through the real guard.
func TestRequireMFAPolicyMatrix(t *testing.T) {
	covered := map[MFAPolicy]map[string]bool{
		MFAPolicyOff:      {},
		MFAPolicyAdmin:    {"admin": true},
		MFAPolicyOperator: {"admin": true, "operator": true},
		MFAPolicyAll:      {"admin": true, "operator": true, "viewer": true},
	}
	for _, p := range []MFAPolicy{MFAPolicyOff, MFAPolicyAdmin, MFAPolicyOperator, MFAPolicyAll} {
		users := newPolicyUsers()
		_, h := policyMFAGuard(p, users, nil)
		for _, role := range []string{"admin", "operator", "viewer"} {
			for _, enrolled := range []bool{false, true} {
				name := map[string]string{"admin": "adm", "operator": "op", "viewer": "view"}[role]
				if enrolled {
					name += "-mfa"
				}
				// An enrolled account can only hold a session (Basic is refused
				// once MFA is on), so test everyone through a local session.
				tok := sessionToken(t, Session{User: name, Role: role, Issuer: LocalSessionIssuer})
				w := serveMFA(h, "GET", "/api/v1/overview", "Bearer axiom."+tok)
				want := 204
				if covered[p][role] && !enrolled {
					want = 403
				}
				if w.Code != want {
					t.Errorf("policy=%s role=%s enrolled=%v: got %d want %d (%s)", p, role, enrolled, w.Code, want, w.Body.String())
				}
				if !enrolled {
					// Basic auth for a not-yet-enrolled account works the same.
					if w := serveMFA(h, "GET", "/api/v1/overview", mfaBasic(name, "pw")); w.Code != want {
						t.Errorf("basic policy=%s role=%s: got %d want %d", p, role, w.Code, want)
					}
				}
			}
		}
	}
}

func TestRequireMFABlockedBodyAndAllowList(t *testing.T) {
	users := newPolicyUsers()
	_, h := policyMFAGuard(MFAPolicyAll, users, nil)
	w := serveMFA(h, "GET", "/api/v1/pods", mfaBasic("op", "pw"))
	if w.Code != 403 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("blocked: %d %v", w.Code, w.Header())
	}
	var body struct {
		Error                 string `json:"error"`
		MFAEnrollmentRequired bool   `json:"mfaEnrollmentRequired"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "two-factor sign-in is required for your role: set it up under Account security" || !body.MFAEnrollmentRequired {
		t.Fatalf("body: %s", w.Body.String())
	}
	allowed := []struct{ method, path string }{
		{"GET", "/api/v1/session"}, {"GET", "/api/v1/auth/providers"},
		{"POST", "/api/v1/auth/logout"}, {"POST", "/api/v1/auth/logout-all"},
		{"GET", "/api/v1/auth/mfa"}, {"POST", "/api/v1/auth/mfa/setup"},
		{"POST", "/api/v1/auth/mfa/enable"}, {"POST", "/api/v1/auth/mfa/disable"},
		{"GET", "/api/v1/health"}, {"GET", "/api/v1/metrics"},
	}
	for _, who := range []string{"adm", "op", "view", "rview"} {
		for _, a := range allowed {
			if w := serveMFA(h, a.method, a.path, mfaBasic(who, "pw")); w.Code != 204 {
				t.Errorf("%s %s %s: %d %s", who, a.method, a.path, w.Code, w.Body.String())
			}
		}
	}
	blocked := []struct{ method, path string }{
		{"GET", "/api/v1/overview"}, {"POST", "/api/v1/ws-ticket"}, {"GET", "/api/v1/users"},
		{"POST", "/api/v1/auth/mfa/other"}, {"GET", "/api/v1/auth/mfa/setup"},
		{"GET", "/api/v1/session/x"}, {"POST", "/api/v1/apply"}, {"DELETE", "/api/v1/users/x/mfa"},
		{"GET", "/api/v1/pods?namespace=team-a"},
	}
	for _, who := range []string{"adm", "op", "view", "rview"} {
		for _, b := range blocked {
			w := serveMFA(h, b.method, b.path, mfaBasic(who, "pw"))
			if w.Code != 403 || !strings.Contains(w.Body.String(), `"mfaEnrollmentRequired":true`) {
				t.Errorf("%s %s %s: %d %s", who, b.method, b.path, w.Code, w.Body.String())
			}
		}
	}
}

func TestRequireMFAEnrolThenWorks(t *testing.T) {
	users := newPolicyUsers()
	_, h := policyMFAGuard(MFAPolicyOperator, users, nil)
	tok := "Bearer axiom." + sessionToken(t, Session{User: "op", Role: "operator", Issuer: LocalSessionIssuer})
	if w := serveMFA(h, "GET", "/api/v1/overview", tok); w.Code != 403 {
		t.Fatalf("before enrolling: %d", w.Code)
	}
	users.enrolled["op"] = true
	if w := serveMFA(h, "GET", "/api/v1/overview", tok); w.Code != 204 {
		t.Fatalf("after enrolling: %d", w.Code)
	}
	// Disabling is allowed, but the next call is blocked again.
	if w := serveMFA(h, "POST", "/api/v1/auth/mfa/disable", tok); w.Code != 204 {
		t.Fatalf("disable: %d", w.Code)
	}
	users.enrolled["op"] = false
	if w := serveMFA(h, "GET", "/api/v1/overview", tok); w.Code != 403 {
		t.Fatalf("after disabling: %d", w.Code)
	}
}

func TestRequireMFAEnvAdmin(t *testing.T) {
	// No AXIOM_TOTP_SECRET: break-glass, exempt under every policy.
	for _, p := range []MFAPolicy{MFAPolicyAdmin, MFAPolicyOperator, MFAPolicyAll} {
		g, h := policyMFAGuard(p, newPolicyUsers(), nil)
		if !g.EnvAdminExemptFromMFA() {
			t.Errorf("policy %s: env admin should be reported exempt", p)
		}
		if w := serveMFA(h, "GET", "/api/v1/users", mfaBasic("root", "rootpw")); w.Code != 204 {
			t.Errorf("policy %s: env admin basic: %d", p, w.Code)
		}
		tok := "Bearer axiom." + sessionToken(t, Session{User: "root", Role: "admin", Issuer: LocalSessionIssuer})
		if w := serveMFA(h, "GET", "/api/v1/users", tok); w.Code != 204 {
			t.Errorf("policy %s: env admin session: %d", p, w.Code)
		}
		if req, _ := g.MFAState(Identity{User: "root", Role: "admin", source: sourceEnv}); req {
			t.Errorf("policy %s: exempt env admin reported as required", p)
		}
	}
	// With AXIOM_TOTP_SECRET the env admin signs in with a code: satisfied.
	et, _ := NewEnvTOTP(GenerateTOTPSecret())
	g, h := policyMFAGuard(MFAPolicyAll, newPolicyUsers(), et)
	if g.EnvAdminExemptFromMFA() {
		t.Error("env admin with a TOTP secret is not exempt, it is enrolled")
	}
	tok := "Bearer axiom." + sessionToken(t, Session{User: "root", Role: "admin", Issuer: LocalSessionIssuer})
	if w := serveMFA(h, "GET", "/api/v1/users", tok); w.Code != 204 {
		t.Errorf("env admin with TOTP session: %d", w.Code)
	}
	if req, en := g.MFAState(Identity{User: "root", Role: "admin", source: sourceEnv}); !req || !en {
		t.Errorf("env admin with TOTP: required=%v enrolled=%v", req, en)
	}
	// Policy off: never exempt-logged.
	if g, _ := policyMFAGuard(MFAPolicyOff, newPolicyUsers(), nil); g.EnvAdminExemptFromMFA() {
		t.Error("policy off reported an exemption")
	}
}

func TestRequireMFAIgnoresExternalIdentities(t *testing.T) {
	users := newPolicyUsers()
	_, h := policyMFAGuard(MFAPolicyAll, users, nil)
	// An SSO session, even one whose name matches an unenrolled local account.
	for _, iss := range []string{"https://idp.example", "saml"} {
		for _, name := range []string{"op", "sso-person"} {
			tok := "Bearer axiom." + sessionToken(t, Session{User: name, Role: "admin", Issuer: iss})
			if w := serveMFA(h, "GET", "/api/v1/overview", tok); w.Code != 204 {
				t.Errorf("sso %s/%s: %d %s", iss, name, w.Code, w.Body.String())
			}
		}
	}
	// API keys.
	if w := serveMFA(h, "GET", "/api/v1/users", "Bearer the-api-key"); w.Code != 204 {
		t.Errorf("api key: %d", w.Code)
	}
	if w := serveMFA(h, "GET", "/api/v1/users", mfaBasic("apikey", "the-api-key")); w.Code != 204 {
		t.Errorf("api key via basic: %d", w.Code)
	}
	g := Guard{RequireMFA: MFAPolicyAll}
	for _, id := range []Identity{{User: "x", Role: "admin"}, {User: "apikey", Role: "admin"}} {
		if req, _ := g.MFAState(id); req {
			t.Errorf("external identity %+v reported as required", id)
		}
	}
}

// A ticket inherits the identity it was minted for, so it cannot be used to
// step around the policy (minting itself is blocked for unenrolled accounts).
func TestRequireMFABlocksTicketMinting(t *testing.T) {
	users := newPolicyUsers()
	_, h := policyMFAGuard(MFAPolicyAll, users, nil)
	if w := serveMFA(h, "POST", "/api/v1/ws-ticket", mfaBasic("view", "pw")); w.Code != 403 {
		t.Fatalf("ticket minting: %d", w.Code)
	}
}
