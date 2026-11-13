package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) externalSecretsHealth(w http.ResponseWriter, r *http.Request) {
	v, err := platform.ListExternalSecretsHealth(r.Context(), s.provider, r.URL.Query().Get("namespace"))
	respond(w, v, err)
}
