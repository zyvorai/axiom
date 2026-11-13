package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) listKarpenter(w http.ResponseWriter, r *http.Request) {
	v, err := platform.ListKarpenter(r.Context(), s.provider)
	respond(w, v, err)
}
