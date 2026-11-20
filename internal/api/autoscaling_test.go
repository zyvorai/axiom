package api

import (
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

func newAutoscalingEnv(t *testing.T) *idEnv {
	t.Helper()
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "viewer1", "viewer-password", "viewer")
	e.addUser(t, "oper1", "oper-password", "operator")
	e.addUser(t, "scoped", "scoped-password", "viewer", "commerce")
	return e
}

func TestAutoscalingViewsEndpoints(t *testing.T) {
	e := newAutoscalingEnv(t)
	admin := withBasic(idAdminUser, idAdminPass)

	var v platform.VPAList
	w := e.do("GET", "/api/v1/vpas?namespace=all", "", admin)
	if w.Code != 200 {
		t.Fatalf("vpas: %d %s", w.Code, w.Body.String())
	}
	decodeInto(t, w, &v)
	if !v.Installed || len(v.Items) != 2 || len(v.Items[0].Containers) == 0 {
		t.Fatalf("vpas: %+v", v)
	}
	for _, k := range []string{`"lowerBound":{"cpu"`, `"uncappedTarget"`, `"updateMode":"Auto"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Errorf("VPA JSON missing %s", k)
		}
	}

	var s platform.ScaledObjectList
	w = e.do("GET", "/api/v1/scaledobjects?namespace=payments", "", admin)
	if w.Code != 200 {
		t.Fatalf("scaledobjects: %d %s", w.Code, w.Body.String())
	}
	decodeInto(t, w, &s)
	if !s.Installed || len(s.Items) != 1 || !s.Items[0].Active || len(s.Items[0].Triggers) != 2 {
		t.Fatalf("scaledobjects: %+v", s)
	}

	// A namespace-restricted viewer sees its own namespace only.
	scoped := withBasic("scoped", "scoped-password")
	w = e.do("GET", "/api/v1/vpas?namespace=commerce", "", scoped)
	if w.Code != 200 {
		t.Fatalf("scoped own namespace: %d", w.Code)
	}
	decodeInto(t, w, &v)
	if len(v.Items) != 1 || v.Items[0].Namespace != "commerce" {
		t.Fatalf("scoped vpas: %+v", v)
	}
}

func TestAutoscalingViewsAccess(t *testing.T) {
	e := newAutoscalingEnv(t)
	creds := map[string][2]string{
		"admin": {idAdminUser, idAdminPass}, "operator": {"oper1", "oper-password"},
		"viewer": {"viewer1", "viewer-password"}, "scoped": {"scoped", "scoped-password"},
	}
	cases := []struct {
		who, path string
		want      int
	}{
		{"admin", "/api/v1/vpas", 200},
		{"operator", "/api/v1/vpas?namespace=data", 200},
		{"viewer", "/api/v1/vpas?namespace=all", 200},
		{"scoped", "/api/v1/vpas?namespace=commerce", 200},
		{"scoped", "/api/v1/vpas?namespace=data", 403},
		{"scoped", "/api/v1/vpas?namespace=all", 403},
		{"scoped", "/api/v1/vpas", 403},
		{"admin", "/api/v1/scaledobjects", 200},
		{"operator", "/api/v1/scaledobjects", 200},
		{"viewer", "/api/v1/scaledobjects?namespace=payments", 200},
		{"scoped", "/api/v1/scaledobjects?namespace=commerce", 200},
		{"scoped", "/api/v1/scaledobjects?namespace=payments", 403},
		{"scoped", "/api/v1/scaledobjects?namespace=all", 403},
		{"scoped", "/api/v1/scaledobjects", 403},
	}
	for _, c := range cases {
		w := e.do("GET", c.path, "", withBasic(creds[c.who][0], creds[c.who][1]))
		if w.Code != c.want {
			t.Errorf("GET %s as %s: got %d, want %d (%s)", c.path, c.who, w.Code, c.want, w.Body.String())
		}
	}
	if w := e.do("GET", "/api/v1/vpas", ""); w.Code != 401 {
		t.Errorf("anonymous: %d", w.Code)
	}
}

func TestHPAMultiMetricOverHTTP(t *testing.T) {
	e := newAutoscalingEnv(t)
	oper := withBasic("oper1", "oper-password")
	body := `{"namespace":"commerce","name":"http-mem-hpa","targetKind":"Deployment","targetName":"checkout","minReplicas":2,"maxReplicas":6,"targetCpuPercent":60,"targetMemoryPercent":75,"scaleDownStabilizationSeconds":300}`
	w := e.do("POST", "/api/v1/hpas", body, oper)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	t.Cleanup(func() { e.do("DELETE", "/api/v1/hpas/commerce/http-mem-hpa", "", withBasic(idAdminUser, idAdminPass)) })
	var h platform.HPASummary
	decodeInto(t, w, &h)
	if len(h.Metrics) != 2 || h.Metrics[1].Name != "memory" || !h.Behavior || h.TargetCPUPercent != 60 {
		t.Fatalf("created: %+v", h)
	}

	for name, bad := range map[string]string{
		"memory > 100":  `{"namespace":"commerce","name":"b1","targetKind":"Deployment","targetName":"checkout","targetMemoryPercent":101}`,
		"window > 3600": `{"namespace":"commerce","name":"b2","targetKind":"Deployment","targetName":"checkout","scaleDownStabilizationSeconds":7200}`,
		"daemonset":     `{"namespace":"commerce","name":"b3","targetKind":"DaemonSet","targetName":"checkout"}`,
		"max > 1000":    `{"namespace":"commerce","name":"b4","targetKind":"Deployment","targetName":"checkout","maxReplicas":1001}`,
		"bad name":      `{"namespace":"commerce","name":"B_4","targetKind":"Deployment","targetName":"checkout"}`,
	} {
		if w := e.do("POST", "/api/v1/hpas", bad, oper); w.Code != 400 {
			t.Errorf("%s: got %d %s", name, w.Code, w.Body.String())
		}
	}
	if w := e.do("POST", "/api/v1/hpas", body, withBasic("viewer1", "viewer-password")); w.Code != 403 {
		t.Errorf("viewer create: %d", w.Code)
	}

	var list []platform.HPASummary
	w = e.do("GET", "/api/v1/hpas?namespace=observability", "", withBasic("viewer1", "viewer-password"))
	decodeInto(t, w, &list)
	if len(list) != 1 || len(list[0].Metrics) != 2 || list[0].Metrics[0].Type != "External" || len(list[0].Conditions) == 0 {
		t.Fatalf("observability hpas: %+v", list)
	}
}
