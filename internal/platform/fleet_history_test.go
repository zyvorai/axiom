package platform

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

// histClock is a settable clock for the fleet state.
type histClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *histClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *histClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newHistFleet(t *testing.T, clusters map[string]*fakeCluster, order ...string) (*SwitchableProvider, *histClock) {
	t.Helper()
	sp := newFleet(t, clusters, order...)
	clk := &histClock{t: time.Unix(1_800_000_000, 0)}
	sp.fleet().now = clk.Now
	sp.fleet().interval = 30 * time.Second
	return sp, clk
}

func rec(t int64, state string) fleetRecord {
	return fleetRecord{FleetSample: FleetSample{T: t, State: state}}
}

func TestFleetHistoryRingCapAndRetention(t *testing.T) {
	st := &fleetState{}
	now := time.Unix(1_800_000_000, 0)
	// 3100 samples a second apart, all inside 24 h: capped at the newest 3000.
	for i := 0; i < 3100; i++ {
		st.appendHistoryLocked("a", rec(now.Unix()-3100+int64(i), ClusterHealthy), now)
	}
	if s := st.history["a"]; len(s) != fleetHistoryMaxSamples || s[0].T != now.Unix()-3000 {
		t.Fatalf("cap: %d first %d", len(s), s[0].T)
	}
	// Samples older than 24 h are pruned on the next append.
	st2 := &fleetState{}
	st2.appendHistoryLocked("b", rec(now.Add(-25*time.Hour).Unix(), ClusterHealthy), now.Add(-25*time.Hour))
	st2.appendHistoryLocked("b", rec(now.Add(-23*time.Hour).Unix(), ClusterHealthy), now.Add(-23*time.Hour))
	st2.appendHistoryLocked("b", rec(now.Unix(), ClusterHealthy), now)
	if s := st2.history["b"]; len(s) != 2 || s[0].T != now.Add(-23*time.Hour).Unix() {
		t.Fatalf("retention: %+v", s)
	}
}

func TestFleetHistoryRecordsProbesAndForgetsUnregistered(t *testing.T) {
	a, b := newFake(healthyOv()), newFake(healthyOv())
	sp, clk := newHistFleet(t, map[string]*fakeCluster{"a": a, "b": b}, "a", "b")
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		sp.ProbeAll(ctx)
		clk.Add(30 * time.Second)
	}
	// The lazy probe in Fleet() records too.
	c := newFake(healthyOv())
	sp.Register("c", "C", c, "")
	sp.Fleet(ctx)
	h, err := sp.FleetHistory(ctx, "", "1h")
	if err != nil || len(h.Clusters) != 3 || h.Clusters[0].Checks != 3 || h.Clusters[2].Checks != 1 || h.IntervalSeconds != 30 {
		t.Fatalf("history %+v %v", h, err)
	}
	s := h.Clusters[0].Samples[0]
	if s.State != ClusterHealthy || s.Nodes != 3 || s.ReadyNodes != 3 || s.Pods != 40 || s.RunningPods != 38 || s.Warnings != 2 || s.CPUPercent != 40 || s.MemoryPercent != 60 {
		t.Fatalf("sample %+v", s)
	}
	if err := sp.Unregister("b"); err != nil {
		t.Fatal(err)
	}
	if _, ok := sp.fleet().history["b"]; ok {
		t.Fatal("unregistered cluster's samples kept")
	}
	// A probe that finishes after the unregister is dropped.
	sp.probe(ctx, "b", b)
	if _, ok := sp.fleet().history["b"]; ok {
		t.Fatal("sample recorded for an unregistered cluster")
	}
	if _, err := sp.FleetHistory(ctx, "b", "1h"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("unknown cluster: %v", err)
	}
	if _, err := sp.FleetHistory(ctx, "", "2h"); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("bad window: %v", err)
	}
	one, err := sp.FleetHistory(ctx, "a", "24h")
	if err != nil || len(one.Clusters) != 1 || one.Clusters[0].Cluster != "a" || one.Clusters[0].Label != "A" || one.Window != "24h" {
		t.Fatalf("one cluster %+v %v", one, err)
	}
}

