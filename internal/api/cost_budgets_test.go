package api

import (
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func newCostEnv(t *testing.T, store platform.CostBudgetStore) *idEnv {
	t.Helper()
	hub := platform.NewUserHub(nil)
	h := NewWithUsers(platform.NewDemoProvider(), "demo", idAdminUser, idAdminPass, "", nil, hub,
		AuthOptions{SessionSecret: "test-secret", CostBudgetStore: store})
	e := &idEnv{h: h, users: hub, store: &memRevStore{}}
	e.addUser(t, "viewer1", "viewer-password", "viewer")
	e.addUser(t, "oper1", "oper-password", "operator")
	e.addUser(t, "scoped", "scoped-password", "viewer", "payments")
	e.addUser(t, "scopedop", "scopedop-password", "operator", "payments")
	return e
}

// asHelm's credential table covers the same five identities.
func asCost(who string) reqOpt { return asHelm(who) }

func auditHas(action, target, user string) bool {
	for _, a := range platform.ListAxiomAudit() {
		if a.Action == action && a.Target == target && a.User == user {
			return true
		}
	}
	return false
}

func TestCostBudgetsCRUD(t *testing.T) {
	store := platform.NewMemoryCostBudgetStore()
	e := newCostEnv(t, store)

	w := e.do("GET", "/api/v1/cost/budgets", "", asCost("viewer"))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("empty list: %d %s", w.Code, w.Body.String())
	}
	var rep platform.CostBudgetReport
	decodeInto(t, w, &rep)
	// Demo estimate: data 370 (incl. GPU), payments 180, commerce 90, observability 80.
	if len(rep.Budgets) != 0 || len(rep.Unbudgeted) != 4 || rep.Unbudgeted[0].Namespace != "data" || rep.Unbudgeted[0].CurrentUSD != 370 {
		t.Fatalf("report %+v", rep)
	}
	if !strings.Contains(w.Body.String(), `"budgets":[]`) {
		t.Fatalf("budgets must be [] not null: %s", w.Body.String())
	}

	w = e.do("PUT", "/api/v1/cost/budgets/payments", jsonBody(map[string]any{"monthlyUsd": 150}), asCost("admin"))
	if w.Code != 200 {
		t.Fatalf("put: %d %s", w.Code, w.Body.String())
	}
	var row platform.CostBudgetRow
	decodeInto(t, w, &row)
	if row.Namespace != "payments" || row.MonthlyUSD != 150 || row.WarnPercent != 80 || row.CurrentUSD != 180 || row.Percent != 120 || row.State != "over" || row.UpdatedBy != idAdminUser {
		t.Fatalf("row %+v", row)
	}
	if !auditHas("cost.budget_set", "payments", idAdminUser) {
		t.Error("budget set not audited")
	}

	w = e.do("PUT", "/api/v1/cost/budgets/commerce", jsonBody(map[string]any{"monthlyUsd": 100, "warnPercent": 85}), asCost("admin"))
	decodeInto(t, w, &row)
	if w.Code != 200 || row.State != "warning" || row.Percent != 90 || row.WarnPercent != 85 {
		t.Fatalf("commerce %d %+v", w.Code, row)
	}
	w = e.do("PUT", "/api/v1/cost/budgets/retired", jsonBody(map[string]any{"monthlyUsd": 50}), asCost("admin"))
	decodeInto(t, w, &row)
	if w.Code != 200 || row.CurrentUSD != 0 || row.State != "ok" {
		t.Fatalf("missing namespace %d %+v", w.Code, row)
	}

	w = e.do("GET", "/api/v1/cost/budgets", "", asCost("operator"))
	decodeInto(t, w, &rep)
	if len(rep.Budgets) != 3 || rep.Budgets[0].Namespace != "payments" || rep.Budgets[0].State != "over" ||
		len(rep.Unbudgeted) != 2 || rep.TotalBudgetUSD != 300 || rep.TotalCurrentUSD != 270 {
		t.Fatalf("report %+v", rep)
	}

	bad := []struct{ path, body string }{
		{"/api/v1/cost/budgets/payments", `{}`},
		{"/api/v1/cost/budgets/payments", `{"monthlyUsd":0}`},
		{"/api/v1/cost/budgets/payments", `{"monthlyUsd":-5}`},
		{"/api/v1/cost/budgets/payments", `{"monthlyUsd":1000000001}`},
		{"/api/v1/cost/budgets/payments", `{"monthlyUsd":10,"warnPercent":0}`},
		{"/api/v1/cost/budgets/payments", `{"monthlyUsd":10,"warnPercent":101}`},
		{"/api/v1/cost/budgets/payments", `{"monthlyUsd":"10"}`},
		{"/api/v1/cost/budgets/payments", `{"monthlyUsd":10,"extra":1}`},
		{"/api/v1/cost/budgets/payments", `not json`},
		{"/api/v1/cost/budgets/Payments", `{"monthlyUsd":10}`},
		{"/api/v1/cost/budgets/team_a", `{"monthlyUsd":10}`},
		{"/api/v1/cost/budgets/-a", `{"monthlyUsd":10}`},
	}
	for _, b := range bad {
		if w := e.do("PUT", b.path, b.body, asCost("admin")); w.Code != 400 {
			t.Errorf("PUT %s %s: %d %s", b.path, b.body, w.Code, w.Body.String())
		}
	}

	if w := e.do("DELETE", "/api/v1/cost/budgets/nothing", "", asCost("admin")); w.Code != 404 {
		t.Fatalf("delete missing: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/cost/budgets/Bad_NS", "", asCost("admin")); w.Code != 400 {
		t.Fatalf("delete invalid: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/cost/budgets/retired", "", asCost("admin")); w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if !auditHas("cost.budget_removed", "retired", idAdminUser) {
		t.Error("budget removal not audited")
	}
	if l, _ := store.List(t.Context()); len(l) != 2 {
		t.Fatalf("store after delete: %+v", l)
	}
}

func TestCostRoutesRoles(t *testing.T) {
	e := newCostEnv(t, nil)
	if w := e.do("PUT", "/api/v1/cost/budgets/data", `{"monthlyUsd":200}`, asCost("admin")); w.Code != 200 {
		t.Fatalf("seed: %d", w.Code)
	}
	cases := []struct {
		who, method, path, body string
		want                    int
	}{
		{"viewer", "GET", "/api/v1/cost/budgets", "", 200},
		{"operator", "GET", "/api/v1/cost/budgets", "", 200},
		{"scoped", "GET", "/api/v1/cost/budgets", "", 403},
		{"scopedop", "GET", "/api/v1/cost/budgets", "", 403},
		{"viewer", "PUT", "/api/v1/cost/budgets/data", `{"monthlyUsd":1}`, 403},
		{"operator", "PUT", "/api/v1/cost/budgets/data", `{"monthlyUsd":1}`, 403},
		{"scopedop", "PUT", "/api/v1/cost/budgets/payments", `{"monthlyUsd":1}`, 403},
		{"viewer", "DELETE", "/api/v1/cost/budgets/data", "", 403},
		{"operator", "DELETE", "/api/v1/cost/budgets/data", "", 403},
		{"scopedop", "DELETE", "/api/v1/cost/budgets/payments", "", 403},
		{"viewer", "GET", "/api/v1/cost/rightsizing", "", 200},
		{"operator", "GET", "/api/v1/cost/rightsizing?namespace=data", "", 200},
		{"scoped", "GET", "/api/v1/cost/rightsizing?namespace=payments", "", 403},
		{"scopedop", "GET", "/api/v1/cost/rightsizing", "", 403},
		// GET /cost keeps its existing behaviour (readable by restricted accounts).
		{"scoped", "GET", "/api/v1/cost", "", 200},
	}
	for _, c := range cases {
		if w := e.do(c.method, c.path, c.body, asCost(c.who)); w.Code != c.want {
			t.Errorf("%s %s %s: %d, want %d (%s)", c.who, c.method, c.path, w.Code, c.want, w.Body.String())
		}
	}
	if w := e.do("DELETE", "/api/v1/cost/budgets/data", "", asCost("admin")); w.Code != 200 {
		t.Fatalf("admin delete: %d", w.Code)
	}
}

func TestCostRightsizingEndpoint(t *testing.T) {
	e := newCostEnv(t, nil)
	w := e.do("GET", "/api/v1/cost/rightsizing", "", asCost("viewer"))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var rep platform.RightsizingReport
	decodeInto(t, w, &rep)
	if rep.Source != "metrics-snapshot" || rep.Window != "7d" || rep.Confidence != "low" || rep.Note == "" ||
		rep.Summary.OverProvisioned == 0 || rep.Summary.UnderProvisioned == 0 || rep.Summary.MonthlySavingsUSD <= 0 || len(rep.Items) == 0 {
		t.Fatalf("report %+v", rep)
	}
	for _, key := range []string{`"requestMilli"`, `"recommendedMilli"`, `"usageMilli"`, `"requestMiB"`, `"recommendedMiB"`, `"usageMiB"`, `"basis":"current"`, `"monthlyDeltaUsd"`, `"verdict"`, `"reason"`} {
		if !strings.Contains(w.Body.String(), key) {
			t.Errorf("response lacks %s", key)
		}
	}

	w = e.do("GET", "/api/v1/cost/rightsizing?namespace=data&window=30d", "", asCost("viewer"))
	decodeInto(t, w, &rep)
	if w.Code != 200 || rep.Window != "30d" || len(rep.Items) != 1 || rep.Items[0].Namespace != "data" {
		t.Fatalf("filtered %d %+v", w.Code, rep)
	}
	for _, q := range []string{"window=2d", "window=7", "window=7d%5D", "namespace=Bad_NS", `namespace=a%22%7D+or+vector(1)+%23`} {
		if w := e.do("GET", "/api/v1/cost/rightsizing?"+q, "", asCost("viewer")); w.Code != 400 {
			t.Errorf("%s: %d %s", q, w.Code, w.Body.String())
		}
	}
}
