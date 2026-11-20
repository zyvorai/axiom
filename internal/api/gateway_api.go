package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) listGatewayAPI(w http.ResponseWriter, r *http.Request) {
	v, err := platform.ListGatewayAPI(r.Context(), s.provider)
	respond(w, v, err)
}
