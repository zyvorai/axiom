package platform

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Fleet health. Every cluster registered on a SwitchableProvider is probed in
// the background: a probe asks the cluster for its Overview (the same call the
// Overview page makes) under a short timeout, so "reachable" means "answers
// API requests", not merely "has a kubeconfig". The result — state, latency,
// last error and a headline summary — is cached and served by GET /api/v1/fleet
// and shown next to each cluster in the switcher.

// Cluster states reported by a probe.
const (
	ClusterHealthy     = "healthy"
	ClusterDegraded    = "degraded"    // reachable, but nodes are not ready or the cluster reports a problem
	ClusterUnreachable = "unreachable" // the probe failed or timed out
	ClusterUnknown     = "unknown"     // not probed yet
)

const (
	// DefaultFleetProbeInterval is how often every cluster is probed.
	DefaultFleetProbeInterval = 30 * time.Second
	// fleetProbeTimeout bounds one probe, so one dead cluster cannot stall the rest.
	fleetProbeTimeoutDefault = 8 * time.Second
	// fleetProbeConcurrency bounds how many clusters are probed at once.
	fleetProbeConcurrency = 8
	// fleetRefreshMinGap is the shortest gap between two on-demand refreshes.
	fleetRefreshMinGap = 3 * time.Second
)

// fleetProbeTimeout is a variable so tests can shorten it.
var fleetProbeTimeout = fleetProbeTimeoutDefault

// ClusterHealth is the cached result of the latest probe of one cluster.
type ClusterHealth struct {
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"` // why it is degraded
	Error     string `json:"error,omitempty"`  // why it is unreachable
	CheckedAt string `json:"checkedAt,omitempty"`
	// Since is when the cluster last changed state.
	Since     string `json:"since,omitempty"`
	LatencyMs int64  `json:"latencyMs"`

	Nodes         int     `json:"nodes"`
	ReadyNodes    int     `json:"readyNodes"`
	Pods          int     `json:"pods"`
	RunningPods   int     `json:"runningPods"`
	Workloads     int     `json:"workloads"`
	VMs           int     `json:"vms"`
	Warnings      int     `json:"warnings"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryPercent float64 `json:"memoryPercent"`
}

// FleetCluster is a registered cluster together with its latest health. The
// identity fields are spelled out (rather than embedding ClusterInfo, which
// carries its own state/latency/error) so no JSON field is ambiguous.
type FleetCluster struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Current bool   `json:"current"`
	Server  string `json:"server,omitempty"`
	ClusterHealth
}

// FleetSummary rolls the whole fleet up. Resource totals only count clusters
// that answered their last probe.
type FleetSummary struct {
	Clusters      int     `json:"clusters"`
	Healthy       int     `json:"healthy"`
	Degraded      int     `json:"degraded"`
	Unreachable   int     `json:"unreachable"`
	Unknown       int     `json:"unknown"`
	Nodes         int     `json:"nodes"`
	ReadyNodes    int     `json:"readyNodes"`
	Pods          int     `json:"pods"`
	RunningPods   int     `json:"runningPods"`
	Workloads     int     `json:"workloads"`
	Warnings      int     `json:"warnings"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryPercent float64 `json:"memoryPercent"`
}

// Fleet is the response of GET /api/v1/fleet.
type Fleet struct {
	Summary         FleetSummary   `json:"summary"`
	Clusters        []FleetCluster `json:"clusters"`
	CheckedAt       string         `json:"checkedAt,omitempty"`
	IntervalSeconds int            `json:"intervalSeconds"`
}

// fleetState is the prober's bookkeeping, owned by SwitchableProvider.
type fleetState struct {
	mu          sync.Mutex
	health      map[string]ClusterHealth
	interval    time.Duration
	lastRefresh time.Time
	running     bool
	now         func() time.Time
}

func (sp *SwitchableProvider) fleet() *fleetState {
	sp.fleetOnce.Do(func() {
		sp.fleetSt = &fleetState{health: map[string]ClusterHealth{}, now: time.Now}
	})
	return sp.fleetSt
}

