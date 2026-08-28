//go:build !kubernetes

package api

import (
	"github.com/zyvorai/axiom/internal/platform"
	"net/http"
)

func RegisterRealtime(mux *http.ServeMux, provider platform.Provider) {
	mux.HandleFunc("GET /api/v1/realtime/capabilities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"interactiveExec": false, "logFollow": true, "mode": "demo"})
	})
}
