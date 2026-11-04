package api

import "net/http"

// registerAutoscaling wires the read-only VPA and KEDA ScaledObject views.
// HPAs keep their routes in server.go (handlers in hpa_crd.go).
func (s *Server) registerAutoscaling(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/vpas", s.listVPAs)
	mux.HandleFunc("GET /api/v1/scaledobjects", s.listScaledObjects)
	s.registerAutoscalingCreate(mux) // autoscaling_create.go
	s.registerAutoscalingEdit(mux)   // autoscaling_edit.go
}

// listVPAs returns VerticalPodAutoscalers with their recommendations;
// {"installed":false,"items":[]} when the VPA CRD is absent.
func (s *Server) listVPAs(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListVPAs(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}

// listScaledObjects returns KEDA ScaledObjects (trigger type and name only);
// {"installed":false,"items":[]} when the KEDA CRD is absent.
func (s *Server) listScaledObjects(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListScaledObjects(r.Context(), r.URL.Query().Get("namespace"))
	respond(w, v, err)
}
