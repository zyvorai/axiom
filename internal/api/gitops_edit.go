package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/zyvorai/axiom/internal/platform"
)

// registerGitOpsEdit wires editing a GitOps application in place. GET .../edit
// is a read (admin, operator, viewer, like the delete preview); the preview
// POST and the PUT are admin-only (internal/auth/policy_gitops.go).
// Namespace-restricted accounts reach none of them (/api/v1/gitops is a
// restricted-sensitive prefix).
func (s *Server) registerGitOpsEdit(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/gitops/{kind}/{namespace}/{name}/edit", s.getGitOpsEditable)
	mux.HandleFunc("POST /api/v1/gitops/{kind}/{namespace}/{name}/edit/preview", s.previewGitOpsEdit)
	mux.HandleFunc("PUT /api/v1/gitops/{kind}/{namespace}/{name}", s.updateGitOps)
}

func gitOpsPathRef(r *http.Request) platform.GitOpsRef {
	return platform.GitOpsRef{Kind: r.PathValue("kind"), Namespace: r.PathValue("namespace"), Name: r.PathValue("name")}
}

func decodeGitOpsEdit(w http.ResponseWriter, r *http.Request) (platform.GitOpsEditRequest, bool) {
	var req platform.GitOpsEditRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, errors.New("invalid request body: "+err.Error()))
		return req, false
	}
	return req, true
}

func (s *Server) getGitOpsEditable(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetGitOpsEditable(r.Context(), gitOpsPathRef(r))
	respond(w, v, err)
}

func (s *Server) previewGitOpsEdit(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeGitOpsEdit(w, r)
	if !ok {
		return
	}
	v, err := s.provider.PreviewGitOpsEdit(r.Context(), gitOpsPathRef(r), req)
	respond(w, v, err)
}

func (s *Server) updateGitOps(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeGitOpsEdit(w, r)
	if !ok {
		return
	}
	ref := gitOpsPathRef(r)
	v, err := s.provider.UpdateGitOpsApp(r.Context(), ref, req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	if !v.Noop {
		// Field names only — never the values.
		fields := make([]string, 0, len(v.Changes))
		for _, c := range v.Changes {
			fields = append(fields, c.Field)
		}
		platform.RecordAxiomAudit("gitops.edit", ref.Kind+"/"+ref.Namespace+"/"+ref.Name, "fields="+strings.Join(fields, ","), s.auditUser(r))
		EmitStateChanged("gitops", "edit")
	}
	writeJSON(w, http.StatusOK, v)
}
