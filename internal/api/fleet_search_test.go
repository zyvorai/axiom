package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

func TestFleetSearchEndpoint(t *testing.T) {
	t.Setenv("AXIOM_DEMO_FLEET_DOWN", "ap-south-1")
	e := newFleetEnv(t)
	var r platform.FleetSearchResult
	w := e.do("GET", "/api/v1/fleet/search?q=payments-api&kind=Workload", "", withBasic("viewer1", "viewer-password"))
	if w.Code != 200 {
		t.Fatalf("search: %d %s", w.Code, w.Body.String())
	}
	decodeInto(t, w, &r)
	if r.Query != "payments-api" || r.Total != 2 || r.Truncated || len(r.Results) != 2 {
		t.Fatalf("result: %+v", r)
	}
	if r.Results[0].Cluster != "eu-west-1" || r.Results[1].Cluster != "us-east-1" || r.Results[0].Kind != "Workload" || r.Results[0].WorkloadKind != "Deployment" {
		t.Fatalf("hits: %+v", r.Results)
	}
	if r.Clusters.Searched != 2 || len(r.Clusters.Skipped) != 1 || r.Clusters.Skipped[0].Cluster != "ap-south-1" || r.Clusters.Skipped[0].Reason != "unreachable" {
		t.Fatalf("clusters: %+v", r.Clusters)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("cache-control: %q", w.Header().Get("Cache-Control"))
	}
	// Arrays are never null, so the UI can map over them.
	if !strings.Contains(w.Body.String(), `"errors":[]`) {
		t.Errorf("errors should be an empty array: %s", w.Body.String())
	}
}

func TestFleetSearchValidation(t *testing.T) {
	e := newFleetEnv(t)
	cases := []struct {
		query string
		want  int
	}{
		{"", 400},
		{"q=", 400},
		{"q=a", 400},
		{"q=%20a%20", 400}, // trimmed before the length check
		{"q=" + strings.Repeat("x", 65), 400},
		{"q=" + strings.Repeat("x", 64), 200},
		{"q=pa", 200},
		{"q=pay&kind=Pod", 200},
		{"q=pay&kind=service", 200},
		{"q=pay&kind=Workload,Namespace", 200},
		{"q=pay&limit=0", 400},
		{"q=pay&limit=201", 400},
		{"q=pay&limit=abc", 400},
		{"q=pay&limit=200", 200},
		{"q=pay&limit=1", 200},
	}
	for _, c := range cases {
		if w := e.do("GET", "/api/v1/fleet/search?"+c.query, "", withBasic(idAdminUser, idAdminPass)); w.Code != c.want {
			t.Errorf("%q: got %d, want %d (%s)", c.query, w.Code, c.want, w.Body.String())
		}
	}
	var r platform.FleetSearchResult
	decodeInto(t, e.do("GET", "/api/v1/fleet/search?q=pay&limit=1", "", withBasic(idAdminUser, idAdminPass)), &r)
	if len(r.Results) != 1 || !r.Truncated || r.Total <= 1 {
		t.Fatalf("limit=1: %+v", r)
	}
}

func TestFleetSearchAccess(t *testing.T) {
	e := newFleetEnv(t)
	creds := map[string][2]string{
		"admin": {idAdminUser, idAdminPass}, "operator": {"oper1", "oper-password"},
		"viewer": {"viewer1", "viewer-password"}, "scoped": {"scoped", "scoped-password"},
	}
	for who, want := range map[string]int{"admin": 200, "operator": 200, "viewer": 200, "scoped": 403} {
		if w := e.do("GET", "/api/v1/fleet/search?q=payments", "", withBasic(creds[who][0], creds[who][1])); w.Code != want {
			t.Errorf("%s: got %d, want %d", who, w.Code, want)
		}
	}
	if w := e.do("GET", "/api/v1/fleet/search?q=payments", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/fleet/search?q=payments", "", withBasic("viewer1", "viewer-password")); w.Code != 403 {
		t.Errorf("viewer POST: %d", w.Code)
	}
}

func TestFleetSearchRateLimit(t *testing.T) {
	e := newFleetEnv(t)
	for i := 0; i < fleetSearchPerWindow; i++ {
		if w := e.do("GET", "/api/v1/fleet/search?q=payments&kind=Namespace", "", withBasic("viewer1", "viewer-password")); w.Code != 200 {
			t.Fatalf("request %d: %d", i, w.Code)
		}
	}
	w := e.do("GET", "/api/v1/fleet/search?q=payments", "", withBasic("viewer1", "viewer-password"))
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("over the limit: %d retry-after=%q", w.Code, w.Header().Get("Retry-After"))
	}
	// Per user: another account still has its own budget.
	if w := e.do("GET", "/api/v1/fleet/search?q=payments", "", withBasic("oper1", "oper-password")); w.Code != 200 {
		t.Fatalf("other user: %d", w.Code)
	}
	// Per server: a fresh handler does not inherit this one's buckets.
	if w := newFleetEnv(t).do("GET", "/api/v1/fleet/search?q=payments", "", withBasic("viewer1", "viewer-password")); w.Code != 200 {
		t.Fatalf("other server: %d", w.Code)
	}
}

func TestFleetSearchWithoutMultiClusterRegistry(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "") // plain DemoProvider: not switchable
	w := e.do("GET", "/api/v1/fleet/search?q=payments", "", withBasic(idAdminUser, idAdminPass))
	if w.Code != 200 {
		t.Fatalf("search: %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{`"query":"payments"`, `"results":[]`, `"total":0`, `"truncated":false`, `"searched":0`, `"skipped":[]`, `"errors":[]`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
}
