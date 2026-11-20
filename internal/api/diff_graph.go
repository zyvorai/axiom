package api

import (
	"errors"
	"net/http"

	"github.com/zyvorai/axiom/internal/auth"
)

// diffManifest previews POST /apply: live vs proposed for the manifest, from a server-side
// apply dry-run on a real cluster. It changes nothing, so it is deliberately not audited, but it
// carries the same RBAC-object guard as apply (a non-admin must not even probe those kinds) and,
// living under /api/v1/apply/, the same namespace-restricted lockout.
func (s *Server) diffManifest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Manifest string `json:"manifest"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if id, ok := auth.IdentityFromContext(r.Context()); ok && id.Role != "admin" && manifestTouchesRBAC(req.Manifest) {
		fail(w, http.StatusForbidden, errors.New("applying RBAC objects (Role, RoleBinding, ClusterRole, ClusterRoleBinding, ServiceAccount) requires the admin role"))
		return
	}
	v, err := s.provider.DiffManifest(r.Context(), req.Manifest)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// resourceGraph returns the relationship graph around a workload or pod (names and statuses
// only — never Secret/ConfigMap contents).
func (s *Server) resourceGraph(w http.ResponseWriter, r *http.Request) {
	v, err := s.provider.ResourceGraph(r.Context(), r.PathValue("namespace"), r.PathValue("kind"), r.PathValue("name"))
	respond(w, v, err)
}
