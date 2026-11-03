package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/zyvorai/axiom/internal/platform"
)

// costAPI serves per-namespace cost budgets and rightsizing recommendations.
// Reads are open to every role except namespace-restricted accounts (both are
// cluster-wide cost data); budget writes are admin-only (auth/policy_cost.go);
// applying a rightsizing recommendation is a workload write (operator and
// admin, like scale).
type costAPI struct {
	s     *Server
	store platform.CostBudgetStore
}

func (s *Server) registerCost(mux *http.ServeMux) {
	store := s.auth.CostBudgetStore
	if store == nil {
		store = platform.NewMemoryCostBudgetStore()
	}
	c := &costAPI{s: s, store: store}
	mux.HandleFunc("GET /api/v1/cost/budgets", c.listBudgets)
	mux.HandleFunc("PUT /api/v1/cost/budgets/{namespace}", c.setBudget)
	mux.HandleFunc("DELETE /api/v1/cost/budgets/{namespace}", c.deleteBudget)
	mux.HandleFunc("GET /api/v1/cost/rightsizing", c.rightsizing)
	mux.HandleFunc("POST /api/v1/cost/rightsizing/apply", c.applyRightsizing)
}

func (c *costAPI) listBudgets(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	budgets, err := c.store.List(r.Context())
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	est, err := c.s.provider.EstimateCost(r.Context())
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, platform.BuildCostBudgetReport(est, budgets))
}

func (c *costAPI) setBudget(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	ns := r.PathValue("namespace")
	if err := platform.ValidateCostNamespace(ns); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	var req struct {
		MonthlyUSD  *float64 `json:"monthlyUsd"`
		WarnPercent *float64 `json:"warnPercent"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("invalid JSON body: %w", err))
		return
	}
	if req.MonthlyUSD == nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("monthlyUsd is required"))
		return
	}
	b := platform.CostBudget{
		Namespace: ns, MonthlyUSD: *req.MonthlyUSD, WarnPercent: platform.CostBudgetDefaultWarnPercent,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339), UpdatedBy: c.s.auditUser(r),
	}
	if req.WarnPercent != nil {
		b.WarnPercent = *req.WarnPercent
	}
	if err := platform.ValidateCostBudget(b); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if err := c.store.Set(r.Context(), b); err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("cost.budget_set", ns,
		"monthlyUsd="+strconv.FormatFloat(b.MonthlyUSD, 'f', -1, 64)+" warnPercent="+strconv.FormatFloat(b.WarnPercent, 'f', -1, 64),
		c.s.auditUser(r))
	// The row reports the current estimate; an estimate failure does not undo the write.
	est, _ := c.s.provider.EstimateCost(r.Context())
	writeJSON(w, http.StatusOK, platform.CostBudgetRowFor(b, est))
}

func (c *costAPI) deleteBudget(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	ns := r.PathValue("namespace")
	if err := platform.ValidateCostNamespace(ns); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if err := c.store.Delete(r.Context(), ns); err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("cost.budget_removed", ns, "", c.s.auditUser(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "namespace": ns})
}

func (c *costAPI) rightsizing(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	q := r.URL.Query()
	ns := q.Get("namespace")
	if ns != "" {
		if err := platform.ValidateCostNamespace(ns); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
	}
	window, err := platform.NormalizeRightsizingWindow(q.Get("window"))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := c.s.provider.ListRightsizing(r.Context(), ns, window)
	respond(w, v, err)
}

// applyRightsizing recomputes the recommendation for one container on the
// server and writes it (or only returns the changes when dryRun is set).
func (c *costAPI) applyRightsizing(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var req platform.RightsizingApplyRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("invalid JSON body: %w", err))
		return
	}
	req, err := platform.NormalizeRightsizingApplyRequest(req)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	res, err := c.s.provider.ApplyRightsizing(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	if !res.DryRun {
		platform.RecordAxiomAudit("cost.rightsizing_apply", req.Kind+"/"+req.Namespace+"/"+req.Workload+"/"+req.Container,
			res.Summary()+" (window "+req.Window+", source "+res.Source+")", c.s.auditUser(r))
		EmitStateChanged("workload", "update")
	}
	writeJSON(w, http.StatusOK, res)
}
