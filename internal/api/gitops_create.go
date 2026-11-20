package api

import (
	"errors"
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

// registerGitOpsCreate wires GitOps application creation. Both routes are
// admin-only (internal/auth/policy_gitops.go): creating an Argo CD / Flux
// application deploys arbitrary workloads, like a Helm install.
func (s *Server) registerGitOpsCreate(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/gitops/preview", s.previewGitOps)
	mux.HandleFunc("POST /api/v1/gitops", s.createGitOps)
}

func decodeGitOpsRequest(w http.ResponseWriter, r *http.Request) (platform.GitOpsCreateRequest, bool) {
	var req platform.GitOpsCreateRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, errors.New("invalid request body: "+err.Error()))
		return req, false
	}
	return req, true
}

// previewGitOps validates and renders the objects without touching the
// cluster, so it works even when the engine's CRDs are absent.
func (s *Server) previewGitOps(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeGitOpsRequest(w, r)
	if !ok {
		return
	}
	v, err := platform.PreviewGitOps(req)
	respond(w, v, err)
}

func (s *Server) createGitOps(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeGitOpsRequest(w, r)
	if !ok {
		return
	}
	v, err := s.provider.CreateGitOpsApp(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	if n := len(v.Created); n > 0 {
		app := v.Created[n-1]
		platform.RecordAxiomAudit("gitops.create", app.Kind+"/"+app.Namespace+"/"+app.Name, req.Engine, s.auditUser(r))
	}
	EmitStateChanged("gitops", "create")
	writeJSON(w, http.StatusCreated, v)
}
