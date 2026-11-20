package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

// listRollouts returns Argo Rollouts and Flagger Canaries (read-only).
func (s *Server) listRollouts(w http.ResponseWriter, r *http.Request) {
	v, err := platform.ListRollouts(r.Context(), s.provider, r.URL.Query().Get("namespace"))
	respond(w, v, err)
}
