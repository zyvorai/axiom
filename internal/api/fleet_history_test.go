package api

import (
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func TestFleetHistoryEndpoint(t *testing.T) {
	t.Setenv("AXIOM_DEMO_FLEET_DOWN", "ap-south-1")
	t.Setenv("AXIOM_DEMO_FLEET_HISTORY_SEED", "")
	e := newFleetEnv(t)
	admin := withBasic(idAdminUser, idAdminPass)
	// Two probe rounds (the first read probes lazily, refresh probes again).
	_ = e.do("GET", "/api/v1/fleet", "", admin)
	_ = e.do("POST", "/api/v1/fleet/refresh", "", admin)

	w := e.do("GET", "/api/v1/fleet/history", "", withBasic("viewer1", "viewer-password"))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("history: %d %s", w.Code, w.Body.String())
	}
	var h platform.FleetHistory
	decodeInto(t, w, &h)
	if h.Window != "24h" || len(h.Clusters) != 3 {
		t.Fatalf("history %+v", h)
	}
	for _, c := range h.Clusters {
		if c.Checks < 1 || len(c.Samples) != c.Checks || c.UptimePercent == nil {
			t.Fatalf("%s: %+v", c.Cluster, c)
		}
		if c.Cluster == "ap-south-1" && (c.CurrentState != "unreachable" || *c.UptimePercent != 0 || len(c.Incidents) != 1 || c.Incidents[0].To != nil) {
			t.Fatalf("down cluster %+v", c)
		}
	}
	if !strings.Contains(w.Body.String(), `"incidents":[]`) {
		t.Errorf("incidents must be [] not null: %s", w.Body.String())
	}

	w = e.do("GET", "/api/v1/fleet/history?cluster=eu-west-1&window=1h", "", admin)
	decodeInto(t, w, &h)
	if w.Code != 200 || h.Window != "1h" || len(h.Clusters) != 1 || h.Clusters[0].Cluster != "eu-west-1" || h.Clusters[0].CurrentState != "healthy" {
		t.Fatalf("one cluster %d %+v", w.Code, h)
	}
	for q, want := range map[string]int{
		"window=2h":                           400,
		"window=24":                           400,
		"cluster=":                            400,
		"cluster=a%0Ab":                       400,
		"cluster=" + strings.Repeat("x", 254): 400,
		"cluster=nope":                        404,
		"cluster=us-east-1":                   200,
	} {
		if w := e.do("GET", "/api/v1/fleet/history?"+q, "", admin); w.Code != want {
			t.Errorf("%s: %d, want %d (%s)", q, w.Code, want, w.Body.String())
		}
	}
}

func TestFleetHistoryAccess(t *testing.T) {
	e := newFleetEnv(t)
	creds := map[string][2]string{
		"admin": {idAdminUser, idAdminPass}, "operator": {"oper1", "oper-password"},
		"viewer": {"viewer1", "viewer-password"}, "scoped": {"scoped", "scoped-password"},
	}
	for who, want := range map[string]int{"admin": 200, "operator": 200, "viewer": 200, "scoped": 403} {
		if w := e.do("GET", "/api/v1/fleet/history?window=6h", "", withBasic(creds[who][0], creds[who][1])); w.Code != want {
			t.Errorf("%s: %d, want %d", who, w.Code, want)
		}
	}
}

func TestFleetHistorySeededDemo(t *testing.T) {
	t.Setenv("AXIOM_DEMO_FLEET_DOWN", "")
	t.Setenv("AXIOM_DEMO_FLEET_HISTORY_SEED", "true")
	e := newFleetEnv(t)
	var h platform.FleetHistory
	decodeInto(t, e.do("GET", "/api/v1/fleet/history?cluster=ap-south-1", "", withBasic(idAdminUser, idAdminPass)), &h)
	c := h.Clusters[0]
	if len(c.Incidents) != 1 || c.Incidents[0].State != "degraded" || c.Incidents[0].Reason == "" || len(c.Samples) > 240 || c.Checks < 2800 {
		t.Fatalf("seeded %+v", c.Incidents)
	}
}
