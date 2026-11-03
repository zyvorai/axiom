package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/zyvorai/axiom/internal/platform"
)

// registerGitOpsDelete wires GitOps application deletion. The preview is a
// read (admin, operator, viewer); DELETE is admin-only
// (internal/auth/policy_gitops.go). Namespace-restricted accounts never reach
// either: /api/v1/gitops is a restricted-sensitive prefix.
func (s *Server) registerGitOpsDelete(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/gitops/{kind}/{namespace}/{name}/delete-preview", s.previewGitOpsDelete)
	mux.HandleFunc("DELETE /api/v1/gitops/{kind}/{namespace}/{name}", s.deleteGitOps)
}

func (s *Server) previewGitOpsDelete(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.PreviewGitOpsDelete(r.Context(), r.PathValue("kind"), r.PathValue("namespace"), r.PathValue("name"))
	respond(w, v, err)
}

// deleteGitOps deletes an application. The provider recomputes the cascade
// at delete time and requires ?confirm=<name> when it cascades, so the typed
// confirmation never depends on the UI alone.
func (s *Server) deleteGitOps(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := platform.GitOpsDeleteRequest{
		Kind: r.PathValue("kind"), Namespace: r.PathValue("namespace"), Name: r.PathValue("name"),
		Confirm: q.Get("confirm"),
	}
	if v := q.Get("removeSource"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			fail(w, http.StatusBadRequest, errors.New("removeSource must be true or false"))
			return
		}
		req.RemoveSource = b
	}
	v, err := s.provider.DeleteGitOpsApp(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("gitops.delete", req.Kind+"/"+req.Namespace+"/"+req.Name, "cascade="+strconv.FormatBool(v.Cascade), s.auditUser(r))
	EmitStateChanged("gitops", "delete")
	writeJSON(w, http.StatusOK, v)
}
