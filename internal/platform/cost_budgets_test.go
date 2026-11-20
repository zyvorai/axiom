package platform

import (
	"context"
	"fmt"
	"testing"
)

func TestCostBudgetState(t *testing.T) {
	cases := []struct {
		monthly, current, warn float64
		percent                float64
		state                  string
	}{
		{100, 0, 80, 0, "ok"},
		{100, 79.9, 80, 79.9, "ok"},
		{100, 80, 80, 80, "warning"},
		{100, 99.99, 80, 100, "over"}, // rounds to 100.0 -> over
		{100, 99.94, 80, 99.9, "warning"},
		{100, 100, 80, 100, "over"},
		{100, 250, 80, 250, "over"},
		{300, 100, 30, 33.3, "warning"},
		{300, 100, 50, 33.3, "ok"},
		{100, 50, 0, 50, "ok"}, // 0 = default 80
		{1000, 1, 100, 0.1, "ok"},
	}
	for _, c := range cases {
		est := CostEstimate{Namespaces: []NamespaceCost{{Namespace: "ns", MonthlyUSD: c.current}}}
		row := CostBudgetRowFor(CostBudget{Namespace: "ns", MonthlyUSD: c.monthly, WarnPercent: c.warn}, est)
		if row.Percent != c.percent || row.State != c.state {
			t.Errorf("%v/%v warn %v: percent %v state %s, want %v %s", c.current, c.monthly, c.warn, row.Percent, row.State, c.percent, c.state)
		}
	}
}

func TestBuildCostBudgetReport(t *testing.T) {
	est := CostEstimate{Namespaces: []NamespaceCost{
		{Namespace: "payments", MonthlyUSD: 180}, {Namespace: "commerce", MonthlyUSD: 90},
		{Namespace: "data", MonthlyUSD: 170}, {Namespace: "observability", MonthlyUSD: 80},
	}}
	budgets := []CostBudget{
		{Namespace: "payments", MonthlyUSD: 150, WarnPercent: 80},
		{Namespace: "commerce", MonthlyUSD: 100, WarnPercent: 90},
		{Namespace: "gone", MonthlyUSD: 50, WarnPercent: 80},
	}
	rep := BuildCostBudgetReport(est, budgets)
	if len(rep.Budgets) != 3 || rep.Budgets[0].Namespace != "payments" || rep.Budgets[0].State != "over" || rep.Budgets[0].Percent != 120 {
		t.Fatalf("budgets %+v", rep.Budgets)
	}
	if rep.Budgets[1].Namespace != "commerce" || rep.Budgets[1].State != "warning" || rep.Budgets[1].CurrentUSD != 90 {
		t.Fatalf("commerce %+v", rep.Budgets[1])
	}
	if g := rep.Budgets[2]; g.Namespace != "gone" || g.CurrentUSD != 0 || g.State != "ok" {
		t.Fatalf("gone %+v", g)
	}
	if len(rep.Unbudgeted) != 2 || rep.Unbudgeted[0].Namespace != "data" || rep.Unbudgeted[1].Namespace != "observability" {
		t.Fatalf("unbudgeted %+v", rep.Unbudgeted)
	}
	if rep.TotalBudgetUSD != 300 || rep.TotalCurrentUSD != 270 {
		t.Fatalf("totals %v %v", rep.TotalBudgetUSD, rep.TotalCurrentUSD)
	}

	var many CostEstimate
	for i := 0; i < 30; i++ {
		many.Namespaces = append(many.Namespaces, NamespaceCost{Namespace: fmt.Sprintf("ns-%02d", i), MonthlyUSD: float64(i)})
	}
	rep = BuildCostBudgetReport(many, nil)
	if len(rep.Unbudgeted) != 20 || rep.Unbudgeted[0].Namespace != "ns-29" || rep.Budgets == nil {
		t.Fatalf("unbudgeted cap: %d %+v", len(rep.Unbudgeted), rep.Unbudgeted[0])
	}
}