func TestFleetHistoryHysteresisRecordsHeldState(t *testing.T) {
	a := newFake(healthyOv())
	sp, clk := newHistFleet(t, map[string]*fakeCluster{"edge": a}, "edge")
	ctx := context.Background()
	sp.ProbeAll(ctx)
	clk.Add(30 * time.Second)
	a.setErr(errors.New("timeout"))
	sp.ProbeAll(ctx) // swallowed: still healthy
	clk.Add(30 * time.Second)
	h, _ := sp.FleetHistory(ctx, "edge", "1h")
	c := h.Clusters[0]
	if c.Checks != 2 || c.Samples[1].State != ClusterHealthy || c.Samples[1].Nodes != 3 || c.FailedChecks != 1 || len(c.Incidents) != 0 || *c.UptimePercent != 100 {
		t.Fatalf("held state %+v", c)
	}
	sp.ProbeAll(ctx) // second failure: unreachable
	clk.Add(30 * time.Second)
	h, _ = sp.FleetHistory(ctx, "edge", "1h")
	c = h.Clusters[0]
	if c.Samples[2].State != ClusterUnreachable || c.FailedChecks != 2 || c.CurrentState != ClusterUnreachable || len(c.Incidents) != 1 || c.Incidents[0].To != nil || c.Incidents[0].Reason == "" {
		t.Fatalf("outage %+v", c)
	}
	if c.Incidents[0].DurationSeconds != 30 {
		t.Fatalf("ongoing duration %+v", c.Incidents[0])
	}
}

func TestDownsampleFleet(t *testing.T) {
	// 600 samples over 600 s into 240 buckets of 2.5 s.
	var recs []fleetRecord
	for i := 0; i < 600; i++ {
		r := rec(int64(i), ClusterHealthy)
		r.LatencyMs = int64(i % 2 * 100) // 0,100,0,100...
		r.CPUPercent = float64(i % 2 * 10)
		r.Pods = i
		recs = append(recs, r)
	}
	recs[301].State = ClusterDegraded
	recs[302].State = ClusterUnreachable
	out := downsampleFleet(recs, 0, 599, 240)
	if len(out) > 240 || len(out) < 200 {
		t.Fatalf("points: %d", len(out))
	}
	var worst string
	for _, p := range out {
		if p.T >= 300 && p.T <= 304 && fleetStateRank(p.State) > fleetStateRank(worst) {
			worst = p.State
		}
		if p.State == ClusterDegraded {
			t.Fatalf("degraded must lose to unreachable in the same bucket: %+v", p)
		}
	}
	if worst != ClusterUnreachable {
		t.Fatalf("worst state lost: %q", worst)
	}
	first := out[0] // samples 0,1,2 -> latency avg 33, cpu 3.3, last pods 2
	if first.LatencyMs != 33 || first.CPUPercent != 3.3 || first.Pods != 2 || first.T != 2 {
		t.Fatalf("first bucket %+v", first)
	}
	// Small series are returned untouched.
	if s := downsampleFleet(recs[:10], 0, 599, 240); len(s) != 10 || s[9].Pods != 9 {
		t.Fatalf("small %+v", s)
	}
	if s := downsampleFleet(nil, 0, 10, 240); s == nil || len(s) != 0 {
		t.Fatal("empty series must be [] not null")
	}
}

