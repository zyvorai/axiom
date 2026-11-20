package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

func newFleetEnv(t *testing.T) *idEnv {
	t.Helper()
	hub := platform.NewUserHub(nil)
	h := NewWithUsers(platform.NewDemoSwitchableProvider(), "demo", idAdminUser, idAdminPass, "", nil, hub,
		AuthOptions{SessionSecret: "test-secret"})
	e := &idEnv{h: h, users: hub, store: &memRevStore{}}
	e.addUser(t, "viewer1", "viewer-password", "viewer")
	e.addUser(t, "oper1", "oper-password", "operator")
	e.addUser(t, "scoped", "scoped-password", "viewer", "payments")
	return e
}

func TestFleetEndpoint(t *testing.T) {
	t.Setenv("AXIOM_DEMO_FLEET_DOWN", "ap-south-1")
	t.Setenv("AXIOM_DEMO_FLEET_DEGRADED", "us-east-1")
	e := newFleetEnv(t)

	var f platform.Fleet
	w := e.do("GET", "/api/v1/fleet", "", withBasic(idAdminUser, idAdminPass))
	if w.Code != 200 {
		t.Fatalf("fleet: %d %s", w.Code, w.Body.String())
	}
	decodeInto(t, w, &f)
	if f.Summary.Clusters != 3 || f.Summary.Healthy != 1 || f.Summary.Degraded != 1 || f.Summary.Unreachable != 1 {
		t.Fatalf("summary: %+v", f.Summary)
	}
	byName := map[string]platform.FleetCluster{}
	for _, c := range f.Clusters {
		byName[c.Name] = c
	}
	down := byName["ap-south-1"]
	if down.State != "unreachable" || !strings.Contains(down.Error, "connection refused") || down.CheckedAt == "" {
		t.Fatalf("unreachable cluster: %+v", down)
	}
	if deg := byName["us-east-1"]; deg.State != "degraded" || deg.Reason == "" {
		t.Fatalf("degraded cluster: %+v", deg)
	}
	// Totals come only from the clusters that answered.
	if f.Summary.Nodes == 0 || f.Summary.Pods == 0 {
		t.Fatalf("totals are empty: %+v", f.Summary)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("fleet must not be cached: %q", w.Header().Get("Cache-Control"))
	}
	// The raw JSON has exactly one state field per cluster (no ambiguous embedding).
	if strings.Count(w.Body.String(), `"state":"unreachable"`) != 1 {
		t.Errorf("state is missing or duplicated in the JSON: %s", w.Body.String())
	}

	// The cluster switcher list carries the same state, and Healthy is false only for the dead one.
	var cl []platform.ClusterInfo
	decodeInto(t, e.do("GET", "/api/v1/clusters", "", withBasic(idAdminUser, idAdminPass)), &cl)
	for _, c := range cl {
		wantHealthy := c.Name != "ap-south-1"
		if c.Healthy != wantHealthy || c.State == "" {
			t.Errorf("cluster %s: %+v", c.Name, c)
		}
	}
}

func TestFleetAccess(t *testing.T) {
	e := newFleetEnv(t)
	cases := []struct {
		who, method, path string
		want              int
	}{
		{"admin", "GET", "/api/v1/fleet", 200},
		{"operator", "GET", "/api/v1/fleet", 200},
		{"viewer", "GET", "/api/v1/fleet", 200},
		{"scoped", "GET", "/api/v1/fleet", 403}, // cluster-wide data is off limits to namespace-restricted accounts
		{"admin", "POST", "/api/v1/fleet/refresh", 200},
		{"operator", "POST", "/api/v1/fleet/refresh", 200},
		{"viewer", "POST", "/api/v1/fleet/refresh", 200}, // it only reads from the clusters
		{"scoped", "POST", "/api/v1/fleet/refresh", 403},
	}
	creds := map[string][2]string{
		"admin": {idAdminUser, idAdminPass}, "operator": {"oper1", "oper-password"},
		"viewer": {"viewer1", "viewer-password"}, "scoped": {"scoped", "scoped-password"},
	}
	for _, c := range cases {
		w := e.do(c.method, c.path, "", withBasic(creds[c.who][0], creds[c.who][1]))
		if w.Code != c.want {
			t.Errorf("%s %s as %s: got %d, want %d", c.method, c.path, c.who, w.Code, c.want)
		}
	}
	if w := e.do("GET", "/api/v1/fleet", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
	// Other fleet-path writes stay blocked for viewers.
	if w := e.do("POST", "/api/v1/fleet/other", "", withBasic("viewer1", "viewer-password")); w.Code != 403 {
		t.Errorf("viewer write under /fleet: %d", w.Code)
	}
}

func TestFleetWithoutMultiClusterRegistry(t *testing.T) {
	e := newIDEnv(t, auth.Guard{}, "") // plain DemoProvider: not switchable
	var f platform.Fleet
	w := e.do("GET", "/api/v1/fleet", "", withBasic(idAdminUser, idAdminPass))
	if w.Code != 200 {
		t.Fatalf("fleet: %d", w.Code)
	}
	decodeInto(t, w, &f)
	if len(f.Clusters) != 0 || f.Summary.Clusters != 0 {
		t.Fatalf("expected an empty fleet: %+v", f)
	}
}
