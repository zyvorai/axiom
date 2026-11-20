package api

import (
	"net/http"
	"strings"

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
		Username   string   `json:"username"`
		Password   string   `json:"password"`
		Role       string   `json:"role"`
		Namespaces []string `json:"namespaces"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	v, err := s.users.Create(r.Context(), req.Username, req.Password, req.Role, req.Namespaces)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("user.create", v.Username, v.Role, s.auditUser(r))
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		fail(w, http.StatusServiceUnavailable, errSSO("user management is not available on this deployment"))
		return
	}
	username := r.PathValue("username")
	var req struct {
		Role       *string   `json:"role"`
		Password   *string   `json:"password"`
		Namespaces *[]string `json:"namespaces"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if req.Role == nil && req.Password == nil && req.Namespaces == nil {
		fail(w, http.StatusBadRequest, errSSO("nothing to update: provide role, password, and/or namespaces"))
		return
	}
	var v platform.UserView
	var err error
	if req.Role != nil {
		v, err = s.users.UpdateRole(r.Context(), username, *req.Role)
		if err != nil {
			fail(w, platform.HTTPStatus(err), err)
			return
		}
		platform.RecordAxiomAudit("user.role_change", username, v.Role, s.auditUser(r))
	}
	if req.Password != nil {
		v, err = s.users.ResetPassword(r.Context(), username, *req.Password)
		if err != nil {
			fail(w, platform.HTTPStatus(err), err)
			return
		}
		platform.RecordAxiomAudit("user.password_reset", username, "", s.auditUser(r))
	}
	if req.Namespaces != nil {
		v, err = s.users.UpdateNamespaces(r.Context(), username, *req.Namespaces)
		if err != nil {
			fail(w, platform.HTTPStatus(err), err)
			return
		}
		platform.RecordAxiomAudit("user.namespaces_change", username, strings.Join(v.Namespaces, ","), s.auditUser(r))
	}
	writeJSON(w, http.StatusOK, v)
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
