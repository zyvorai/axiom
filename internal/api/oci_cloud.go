package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) refreshOCICloudToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Registry string `json:"registry"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	token, note, err := platform.RefreshOCICloudToken(r.Context(), req.Provider, req.Registry)
	if err != nil {
		fail(w, 502, err)
		return
	}
	platform.RecordAxiomAudit("helm.oci_cloud_refresh", req.Registry, req.Provider, s.auditUser(r))
	writeJSON(w, 200, map[string]any{
		"ok": true, "provider": req.Provider, "registry": req.Registry,
		"password": token, "note": note,
		"hint": "Write-only: store via POST /api/v1/helm/registry-credentials; token is short-lived",
	})
}
