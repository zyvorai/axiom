package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func TestManifestTouchesRBAC(t *testing.T) {
	yes := []string{
		"apiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRoleBinding\nmetadata:\n  name: x\n",
		"kind: Role\nmetadata: {name: r}\n",
		"---\nkind: ConfigMap\n---\nkind: ServiceAccount\nmetadata: {name: sa}\n",
		`{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"ClusterRole","metadata":{"name":"x"}}`,
		"  kind: 'RoleBinding'\n",
	}
	no := []string{
		"kind: Deployment\nmetadata:\n  name: roles\n",
		"kind: ConfigMap\ndata:\n  kind: Role-ish\n",
		"",
	}
	for _, m := range yes {
		if !manifestTouchesRBAC(m) {
			t.Errorf("expected RBAC detection for %q", m)
		}
	}
	for _, m := range no {
		if manifestTouchesRBAC(m) {
			t.Errorf("unexpected RBAC detection for %q", m)
		}
	}
}

func TestStripWindowCredentialsRemovesAPIKey(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?api_key=k&access_token=t&axiom_session=s&keep=1", nil)
	r.Header.Set("Authorization", "Bearer k")
	r.Header.Set("Cookie", "axiom_session=s")
	stripWindowCredentials(r)
	q, _ := url.ParseQuery(r.URL.RawQuery)
	if q.Has("api_key") || q.Has("access_token") || q.Has("axiom_session") || q.Get("keep") != "1" {
		t.Fatalf("unexpected query after strip: %q", r.URL.RawQuery)
	}
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
		t.Fatal("credential headers not stripped")
	}
}

func TestProxyPodWindowSandboxesResponse(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "axiom_session", Value: "evil"})
		w.Write([]byte("<script>alert(1)</script>"))
	}))
	defer up.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/pods/ns/p/window/8080/", nil)
	proxyPodWindow(rec, req, up.URL, "ns", "p", 8080)
	res := rec.Result()
	if res.Header.Get("Set-Cookie") != "" {
		t.Fatal("upstream Set-Cookie must be dropped")
	}
	if csp := res.Header.Get("Content-Security-Policy"); csp == "" || csp[:7] != "sandbox" {
		t.Fatalf("expected sandbox CSP, got %q", csp)
	}
}

func TestMetricsSeriesAreBounded(t *testing.T) {
	m := &metricsRegistry{requests: map[metricKey]*metricValue{}}
	for i := 0; i < maxMetricSeries*3; i++ {
		m.record("GET", "/api/v1/random"+strconv.Itoa(i), 404, 0.001)
		m.record("BREW", "/api/v1/pods", 200, 0.001)
	}
	if n := len(m.requests); n > maxMetricSeries+1 {
		t.Fatalf("metric series must stay bounded, have %d", n)
	}
	for k := range m.requests {
		if k.method == "BREW" {
			t.Fatal("arbitrary HTTP methods must be folded into OTHER")
		}
	}
}

func TestRouteLabelRejectsOddSegments(t *testing.T) {
	if got := routeLabel("/api/v1/pods/default/x"); got != "/api/v1/pods" {
		t.Fatalf("got %q", got)
	}
	if got := routeLabel("/api/v1/%0a%22injected"); got != "/other" && !strings.HasPrefix(got, "/api/v1/") {
		t.Fatalf("got %q", got)
	}
}

func TestAuthProvidersHidesDemoDetailsOutsideDemo(t *testing.T) {
	get := func(mode string) map[string]any {
		h := New(platform.NewDemoProvider(), mode, "admin", "pw", "", AuthOptions{})
		r := httptest.NewRequest("GET", "/api/v1/auth/providers", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	demo, prod := get("demo"), get("kubernetes")
	if demo["lab"].(map[string]any)["hint"] != "demo / demo" {
		t.Fatalf("demo mode should still advertise the demo login: %v", demo["lab"])
	}
	if h := prod["lab"].(map[string]any)["hint"]; h != "" {
		t.Fatalf("non-demo mode must not advertise credentials, got %v", h)
	}
	if u := prod["local"].(map[string]any)["default_username"]; u != "" {
		t.Fatalf("non-demo mode must not reveal the default username, got %v", u)
	}
}
