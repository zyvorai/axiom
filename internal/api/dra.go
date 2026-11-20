package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) listDRA(w http.ResponseWriter, r *http.Request) {
	v, err := platform.ListDRA(r.Context(), s.provider)
	respond(w, v, err)
}
