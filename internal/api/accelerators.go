package api

import (
	"net/http"
)

func (s *Server) listAccelerators(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ListAccelerators(r.Context())
	respond(w, v, err)
}