func TestFleetUptime(t *testing.T) {
	iv := 30 * time.Second
	if fleetUptime(nil, iv) != nil {
		t.Fatal("no samples: null")
	}
	if u := fleetUptime([]fleetRecord{rec(0, ClusterHealthy)}, iv); *u != 100 {
		t.Fatalf("single healthy %v", *u)
	}
	if u := fleetUptime([]fleetRecord{rec(0, ClusterUnreachable)}, iv); *u != 0 {
		t.Fatalf("single down %v", *u)
	}
	// healthy 0..30, degraded (reachable) 30..60, unreachable 60..90, healthy last (30).
	u := fleetUptime([]fleetRecord{rec(0, ClusterHealthy), rec(30, ClusterDegraded), rec(60, ClusterUnreachable), rec(90, ClusterHealthy)}, iv)
	if *u != 75 {
		t.Fatalf("uptime %v", *u)
	}
	// A long gap (prober stopped) counts at most 2 intervals: healthy 60 of 60+30.
	u = fleetUptime([]fleetRecord{rec(0, ClusterHealthy), rec(3600, ClusterUnreachable)}, iv)
	if *u != 66.67 {
		t.Fatalf("capped gap %v", *u)
	}
	// Equal timestamps: fall back to the count for all but the last.
	u = fleetUptime([]fleetRecord{rec(5, ClusterUnreachable), rec(5, ClusterHealthy)}, iv)
	if *u != 100 {
		t.Fatalf("same second %v", *u)
	}
}

func TestFleetIncidents(t *testing.T) {
	iv := 30 * time.Second
	now := time.Unix(1000, 0)
	recs := []fleetRecord{
		rec(0, ClusterHealthy),
		rec(30, ClusterDegraded), rec(60, ClusterUnreachable), rec(90, ClusterDegraded),
		rec(120, ClusterHealthy), // gap to the next run is 30 s < 60 s: merged
		rec(150, ClusterDegraded),
		rec(180, ClusterHealthy), rec(210, ClusterHealthy), rec(240, ClusterHealthy),
		rec(270, ClusterDegraded), // separate, ongoing
	}
	recs[2].reason = "connection refused"
	recs[9].reason = "1 of 3 nodes are not ready"
	inc := fleetIncidents(recs, iv, now)
	if len(inc) != 2 {
		t.Fatalf("incidents %+v", inc)
	}
	if inc[0].State != ClusterUnreachable || inc[0].From != time.Unix(30, 0).UTC().Format(time.RFC3339) || inc[0].To == nil ||
		*inc[0].To != time.Unix(180, 0).UTC().Format(time.RFC3339) || inc[0].DurationSeconds != 150 || inc[0].Reason != "connection refused" {
		t.Fatalf("merged %+v", inc[0])
	}
	if inc[1].State != ClusterDegraded || inc[1].To != nil || inc[1].DurationSeconds != 730 || inc[1].Reason != "1 of 3 nodes are not ready" {
		t.Fatalf("ongoing %+v", inc[1])
	}
	if got := fleetIncidents([]fleetRecord{rec(0, ClusterHealthy)}, iv, now); got == nil || len(got) != 0 {
		t.Fatalf("no incidents must be []: %+v", got)
	}
}

func TestFleetHistoryWindowFilterAndDownsample(t *testing.T) {
	a := newFake(healthyOv())
	sp, clk := newHistFleet(t, map[string]*fakeCluster{"a": a}, "a")
	ctx := context.Background()
	// 2 h of probes every 10 s = 720 samples.
	for i := 0; i < 720; i++ {
		sp.probe(ctx, "a", a)
		clk.Add(10 * time.Second)
	}
	h1, _ := sp.FleetHistory(ctx, "a", "1h")
	h24, _ := sp.FleetHistory(ctx, "a", "24h")
	if h1.Clusters[0].Checks != 360 || h24.Clusters[0].Checks != 720 {
		t.Fatalf("checks %d %d", h1.Clusters[0].Checks, h24.Clusters[0].Checks)
	}
	if n := len(h1.Clusters[0].Samples); n > 240 || n == 0 {
		t.Fatalf("1h points %d", n)
	}
	if n := len(h24.Clusters[0].Samples); n > 240 || n == 0 {
		t.Fatalf("24h points %d", n)
	}
	if *h24.Clusters[0].UptimePercent != 100 || h24.Clusters[0].CurrentState != ClusterHealthy || h24.Clusters[0].Since == "" {
		t.Fatalf("summary %+v", h24.Clusters[0])
	}
}

