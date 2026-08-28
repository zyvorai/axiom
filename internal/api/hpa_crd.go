package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

// --- HPA ---

func (s *Server) listHPAs(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListHPAs(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) createHPA(w http.ResponseWriter, r *http.Request) {
	var req platform.CreateHPARequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateHPA(r.Context(), req)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	platform.RecordAxiomAudit("hpa.create", req.Namespace+"/"+req.Name, req.TargetKind+"/"+req.TargetName, s.auditUser(r))
	EmitStateChanged("hpa", "create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteHPA(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteHPA(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("hpa.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("hpa", "delete")
	}
	respondAction(w, err)
}

// --- PodDisruptionBudgets ---

func (s *Server) listPodDisruptionBudgets(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListPodDisruptionBudgets(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

// --- NetworkPolicies ---

func (s *Server) listNetworkPolicies(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListNetworkPolicies(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) deleteNetworkPolicy(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteNetworkPolicy(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("networkpolicy.delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("networkpolicy", "delete")
	}
	respondAction(w, err)
}

// --- Generic apply ---

func (s *Server) applyManifest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Manifest string `json:"manifest"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.ApplyManifest(r.Context(), req.Manifest)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if v.Valid {
		platform.RecordAxiomAudit("manifest.apply", v.Namespace+"/"+v.Name, v.Kind, s.auditUser(r))
		EmitStateChanged("manifest", "apply")
	}
	writeJSON(w, http.StatusOK, v)
}

// --- Per-pod metrics ---

func (s *Server) podMetrics(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetPodMetrics(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	respond(w, v, err)
}

// --- Generic CRD browser ---

func (s *Server) listCRDs(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListCRDs(r.Context())
	respond(w, v, err)
}

func (s *Server) listCRDInstances(w http.ResponseWriter, r *http.Request) {
	group, version, plural := r.PathValue("group"), r.PathValue("version"), r.PathValue("plural")
	v, err := s.provider.ListCRDInstances(r.Context(), group, version, plural, r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

func (s *Server) getCRDInstance(w http.ResponseWriter, r *http.Request) {
	group, version, plural, name := r.PathValue("group"), r.PathValue("version"), r.PathValue("plural"), r.PathValue("name")
	v, err := s.provider.GetCRDInstance(r.Context(), group, version, plural, r.URL.Query().Get("namespace"), name)
	respond(w, v, err)
}
