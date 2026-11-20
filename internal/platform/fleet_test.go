package platform

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeCluster is a Provider whose Overview is scripted.
type fakeCluster struct {
	Provider
	ov    atomic.Value // Overview
	err   atomic.Value // error holder
	delay time.Duration
	calls atomic.Int32
}

type errBox struct{ err error }

func newFake(ov Overview) *fakeCluster {
	f := &fakeCluster{}
	f.ov.Store(ov)
	f.err.Store(errBox{})
	return f
}

func (f *fakeCluster) setErr(err error)    { f.err.Store(errBox{err}) }
func (f *fakeCluster) setOv(ov Overview) { f.ov.Store(ov) }

func (f *fakeCluster) Overview(ctx context.Context) (Overview, error) {
	f.calls.Add(1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return Overview{}, ctx.Err()
		}
	}
	if e := f.err.Load().(errBox).err; e != nil {
		return Overview{}, e
	}
	return f.ov.Load().(Overview), nil
}

func healthyOv() Overview {
	return Overview{Health: "Healthy", Nodes: 3, ReadyNodes: 3, Pods: 40, RunningPods: 38, Workloads: 9, Warnings: 2, CPUPercent: 40, MemoryPercent: 60}
}

func TestClassifyProbe(t *testing.T) {
	cases := []struct {
		name   string
		ov     Overview
		err    error
		state  string
		reason string
	}{
		{"healthy", healthyOv(), nil, ClusterHealthy, ""},
		{"empty health string is healthy", Overview{Nodes: 1, ReadyNodes: 1}, nil, ClusterHealthy, ""},
		{"a node not ready", Overview{Health: "Healthy", Nodes: 3, ReadyNodes: 2}, nil, ClusterDegraded, "1 of 3 nodes are not ready"},
		{"no ready nodes", Overview{Health: "Healthy", Nodes: 2, ReadyNodes: 0}, nil, ClusterDegraded, "2 of 2 nodes are not ready"},
		{"reports a problem", Overview{Health: "Degraded", Nodes: 1, ReadyNodes: 1}, nil, ClusterDegraded, "the cluster reports Degraded"},
		{"error", Overview{}, errors.New("dial tcp 10.0.0.5:6443:\n\tconnect: connection refused"), ClusterUnreachable, ""},
	}
	for _, c := range cases {
		h := classifyProbe(c.ov, c.err)
		if h.State != c.state || h.Reason != c.reason {
			t.Errorf("%s: got state=%q reason=%q, want %q %q", c.name, h.State, h.Reason, c.state, c.reason)
		}
		if c.err != nil && (strings.ContainsAny(h.Error, "\n\t") || h.Error == "") {
			t.Errorf("%s: error not flattened: %q", c.name, h.Error)
		}
	}
	long := classifyProbe(Overview{}, errors.New(strings.Repeat("x", 1000)))
	if len(long.Error) > 250 {
		t.Errorf("error not truncated: %d bytes", len(long.Error))
	}
}

func newFleet(t *testing.T, clusters map[string]*fakeCluster, order ...string) *SwitchableProvider {
	t.Helper()
	sp := NewSwitchableProvider()
	for _, n := range order {
		sp.Register(n, strings.ToUpper(n), clusters[n], "https://"+n+".example:6443")
	}
	return sp
}