// classifyProbe turns a probe outcome into a ClusterHealth (without timing or
// transition bookkeeping).
func classifyProbe(ov Overview, err error) ClusterHealth {
	if err != nil {
		return ClusterHealth{State: ClusterUnreachable, Error: probeError(err)}
	}
	h := ClusterHealth{
		State: ClusterHealthy,
		Nodes: ov.Nodes, ReadyNodes: ov.ReadyNodes,
		Pods: ov.Pods, RunningPods: ov.RunningPods,
		Workloads: ov.Workloads, VMs: ov.VMs, Warnings: ov.Warnings,
		CPUPercent: ov.CPUPercent, MemoryPercent: ov.MemoryPercent,
	}
	switch {
	case ov.Nodes > 0 && ov.ReadyNodes < ov.Nodes:
		h.State = ClusterDegraded
		h.Reason = fmt.Sprintf("%d of %d nodes are not ready", ov.Nodes-ov.ReadyNodes, ov.Nodes)
	case ov.Health != "" && !strings.EqualFold(ov.Health, "Healthy"):
		h.State = ClusterDegraded
		h.Reason = "the cluster reports " + ov.Health
	}
	return h
}

// probeError makes a transport error short and single-line for the UI.
func probeError(err error) string {
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if len(msg) > 240 {
		msg = msg[:240] + "…"
	}
	return msg
}

// probe asks one cluster for its Overview and records the outcome. It returns
// the new health and whether the state changed.
func (sp *SwitchableProvider) probe(ctx context.Context, name string, p Provider) ClusterHealth {
	st := sp.fleet()
	pctx, cancel := context.WithTimeout(ctx, fleetProbeTimeout)
	defer cancel()
	start := st.now()
	var ov Overview
	var err error
	if p == nil {
		err = fmt.Errorf("no provider registered")
	} else {
		ov, err = p.Overview(pctx)
		if err == nil && pctx.Err() != nil {
			err = pctx.Err()
		}
	}
	h := classifyProbe(ov, err)
	now := st.now()
	h.LatencyMs = now.Sub(start).Milliseconds()
	h.CheckedAt = now.UTC().Format(time.RFC3339)

	st.mu.Lock()
	prev, had := st.health[name]
	if had && prev.State == h.State {
		h.Since = prev.Since
	} else {
		h.Since = h.CheckedAt
	}
	st.health[name] = h
	st.mu.Unlock()

	if had && prev.State != h.State && prev.State != ClusterUnknown {
		recordFleetTransition(name, prev.State, h)
	}
	return h
}

func recordFleetTransition(name, from string, to ClusterHealth) {
	switch to.State {
	case ClusterUnreachable:
		RecordAxiomAudit("fleet.cluster_unreachable", name, to.Error, "fleet")
	case ClusterDegraded:
		RecordAxiomAudit("fleet.cluster_degraded", name, to.Reason, "fleet")
	case ClusterHealthy:
		RecordAxiomAudit("fleet.cluster_recovered", name, "was "+from, "fleet")
	}
}

type probeTarget struct {
	name string
	p    Provider
}

func (sp *SwitchableProvider) targets(only string) []probeTarget {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	out := make([]probeTarget, 0, len(sp.order))
	for _, n := range sp.order {
		if only != "" && n != only {
			continue
		}
		out = append(out, probeTarget{name: n, p: sp.entries[n].provider})
	}
	return out
}

// ProbeAll probes every registered cluster (bounded concurrency, per-probe
// timeout) and waits for the results.
func (sp *SwitchableProvider) ProbeAll(ctx context.Context) {
	sp.probeTargets(ctx, sp.targets(""))
}

