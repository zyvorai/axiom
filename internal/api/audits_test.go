package api

import (
	"context"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

// auditProv feeds the audits findings the demo data does not contain.
type auditProv struct{ *platform.DemoProvider }

func (auditProv) ListResourceQuotas(context.Context, string) ([]platform.ResourceQuotaSummary, error) {
	return []platform.ResourceQuotaSummary{
		{Namespace: "tenant-a", Name: "q", Hard: map[string]string{"pods": "10", "requests.cpu": "4"}, Used: map[string]string{"pods": "10", "requests.cpu": "3500m"}},
		{Namespace: "tenant-b", Name: "q", Hard: map[string]string{"pods": "10"}, Used: map[string]string{"pods": "2"}},
	}, nil
}

func (auditProv) ListWorkloads(context.Context, string) ([]platform.Workload, error) {
	return []platform.Workload{
		{Namespace: "shop", Name: "web", Kind: "Deployment", Image: "nginx:latest"},
		{Namespace: "shop", Name: "pinned", Kind: "Deployment", Image: "nginx@sha256:abc"},
	}, nil
}

func (auditProv) ListServices(context.Context, string) ([]platform.Service, error) {
	return []platform.Service{{Namespace: "shop", Name: "web", Type: "ClusterIP", Selector: map[string]string{"app": "wbe"}}}, nil
}

func (auditProv) ListPods(context.Context, string) ([]platform.Pod, error) {
	return []platform.Pod{{Namespace: "shop", Name: "web-1", Labels: map[string]string{"app": "web"}}}, nil
}

func newAuditEnv(t *testing.T) *idEnv {
	t.Helper()
	hub := platform.NewUserHub(nil)
	h := NewWithUsers(auditProv{platform.NewDemoProvider()}, "demo", idAdminUser, idAdminPass, "", nil, hub,
		AuthOptions{SessionSecret: "test-secret", CostBudgetStore: platform.NewMemoryCostBudgetStore()})
	e := &idEnv{h: h, users: hub, store: &memRevStore{}}
	e.addUser(t, "viewer1", "viewer-password", "viewer")
	e.addUser(t, "oper1", "oper-password", "operator")
	e.addUser(t, "scoped", "scoped-password", "viewer", "payments")
	return e
}

func TestAuditEndpoints(t *testing.T) {
	e := newAuditEnv(t)
	cases := []struct {
		path string
		kind string
		want int
	}{
		{"/api/v1/audits/quota", "exhausted", 1},
		{"/api/v1/audits/quota?threshold=95", "near-limit", 0},
		{"/api/v1/audits/quota?threshold=50", "near-limit", 1},
		{"/api/v1/audits/quota?namespace=tenant-b", "exhausted", 0},
		{"/api/v1/audits/images", "latest-tag", 1},
		{"/api/v1/audits/images?namespace=other", "latest-tag", 0},
		{"/api/v1/audits/services", "selector-matches-nothing", 1},
		{"/api/v1/audits/autoscaling", "cannot-scale", 1}, // from the demo HPAs
	}
	for _, c := range cases {
		w := e.do("GET", c.path, "", asCost("viewer"))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d %s", c.path, w.Code, w.Body.String())
		}
		var rep platform.AuditReport
		decodeInto(t, w, &rep)
		if rep.Summary.ByKind[c.kind] != c.want {
			t.Errorf("%s: %s=%d want %d (%+v)", c.path, c.kind, rep.Summary.ByKind[c.kind], c.want, rep.Summary)
		}
		if rep.Findings == nil || rep.Note == "" {
			t.Errorf("%s: findings/note missing", c.path)
		}
	}
}

func TestAuditEndpointsReject(t *testing.T) {
	e := newAuditEnv(t)
	for _, p := range []string{"quota", "images", "autoscaling", "services"} {
		url := "/api/v1/audits/" + p
		if w := e.do("GET", url+"?namespace=Bad_NS", "", asCost("viewer")); w.Code != 400 {
			t.Errorf("%s bad namespace: %d", p, w.Code)
		}
		if w := e.do("GET", url, "", asCost("scoped")); w.Code != 403 {
			t.Errorf("%s restricted account: %d", p, w.Code)
		}
		if w := e.do("POST", url, "", asCost("operator")); w.Code == 200 {
			t.Errorf("%s accepted POST", p)
		}
	}
	for _, bad := range []string{"0", "101", "abc", "-5", "1e1"} {
		if w := e.do("GET", "/api/v1/audits/quota?threshold="+bad, "", asCost("viewer")); w.Code != 400 {
			t.Errorf("threshold=%q: %d", bad, w.Code)
		}
	}
}

func TestAuditMCPTools(t *testing.T) {
	e := newAuditEnv(t)
	w := e.do("GET", "/api/v1/mcp/tools", "", asCost("viewer"))
	for _, name := range []string{"quota_headroom", "image_hygiene", "autoscaler_saturation", "service_wiring"} {
		if !strings.Contains(w.Body.String(), name) {
			t.Errorf("MCP tool %s missing", name)
		}
	}
	call := func(name, args string) string {
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
		return e.do("POST", "/api/v1/mcp", body, asCost("operator")).Body.String()
	}
	if out := call("quota_headroom", `{"threshold":"50"}`); !strings.Contains(out, "near-limit") {
		t.Errorf("quota_headroom: %s", out)
	}
	if out := call("quota_headroom", `{"threshold":"0"}`); !strings.Contains(out, "threshold must be") {
		t.Errorf("quota_headroom bad threshold: %s", out)
	}
	if out := call("image_hygiene", `{}`); !strings.Contains(out, "latest-tag") {
		t.Errorf("image_hygiene: %s", out)
	}
	if out := call("service_wiring", `{}`); !strings.Contains(out, "selector-matches-nothing") {
		t.Errorf("service_wiring: %s", out)
	}
	if out := call("autoscaler_saturation", `{"namespace":"Bad_NS"}`); !strings.Contains(out, `"isError":true`) {
		t.Errorf("autoscaler_saturation bad ns: %s", out)
	}
}
