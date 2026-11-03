package api

import (
	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

// registerFleet wires the cross-cluster overview.
func (s *Server) registerFleet(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/fleet", s.fleet)
	mux.HandleFunc("POST /api/v1/fleet/refresh", s.refreshFleet)
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
