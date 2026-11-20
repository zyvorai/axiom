package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) listCertificates(w http.ResponseWriter, r *http.Request) {
	v, err := platform.ListCertManager(r.Context(), s.provider)
	respond(w, v, err)
}
