//go:build !kubernetes

package api

import (
	"github.com/zyvorai/axiom/internal/platform"
	"net/http"
)

// podProxy reports whether pod windows use the separate origin (window-link).
func RegisterRealtime(mux *http.ServeMux, provider platform.Provider, podProxy func() bool) {
	mux.HandleFunc("GET /api/v1/realtime/capabilities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"interactiveExec": false, "logFollow": true, "mode": "demo", "podProxy": podProxy != nil && podProxy()})
	})
}
