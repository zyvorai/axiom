package auth

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

// TestGuardPolicyAI covers the opt-in AI assistant routes. POST /ai/explain only
// reads, but data leaves the cluster, so it is namespace-scoped; the settings and
// connection-test routes stay admin-only. (Same rows style as TestGuardPolicy.)
func TestGuardPolicyAI(t *testing.T) {
	h := policyGuard()
	cases := []struct {
		name, user, method, path string
		want                     int
	}{
		// Explain is a POST that only reads: a viewer is allowed through the guard.
		{"viewer explain", "viewer", "POST", "/api/v1/ai/explain?namespace=team-a", 204},
		{"operator explain", "oper", "POST", "/api/v1/ai/explain?namespace=team-a", 204},
		{"admin explain", "demo", "POST", "/api/v1/ai/explain?namespace=team-a", 204},
		// ...but a viewer still cannot reach any other write.
		{"viewer other ai POST denied", "viewer", "POST", "/api/v1/ai/other", 403},
		{"viewer put integrations denied", "viewer", "PUT", "/api/v1/integrations", 403},
		// Status is readable by everyone signed in and carries no cluster data.
		{"viewer ai status", "viewer", "GET", "/api/v1/ai/status", 204},
		{"restricted viewer ai status", "rviewer", "GET", "/api/v1/ai/status", 204},
		// Namespace-restricted identities: only their namespaces, never unscoped.
		{"restricted viewer own ns", "rviewer", "POST", "/api/v1/ai/explain?namespace=team-a", 204},
		{"restricted viewer other ns", "rviewer", "POST", "/api/v1/ai/explain?namespace=team-b", 403},
		{"restricted viewer no ns", "rviewer", "POST", "/api/v1/ai/explain", 403},
		{"restricted viewer all ns", "rviewer", "POST", "/api/v1/ai/explain?namespace=all", 403},
		{"restricted viewer mixed ns", "rviewer", "POST", "/api/v1/ai/explain?namespace=team-a&namespace=team-b", 403},
		{"restricted operator own ns", "roper", "POST", "/api/v1/ai/explain?namespace=team-a", 204},
		{"restricted operator other ns", "roper", "POST", "/api/v1/ai/explain?namespace=kube-system", 403},
		// Configuration and connection test: admin only.
		{"operator ai test", "oper", "POST", "/api/v1/integrations/ai/test", 403},
		{"viewer ai test", "viewer", "POST", "/api/v1/integrations/ai/test", 403},
		{"admin ai test", "demo", "POST", "/api/v1/integrations/ai/test", 204},
		{"restricted operator ai test", "roper", "POST", "/api/v1/integrations/ai/test", 403},
		{"operator put integrations", "oper", "PUT", "/api/v1/integrations", 403},
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
