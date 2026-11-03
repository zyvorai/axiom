package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zyvorai/axiom/internal/platform"
)

// Fleet search rate limit: every search fans out to every cluster's API
// server, so a search box must not become a way to hammer them.
const (
	fleetSearchPerWindow = 20
	fleetSearchWindow    = 10 * time.Second
)

// fleetSearchRL is shared by every Server; keys carry the server pointer so
// two handlers (e.g. in tests) never share a bucket.
var fleetSearchRL aiRateLimiter

// registerFleetSearch wires GET /api/v1/fleet/search. Access follows
// /api/v1/fleet: every unrestricted role may search (it only reads), and
// namespace-restricted accounts are refused by the guard.
func (s *Server) registerFleetSearch(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/fleet/search", s.fleetSearch)
}

func (s *Server) fleetSearch(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	q := strings.TrimSpace(qs.Get("q"))
	if n := utf8.RuneCountInString(q); n < platform.FleetSearchMinQuery || n > platform.FleetSearchMaxQuery {
		fail(w, http.StatusBadRequest, fmt.Errorf("q must be %d to %d characters", platform.FleetSearchMinQuery, platform.FleetSearchMaxQuery))
		return
	}
	kinds, err := platform.ParseFleetSearchKinds(qs.Get("kind"))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	limit := platform.FleetSearchDefaultLimit
	if raw := strings.TrimSpace(qs.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > platform.FleetSearchMaxLimit {
			fail(w, http.StatusBadRequest, fmt.Errorf("limit must be between 1 and %d", platform.FleetSearchMaxLimit))
			return
		}
		limit = n
	}

	key := fmt.Sprintf("%p|%s", s, s.auditUser(r))
	if ok, wait := fleetSearchRL.allow(key, fleetSearchPerWindow, fleetSearchWindow); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, errors.New("too many fleet searches; try again shortly"))
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	sp := s.switchable()
	if sp == nil {
		writeJSON(w, http.StatusOK, platform.EmptyFleetSearch(q))
		return
	}
	writeJSON(w, http.StatusOK, sp.SearchFleet(r.Context(), q, kinds, limit))
}
