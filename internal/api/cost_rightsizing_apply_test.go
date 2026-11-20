package api

import (
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

const rsApplyPath = "/api/v1/cost/rightsizing/apply"

func rsApplyBody(extra map[string]any) string {
	b := map[string]any{"namespace": "payments", "kind": "Deployment", "workload": "payments-api", "container": "api",
		"resources": []string{"cpu", "memory"}, "window": "7d"}
	for k, v := range extra {
		b[k] = v
	}
	return jsonBody(b)
}

func rsReportItem(t *testing.T, e *idEnv, ns, workload, container string) platform.RightsizingItem {
	t.Helper()
	var rep platform.RightsizingReport
	w := e.do("GET", "/api/v1/cost/rightsizing?namespace="+ns, "", asCost("viewer"))
	if w.Code != 200 {
		t.Fatalf("report: %d %s", w.Code, w.Body.String())
	}
	decodeInto(t, w, &rep)
	for _, it := range rep.Items {
		if it.Workload == workload && it.Container == container {
			return it
		}
	}
	t.Fatalf("no %s/%s in %+v", workload, container, rep.Items)
	return platform.RightsizingItem{}
}

func TestRightsizingApplyFlow(t *testing.T) {
	e := newCostEnv(t, nil)
	it := rsReportItem(t, e, "payments", "payments-api", "api")
	if it.Verdict != "over" {
		t.Fatalf("seed %+v", it)
	}
	observed := map[string]any{"cpuRequestMilli": it.CPU.RequestMilli, "memoryRequestMiB": it.Memory.RequestMiB}

	// Dry run: changes, nothing written, nothing audited.
	w := e.do("POST", rsApplyPath, rsApplyBody(map[string]any{"dryRun": true, "observed": observed}), asCost("operator"))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("dry run: %d %s", w.Code, w.Body.String())
	}
	var dry platform.RightsizingApplyResult
	decodeInto(t, w, &dry)
	if !dry.OK || !dry.DryRun || dry.Verdict != "over" || dry.Source != "metrics-snapshot" || dry.Confidence == "" || dry.Rollout == "" || len(dry.Changes) != 2 ||
		dry.Changes[0].Resource != "cpu" || dry.Changes[0].Field != "requests" || dry.Changes[0].To != "420m" {
		t.Fatalf("dry run body %+v", dry)
	}
	if strings.Contains(w.Body.String(), `"note"`) {
		t.Fatalf("no limit was raised, so no note: %s", w.Body.String())
	}
	if auditHas("cost.rightsizing_apply", "Deployment/payments/payments-api/api", "oper1") {
		t.Fatal("a dry run must not be audited")
	}
	if again := rsReportItem(t, e, "payments", "payments-api", "api"); again.Verdict != "over" {
		t.Fatalf("dry run wrote: %+v", again)
	}

	// Real apply.
	w = e.do("POST", rsApplyPath, rsApplyBody(map[string]any{"observed": observed}), asCost("operator"))
	if w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body.String())
	}
	var res platform.RightsizingApplyResult
	decodeInto(t, w, &res)
	if res.DryRun || len(res.Changes) != 2 || !strings.Contains(w.Body.String(), `"dryRun":false`) {
		t.Fatalf("apply body %s", w.Body.String())
	}
	if !auditHas("cost.rightsizing_apply", "Deployment/payments/payments-api/api", "oper1") {
		t.Fatal("apply not audited")
	}
	var detail string
	for _, a := range platform.ListAxiomAudit() {
		if a.Action == "cost.rightsizing_apply" && a.User == "oper1" {
			detail = a.Detail
		}
	}
	if !strings.Contains(detail, "cpu requests 2->420m") || !strings.Contains(detail, "memory requests 4Gi->1040Mi") {
		t.Fatalf("audit detail %q", detail)
	}
	if after := rsReportItem(t, e, "payments", "payments-api", "api"); after.Verdict != "ok" || after.CPU.RequestMilli != 420 {
		t.Fatalf("after apply: %+v", after)
	}

	// Same (now stale) view again, then a fresh view of a right-sized container.
	w = e.do("POST", rsApplyPath, rsApplyBody(map[string]any{"observed": observed}), asCost("admin"))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "the live requests changed since you looked") {
		t.Fatalf("stale: %d %s", w.Code, w.Body.String())
	}
	w = e.do("POST", rsApplyPath, rsApplyBody(map[string]any{"dryRun": true}), asCost("admin"))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "already right-sized") {
		t.Fatalf("already right-sized: %d %s", w.Code, w.Body.String())
	}

	// Admin applies an under-provisioned StatefulSet with a limit that must rise.
	w = e.do("POST", rsApplyPath, jsonBody(map[string]any{"namespace": "data", "kind": "StatefulSet", "workload": "redis", "container": "redis", "resources": []string{"memory"}}), asCost("admin"))
	if w.Code != 200 {
		t.Fatalf("redis: %d %s", w.Code, w.Body.String())
	}
	var redis platform.RightsizingApplyResult
	decodeInto(t, w, &redis)
	if len(redis.Changes) != 2 || redis.Changes[1].Field != "limits" || redis.Changes[1].Note != "limit raised to stay >= the request" {
		t.Fatalf("redis changes %+v", redis.Changes)
	}
	if !auditHas("cost.rightsizing_apply", "StatefulSet/data/redis/redis", idAdminUser) {
		t.Fatal("admin apply not audited")
	}
}

