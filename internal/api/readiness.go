package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/zyvorai/axiom/internal/platform"
)

// Read-only readiness reports: backup coverage, disruption (PDB) audit and
// the deprecated-API manifest check. None of them changes the cluster.

func (s *Server) registerReadiness(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/backups/coverage", s.backupCoverage)
	mux.HandleFunc("GET /api/v1/disruption/audit", s.disruptionAudit)
	mux.HandleFunc("POST /api/v1/manifests/deprecations", s.manifestDeprecations)
}

func (s *Server) backupCoverage(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	maxAge := platform.BackupCoverageDefaultMaxAgeDays
	if v := r.URL.Query().Get("maxAgeDays"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > platform.BackupCoverageMaxAgeDaysCap {
			fail(w, http.StatusBadRequest, fmt.Errorf("maxAgeDays must be a whole number from 1 to %d", platform.BackupCoverageMaxAgeDaysCap))
			return
		}
		maxAge = n
	}
	v, err := platform.BackupCoverage(r.Context(), s.provider, maxAge)
	respond(w, v, err)
}

func (s *Server) disruptionAudit(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	ns := r.URL.Query().Get("namespace")
	if ns != "" && ns != "all" {
		if err := platform.ValidateCostNamespace(ns); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
	}
	v, err := platform.DisruptionAudit(r.Context(), s.provider, ns)
	respond(w, v, err)
}

// manifestDeprecations takes the manifest as raw YAML or as {"manifest": "..."}.
func (s *Server) manifestDeprecations(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, platform.APIDeprecationMaxBytes+4096))
	if err != nil {
		fail(w, http.StatusRequestEntityTooLarge, errors.New("manifest is too large"))
		return
	}
	manifest := string(body)
	// A JSON envelope {"manifest": "..."} is unwrapped; anything else is
	// treated as the manifest itself (YAML, or a JSON object).
	if t := strings.TrimSpace(manifest); strings.HasPrefix(t, "{") {
		var env struct {
			Manifest string `json:"manifest"`
			YAML     string `json:"yaml"`
		}
		if json.Unmarshal(body, &env) == nil {
			if env.Manifest != "" {
				manifest = env.Manifest
			} else if env.YAML != "" {
				manifest = env.YAML
			}
		}
	}
	rep, err := platform.CheckAPIDeprecations(manifest)
	if err != nil {
		if errors.Is(err, platform.ErrAPIDeprecationInput) {
			fail(w, http.StatusBadRequest, err)
			return
		}
		fail(w, http.StatusInternalServerError, err)
		return
	}
	respond(w, rep, nil)
}
