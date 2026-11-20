package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func troubleshootGuard() http.Handler {
	users := &fakeUserAuthenticator{users: map[string]struct {
		password   string
		role       string
		namespaces []string
	}{
		"viewer":  {password: "pw", role: "viewer"},
		"oper":    {password: "pw", role: "operator"},
		"admin2":  {password: "pw", role: "admin"},
		"rviewer": {password: "pw", role: "viewer", namespaces: []string{"team-a"}},
		"roper":   {password: "pw", role: "operator", namespaces: []string{"team-a"}},
		"radmin":  {password: "pw", role: "admin", namespaces: []string{"team-a"}},
	}}
	g := Guard{Username: "demo", Password: "demo", Users: users}
	return g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
}

// Policy rows for the Workstream B routes: timeline, health check, workload
// logs, ephemeral debug containers and the node shell.
func TestGuardPolicyTroubleshooting(t *testing.T) {
	h := troubleshootGuard()
	cases := []struct {
		name, user, method, path string
		want                     int
	}{
		// Timeline and health check are read-only and open to every role.
		{"viewer timeline", "viewer", "GET", "/api/v1/timeline", 204},
		{"viewer timeline object filter", "viewer", "GET", "/api/v1/timeline?namespace=payments&object=Deployment/payments-api", 204},
		{"viewer diagnose", "viewer", "GET", "/api/v1/diagnose", 204},
		{"operator diagnose namespaced", "oper", "GET", "/api/v1/diagnose?namespace=payments", 204},
		// Namespace-restricted identities must name an owned namespace.
		{"restricted timeline own ns", "rviewer", "GET", "/api/v1/timeline?namespace=team-a", 204},
		{"restricted timeline other ns", "rviewer", "GET", "/api/v1/timeline?namespace=team-b", 403},
		{"restricted timeline no ns", "rviewer", "GET", "/api/v1/timeline", 403},
		{"restricted timeline all ns", "rviewer", "GET", "/api/v1/timeline?namespace=all", 403},
		{"restricted diagnose own ns", "rviewer", "GET", "/api/v1/diagnose?namespace=team-a", 204},
		{"restricted diagnose no ns", "rviewer", "GET", "/api/v1/diagnose", 403},
		{"restricted diagnose other ns", "rviewer", "GET", "/api/v1/diagnose?namespace=kube-system", 403},
		// Workload logs follow the workloads path-namespace rules; viewers may read logs.
		{"viewer workload logs", "viewer", "GET", "/api/v1/workloads/ns/Deployment/web/logs?tail=50", 204},
		{"restricted workload logs own ns", "rviewer", "GET", "/api/v1/workloads/team-a/Deployment/web/logs", 204},
		{"restricted workload logs other ns", "rviewer", "GET", "/api/v1/workloads/team-b/Deployment/web/logs", 403},
		{"restricted workload logs query mismatch", "rviewer", "GET", "/api/v1/workloads/team-b/Deployment/web/logs?namespace=team-a", 403},
		// Debug containers: a write (operator+), never a viewer.
		{"viewer debug", "viewer", "POST", "/api/v1/pods/ns/p/debug", 403},
		{"viewer debug via GET", "viewer", "GET", "/api/v1/pods/ns/p/debug", 403},
		{"operator debug", "oper", "POST", "/api/v1/pods/ns/p/debug", 204},
		{"admin debug", "admin2", "POST", "/api/v1/pods/ns/p/debug", 204},
		{"restricted operator debug own ns", "roper", "POST", "/api/v1/pods/team-a/p/debug", 204},
		{"restricted operator debug other ns", "roper", "POST", "/api/v1/pods/team-b/p/debug", 403},
		{"restricted viewer debug own ns", "rviewer", "POST", "/api/v1/pods/team-a/p/debug", 403},
		{"debug capabilities for viewer", "viewer", "GET", "/api/v1/debug/capabilities", 204},
		// Node shell: admin only, never namespace-restricted, never viewers/operators.
		{"viewer node shell", "viewer", "GET", "/api/v1/nodes/n1/shell", 403},
		{"operator node shell", "oper", "GET", "/api/v1/nodes/n1/shell", 403},
		{"admin node shell", "admin2", "GET", "/api/v1/nodes/n1/shell", 204},
		{"built-in admin node shell", "demo", "GET", "/api/v1/nodes/n1/shell?shell=/bin/bash", 204},
		{"restricted admin node shell", "radmin", "GET", "/api/v1/nodes/n1/shell", 403},
		{"restricted operator node shell", "roper", "GET", "/api/v1/nodes/n1/shell", 403},
		{"restricted viewer node shell", "rviewer", "GET", "/api/v1/nodes/n1/shell", 403},
		{"node shell trailing slash operator", "oper", "GET", "/api/v1/nodes/n1/shell/", 403},
		// Look-alikes are not the node shell.
		{"node detail still readable", "viewer", "GET", "/api/v1/nodes/n1", 204},
		{"node metrics not a shell", "oper", "GET", "/api/v1/nodes/shell/metrics", 204},
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
				t.Fatalf("%s %s as %s: got %d (%s), want %d", tc.method, tc.path, tc.user, w.Code, w.Body.String(), tc.want)
			}
		})
	}
}

func TestIsNodeShellPath(t *testing.T) {
	yes := []string{"/api/v1/nodes/n1/shell", "/api/v1/nodes/ip-10-0-0-1.ec2.internal/shell", "/api/v1/nodes/n1/shell/"}
	no := []string{"/api/v1/nodes", "/api/v1/nodes/n1", "/api/v1/nodes/n1/metrics", "/api/v1/nodes//shell", "/api/v1/nodes/n1/shell/x", "/api/v1/pods/ns/p/shell", "/api/v1/nodes/shell"}
	for _, p := range yes {
		if !isNodeShellPath(p) {
			t.Errorf("%q should be the node shell", p)
		}
	}
	for _, p := range no {
		if isNodeShellPath(p) {
			t.Errorf("%q must not be the node shell", p)
		}
	}
}
