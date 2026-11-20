package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) veleroRestores(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListVeleroRestores(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

// previewVeleroRestore computes what a restore would do without creating it.
func (s *Server) previewVeleroRestore(w http.ResponseWriter, r *http.Request) {
	var req platform.VeleroRestoreRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.PreviewVeleroRestore(r.Context(), req)
	respond(w, v, err)
}

// createVeleroRestore re-plans the restore and requires confirm=<name>
// server-side, so the typed confirmation never depends on the UI alone.
func (s *Server) createVeleroRestore(w http.ResponseWriter, r *http.Request) {
	var req platform.VeleroRestoreRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.provider.CreateVeleroRestore(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("velero-restore.create", req.Namespace+"/"+req.Name, platform.RestoreSummaryDetail(req), s.auditUser(r))
	EmitStateChanged("velero-restore", "create")
	writeJSON(w, http.StatusCreated, v)
}
