package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) registerUpgradeRisk(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/upgrade-risk", s.upgradeRisk)
	mux.HandleFunc("POST /api/v1/upgrade-risk/agent-run", s.upgradeRiskAgentRun)
}

func (s *Server) upgradeRisk(w http.ResponseWriter, r *http.Request) {
	rep, err := platform.AssessUpgradeRisk(r.Context(), s.provider)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// upgradeRiskAgentRun creates a ProposeOnly agent run from the current report.
func (s *Server) upgradeRiskAgentRun(w http.ResponseWriter, r *http.Request) {
	rep, err := platform.AssessUpgradeRisk(r.Context(), s.provider)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	dry := true
	req := platform.CreateAgentRunRequest{
		Namespace:    "default",
		Name:         "upgrade-risk",
		Goal:         "Review cluster upgrade readiness and propose safe prep actions",
		ExtraContext: rep.Summary + " status=" + rep.Status,
		DryRun:       &dry,
		Source:       platform.AgentSourceManual,
	}
	run, err := s.provider.CreateAgentRun(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	planned, _ := platform.PlanAgentRun(r.Context(), s.provider, s.aiSettings(), s.mode == "demo", run, req.ExtraContext)
	if updated, uerr := s.provider.UpdateAgentRun(r.Context(), planned); uerr == nil {
		run = updated
	} else {
		run = planned
	}
	platform.RecordAxiomAudit("agentrun.create", run.Namespace+"/"+run.Name, "upgrade-risk", s.auditUser(r))
	writeJSON(w, http.StatusCreated, map[string]any{"report": rep, "run": run})
}
