package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zyvorai/axiom/internal/platform"
)

// parseChangeParams reads at (RFC3339, default now) and window (a duration
// such as 30m, 2h or 7d, default 60m, max 7d).
func parseChangeParams(atS, windowS string, now time.Time) (time.Time, time.Duration, error) {
	at := now
	if atS = strings.TrimSpace(atS); atS != "" {
		t, err := time.Parse(time.RFC3339, atS)
		if err != nil {
			return at, 0, fmt.Errorf("invalid at %q: use an RFC3339 time", atS)
		}
		at = t
	}
	window := platform.DefaultChangeWindow
	if windowS = strings.TrimSpace(windowS); windowS != "" {
		since, err := parseSince(windowS, now)
		if err != nil || since.IsZero() {
			return at, 0, fmt.Errorf("invalid window %q: use a duration such as 30m, 2h or 7d", windowS)
		}
		window = now.Sub(since)
		if window <= 0 {
			return at, 0, fmt.Errorf("window must be positive")
		}
	}
	if window > platform.MaxChangeWindow {
		window = platform.MaxChangeWindow
	}
	return at, window, nil
}

// changes answers "what changed before this?": ranked recent changes around a
// namespace/workload and time. Namespace restriction is enforced by the auth
// guard (a restricted caller must pass an owned ?namespace=).
func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ns := q.Get("namespace")
	if ns == "all" {
		ns = ""
	}
	at, window, err := parseChangeParams(q.Get("at"), q.Get("window"), time.Now())
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, platform.CollectChanges(r.Context(), s.provider, ns, strings.TrimSpace(q.Get("workload")), at, window))
}
