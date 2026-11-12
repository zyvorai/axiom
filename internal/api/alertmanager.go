package api

import (
	"net/http"
	"strings"

	"github.com/zyvorai/axiom/internal/platform"
)

func (s *Server) registerAlertmanager(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/alertmanager/alerts", s.amAlerts)
	mux.HandleFunc("GET /api/v1/alertmanager/silences", s.amSilences)
	mux.HandleFunc("POST /api/v1/alertmanager/silences", s.amCreateSilence)
	mux.HandleFunc("DELETE /api/v1/alertmanager/silences/{id}", s.amExpireSilence)
	mux.HandleFunc("GET /api/v1/alertmanager/status", s.amStatus)
}

func (s *Server) amClient() *platform.AlertmanagerClient {
	return platform.AlertmanagerFromEnv()
}

func (s *Server) amStatus(w http.ResponseWriter, r *http.Request) {
	c := s.amClient()
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": c != nil,
		"note":       "Set AXIOM_ALERTMANAGER_URL to list firing alerts and manage silences",
	})
}

func (s *Server) amAlerts(w http.ResponseWriter, r *http.Request) {
	c := s.amClient()
	if c == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "alerts": []any{}, "note": "Alertmanager is not configured"})
		return
	}
	alerts, err := c.ListFiring(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "alerts": []any{}, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "alerts": alerts})
}

func (s *Server) amSilences(w http.ResponseWriter, r *http.Request) {
	c := s.amClient()
	if c == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "silences": []any{}})
		return
	}
	silences, err := c.ListSilences(r.Context())
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "silences": silences})
}

func (s *Server) amCreateSilence(w http.ResponseWriter, r *http.Request) {
	c := s.amClient()
	if c == nil {
		fail(w, http.StatusBadRequest, errSSO("Alertmanager is not configured (AXIOM_ALERTMANAGER_URL)"))
		return
	}
	var req platform.CreateSilenceRequest
	if err := decode(r, &req); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.CreatedBy) == "" {
		req.CreatedBy = s.auditUser(r)
	}
	sil, err := c.CreateSilence(r.Context(), req)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("alertmanager.silence_create", sil.ID, req.Comment, s.auditUser(r))
	writeJSON(w, http.StatusCreated, sil)
}

func (s *Server) amExpireSilence(w http.ResponseWriter, r *http.Request) {
	c := s.amClient()
	if c == nil {
		fail(w, http.StatusBadRequest, errSSO("Alertmanager is not configured"))
		return
	}
	id := r.PathValue("id")
	if err := c.ExpireSilence(r.Context(), id); err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	platform.RecordAxiomAudit("alertmanager.silence_expire", id, "", s.auditUser(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}