func (sp *SwitchableProvider) probeTargets(ctx context.Context, ts []probeTarget) {
	sem := make(chan struct{}, fleetProbeConcurrency)
	var wg sync.WaitGroup
	for _, t := range ts {
		wg.Add(1)
		sem <- struct{}{}
		go func(t probeTarget) {
			defer wg.Done()
			defer func() { <-sem }()
			sp.probe(ctx, t.name, t.p)
		}(t)
	}
	wg.Wait()
	st := sp.fleet()
	st.mu.Lock()
	st.lastRefresh = st.now()
	st.mu.Unlock()
}

// StartFleetProber probes the fleet now and then every interval until ctx is
// cancelled. An interval <= 0 disables the background probing (clusters are
// then probed only when /fleet is read).
func (sp *SwitchableProvider) StartFleetProber(ctx context.Context, interval time.Duration) {
	st := sp.fleet()
	st.mu.Lock()
	st.interval = interval
	if interval <= 0 || st.running {
		st.mu.Unlock()
		return
	}
	st.running = true
	st.mu.Unlock()
	go func() {
		sp.ProbeAll(ctx)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sp.ProbeAll(ctx)
			}
		}
	}()
}

// RefreshFleet probes everything now, unless it was just refreshed (the
// refresh button must not become a way to hammer every cluster's API server).
func (sp *SwitchableProvider) RefreshFleet(ctx context.Context) {
	st := sp.fleet()
	st.mu.Lock()
	recent := st.now().Sub(st.lastRefresh) < fleetRefreshMinGap
	st.mu.Unlock()
	if recent {
		return
	}
	sp.ProbeAll(ctx)
}

// forget drops a cluster's cached health (it was unregistered).
func (sp *SwitchableProvider) forgetHealth(name string) {
	st := sp.fleet()
	st.mu.Lock()
	delete(st.health, name)
	st.mu.Unlock()
}

// healthOf returns a cluster's cached health, or an "unknown" placeholder.
func (sp *SwitchableProvider) healthOf(name string) ClusterHealth {
	st := sp.fleet()
	st.mu.Lock()
	defer st.mu.Unlock()
	if h, ok := st.health[name]; ok {
		return h
	}
	return ClusterHealth{State: ClusterUnknown}
}

// Fleet returns every cluster with its cached health and a roll-up. Clusters
// that have never been probed are probed first, so the answer is complete even
// when the background prober is off or has not run yet.
func (sp *SwitchableProvider) Fleet(ctx context.Context) Fleet {
	var missing []probeTarget
	for _, t := range sp.targets("") {
		if sp.healthOf(t.name).State == ClusterUnknown {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		sp.probeTargets(ctx, missing)
	}
	infos := sp.Clusters(ctx)
	out := Fleet{Clusters: make([]FleetCluster, 0, len(infos))}
	var newest string
	var cpuSum, memSum float64
	var reachable int
	for _, info := range infos {
		h := sp.healthOf(info.Name)
		out.Clusters = append(out.Clusters, FleetCluster{Name: info.Name, Label: info.Label, Current: info.Current, Server: info.Server, ClusterHealth: h})
		if h.CheckedAt > newest {
			newest = h.CheckedAt
		}
		s := &out.Summary
		s.Clusters++
		switch h.State {
		case ClusterHealthy:
			s.Healthy++
		case ClusterDegraded:
			s.Degraded++
		case ClusterUnreachable:
			s.Unreachable++
		default:
			s.Unknown++
		}
		if h.State == ClusterHealthy || h.State == ClusterDegraded {
			reachable++
			s.Nodes += h.Nodes
			s.ReadyNodes += h.ReadyNodes
			s.Pods += h.Pods
			s.RunningPods += h.RunningPods
			s.Workloads += h.Workloads
			s.Warnings += h.Warnings
			cpuSum += h.CPUPercent
			memSum += h.MemoryPercent
		}
	}
	if reachable > 0 {
		out.Summary.CPUPercent = round1(cpuSum / float64(reachable))
		out.Summary.MemoryPercent = round1(memSum / float64(reachable))
	}
	out.CheckedAt = newest
	st := sp.fleet()
	st.mu.Lock()
	out.IntervalSeconds = int(st.interval / time.Second)
	st.mu.Unlock()
	return out
}