func TestFleetSummaryAndLazyProbe(t *testing.T) {
	a, b, c := newFake(healthyOv()), newFake(Overview{Health: "Healthy", Nodes: 2, ReadyNodes: 1, Pods: 10, RunningPods: 9, Workloads: 3, Warnings: 1, CPUPercent: 20, MemoryPercent: 30}), newFake(healthyOv())
	c.setErr(errors.New("connection refused"))
	sp := newFleet(t, map[string]*fakeCluster{"a": a, "b": b, "c": c}, "a", "b", "c")

	// Nothing has probed yet; reading the fleet probes what is missing.
	f := sp.Fleet(context.Background())
	if a.calls.Load() != 1 || b.calls.Load() != 1 || c.calls.Load() != 1 {
		t.Fatalf("expected one lazy probe each, got %d %d %d", a.calls.Load(), b.calls.Load(), c.calls.Load())
	}
	s := f.Summary
	if s.Clusters != 3 || s.Healthy != 1 || s.Degraded != 1 || s.Unreachable != 1 || s.Unknown != 0 {
		t.Fatalf("bad counts: %+v", s)
	}
	// The unreachable cluster contributes nothing to the totals.
	if s.Nodes != 5 || s.ReadyNodes != 4 || s.Pods != 50 || s.RunningPods != 47 || s.Workloads != 12 || s.Warnings != 3 {
		t.Fatalf("bad totals: %+v", s)
	}
	if s.CPUPercent != 30 || s.MemoryPercent != 45 {
		t.Fatalf("utilisation should average the reachable clusters: %+v", s)
	}
	if f.Clusters[2].State != ClusterUnreachable || f.Clusters[2].Error == "" || f.Clusters[1].Reason == "" {
		t.Fatalf("per-cluster detail missing: %+v %+v", f.Clusters[2], f.Clusters[1])
	}
	// A second read serves the cache and does not probe again.
	sp.Fleet(context.Background())
	if a.calls.Load() != 1 {
		t.Fatalf("reading the fleet re-probed: %d", a.calls.Load())
	}
	// The cluster list reflects it too (Healthy is false only for unreachable).
	healthy := map[string]bool{}
	for _, ci := range sp.Clusters(context.Background()) {
		healthy[ci.Name] = ci.Healthy
	}
	if !healthy["a"] || !healthy["b"] || healthy["c"] {
		t.Fatalf("ClusterInfo.Healthy: %+v", healthy)
	}
}

func TestOneDeadClusterDoesNotStallTheOthers(t *testing.T) {
	old := fleetProbeTimeout
	fleetProbeTimeout = 60 * time.Millisecond
	defer func() { fleetProbeTimeout = old }()

	slow := newFake(healthyOv())
	slow.delay = 5 * time.Second // never answers within the timeout
	fast := newFake(healthyOv())
	sp := newFleet(t, map[string]*fakeCluster{"slow": slow, "fast": fast}, "slow", "fast")

	start := time.Now()
	f := sp.Fleet(context.Background())
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("a hung cluster stalled the probe for %v", took)
	}
	if f.Clusters[0].State != ClusterUnreachable || !strings.Contains(f.Clusters[0].Error, "deadline") {
		t.Fatalf("timed-out cluster: %+v", f.Clusters[0])
	}
	if f.Clusters[1].State != ClusterHealthy {
		t.Fatalf("healthy cluster: %+v", f.Clusters[1])
	}
}

func TestStateTransitionsAreTrackedAndAudited(t *testing.T) {
	a := newFake(healthyOv())
	sp := newFleet(t, map[string]*fakeCluster{"edge": a}, "edge")
	ctx := context.Background()
	sp.ProbeAll(ctx)
	first := sp.healthOf("edge")
	if first.State != ClusterHealthy || first.Since == "" || first.CheckedAt == "" {
		t.Fatalf("first probe: %+v", first)
	}
	time.Sleep(1100 * time.Millisecond) // RFC3339 has one-second resolution
	sp.ProbeAll(ctx)
	if again := sp.healthOf("edge"); again.Since != first.Since {
		t.Fatalf("Since moved without a state change: %q -> %q", first.Since, again.Since)
	}

	a.setErr(errors.New("connection refused"))
	sp.ProbeAll(ctx)
	sp.ProbeAll(ctx) // a second failed probe confirms the outage
	down := sp.healthOf("edge")
	if down.State != ClusterUnreachable || down.Since == "" {
		t.Fatalf("not unreachable: %+v", down)
	}
	a.setErr(nil)
	a.setOv(Overview{Health: "Healthy", Nodes: 2, ReadyNodes: 1})
	sp.ProbeAll(ctx)
	if sp.healthOf("edge").State != ClusterDegraded {
		t.Fatalf("not degraded: %+v", sp.healthOf("edge"))
	}
	a.setOv(healthyOv())
	sp.ProbeAll(ctx)
	if sp.healthOf("edge").State != ClusterHealthy {
		t.Fatalf("not recovered")
	}
	// The audit list is newest first: take the three latest and put them in order.
	latest := ListAxiomAudit()[:3]
	var actions []string
	for i := len(latest) - 1; i >= 0; i-- {
		actions = append(actions, latest[i].Action)
	}
	want := []string{"fleet.cluster_unreachable", "fleet.cluster_degraded", "fleet.cluster_recovered"}
	if strings.Join(actions, ",") != strings.Join(want, ",") {
		t.Fatalf("audit trail: got %v, want %v", actions, want)
	}
}

