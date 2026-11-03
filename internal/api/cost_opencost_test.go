package api

import (
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func TestCostEndpointSourceAndWindow(t *testing.T) {
	t.Setenv("AXIOM_DEMO_OPENCOST", "")
	e := newCostEnv(t, platform.NewMemoryCostBudgetStore())

	var est platform.CostEstimate
	w := e.do("GET", "/api/v1/cost", "", asCost("viewer"))
	if w.Code != 200 {
		t.Fatalf("cost: %d %s", w.Code, w.Body.String())
	}
	decodeInto(t, w, &est)
	if est.Source != "estimate" || est.Window != "30d" || est.TotalMonthlyUSD != 520 || strings.Contains(w.Body.String(), `"cpuUsd"`) {
		t.Fatalf("estimate %+v", est)
	}
	// The window is validated even when it has no effect.
	for _, bad := range []string{"1d", "90d", "abc", "30"} {
		if w := e.do("GET", "/api/v1/cost?window="+bad, "", asCost("viewer")); w.Code != 400 {
			t.Errorf("window=%s: %d", bad, w.Code)
		}
	}
	decodeInto(t, e.do("GET", "/api/v1/cost?window=7d", "", asCost("viewer")), &est)
	if est.Source != "estimate" || est.Window != "7d" || est.TotalMonthlyUSD != 520 {
		t.Fatalf("estimate 7d %+v", est)
	}
	// GET /cost stays readable by namespace-restricted accounts, as before.
	if w := e.do("GET", "/api/v1/cost", "", asCost("scoped")); w.Code != 200 {
		t.Fatalf("scoped: %d", w.Code)
	}

	t.Setenv("AXIOM_DEMO_OPENCOST", "true")
	w = e.do("GET", "/api/v1/cost?window=14d", "", asCost("viewer"))
	decodeInto(t, w, &est)
	if w.Code != 200 || est.Source != "opencost" || est.Window != "14d" || est.IdleMonthlyUSD <= 0 || est.TotalMonthlyUSD == 520 || !strings.Contains(est.Note, "OpenCost") {
		t.Fatalf("opencost %d %+v", w.Code, est)
	}
	for _, f := range []string{`"cpuUsd"`, `"memoryUsd"`, `"storageUsd"`, `"networkUsd"`, `"otherUsd"`, `"idleMonthlyUsd"`} {
		if !strings.Contains(w.Body.String(), f) {
			t.Errorf("missing %s in %s", f, w.Body.String())
		}
	}
	if w := e.do("GET", "/api/v1/cost?window=60d", "", asCost("viewer")); w.Code != 400 {
		t.Errorf("bad window with OpenCost: %d", w.Code)
	}
}

func TestCostBudgetsReportSource(t *testing.T) {
	store := platform.NewMemoryCostBudgetStore()
	e := newCostEnv(t, store)
	t.Setenv("AXIOM_DEMO_OPENCOST", "")
	var rep platform.CostBudgetReport
	decodeInto(t, e.do("GET", "/api/v1/cost/budgets", "", asCost("viewer")), &rep)
	if rep.Source != "estimate" {
		t.Fatalf("source %+v", rep)
	}
	t.Setenv("AXIOM_DEMO_OPENCOST", "true")
	w := e.do("PUT", "/api/v1/cost/budgets/payments", jsonBody(map[string]any{"monthlyUsd": 150}), asCost("admin"))
	var row platform.CostBudgetRow
	decodeInto(t, w, &row)
	// Demo actual for payments is 147 (estimate 180): under budget, but warning.
	if w.Code != 200 || row.CurrentUSD != 147 || row.State != "warning" {
		t.Fatalf("row %d %+v", w.Code, row)
	}
	decodeInto(t, e.do("GET", "/api/v1/cost/budgets", "", asCost("viewer")), &rep)
	if rep.Source != "opencost" || rep.Budgets[0].CurrentUSD != 147 || rep.TotalCurrentUSD != 147 {
		t.Fatalf("report %+v", rep)
	}
}
