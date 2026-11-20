package api

import (
	"errors"
	"net/http"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

// VPA and KEDA ScaledObject create / preview / delete, and the
// TriggerAuthentication name list. Operators and admins may use them (the
// same class of action as creating an HPA); viewers are refused by the guard.
// The create and preview bodies carry the namespace, which the guard cannot
// see, so every body handler checks it against the caller first.

var errNamespaceRestricted = errors.New("this account is restricted to specific namespaces")

func (s *Server) registerAutoscalingCreate(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/vpas/preview", s.previewVPA)
	mux.HandleFunc("POST /api/v1/vpas", s.createVPA)
	mux.HandleFunc("DELETE /api/v1/vpas/{namespace}/{name}", s.deleteVPA)
	mux.HandleFunc("POST /api/v1/scaledobjects/preview", s.previewScaledObject)
	mux.HandleFunc("POST /api/v1/scaledobjects", s.createScaledObject)
	mux.HandleFunc("DELETE /api/v1/scaledobjects/{namespace}/{name}", s.deleteScaledObject)
	mux.HandleFunc("GET /api/v1/triggerauthentications", s.listTriggerAuthentications)
}

// decodeAutoscalerBody decodes the typed request and enforces the caller's
// namespace restriction on its namespace. ok is false once a response is sent.
func decodeAutoscalerBody[T any](w http.ResponseWriter, r *http.Request, nsOf func(T) string) (T, bool) {
	var req T
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, errors.New("invalid request body: "+err.Error()))
		return req, false
	}
	if !auth.NamespaceAllowed(r.Context(), nsOf(req)) {
		fail(w, http.StatusForbidden, errNamespaceRestricted)
		return req, false
	}
	return req, true
}

func vpaNamespace(req platform.VPACreateRequest) string { return req.Namespace }
func scaledObjectNamespace(req platform.ScaledObjectCreateRequest) string {
	return req.Namespace
}

// previewVPA validates and renders without touching the cluster, so it works
// even when the VPA CRD is absent.
func (s *Server) previewVPA(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAutoscalerBody(w, r, vpaNamespace)
	if !ok {
		return
	}
	v, err := platform.PreviewVPA(req)
	respond(w, v, err)
}

func (s *Server) createVPA(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAutoscalerBody(w, r, vpaNamespace)
	if !ok {
		return
	}
	v, err := s.provider.CreateVPA(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("autoscale.vpa_create", req.Namespace+"/"+req.Name, req.TargetKind+"/"+req.TargetName, s.auditUser(r))
	EmitStateChanged("autoscaling", "vpa_create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteVPA(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteVPA(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("autoscale.vpa_delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("autoscaling", "vpa_delete")
	}
	respondAction(w, err)
}

// previewScaledObject validates and renders without touching the cluster
// (no KEDA CRD needed; authenticationRef existence is checked by create).
func (s *Server) previewScaledObject(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAutoscalerBody(w, r, scaledObjectNamespace)
	if !ok {
		return
	}
	v, err := platform.PreviewScaledObject(req)
	respond(w, v, err)
}

func (s *Server) createScaledObject(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAutoscalerBody(w, r, scaledObjectNamespace)
	if !ok {
		return
	}
	v, err := s.provider.CreateScaledObject(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	// The audit detail is the target only: trigger settings are never logged.
	platform.RecordAxiomAudit("autoscale.keda_create", req.Namespace+"/"+req.Name, req.TargetKind+"/"+req.TargetName, s.auditUser(r))
	EmitStateChanged("autoscaling", "keda_create")
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteScaledObject(w http.ResponseWriter, r *http.Request) {
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	err := s.provider.DeleteScaledObject(r.Context(), ns, name)
	if err == nil {
		platform.RecordAxiomAudit("autoscale.keda_delete", ns+"/"+name, "", s.auditUser(r))
		EmitStateChanged("autoscaling", "keda_delete")
	}
	respondAction(w, err)
}

// listTriggerAuthentications returns TriggerAuthentication names (for the
// ScaledObject form's dropdown); {"installed":false,"items":[]} without KEDA.
func (s *Server) listTriggerAuthentications(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListTriggerAuthentications(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}