func TestValidateCostBudget(t *testing.T) {
	good := CostBudget{Namespace: "team-a", MonthlyUSD: 100, WarnPercent: 80}
	if err := ValidateCostBudget(good); err != nil {
		t.Fatal(err)
	}
	bad := []CostBudget{
		{Namespace: "Team", MonthlyUSD: 1, WarnPercent: 80},
		{Namespace: "-a", MonthlyUSD: 1, WarnPercent: 80},
		{Namespace: "", MonthlyUSD: 1, WarnPercent: 80},
		{Namespace: "a.b", MonthlyUSD: 1, WarnPercent: 80},
		{Namespace: fmt.Sprintf("%064d", 0), MonthlyUSD: 1, WarnPercent: 80},
		{Namespace: "a", MonthlyUSD: 0, WarnPercent: 80},
		{Namespace: "a", MonthlyUSD: -1, WarnPercent: 80},
		{Namespace: "a", MonthlyUSD: 1e9 + 1, WarnPercent: 80},
		{Namespace: "a", MonthlyUSD: 1, WarnPercent: 0},
		{Namespace: "a", MonthlyUSD: 1, WarnPercent: 101},
	}
	for _, b := range bad {
		if err := ValidateCostBudget(b); err == nil || HTTPStatus(err) != 400 {
			t.Errorf("%+v accepted (%v)", b, err)
		}
	}
	if err := ValidateCostBudget(CostBudget{Namespace: "a", MonthlyUSD: 1e9, WarnPercent: 100}); err != nil {
		t.Errorf("limits are inclusive: %v", err)
	}
}

// testCostBudgetStore exercises any CostBudgetStore implementation.
func testCostBudgetStore(t *testing.T, s CostBudgetStore) {
	t.Helper()
	ctx := context.Background()
	if l, err := s.List(ctx); err != nil || len(l) != 0 {
		t.Fatalf("empty: %v %v", l, err)
	}
	for _, b := range []CostBudget{{Namespace: "b", MonthlyUSD: 10, WarnPercent: 80}, {Namespace: "a", MonthlyUSD: 20, WarnPercent: 50}} {
		if err := s.Set(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Set(ctx, CostBudget{Namespace: "b", MonthlyUSD: 15, WarnPercent: 90}); err != nil {
		t.Fatal(err)
	}
	l, _ := s.List(ctx)
	if len(l) != 2 || l[0].Namespace != "a" || l[1].MonthlyUSD != 15 || l[1].WarnPercent != 90 {
		t.Fatalf("after set: %+v", l)
	}
	if err := s.Delete(ctx, "missing"); HTTPStatus(err) != 404 {
		t.Fatalf("delete missing: %v", err)
	}
	if err := s.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	l, _ = s.List(ctx)
	if len(l) != 1 || l[0].Namespace != "b" {
		t.Fatalf("after delete: %+v", l)
	}
}

func TestMemoryCostBudgetStore(t *testing.T) {
	testCostBudgetStore(t, NewMemoryCostBudgetStore())
	if _, ok := OpenCostBudgetStore("demo", nil).(*MemoryCostBudgetStore); !ok {
		t.Fatal("demo mode must use the memory store")
	}
}

func TestCostBudgetListSetCap(t *testing.T) {
	var list []CostBudget
	for i := 0; i < costBudgetMaxCount; i++ {
		var err error
		list, err = costBudgetListSet(list, CostBudget{Namespace: fmt.Sprintf("n%d", i), MonthlyUSD: 1, WarnPercent: 80})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := costBudgetListSet(list, CostBudget{Namespace: "extra", MonthlyUSD: 1, WarnPercent: 80}); HTTPStatus(err) != 409 {
		t.Fatalf("cap: %v", err)
	}
	if _, err := costBudgetListSet(list, CostBudget{Namespace: "n5", MonthlyUSD: 2, WarnPercent: 80}); err != nil {
		t.Fatalf("replace when full: %v", err)
	}
}
