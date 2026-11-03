package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func policyGuard() http.Handler {
	users := &fakeUserAuthenticator{users: map[string]struct {
		password   string
		role       string
		namespaces []string
	}{
		"viewer":  {password: "pw", role: "viewer"},
		"oper":    {password: "pw", role: "operator"},
		"rviewer": {password: "pw", role: "viewer", namespaces: []string{"team-a"}},
		"roper":   {password: "pw", role: "operator", namespaces: []string{"team-a"}},
	}}
	g := Guard{Username: "demo", Password: "demo", Users: users}
	return g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
}

func TestGuardPolicy(t *testing.T) {
	h := policyGuard()
	cases := []struct {
		name, user, method, path string
		want                     int
	}{
		// Viewer must not get interactive sessions through GET/WebSocket routes.
		{"viewer terminal", "viewer", "GET", "/api/v1/pods/ns/p/terminal", 403},
		{"viewer vnc", "viewer", "GET", "/api/v1/vms/ns/v/vnc", 403},
		{"viewer console", "viewer", "GET", "/api/v1/vms/ns/v/console", 403},
		{"viewer pod window proxy", "viewer", "GET", "/api/v1/pods/ns/p/window/8080/", 403},
		{"viewer logs still allowed", "viewer", "GET", "/api/v1/pods/ns/p/logs", 204},
		{"viewer pod detail still allowed", "viewer", "GET", "/api/v1/pods/ns/p", 204},
		{"operator terminal allowed", "oper", "GET", "/api/v1/pods/ns/p/terminal", 204},
		// Secret values.
		{"viewer secret values", "viewer", "GET", "/api/v1/secrets/ns/s?values=true", 403},
		{"viewer secret decode", "viewer", "GET", "/api/v1/secrets/ns/s?decode=true", 403},
		{"viewer secret metadata ok", "viewer", "GET", "/api/v1/secrets/ns/s", 204},
		{"operator secret values ok", "oper", "GET", "/api/v1/secrets/ns/s?values=true", 204},
		{"operator axiom-system secret values", "oper", "GET", "/api/v1/secrets/axiom-system/zyvor-axiom-remote-clusters?values=true", 403},
		{"admin axiom-system secret values", "demo", "GET", "/api/v1/secrets/axiom-system/zyvor-axiom-remote-clusters?values=true", 204},
		// Operator is not an admin.
		{"operator rbac role create", "oper", "POST", "/api/v1/rbac/roles", 403},
		{"operator rbac read ok", "oper", "GET", "/api/v1/rbac", 204},
		{"operator helm install", "oper", "POST", "/api/v1/helm/install", 403},
		{"operator helm upgrade", "oper", "POST", "/api/v1/helm/upgrade", 403},
		{"operator integration test", "oper", "POST", "/api/v1/integrations/slack/test", 403},
		{"operator integrations read ok", "oper", "GET", "/api/v1/integrations", 204},
		{"operator apply still allowed", "oper", "POST", "/api/v1/apply", 204},
		{"operator helm rollback allowed", "oper", "POST", "/api/v1/helm/releases/ns/r/rollback", 204},
		{"admin helm install", "demo", "POST", "/api/v1/helm/install", 204},
		// Namespace restriction: path namespace must be authorized even when a
		// permitted ?namespace= is attached.
		{"path/query mismatch", "rviewer", "GET", "/api/v1/secrets/kube-system/x?namespace=team-a", 403},
		{"path allowed + query allowed", "rviewer", "GET", "/api/v1/secrets/team-a/x?namespace=team-a", 204},
		{"query allowed on static subroute", "rviewer", "GET", "/api/v1/events/stream?namespace=team-a", 204},
		{"crd route keeps query semantics", "rviewer", "GET", "/api/v1/crds/apps/v1/deployments?namespace=team-a", 204},
		// Restricted identities cannot reach cluster-wide admin surfaces.
		{"restricted operator apply", "roper", "POST", "/api/v1/apply", 403},
		{"restricted viewer audit", "rviewer", "GET", "/api/v1/audit", 403},
		{"restricted operator delete namespace", "roper", "DELETE", "/api/v1/namespaces/kube-system", 403},
		{"restricted operator helm install", "roper", "POST", "/api/v1/helm/install", 403},
		{"restricted operator own-namespace write", "roper", "POST", "/api/v1/workloads/team-a/Deployment/x/restart", 204},
		{"restricted operator other-namespace write", "roper", "POST", "/api/v1/workloads/team-b/Deployment/x/restart", 403},

		// --- Workstream C: RBAC viewer (read-only; viewers + restricted denied) ---
		{"viewer rbac subjects", "viewer", "GET", "/api/v1/rbac/subjects", 403},
		{"viewer rbac subject detail", "viewer", "GET", "/api/v1/rbac/subjects/User/alice", 403},
		{"viewer rbac who-can", "viewer", "GET", "/api/v1/rbac/who-can?verb=get&resource=secrets", 403},
		{"operator rbac subjects", "oper", "GET", "/api/v1/rbac/subjects", 204},
		{"operator rbac subject detail", "oper", "GET", "/api/v1/rbac/subjects/ServiceAccount/web?namespace=default", 204},
		{"operator rbac who-can", "oper", "GET", "/api/v1/rbac/who-can?verb=get&resource=secrets", 204},
		{"operator still cannot write rbac subjects", "oper", "POST", "/api/v1/rbac/subjects", 403},
		{"operator unknown rbac sub-path still forbidden", "oper", "GET", "/api/v1/rbac/anything-else", 403},
		{"admin rbac subjects", "demo", "GET", "/api/v1/rbac/subjects", 204},
		{"admin rbac who-can", "demo", "GET", "/api/v1/rbac/who-can?verb=list&resource=pods&namespace=team-a", 204},
		{"restricted viewer rbac subjects", "rviewer", "GET", "/api/v1/rbac/subjects", 403},
		{"restricted operator rbac who-can in own ns", "roper", "GET", "/api/v1/rbac/who-can?verb=get&resource=pods&namespace=team-a", 403},
		{"restricted operator rbac subject detail", "roper", "GET", "/api/v1/rbac/subjects/User/x", 403},
		{"viewer service accounts list ok", "viewer", "GET", "/api/v1/serviceaccounts?namespace=team-a", 204},
		{"restricted serviceaccounts own ns", "rviewer", "GET", "/api/v1/serviceaccounts?namespace=team-a", 204},
		{"restricted serviceaccounts other ns", "rviewer", "GET", "/api/v1/serviceaccounts?namespace=team-b", 403},
		{"restricted serviceaccounts all namespaces", "rviewer", "GET", "/api/v1/serviceaccounts", 403},
		// --- NetworkPolicy analysis (namespace in the path) ---
		{"viewer netpol analysis", "viewer", "GET", "/api/v1/networking/policies/team-a/analysis", 204},
		{"restricted netpol analysis own ns", "rviewer", "GET", "/api/v1/networking/policies/team-a/analysis", 204},
		{"restricted netpol analysis other ns", "rviewer", "GET", "/api/v1/networking/policies/team-b/analysis", 403},
		{"restricted netpol analysis path/query mismatch", "rviewer", "GET", "/api/v1/networking/policies/team-b/analysis?namespace=team-a", 403},
		// --- Security reports ---
		{"viewer security reports", "viewer", "GET", "/api/v1/security/reports", 204},
		{"restricted security reports own ns", "rviewer", "GET", "/api/v1/security/reports?namespace=team-a", 204},
		{"restricted security reports no ns", "rviewer", "GET", "/api/v1/security/reports", 403},
		{"restricted security reports other ns", "rviewer", "GET", "/api/v1/security/reports?namespace=team-b", 403},
		{"restricted security drilldown own ns", "rviewer", "GET", "/api/v1/security/reports/team-a/Deployment/web", 204},
		{"restricted security drilldown other ns", "rviewer", "GET", "/api/v1/security/reports/team-b/Deployment/web", 403},
		{"restricted security drilldown mismatch", "rviewer", "GET", "/api/v1/security/reports/team-b/Deployment/web?namespace=team-a", 403},
		// --- Stream tickets: minting is allowed for every authenticated role ---
		{"viewer can mint a ticket", "viewer", "POST", "/api/v1/ws-ticket", 204},
		{"operator can mint a ticket", "oper", "POST", "/api/v1/ws-ticket", 204},
		{"restricted viewer can mint a ticket", "rviewer", "POST", "/api/v1/ws-ticket", 204},
		{"restricted operator can mint a ticket", "roper", "POST", "/api/v1/ws-ticket", 204},
		{"ticket exemption is not a prefix", "viewer", "POST", "/api/v1/ws-ticket/other", 403},
		{"ticket exemption does not cover restricted writes", "roper", "POST", "/api/v1/ws-ticket/other", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			pw := "pw"
			if tc.user == "demo" {
				pw = "demo"
			}
			r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(tc.user+":"+pw)))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("%s %s as %s: got %d, want %d", tc.method, tc.path, tc.user, w.Code, tc.want)
			}
		})
	}
}
