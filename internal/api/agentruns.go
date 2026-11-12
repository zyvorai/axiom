package api

import (
	"net/http"
	"strings"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) listAgentRuns(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListAgentRuns(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) getAgentRun(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetAgentRun(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	if err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) createAgentRun(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateAgentRunRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Source) == "" {
		req.Source = platform.AgentSourceManual
	}
	v, err := s.provider.CreateAgentRun(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	// Built-in planner: fill plan after create (works without Hermes CRD).
	planned, err := platform.PlanAgentRun(r.Context(), s.provider, s.aiSettings(), s.mode == "demo", v, req.ExtraContext)
	if err == nil {
		if updated, uerr := s.provider.UpdateAgentRun(r.Context(), planned); uerr == nil {
			v = updated
		} else {
			v = planned
		}
	}
	platform.RecordAxiomAudit("agentrun.create", req.Namespace+"/"+v.Name, req.Goal, s.auditUser(r))
	EmitStateChanged("agentrun", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) approveAgentRun(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	v, err := s.provider.ApproveAgentRun(r.Context(), ns, name)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("agentrun.approve", ns+"/"+name, v.Phase, s.auditUser(r))
	if len(v.Executed) > 0 {
		platform.RecordAxiomAudit("agentrun.execute", ns+"/"+name, v.Message, s.auditUser(r))
	}
	EmitStateChanged("agentrun", "approve")
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) denyAgentRun(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	v, err := s.provider.DenyAgentRun(r.Context(), ns, name)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("agentrun.deny", ns+"/"+name, v.Phase, s.auditUser(r))
	EmitStateChanged("agentrun", "deny")
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) listPlatformIntents(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListPlatformIntents(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}
