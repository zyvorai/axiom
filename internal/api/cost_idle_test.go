package api

import (
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func TestCostIdleEndpoint(t *testing.T) {
	e := newCostEnv(t, platform.NewMemoryCostBudgetStore())
	w := e.do("GET", "/api/v1/cost/idle", "", asCost("viewer"))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("idle: %d %s", w.Code, w.Body.String())
	}
	var rep platform.IdleReport
	decodeInto(t, w, &rep)
	if rep.MinAgeDays != 7 || rep.Summary.Total == 0 || len(rep.Findings) == 0 {
		t.Fatalf("report %+v", rep)
	}
	if w := e.do("GET", "/api/v1/cost/idle?minAgeDays=abc", "", asCost("viewer")); w.Code != 400 {
		t.Fatalf("bad minAgeDays: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/cost/idle?namespace=Bad_NS", "", asCost("viewer")); w.Code != 400 {
		t.Fatalf("bad namespace: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/cost/idle", "", asCost("scoped")); w.Code != 403 {
		t.Fatalf("restricted account: %d", w.Code)
	}
}
