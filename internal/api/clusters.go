package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) switchable() *platform.SwitchableProvider {
	sp, _ := s.provider.(*platform.SwitchableProvider)
	return sp
}

func (s *Server) listClusters(w http.ResponseWriter, r *http.Request) {
	sp := s.switchable()
	if sp == nil {
		writeJSON(w, http.StatusOK, []platform.ClusterInfo{})
		return
	}
	writeJSON(w, http.StatusOK, sp.Clusters(r.Context()))
}

func (s *Server) activateCluster(w http.ResponseWriter, r *http.Request) {
	sp := s.switchable()
	if sp == nil {
		fail(w, http.StatusBadRequest, errSSO("multi-cluster switching is not available on this deployment"))
		return
	}
	name := r.PathValue("name")
	if err := sp.SetActive(r.Context(), name); err != nil {
		fail(w, http.StatusNotFound, err)
		return
	}
	platform.RecordAxiomAudit("cluster.activate", name, "", s.auditUser(r))
	EmitStateChanged("cluster", "activate")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "active": name})
}
