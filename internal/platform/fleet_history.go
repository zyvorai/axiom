package platform

import (
	"context"
	"errors"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Fleet health history. Every probe (background, refresh or the lazy probe
// in Fleet) appends one sample per cluster AFTER the hysteresis decision, so
// the history shows the state the console showed. Samples live in memory; a
// downsampled copy is persisted periodically and restored at startup (see
// fleet_history_store.go). Restored samples carry the restored flag.

const (
	// fleetHistoryRetention is how far back samples are kept.
	fleetHistoryRetention = 24 * time.Hour
	// fleetHistoryMaxSamples caps one cluster's ring (24 h at 30 s is 2880).
	fleetHistoryMaxSamples = 3000
	// fleetHistoryMaxPoints is the most points one cluster's series returns.
	fleetHistoryMaxPoints = 240
)

// fleetHistoryWindows are the windows GET /fleet/history accepts.
var fleetHistoryWindows = map[string]time.Duration{"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour}

// FleetSample is one probe outcome (or, after downsampling, one bucket).
type FleetSample struct {
	T             int64   `json:"t"` // unix seconds
	State         string  `json:"state"`
	LatencyMs     int64   `json:"latencyMs"`
	ReadyNodes    int     `json:"readyNodes"`
	Nodes         int     `json:"nodes"`
	RunningPods   int     `json:"runningPods"`
	Pods          int     `json:"pods"`
	Warnings      int     `json:"warnings"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryPercent float64 `json:"memoryPercent"`
	// Restored marks a sample (a 5-minute bucket) restored from the persisted
	// history at startup, i.e. one recorded before this process started.
	Restored bool `json:"restored,omitempty"`
}

// fleetRecord is a stored sample plus what incidents and failed-check counts
// need but the series does not carry.
type fleetRecord struct {
	FleetSample
	failed bool   // the probe itself failed (also when hysteresis held the state)
	reason string // why it was degraded / unreachable
	// n and nFailed are the probe counts a restored bucket stands for (the
	// Restored flag is set); a live record is one probe.
	n, nFailed int
}

// checks is how many probes the record stands for.
func (r fleetRecord) checks() int {
	if r.Restored && r.n > 0 {
		return r.n
	}
	return 1
}

// failedChecks is how many of those probes failed.
func (r fleetRecord) failedChecks() int {
	if r.Restored {
		return r.nFailed
	}
	if r.failed {
		return 1
	}
	return 0
}

// FleetGap is a stretch without samples across a restart: from the last
// restored sample to the first live one.
type FleetGap struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// FleetIncident is a contiguous run of non-healthy samples.
type FleetIncident struct {
	State           string  `json:"state"` // the worst state in the run
	From            string  `json:"from"`
	To              *string `json:"to"` // null while ongoing
	DurationSeconds int64   `json:"durationSeconds"`
	Reason          string  `json:"reason,omitempty"`
}

// FleetClusterHistory is one cluster's history over the window.
type FleetClusterHistory struct {
	Cluster string        `json:"cluster"`
	Label   string        `json:"label"`
	Samples []FleetSample `json:"samples"`
	// UptimePercent is the time-weighted share of reachable (healthy or
	// degraded) samples; null when there are none.
	UptimePercent *float64        `json:"uptimePercent"`
	Checks        int             `json:"checks"`
	FailedChecks  int             `json:"failedChecks"`
	CurrentState  string          `json:"currentState"`
	Since         string          `json:"since,omitempty"`
	Incidents     []FleetIncident `json:"incidents"`
	// Gaps are restart gaps (no samples, not counted as downtime).
	Gaps []FleetGap `json:"gaps"`
}

// FleetHistory is the response of GET /api/v1/fleet/history.
type FleetHistory struct {
	Window          string                `json:"window"`
	IntervalSeconds int                   `json:"intervalSeconds"`
	Clusters        []FleetClusterHistory `json:"clusters"`
	// Durable is true when a persistent store is configured and the last save
	// succeeded within 3 persistence intervals.
	Durable bool `json:"durable"`
	// RestoredFrom is when the newest snapshot restored at startup was saved
	// (RFC3339), null when nothing was restored.
	RestoredFrom *string `json:"restoredFrom"`
}

// NormalizeFleetHistoryWindow validates ?window= ("" = 24h).
func NormalizeFleetHistoryWindow(w string) (string, error) {
	w = strings.TrimSpace(w)
	if w == "" {
		return "24h", nil
	}
	if _, ok := fleetHistoryWindows[w]; !ok {
		return "", NewStatusError(http.StatusBadRequest, errors.New("window must be one of 1h, 6h, 24h"))
	}
	return w, nil
}

// ValidateFleetClusterName checks ?cluster=: kubeconfig context names are
// free-form, so only length and control characters are refused.
func ValidateFleetClusterName(name string) error {
	if name == "" || len(name) > 253 {
		return NewStatusError(http.StatusBadRequest, errors.New("cluster must be 1 to 253 characters"))
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return NewStatusError(http.StatusBadRequest, errors.New("cluster contains invalid characters"))
		}
	}
	return nil
}

// recordHistory appends one sample for a probed cluster. Samples for a
// cluster that is no longer registered are dropped: sp.mu is held (read)
// across the check and the append, so Unregister cannot slip in between.
func (sp *SwitchableProvider) recordHistory(name string, h ClusterHealth, failed bool, at time.Time) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	if _, ok := sp.entries[name]; !ok {
		return
	}
	st := sp.fleet()
	st.mu.Lock()
	defer st.mu.Unlock()
	rec := fleetRecord{FleetSample: FleetSample{
		T: at.Unix(), State: h.State, LatencyMs: h.LatencyMs,
		ReadyNodes: h.ReadyNodes, Nodes: h.Nodes, RunningPods: h.RunningPods, Pods: h.Pods,
		Warnings: h.Warnings, CPUPercent: h.CPUPercent, MemoryPercent: h.MemoryPercent,
	}, failed: failed}
	switch h.State {
	case ClusterDegraded:
		rec.reason = h.Reason
	case ClusterUnreachable:
		rec.reason = h.Error
	}
	st.appendHistoryLocked(name, rec, at)
}

// appendHistoryLocked adds rec and prunes by age and count. st.mu held.
func (st *fleetState) appendHistoryLocked(name string, rec fleetRecord, now time.Time) {
	if st.history == nil {
		st.history = map[string][]fleetRecord{}
	}
	s := append(st.history[name], rec)
	cut := now.Add(-fleetHistoryRetention).Unix()
	i := 0
	for i < len(s) && s[i].T < cut {
		i++
	}
	if len(s)-i > fleetHistoryMaxSamples {
		i = len(s) - fleetHistoryMaxSamples
	}
	if i > 0 {
		s = s[i:]
	}
	st.history[name] = s
}

// historyInterval is the probe interval history math assumes.
func (st *fleetState) historyInterval() time.Duration {
	if st.interval > 0 {
		return st.interval
	}
	return DefaultFleetProbeInterval
}

// FleetHistory returns every cluster's (or one cluster's) history over
// window (already normalised). An unknown cluster is a 404.
func (sp *SwitchableProvider) FleetHistory(ctx context.Context, cluster, window string) (FleetHistory, error) {
	win, ok := fleetHistoryWindows[window]
	if !ok {
		return FleetHistory{}, NewStatusError(http.StatusBadRequest, errors.New("window must be one of 1h, 6h, 24h"))
	}
	type ident struct{ name, label string }
	sp.mu.RLock()
	var ids []ident
	for _, n := range sp.order {
		if cluster != "" && n != cluster {
			continue
		}
		ids = append(ids, ident{n, sp.entries[n].label})
	}
	sp.mu.RUnlock()
	if cluster != "" && len(ids) == 0 {
		return FleetHistory{}, notFoundf("unknown cluster %q", cluster)
	}

	st := sp.fleet()
	st.mu.Lock()
	now := st.now()
	interval := st.historyInterval()
	from := now.Add(-win).Unix()
	recs := make([][]fleetRecord, len(ids))
	heads := make([]ClusterHealth, len(ids))
	for i, id := range ids {
		all := st.history[id.name]
		j := sort.Search(len(all), func(k int) bool { return all[k].T >= from })
		recs[i] = append([]fleetRecord(nil), all[j:]...)
		heads[i] = ClusterHealth{State: ClusterUnknown}
		if h, ok := st.health[id.name]; ok {
			heads[i] = h
		}
	}
	durable := st.durableLocked(now)
	var restoredFrom *string
	if !st.restoredFrom.IsZero() {
		s := st.restoredFrom.UTC().Format(time.RFC3339)
		restoredFrom = &s
	}
	st.mu.Unlock()

	out := FleetHistory{Window: window, IntervalSeconds: int(interval / time.Second), Clusters: make([]FleetClusterHistory, 0, len(ids)),
		Durable: durable, RestoredFrom: restoredFrom}
	for i, id := range ids {
		r := recs[i]
		ch := FleetClusterHistory{
			Cluster: id.name, Label: id.label,
			Samples:       downsampleFleet(r, from, now.Unix(), fleetHistoryMaxPoints),
			UptimePercent: fleetUptime(r, interval),
			CurrentState:  heads[i].State, Since: heads[i].Since,
			Incidents: fleetIncidents(r, interval, now),
			Gaps:      fleetGaps(r, interval),
		}
		for _, s := range r {
			ch.Checks += s.checks()
			ch.FailedChecks += s.failedChecks()
		}
		out.Clusters = append(out.Clusters, ch)
	}
	return out, nil
}

// fleetStateRank orders states by badness (worst wins in a bucket).
func fleetStateRank(s string) int {
	switch s {
	case ClusterUnreachable:
		return 3
	case ClusterDegraded:
		return 2
	case ClusterHealthy:
		return 1
	}
	return 0
}

func fleetReachable(s string) bool { return s == ClusterHealthy || s == ClusterDegraded }

// downsampleFleet returns recs as-is when they fit in max points, else one
// point per non-empty time bucket ([from, to) split into max buckets): the
// worst state, average latency / CPU / memory, the last counts and the last
// sample's time.
func downsampleFleet(recs []fleetRecord, from, to int64, max int) []FleetSample {
	out := make([]FleetSample, 0, min(len(recs), max))
	if len(recs) <= max {
		for _, r := range recs {
			out = append(out, r.FleetSample)
		}
		return out
	}
	for _, g := range fleetBucketGroups(recs, from, to, max) {
		out = append(out, summarizeFleetGroup(g))
	}
	return out
}

// fleetBucketGroups splits recs (sorted by time) into the non-empty buckets
// of [from, to] cut into max equal buckets.
func fleetBucketGroups(recs []fleetRecord, from, to int64, max int) [][]fleetRecord {
	var out [][]fleetRecord
	if len(recs) == 0 || max < 1 {
		return out
	}
	span := to - from + 1
	if span < 1 {
		span = 1
	}
	bucketOf := func(t int64) int {
		b := int((t - from) * int64(max) / span)
		if b < 0 {
			b = 0
		}
		if b >= max {
			b = max - 1
		}
		return b
	}
	start, cur := 0, bucketOf(recs[0].T)
	for i := 1; i < len(recs); i++ {
		if b := bucketOf(recs[i].T); b != cur {
			out = append(out, recs[start:i])
			start, cur = i, b
		}
	}
	return append(out, recs[start:])
}

// summarizeFleetGroup folds one bucket into a point: the worst state, average
// latency / CPU / memory, the last counts and the last sample's time.
func summarizeFleetGroup(group []fleetRecord) FleetSample {
	p := group[len(group)-1].FleetSample
	var lat, cpu, mem float64
	for _, g := range group {
		if fleetStateRank(g.State) > fleetStateRank(p.State) {
			p.State = g.State
		}
		lat += float64(g.LatencyMs)
		cpu += g.CPUPercent
		mem += g.MemoryPercent
	}
	n := float64(len(group))
	p.LatencyMs = int64(math.Round(lat / n))
	p.CPUPercent = round1(cpu / n)
	p.MemoryPercent = round1(mem / n)
	return p
}

// fleetUptime is the reachable share of the samples, each weighted by the
// gap to the next sample capped at 2x the probe interval (the last sample
// counts one interval). Equal timestamps fall back to a plain count.
// Restored buckets are weighted by the gap to the next restored bucket capped
// at the bucket width, and the restart gap (last restored sample to first
// live one) counts at most one probe interval, so downtime of the console is
// never counted as downtime of the cluster.
func fleetUptime(recs []fleetRecord, interval time.Duration) *float64 {
	if len(recs) == 0 {
		return nil
	}
	limit := 2 * interval.Seconds()
	var up, total float64
	upCount := 0
	for i, r := range recs {
		w := interval.Seconds()
		if i+1 < len(recs) {
			gap := float64(recs[i+1].T - r.T)
			switch {
			case r.Restored && recs[i+1].Restored:
				w = math.Min(gap, fleetHistoryBucket.Seconds())
			case r.Restored:
				w = math.Min(gap, interval.Seconds())
			default:
				w = math.Min(gap, limit)
			}
		}
		if w < 0 {
			w = 0
		}
		total += w
		if fleetReachable(r.State) {
			up += w
			upCount++
		}
	}
	var pct float64
	if total > 0 {
		pct = up / total * 100
	} else {
		pct = float64(upCount) / float64(len(recs)) * 100
	}
	pct = round2(pct)
	return &pct
}

// fleetIncidents extracts contiguous non-healthy runs. A run ends at the
// first healthy sample after it (that sample's time is "to"); runs separated
// by a healthy stretch shorter than 2 probe intervals are merged. A run that
// reaches the newest sample is ongoing (to = null, duration up to now). A run
// still open at the last restored sample ends there: the restart gap is not
// an outage, and the first live probe starts afresh.
func fleetIncidents(recs []fleetRecord, interval time.Duration, now time.Time) []FleetIncident {
	type run struct {
		from, to    int64 // to = -1 while ongoing
		state       string
		worstReason string
	}
	var runs []run
	var cur *run
	for i, r := range recs {
		if cur != nil && i > 0 && recs[i-1].Restored && !r.Restored {
			cur.to = recs[i-1].T
			runs = append(runs, *cur)
			cur = nil
		}
		if r.State == ClusterHealthy || r.State == ClusterUnknown || r.State == "" {
			if cur != nil {
				cur.to = r.T
				runs = append(runs, *cur)
				cur = nil
			}
			continue
		}
		if cur == nil {
			cur = &run{from: r.T, to: -1, state: r.State}
		}
		if fleetStateRank(r.State) > fleetStateRank(cur.state) {
			cur.state, cur.worstReason = r.State, ""
		}
		if r.State == cur.state && r.reason != "" {
			cur.worstReason = r.reason
		}
	}
	if cur != nil {
		runs = append(runs, *cur)
	}
	// Merge runs whose healthy gap is shorter than 2 probe intervals.
	gap := int64((2 * interval) / time.Second)
	merged := make([]run, 0, len(runs))
	for _, r := range runs {
		if n := len(merged); n > 0 && merged[n-1].to >= 0 && r.from-merged[n-1].to < gap {
			last := &merged[n-1]
			last.to = r.to
			if fleetStateRank(r.state) > fleetStateRank(last.state) {
				last.state, last.worstReason = r.state, r.worstReason
			} else if r.state == last.state && r.worstReason != "" {
				last.worstReason = r.worstReason
			}
			continue
		}
		merged = append(merged, r)
	}
	out := make([]FleetIncident, 0, len(merged))
	for _, r := range merged {
		inc := FleetIncident{State: r.state, From: time.Unix(r.from, 0).UTC().Format(time.RFC3339), Reason: r.worstReason}
		end := now.Unix()
		if r.to >= 0 {
			end = r.to
			s := time.Unix(r.to, 0).UTC().Format(time.RFC3339)
			inc.To = &s
		}
		if d := end - r.from; d > 0 {
			inc.DurationSeconds = d
		}
		out = append(out, inc)
	}
	return out
}

// fleetGaps lists the restart gaps: a restored sample followed by a live one
// more than 2 probe intervals later.
func fleetGaps(recs []fleetRecord, interval time.Duration) []FleetGap {
	out := []FleetGap{}
	limit := int64((2 * interval) / time.Second)
	for i := 1; i < len(recs); i++ {
		if recs[i-1].Restored && !recs[i].Restored && recs[i].T-recs[i-1].T > limit {
			out = append(out, FleetGap{
				From: time.Unix(recs[i-1].T, 0).UTC().Format(time.RFC3339),
				To:   time.Unix(recs[i].T, 0).UTC().Format(time.RFC3339),
			})
		}
	}
	return out
}

// forgetHistory drops a cluster's samples (it was unregistered). st.mu held.
func (st *fleetState) forgetHistoryLocked(name string) {
	delete(st.history, name)
}

// ---------------------------------------------------------------------------
// Demo backfill (AXIOM_DEMO_FLEET_HISTORY_SEED=true)

// seedFleetHistoryForDemo backfills 24 h of plausible samples at the default
// probe interval for every registered demo cluster, so the Fleet page (and
// an e2e) has a history without waiting. One cluster gets a 20-minute
// incident about 3 hours ago: the first AXIOM_DEMO_FLEET_DOWN cluster
// (unreachable) if set, else ap-south-1 (degraded, one node not ready).
func seedFleetHistoryForDemo(sp *SwitchableProvider, down map[string]bool) {
	if !truthyEnv(os.Getenv("AXIOM_DEMO_FLEET_HISTORY_SEED")) {
		return
	}
	incidentOn, incidentState := "ap-south-1", ClusterDegraded
	sp.mu.RLock()
	type target struct {
		name string
		p    Provider
	}
	var ts []target
	for _, n := range sp.order {
		ts = append(ts, target{n, sp.entries[n].provider})
		if down[n] && incidentState != ClusterUnreachable {
			incidentOn, incidentState = n, ClusterUnreachable
		}
	}
	sp.mu.RUnlock()

	st := sp.fleet()
	now := st.now()
	step := DefaultFleetProbeInterval
	n := int(fleetHistoryRetention/step) - 1
	incEnd := now.Add(-3 * time.Hour)
	incStart := incEnd.Add(-20 * time.Minute)
	for ci, t := range ts {
		// The demo overview is static; ignore a simulated failure here.
		var base Overview
		if dp := unwrapDemo(t.p); dp != nil {
			base, _ = dp.Overview(context.Background())
		}
		if base.Nodes == 0 {
			base = Overview{Nodes: 3, ReadyNodes: 3, Pods: 40, RunningPods: 38, Warnings: 1, CPUPercent: 35, MemoryPercent: 55}
		}
		st.mu.Lock()
		for i := n; i >= 1; i-- {
			at := now.Add(-time.Duration(i) * step)
			// Deterministic daily-ish wave plus a small per-cluster offset.
			phase := float64(i)/float64(n)*2*math.Pi + float64(ci)
			h := ClusterHealth{
				State: ClusterHealthy, LatencyMs: int64(40 + 15*ci + int(10*math.Abs(math.Sin(phase*7)))),
				Nodes: base.Nodes, ReadyNodes: base.Nodes, Pods: base.Pods, RunningPods: base.RunningPods,
				Warnings:      base.Warnings,
				CPUPercent:    round1(clampPct(base.CPUPercent + 12*math.Sin(phase))),
				MemoryPercent: round1(clampPct(base.MemoryPercent + 6*math.Sin(phase+1))),
			}
			rec := fleetRecord{}
			if t.name == incidentOn && !at.Before(incStart) && at.Before(incEnd) {
				if incidentState == ClusterUnreachable {
					h = ClusterHealth{State: ClusterUnreachable, LatencyMs: 8000}
					rec.failed = true
					rec.reason = "connection refused (simulated)"
				} else {
					h.State = ClusterDegraded
					h.ReadyNodes = max(0, base.Nodes-1)
					rec.reason = "1 of " + strconv.Itoa(base.Nodes) + " nodes are not ready"
				}
			}
			rec.FleetSample = FleetSample{T: at.Unix(), State: h.State, LatencyMs: h.LatencyMs, ReadyNodes: h.ReadyNodes, Nodes: h.Nodes,
				RunningPods: h.RunningPods, Pods: h.Pods, Warnings: h.Warnings, CPUPercent: h.CPUPercent, MemoryPercent: h.MemoryPercent}
			st.appendHistoryLocked(t.name, rec, now)
		}
		st.mu.Unlock()
	}
}

func unwrapDemo(p Provider) *DemoProvider {
	for i := 0; i < 8 && p != nil; i++ {
		switch v := p.(type) {
		case *DemoProvider:
			return v
		case *clusterLabeledProvider:
			p = v.Provider
		case *simulatedClusterProvider:
			p = v.Provider
		default:
			return nil
		}
	}
	return nil
}