func TestRefreshIsThrottledAndUnregisterForgets(t *testing.T) {
	a, b := newFake(healthyOv()), newFake(healthyOv())
	sp := newFleet(t, map[string]*fakeCluster{"a": a, "b": b}, "a", "b")
	ctx := context.Background()
	sp.RefreshFleet(ctx)
	sp.RefreshFleet(ctx) // within the minimum gap: no second probe
	if a.calls.Load() != 1 {
		t.Fatalf("refresh was not throttled: %d probes", a.calls.Load())
	}
	if err := sp.Unregister("b"); err != nil {
		t.Fatal(err)
	}
	if h := sp.healthOf("b"); h.State != ClusterUnknown {
		t.Fatalf("unregistered cluster kept its health: %+v", h)
	}
	if f := sp.Fleet(ctx); f.Summary.Clusters != 1 {
		t.Fatalf("fleet still lists it: %+v", f.Summary)
	}
}

func TestBackgroundProberRunsAndStops(t *testing.T) {
	a := newFake(healthyOv())
	sp := newFleet(t, map[string]*fakeCluster{"a": a}, "a")
	ctx, cancel := context.WithCancel(context.Background())
	sp.StartFleetProber(ctx, 20*time.Millisecond)
	sp.StartFleetProber(ctx, 20*time.Millisecond) // a second start must not double the probing
	deadline := time.Now().Add(3 * time.Second)
	for a.calls.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if a.calls.Load() < 4 {
		t.Fatalf("prober did not keep probing: %d", a.calls.Load())
	}
	cancel()
	time.Sleep(60 * time.Millisecond)
	n := a.calls.Load()
	time.Sleep(120 * time.Millisecond)
	if a.calls.Load() != n {
		t.Fatalf("prober kept running after cancel")
	}

	// A cluster registered while the prober runs is probed straight away.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	sp2 := newFleet(t, map[string]*fakeCluster{"a": newFake(healthyOv())}, "a")
	sp2.StartFleetProber(ctx2, time.Hour)
	late := newFake(healthyOv())
	sp2.Register("late", "Late", late, "")
	for i := 0; i < 100 && late.calls.Load() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if late.calls.Load() == 0 {
		t.Fatal("a cluster registered at runtime was not probed")
	}
}

func TestDemoFleetSimulation(t *testing.T) {
	t.Setenv("AXIOM_DEMO_FLEET_DOWN", "ap-south-1")
	t.Setenv("AXIOM_DEMO_FLEET_DEGRADED", "us-east-1")
	f := NewDemoSwitchableProvider().Fleet(context.Background())
	got := map[string]string{}
	for _, c := range f.Clusters {
		got[c.Name] = c.State
	}
	if got["eu-west-1"] != ClusterHealthy || got["us-east-1"] != ClusterDegraded || got["ap-south-1"] != ClusterUnreachable {
		t.Fatalf("simulated states: %+v", got)
	}
	if f.Summary.Unreachable != 1 || f.Summary.Degraded != 1 || f.Summary.Healthy != 1 {
		t.Fatalf("summary: %+v", f.Summary)
	}
	// Without the switches the demo fleet is all healthy, as before.
	t.Setenv("AXIOM_DEMO_FLEET_DOWN", "")
	t.Setenv("AXIOM_DEMO_FLEET_DEGRADED", "")
	if s := NewDemoSwitchableProvider().Fleet(context.Background()).Summary; s.Healthy != 3 {
		t.Fatalf("default demo fleet: %+v", s)
	}
}
