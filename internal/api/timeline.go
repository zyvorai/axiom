package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"net/http"

	"github.com/zyvorai/axiom/internal/platform"
)

const (
	defaultTimelineLimit = 200
	maxTimelineLimit     = 1000
	// maxTimelineRolloutWorkloads bounds the per-Deployment rollout lookups of
	// an unfiltered timeline (each is a ReplicaSet list on a real cluster).
	maxTimelineRolloutWorkloads = 25
)

// parseSince accepts an RFC3339 timestamp or a lookback such as "90m", "6h" or
// "7d". An empty value means "no lower bound".
func parseSince(v string, now time.Time) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if strings.HasSuffix(v, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(v, "d")); err == nil && n >= 0 {
			return now.Add(-time.Duration(n) * 24 * time.Hour), nil
		}
	}
	if d, err := time.ParseDuration(v); err == nil && d >= 0 {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid since %q: use an RFC3339 time or a duration such as 6h or 7d", v)
}

func eventKey(e platform.Event) string {
	return e.Namespace + "|" + e.Object + "|" + e.Reason + "|" + e.Message
}

// timeline merges Kubernetes events, Axiom audit entries, alert warnings and
// rollout revisions into one newest-first feed. Namespace restriction is
// enforced by the auth guard (a restricted caller must pass ?namespace= for a
// namespace it owns); the handler additionally scopes every source to that
// namespace, so cluster-wide audit actions (node cordon, user changes) never
// reach a namespaced query.
func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ns := q.Get("namespace")
	if ns == "all" {
		ns = ""
	}
	object := strings.TrimSpace(q.Get("object"))
	now := time.Now()
	since, err := parseSince(q.Get("since"), now)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	limit := defaultTimelineLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			fail(w, http.StatusBadRequest, fmt.Errorf("invalid limit %q", v))
			return
		}
		limit = min(n, maxTimelineLimit)
	}

	events, err := s.provider.ListEvents(r.Context(), ns)
	if err != nil {
		fail(w, platform.HTTPStatus(err), err)
		return
	}
	entries := make([]platform.TimelineEntry, 0, len(events)+64)

	// Warning events that the alerts view also reports are emitted once, as
	// alerts; everything else stays an event.
	alertKeys := map[string]bool{}
	if alerts, aerr := s.provider.ListAlerts(r.Context(), ns); aerr == nil {
		for _, a := range alerts.Warnings {
			if ns != "" && !strings.EqualFold(a.Namespace, ns) {
				continue
			}
			alertKeys[eventKey(a)] = true
			entries = append(entries, platform.TimelineFromAlert(a))
		}
	}
	for _, e := range events {
		if ns != "" && !strings.EqualFold(e.Namespace, ns) {
			continue
		}
		if strings.EqualFold(e.Type, "Warning") && alertKeys[eventKey(e)] {
			continue
		}
		entries = append(entries, platform.TimelineFromEvent(e))
	}

	for _, a := range platform.ListAxiomAudit() {
		if a.Action == "pod.exec" {
			a.Detail = "" // the command line may carry secrets; the fact of the exec is what matters here
		}
		t := platform.TimelineFromAudit(a)
		if ns != "" && !strings.EqualFold(t.Namespace, ns) {
			continue
		}
		entries = append(entries, t)
	}

	entries = append(entries, s.rolloutTimeline(r.Context(), ns, object, now)...)

	out := make([]platform.TimelineEntry, 0, len(entries))
	for _, e := range entries {
		if !since.IsZero() && e.Time.Before(since) {
			continue
		}
		if object != "" && !platform.TimelineObjectMatches(e.Object, object) {
			continue
		}
		out = append(out, e)
	}
	platform.SortTimeline(out)
	if len(out) > limit {
		out = out[:limit]
	}
	writeJSON(w, http.StatusOK, out)
}

// rolloutTimeline returns Deployment revision-created entries: for the one
// workload named by the object filter, or (unfiltered) for the namespace's
// Deployments up to a cap. Lookup failures are skipped, never fatal.
func (s *Server) rolloutTimeline(ctx context.Context, ns, object string, now time.Time) []platform.TimelineEntry {
	type target struct{ ns, kind, name string }
	var targets []target
	if object != "" {
		kind, name, ok := strings.Cut(object, "/")
		if !ok || ns == "" || !strings.EqualFold(kind, "Deployment") {
			return nil
		}
		targets = append(targets, target{ns, "Deployment", name})
	} else {
		wls, err := s.provider.ListWorkloads(ctx, ns)
		if err != nil {
			return nil
		}
		for _, wl := range wls {
			if wl.Kind == "Deployment" && len(targets) < maxTimelineRolloutWorkloads {
				targets = append(targets, target{wl.Namespace, wl.Kind, wl.Name})
			}
		}
	}
	var out []platform.TimelineEntry
	for _, t := range targets {
		st, err := s.provider.GetRolloutStatus(ctx, t.ns, t.kind, t.name)
		if err != nil {
			continue
		}
		for _, rev := range st.History {
			out = append(out, platform.TimelineFromRollout(t.ns, t.kind, t.name, rev, now))
		}
	}
	return out
}
