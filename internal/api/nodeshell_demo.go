//go:build !kubernetes

package api

import (
	"errors"
	"net/http"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

func debugInteractive() bool { return false }

// registerNodeShell in demo mode only answers the refusal paths, so a disabled
// or unsupported node shell fails with a clear error instead of a 404.
func registerNodeShell(mux *http.ServeMux, _ platform.Provider) {
	mux.HandleFunc("GET /api/v1/nodes/{name}/shell", func(w http.ResponseWriter, r *http.Request) {
		if !platform.NodeShellEnabled() {
			fail(w, platform.HTTPStatus(platform.ErrNodeShellDisabled), platform.ErrNodeShellDisabled)
			return
		}
		if id, ok := auth.IdentityFromContext(r.Context()); ok && id.Role != "admin" {
			fail(w, http.StatusForbidden, errors.New("node shell requires the admin role"))
			return
		}
		fail(w, http.StatusNotImplemented, errors.New("node shell requires a live Kubernetes connection (not available in demo mode)"))
	})
}
