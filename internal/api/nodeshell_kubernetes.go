//go:build kubernetes

package api

import (
	"errors"
	"net/http"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

func debugInteractive() bool { return true }

// registerNodeShell serves GET /api/v1/nodes/{name}/shell (WebSocket). The auth
// guard already restricts it to unrestricted admins; the handler re-checks the
// role so the route stays safe if it is ever mounted without the guard.
func registerNodeShell(mux *http.ServeMux, provider platform.Provider) {
	mux.HandleFunc("GET /api/v1/nodes/{name}/shell", func(w http.ResponseWriter, r *http.Request) {
		if !platform.NodeShellEnabled() {
			fail(w, platform.HTTPStatus(platform.ErrNodeShellDisabled), platform.ErrNodeShellDisabled)
			return
		}
		if id, ok := auth.IdentityFromContext(r.Context()); ok && id.Role != "admin" {
			fail(w, http.StatusForbidden, errors.New("node shell requires the admin role"))
			return
		}
		p, ok := platform.CurrentKubernetesProvider(r.Context(), provider)
		if !ok {
			fail(w, http.StatusNotImplemented, errors.New("node shell requires a Kubernetes provider"))
			return
		}
		node, user := r.PathValue("name"), auditUserFromRequest(r)
		platform.RecordAxiomAudit("node.shell", node, "", user)
		// Errors before the WebSocket upgrade leave the response unwritten; once
		// upgraded NodeShellWS returns nil, so a non-nil error is always safe to report.
		if err := p.NodeShellWS(r.Context(), w, r, node, r.URL.Query().Get("shell"), user); err != nil {
			fail(w, platform.HTTPStatus(err), err)
		}
	})
}
