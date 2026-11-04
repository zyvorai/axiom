//go:build !kubernetes

package api

import (
	"github.com/zyvorai/axiom/internal/platform"
	"net/http"
)

// podProxy reports the separate pod proxy origin's mode ("" when off; see
// podProxyCapabilities).
func RegisterRealtime(mux *http.ServeMux, provider platform.Provider, podProxy func() string) {
	mux.HandleFunc("GET /api/v1/realtime/capabilities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, podProxyCapabilities(map[string]any{"interactiveExec": false, "logFollow": true, "mode": "demo"}, podProxy))
	})
}
