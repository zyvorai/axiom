package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// VPA and KEDA ScaledObject edit in place, with the GitOps edit's design
// (internal/platform/autoscaling_edit.go). GET .../edit is a namespaced read
// (viewer, operator, admin); the preview POST and the PUT are operator/admin
// like create, viewers being refused by the guard. The namespace is in the
// path, so the guard authorizes it for restricted accounts; the body may echo
// it, and must then match the path.

func (s *Server) registerAutoscalingEdit(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/vpas/{namespace}/{name}/edit", s.getVPAEditable)
	mux.HandleFunc("POST /api/v1/vpas/{namespace}/{name}/edit/preview", s.previewVPAEdit)
	mux.HandleFunc("PUT /api/v1/vpas/{namespace}/{name}", s.updateVPA)
	mux.HandleFunc("GET /api/v1/scaledobjects/{namespace}/{name}/edit", s.getScaledObjectEditable)
	mux.HandleFunc("POST /api/v1/scaledobjects/{namespace}/{name}/edit/preview", s.previewScaledObjectEdit)
	mux.HandleFunc("PUT /api/v1/scaledobjects/{namespace}/{name}", s.updateScaledObject)
}

var errEditNamespaceMismatch = errors.New("namespace in the body does not match the namespace in the path; an autoscaler cannot be moved to another namespace")

// decodeAutoscalerEdit decodes the edit body, requires its (optional)
// namespace to equal the path's, and re-checks the caller may act there.
func decodeAutoscalerEdit[T any](w http.ResponseWriter, r *http.Request, nsOf func(T) string) (T, string, string, bool) {
	var req T
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, errors.New("invalid request body: "+err.Error()))
		return req, ns, name, false
	}
	if b := nsOf(req); b != "" && b != ns {
		fail(w, http.StatusBadRequest, errEditNamespaceMismatch)
		return req, ns, name, false
	}
	if !auth.NamespaceAllowed(r.Context(), ns) {
		fail(w, http.StatusForbidden, errNamespaceRestricted)
		return req, ns, name, false
	}
	return req, ns, name, true
}

func vpaEditNamespace(req platform.VPAEditRequest) string { return req.Namespace }
func scaledObjectEditNamespace(req platform.ScaledObjectEditRequest) string {
	return req.Namespace
}

func (s *Server) getVPAEditable(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetVPAEditable(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	respond(w, v, err)
}

func (s *Server) previewVPAEdit(w http.ResponseWriter, r *http.Request) {
	req, ns, name, ok := decodeAutoscalerEdit(w, r, vpaEditNamespace)
	if !ok {
		return
	}
	v, err := s.provider.PreviewVPAEdit(r.Context(), ns, name, req)
	respond(w, v, err)
}

func (s *Server) updateVPA(w http.ResponseWriter, r *http.Request) {
	req, ns, name, ok := decodeAutoscalerEdit(w, r, vpaEditNamespace)
	if !ok {
		return
	}
	v, err := s.provider.UpdateVPA(r.Context(), ns, name, req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	s.auditAutoscalerEdit(r, "autoscale.vpa_edit", ns+"/"+name, v)
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) getScaledObjectEditable(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.GetScaledObjectEditable(r.Context(), r.PathValue("namespace"), r.PathValue("name"))
	respond(w, v, err)
}

func (s *Server) previewScaledObjectEdit(w http.ResponseWriter, r *http.Request) {
	req, ns, name, ok := decodeAutoscalerEdit(w, r, scaledObjectEditNamespace)
	if !ok {
		return
	}
	v, err := s.provider.PreviewScaledObjectEdit(r.Context(), ns, name, req)
	respond(w, v, err)
}

func (s *Server) updateScaledObject(w http.ResponseWriter, r *http.Request) {
	req, ns, name, ok := decodeAutoscalerEdit(w, r, scaledObjectEditNamespace)
	if !ok {
		return
	}
	v, err := s.provider.UpdateScaledObject(r.Context(), ns, name, req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	s.auditAutoscalerEdit(r, "autoscale.keda_edit", ns+"/"+name, v)
	writeJSON(w, http.StatusOK, v)
}

// auditAutoscalerEdit records the changed field NAMES only — never values
// (trigger settings can hold endpoints).
func (s *Server) auditAutoscalerEdit(r *http.Request, action, target string, v platform.AutoscalerEditResult) {
	if v.Noop {
		return
	}
	platform.RecordAxiomAudit(action, target, "fields="+strings.Join(v.Changed, ","), s.auditUser(r))
	EmitStateChanged("autoscaling", "edit")
}
