package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		writeJSON(w, http.StatusOK, []platform.UserView{})
		return
	}
	writeJSON(w, http.StatusOK, s.users.List())
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		fail(w, http.StatusServiceUnavailable, errSSO("user management is not available on this deployment"))
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.users.Create(r.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	platform.RecordAxiomAudit("user.create", v.Username, v.Role, s.auditUser(r))
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		fail(w, http.StatusServiceUnavailable, errSSO("user management is not available on this deployment"))
		return
	}
	username := r.PathValue("username")
	err := s.users.Delete(r.Context(), username)
	if err == nil {
		platform.RecordAxiomAudit("user.delete", username, "", s.auditUser(r))
	}
	respondAction(w, err)
}
