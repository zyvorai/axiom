package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/zyvorai/axiom/internal/platform"
)

type registerClusterRequest struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Kubeconfig string `json:"kubeconfig"`
}

func (s *Server) registerCluster(w http.ResponseWriter, r *http.Request) {
	sp := s.switchable()
	if sp == nil {
		fail(w, http.StatusBadRequest, errSSO("multi-cluster registry is not available on this deployment"))
		return
	}
	var req registerClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Label = strings.TrimSpace(req.Label)
	if req.Name == "" {
		fail(w, http.StatusBadRequest, errSSO("name is required"))
		return
	}
	if req.Label == "" {
		req.Label = req.Name
	}
	p, server, err := platform.ProviderFromKubeconfigYAML(req.Kubeconfig, req.Name)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	sp.Register(req.Name, req.Label, p, server)
	_ = platform.SaveRemoteCluster(r.Context(), s.mode, s.provider, req.Name, req.Label, req.Kubeconfig)
	platform.RecordAxiomAudit("cluster.register", req.Name, req.Label, s.auditUser(r))
	EmitStateChanged("cluster", "register")
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "name": req.Name, "label": req.Label, "server": server})
}

func (s *Server) unregisterCluster(w http.ResponseWriter, r *http.Request) {
	sp := s.switchable()
	if sp == nil {
		fail(w, http.StatusBadRequest, errSSO("multi-cluster registry is not available on this deployment"))
		return
	}
	name := r.PathValue("name")
	if err := sp.Unregister(name); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	_ = platform.DeleteRemoteCluster(r.Context(), s.mode, s.provider, name)
	platform.RecordAxiomAudit("cluster.unregister", name, "", s.auditUser(r))
	EmitStateChanged("cluster", "unregister")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
