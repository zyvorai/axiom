//go:build kubernetes

package api

import (
	"errors"
	"github.com/zyvorai/axiom/internal/platform"
	"net/http"
)

// podProxy reports whether pod windows use the separate origin (window-link).
func RegisterRealtime(mux *http.ServeMux, provider platform.Provider, podProxy func() bool) {
	mux.HandleFunc("GET /api/v1/realtime/capabilities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"interactiveExec": true, "logFollow": true, "mode": "kubernetes", "podProxy": podProxy != nil && podProxy()})
	})
	mux.HandleFunc("GET /api/v1/pods/{namespace}/{name}/terminal", func(w http.ResponseWriter, r *http.Request) {
		p, ok := platform.CurrentKubernetesProvider(r.Context(), provider)
		if !ok {
			fail(w, 500, errors.New("interactive terminal requires Kubernetes provider"))
			return
		}
		platform.RecordAxiomAudit("pod.terminal", r.PathValue("namespace")+"/"+r.PathValue("name"), r.URL.Query().Get("container"), auditUserFromRequest(r))
		// ExecWS upgrades the connection; errors before upgrade leave no body,
		// errors after upgrade are already on the wire / close the socket.
		_ = p.ExecWS(r.Context(), w, r, r.PathValue("namespace"), r.PathValue("name"), r.URL.Query().Get("container"), r.URL.Query().Get("shell"))
	})
}
