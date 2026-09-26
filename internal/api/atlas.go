package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) atlasSummary(w http.ResponseWriter, r *http.Request) {
	atlas := s.atlasClient()
	console := ""
	if s.hub != nil {
		console = s.hub.Get().AtlasConsoleURL
	}
	if atlas == nil || !atlas.Enabled() {
		if s.mode == "demo" {
			writeJSON(w, http.StatusOK, platform.DemoAtlasSummary().WithConsole(console))
			return
		}
		writeJSON(w, http.StatusOK, platform.AtlasSummary{
			Available:  false,
			ConsoleURL: console,
			Reason:     "Configure Atlas gateway URL in Settings → Integrations",
		})
		return
	}
	writeJSON(w, http.StatusOK, atlas.Summary(r.Context()).WithConsole(console))
}

func (s *Server) atlasProxy(w http.ResponseWriter, r *http.Request) {
	atlas := s.atlasClient()
	if atlas == nil || !atlas.Enabled() {
		fail(w, http.StatusServiceUnavailable, fmt.Errorf("atlas not configured — set it in Settings → Integrations"))
		return
	}
	path := strings.Trim(r.PathValue("path"), "/")
	if path == "" {
		fail(w, http.StatusBadRequest, fmt.Errorf("path required"))
		return
	}
	allowed := map[string]bool{
		"health": true, "version": true, "livez": true, "readyz": true,
		"backends": true, "backends/summary": true,
		"clusters": true, "pools": true, "volumes": true,
		"policies": true, "buckets": true, "storage-classes": true,
		"metrics/summary": true, "ceph/status": true, "ceph/rook-status": true,
		"jobs": true, "kubernetes/pvcs": true, "kubernetes/pvs": true,
	}
	base := path
	if i := strings.IndexByte(path, '?'); i >= 0 {
		base = path[:i]
	}
	if !allowed[base] && !strings.HasPrefix(base, "jobs/") {
		fail(w, http.StatusForbidden, fmt.Errorf("atlas path not allowed"))
		return
	}
	raw, code, err := atlas.Get(r.Context(), path)
	if err != nil {
		if code == 0 {
			fail(w, http.StatusBadGateway, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write(raw)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (s *Server) atlasCreateVolume(w http.ResponseWriter, r *http.Request) {
	atlas := s.atlasClient()
	if (atlas == nil || !atlas.Enabled()) && s.mode == "demo" {
		// The demo summary presents a connected fabric, so accept the request the way a gateway would.
		var req platform.CreateAtlasVolumeRequest
		if err := decode(r, &req); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		platform.RecordAxiomAudit("atlas.volume.create", req.Namespace+"/"+req.Name, req.Policy, s.auditUser(r))
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "demo": true, "name": req.Name})
		return
	}
	if atlas == nil || !atlas.Enabled() {
		fail(w, http.StatusServiceUnavailable, fmt.Errorf("atlas not configured — set it in Settings → Integrations"))
		return
	}
	var req platform.CreateAtlasVolumeRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	raw, code, err := atlas.CreateVolume(r.Context(), req)
	if err != nil {
		if code == 0 {
			fail(w, http.StatusBadRequest, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write(raw)
		return
	}
	platform.RecordAxiomAudit("atlas.volume.create", req.Namespace+"/"+req.Name, req.Policy, s.auditUser(r))
	EmitStateChanged("atlas", "volume.create")
	w.Header().Set("Content-Type", "application/json")
	if code == 0 {
		code = http.StatusAccepted
	}
	w.WriteHeader(code)
	_, _ = w.Write(raw)
}