func TestNormalizeFleetHistoryInputs(t *testing.T) {
	for in, want := range map[string]string{"": "24h", "1h": "1h", "6h": "6h", " 24h ": "24h"} {
		if got, err := NormalizeFleetHistoryWindow(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"2h", "1d", "24", "60m"} {
		if _, err := NormalizeFleetHistoryWindow(bad); HTTPStatus(err) != http.StatusBadRequest {
			t.Errorf("%q accepted", bad)
		}
	}
	for _, bad := range []string{"", "a\nb", "x\x00", string(make([]byte, 254))} {
		if err := ValidateFleetClusterName(bad); HTTPStatus(err) != http.StatusBadRequest {
			t.Errorf("%q accepted", bad)
		}
	}
	if err := ValidateFleetClusterName("arn:aws:eks:eu-west-1:123:cluster/prod"); err != nil {
		t.Errorf("context name refused: %v", err)
	}
}

func TestSeedFleetHistoryForDemo(t *testing.T) {
	t.Setenv("AXIOM_DEMO_FLEET_HISTORY_SEED", "")
	if sp := NewDemoSwitchableProvider(); len(sp.fleet().history) != 0 {
		t.Fatal("seeded without the switch")
	}
	t.Setenv("AXIOM_DEMO_FLEET_HISTORY_SEED", "true")
	t.Setenv("AXIOM_DEMO_FLEET_DOWN", "")
	sp := NewDemoSwitchableProvider()
	h, err := sp.FleetHistory(context.Background(), "", "24h")
	if err != nil || len(h.Clusters) != 3 {
		t.Fatalf("%+v %v", h, err)
	}
	for _, c := range h.Clusters {
		if c.Checks < 2800 || len(c.Samples) > 240 || c.Samples[0].Nodes == 0 {
			t.Fatalf("%s: %d checks, %d points", c.Cluster, c.Checks, len(c.Samples))
		}
		switch c.Cluster {
		case "ap-south-1":
			if len(c.Incidents) != 1 || c.Incidents[0].State != ClusterDegraded || c.Incidents[0].To == nil ||
				c.Incidents[0].DurationSeconds < 19*60 || c.Incidents[0].DurationSeconds > 21*60 || *c.UptimePercent != 100 {
				t.Fatalf("ap-south-1 %+v", c.Incidents)
			}
		default:
			if len(c.Incidents) != 0 {
				t.Fatalf("%s incidents %+v", c.Cluster, c.Incidents)
			}
		}
	}
	t.Setenv("AXIOM_DEMO_FLEET_DOWN", "us-east-1")
	sp = NewDemoSwitchableProvider()
	h, _ = sp.FleetHistory(context.Background(), "us-east-1", "24h")
	c := h.Clusters[0]
	if len(c.Incidents) != 1 || c.Incidents[0].State != ClusterUnreachable || c.FailedChecks < 30 || *c.UptimePercent >= 100 {
		t.Fatalf("down cluster %+v uptime %v", c.Incidents, *c.UptimePercent)
	}
	// Live probes continue the seeded series.
	sp.ProbeAll(context.Background())
	h, _ = sp.FleetHistory(context.Background(), "us-east-1", "24h")
	if h.Clusters[0].Checks != c.Checks+1 {
		t.Fatalf("live probe not appended: %d vs %d", h.Clusters[0].Checks, c.Checks)
	}
}

func TestFleetHistoryConcurrentProbesAndReads(t *testing.T) {
	a, b := newFake(healthyOv()), newFake(healthyOv())
	sp, clk := newHistFleet(t, map[string]*fakeCluster{"a": a, "b": b}, "a", "b")
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				sp.ProbeAll(ctx)
				clk.Add(time.Second)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				if _, err := sp.FleetHistory(ctx, "", "1h"); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		sp.Register("c", "C", newFake(healthyOv()), "")
		_ = sp.Unregister("c")
	}()
	wg.Wait()
	h, _ := sp.FleetHistory(ctx, "a", "1h")
	if h.Clusters[0].Checks != 100 {
		t.Fatalf("checks %d", h.Clusters[0].Checks)
	}
}
