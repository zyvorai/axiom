package api

import (
	"errors"
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) registerGitOpsSuspend(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/gitops/{kind}/{namespace}/{name}/suspend", s.suspendGitOps)
}

func (s *Server) suspendGitOps(w http.ResponseWriter, r *http.Request) {
	kind, ns, name := r.PathValue("kind"), r.PathValue("namespace"), r.PathValue("name")
	var req platform.GitOpsSuspendRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, errors.New("invalid request body: "+err.Error()))
		return
	}
	res, err := platform.SuspendGitOpsApp(r.Context(), s.provider, kind, ns, name, req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	action := "gitops.resume"
	if req.Suspend {
		action = "gitops.suspend"
	}
	platform.RecordAxiomAudit(action, kind+"/"+ns+"/"+name, "", s.auditUser(r))
	EmitStateChanged("gitops", "suspend")
	writeJSON(w, http.StatusOK, res)
}
