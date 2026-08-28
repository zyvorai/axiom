package api

import (
	"net/http"
)

func (s *Server) havenIdentity(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "identity"
	}
	v, err := s.provider.GetHavenIdentity(r.Context(), ns)
	respond(w, v, err)
}

func (s *Server) listIdentityPlanes(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "all"
	}
	v, err := s.provider.ListIdentityPlanes(r.Context(), ns)
	respond(w, v, err)
}

func (s *Server) havenAdmin(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	plane := r.URL.Query().Get("plane")
	if ns == "" {
		ns = "identity"
	}
	if plane == "" {
		plane = "platform"
	}
	v, err := s.provider.GetHavenAdminBootstrap(r.Context(), ns, plane)
	respond(w, v, err)
}