func TestRightsizingApplyAccess(t *testing.T) {
	e := newCostEnv(t, nil)
	dry := rsApplyBody(map[string]any{"dryRun": true})
	for _, tc := range []struct {
		who  string
		want int
	}{
		{"viewer", 403}, {"scoped", 403}, {"scopedop", 403}, {"operator", 200}, {"admin", 200},
	} {
		if w := e.do("POST", rsApplyPath, dry, asCost(tc.who)); w.Code != tc.want {
			t.Errorf("%s dry run: %d, want %d (%s)", tc.who, w.Code, tc.want, w.Body.String())
		}
	}
	if w := e.do("POST", rsApplyPath, dry); w.Code != 401 {
		t.Errorf("anonymous: %d", w.Code)
	}
}

func TestRightsizingApplyValidation(t *testing.T) {
	e := newCostEnv(t, nil)
	for name, body := range map[string]string{
		"not json":          "{",
		"unknown field":     rsApplyBody(map[string]any{"cpuMilli": 100}),
		"target numbers":    rsApplyBody(map[string]any{"recommended": map[string]any{"cpuMilli": 1}}),
		"bad kind":          rsApplyBody(map[string]any{"kind": "Job"}),
		"bad namespace":     rsApplyBody(map[string]any{"namespace": "Payments"}),
		"bad workload":      rsApplyBody(map[string]any{"workload": "a/b"}),
		"bad container":     rsApplyBody(map[string]any{"container": "API"}),
		"empty resources":   rsApplyBody(map[string]any{"resources": []string{}}),
		"unknown resource":  rsApplyBody(map[string]any{"resources": []string{"gpu"}}),
		"bad window":        rsApplyBody(map[string]any{"window": "2d"}),
		"negative observed": rsApplyBody(map[string]any{"observed": map[string]any{"cpuRequestMilli": -1}}),
	} {
		if w := e.do("POST", rsApplyPath, body, asCost("operator")); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	for name, body := range map[string]string{
		"unknown workload":  rsApplyBody(map[string]any{"workload": "nope"}),
		"unknown container": rsApplyBody(map[string]any{"container": "nope"}),
	} {
		if w := e.do("POST", rsApplyPath, body, asCost("operator")); w.Code != 404 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	// The GET still answers and the apply route takes POST only.
	if w := e.do("GET", rsApplyPath, "", asCost("operator")); w.Code != 405 && w.Code != 404 {
		t.Errorf("GET apply: %d", w.Code)
	}
}
