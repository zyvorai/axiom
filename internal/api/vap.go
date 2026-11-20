package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) listAdmissionPolicies(w http.ResponseWriter, r *http.Request) {
	v, err := platform.ListValidatingAdmissionPolicies(r.Context(), s.provider)
	respond(w, v, err)
}
