package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/zyvorai/axiom/internal/platform"
)

// registerFleet wires the cross-cluster overview.
func (s *Server) registerFleet(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/fleet", s.fleet)
	mux.HandleFunc("POST /api/v1/fleet/refresh", s.refreshFleet)
	mux.HandleFunc("GET /api/v1/fleet/history", s.fleetHistory)
	mux.HandleFunc("GET /api/v1/fleet/outage-narrative", s.fleetOutageNarrative)
}

func (s *Server) fleetOutageNarrative(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cluster := strings.TrimSpace(q.Get("cluster"))
	if cluster == "" {
		fail(w, http.StatusBadRequest, fmt.Errorf("cluster is required"))
		return
	}
	writeJSON(w, http.StatusOK, platform.BuildOutageNarrative(r.Context(), s.provider, cluster, q.Get("from"), q.Get("state")))
}

// fleetHistory returns the probe history (uptime, incidents and a
// downsampled series) of every cluster, or of ?cluster=, over ?window=.
// Access is that of /fleet (it is under the same restricted prefix).
func (s *Server) fleetHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	window, err := platform.NormalizeFleetHistoryWindow(q.Get("window"))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	cluster := q.Get("cluster")
	if q.Has("cluster") {
		if err := platform.ValidateFleetClusterName(cluster); err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	sp := s.switchable()
	if sp == nil {
		if cluster != "" {
			fail(w, http.StatusNotFound, fmt.Errorf("unknown cluster %q", cluster))
			return
		}
		writeJSON(w, http.StatusOK, platform.FleetHistory{Window: window, Clusters: []platform.FleetClusterHistory{}})
		return
	}
	h, err := sp.FleetHistory(r.Context(), cluster, window)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	writeJSON(w, http.StatusOK, h)
}

// fleet returns every registered cluster with its latest health and a roll-up.
// It reads the prober's cache (probing any cluster that has never been probed),
// so it is cheap to poll.
func (s *Server) fleet(w http.ResponseWriter, r *http.Request) {
	sp := s.switchable()
	if sp == nil {
		writeJSON(w, http.StatusOK, platform.Fleet{Clusters: []platform.FleetCluster{}})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, sp.Fleet(r.Context()))
}

// refreshFleet probes every cluster now (rate-limited inside the platform) and
// returns the fresh fleet. It only reads from the clusters, so every
// authenticated role may use it.
func (s *Server) refreshFleet(w http.ResponseWriter, r *http.Request) {
	sp := s.switchable()
	if sp == nil {
		writeJSON(w, http.StatusOK, platform.Fleet{Clusters: []platform.FleetCluster{}})
		return
	}
	sp.RefreshFleet(r.Context())
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, sp.Fleet(r.Context()))
}
