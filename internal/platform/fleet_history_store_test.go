package platform

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureFleetHistoryLog swaps the persistence logger for the test.
func captureFleetHistoryLog(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var lines []string
	prev := fleetHistoryLogf
	fleetHistoryLogf = func(format string, args ...any) {
		mu.Lock()
		lines = append(lines, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	t.Cleanup(func() { fleetHistoryLogf = prev })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), lines...)
	}
}

// persistentMemStore is a durable-looking store over shared memory blobs, so
// several "replicas" can share one object like they share the ConfigMap.
func persistentMemStore(blobs *memoryFleetHistoryBlobs, replica string, clk *histClock) *keyedFleetHistoryStore {
	s := newKeyedFleetHistoryStore(blobs, replica, true)
	if clk != nil {
		s.now = clk.Now
	}
	return s
}

func bkt(t int64, state string, n, f int) FleetHistoryBucket {
	return FleetHistoryBucket{FleetSample: FleetSample{T: t, State: state, Nodes: 3, ReadyNodes: 3, LatencyMs: 40}, Checks: n, Failed: f}
}

// probeFor runs probes every step for d on the clock.
func probeFor(sp *SwitchableProvider, clk *histClock, d, step time.Duration) {
	for el := time.Duration(0); el < d; el += step {
		sp.ProbeAll(context.Background())
		clk.Add(step)
	}
}

func TestFleetHistoryPersistRoundTrip(t *testing.T) {
	a, b := newFake(healthyOv()), newFake(healthyOv())
	sp, clk := newHistFleet(t, map[string]*fakeCluster{"a": a, "b": b}, "a", "b")
	probeFor(sp, clk, 2*time.Hour, 30*time.Second)
	b.setErr(errors.New("connection refused"))
	probeFor(sp, clk, 20*time.Minute, 30*time.Second)
	b.setErr(nil)
	b.setOv(Overview{Health: "Healthy", Nodes: 3, ReadyNodes: 2, Pods: 10, RunningPods: 9})
	probeFor(sp, clk, 30*time.Minute, 30*time.Second)

	store := NewMemoryFleetHistoryStore("pod-a")
	if err := sp.saveFleetHistory(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	snap1 := sp.fleetHistorySnapshot()
	for name, bs := range snap1.Clusters {
		if len(bs) > fleetHistoryPersistPoints || len(bs) < 30 {
			t.Fatalf("%s: %d buckets", name, len(bs))
		}
	}
	var failed, checks int
	var reasons []string
	for _, x := range snap1.Clusters["b"] {
		failed += x.Failed
		checks += x.Checks
		if x.Reason != "" {
			reasons = append(reasons, x.Reason)
		}
	}
	before, _ := sp.FleetHistory(context.Background(), "b", "24h")
	if checks != before.Clusters[0].Checks || failed != before.Clusters[0].FailedChecks || failed < 39 || len(reasons) == 0 {
		t.Fatalf("totals: %d/%d vs %+v; reasons %v", failed, checks, before.Clusters[0], reasons)
	}

	// A new process with the same clusters restores the identical buckets.
	sp2 := newFleet(t, map[string]*fakeCluster{"a": a, "b": b}, "a", "b")
	sp2.fleet().now = clk.Now
	sp2.fleet().interval = 30 * time.Second
	n, err := sp2.RestoreFleetHistory(context.Background(), store)
	if err != nil || n == 0 {
		t.Fatalf("restore %d %v", n, err)
	}
	for _, r := range sp2.fleet().history["a"] {
		if !r.Restored {
			t.Fatalf("not marked restored: %+v", r)
		}
	}
	snap2 := sp2.fleetHistorySnapshot()
	if !reflect.DeepEqual(snap1.Clusters, snap2.Clusters) {
		t.Fatalf("round trip differs:\n%+v\n%+v", snap1.Clusters["b"][:3], snap2.Clusters["b"][:3])
	}
	after, _ := sp2.FleetHistory(context.Background(), "b", "24h")
	c := after.Clusters[0]
	if c.Checks != checks || c.FailedChecks != failed || len(c.Incidents) == 0 || c.Incidents[0].Reason == "" || after.RestoredFrom == nil {
		t.Fatalf("restored history %+v from %v", c, after.RestoredFrom)
	}
	if after.Durable {
		t.Fatal("an in-memory store is never durable")
	}
}

func TestFleetHistorySizeCap(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	now := time.Unix(1_800_000_000, 0)
	from, _ := fleetPersistWindow(now)
	snap := FleetHistorySnapshot{Version: 1, SavedAt: now.Unix(), BucketSeconds: 300, Clusters: map[string][]FleetHistoryBucket{}}
	for c := 0; c < 20; c++ {
		var bs []FleetHistoryBucket
		for i := 0; i < fleetHistoryPersistPoints; i++ {
			b := bkt(from+int64(i)*300+int64(rng.IntN(300)), ClusterHealthy, 10, rng.IntN(2))
			b.LatencyMs = int64(rng.IntN(5000))
			b.CPUPercent = round1(rng.Float64() * 100)
			b.MemoryPercent = round1(rng.Float64() * 100)
			b.Pods, b.RunningPods, b.Warnings = rng.IntN(5000), rng.IntN(5000), rng.IntN(500)
			if rng.IntN(10) == 0 {
				b.State, b.Reason = ClusterDegraded, fmt.Sprintf("%d of 3 nodes are not ready", rng.IntN(3))
			}
			bs = append(bs, b)
		}
		snap.Clusters[fmt.Sprintf("arn:aws:eks:eu-west-1:123456789012:cluster/prod-%02d", c)] = bs
	}
	enc, dropped, err := encodeFleetSnapshot(snap, fleetHistoryMaxObjectBytes)
	if err != nil || len(enc) > fleetHistoryMaxObjectBytes {
		t.Fatalf("20 x 288: %d bytes, %v", len(enc), err)
	}
	t.Logf("20 clusters x 288 points: %d bytes encoded, %d dropped", len(enc), dropped)

	// Under a tight budget the oldest buckets of the largest cluster go first.
	small := FleetHistorySnapshot{Version: 1, Clusters: map[string][]FleetHistoryBucket{
		"big": snap.Clusters["arn:aws:eks:eu-west-1:123456789012:cluster/prod-00"],
		"sml": snap.Clusters["arn:aws:eks:eu-west-1:123456789012:cluster/prod-01"][200:],
	}}
	full, _ := encodeFleetSnapshotRaw(small)
	budget := len(full) * 3 / 4
	enc, dropped, err = encodeFleetSnapshot(small, budget)
	if err != nil || len(enc) > budget || dropped == 0 {
		t.Fatalf("trim: %d > %d, dropped %d, %v", len(enc), budget, dropped, err)
	}
	got, err := decodeFleetSnapshot(enc)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Clusters["sml"]) != 88 || len(got.Clusters["big"]) != 288-dropped ||
		got.Clusters["big"][0].T != small.Clusters["big"][dropped].T || got.Clusters["big"][len(got.Clusters["big"])-1].T != small.Clusters["big"][287].T {
		t.Fatalf("trimmed wrong: big %d sml %d dropped %d", len(got.Clusters["big"]), len(got.Clusters["sml"]), dropped)
	}
	if len(small.Clusters["big"]) != 288 {
		t.Fatal("the caller's snapshot was modified")
	}

	// The store counts the other replicas' keys and logs the trim once.
	logs := captureFleetHistoryLog(t)
	blobs := &memoryFleetHistoryBlobs{data: map[string]string{"history-other": strings.Repeat("x", 1000)}}
	s := persistentMemStore(blobs, "me", nil)
	s.now = func() time.Time { return now }
	s.limit = budget + 1000 + len("history-other") + len("history-me")
	for i := 0; i < 2; i++ {
		if err := s.Save(context.Background(), small); err != nil {
			t.Fatal(err)
		}
	}
	if total := len(blobs.data["history-me"]) + len(blobs.data["history-other"]) + len("history-me") + len("history-other"); total > s.limit {
		t.Fatalf("object %d > %d", total, s.limit)
	}
	if l := logs(); len(l) != 1 || !strings.Contains(l[0], "dropped") {
		t.Fatalf("trim log %v", l)
	}
	s.limit = 1000
	if err := s.Save(context.Background(), small); err == nil {
		t.Fatal("no room must fail")
	}
}

func TestFleetHistoryMultiReplicaMergeAndStaleKeys(t *testing.T) {
	a, b := newFake(healthyOv()), newFake(healthyOv())
	sp, clk := newHistFleet(t, map[string]*fakeCluster{"a": a, "b": b}, "a", "b")
	now := clk.Now().Unix()
	blobs := &memoryFleetHistoryBlobs{}
	ra, rb, rc, rd := persistentMemStore(blobs, "pod-a", clk), persistentMemStore(blobs, "pod-b", clk), persistentMemStore(blobs, "pod-c", clk), persistentMemStore(blobs, "pod/d", clk)
	save := func(s *keyedFleetHistoryStore, savedAt int64, clusters map[string][]FleetHistoryBucket) {
		t.Helper()
		if err := s.Save(context.Background(), FleetHistorySnapshot{Version: 1, SavedAt: savedAt, BucketSeconds: 300, Clusters: clusters}); err != nil {
			t.Fatal(err)
		}
	}
	old := now - int64(25*time.Hour/time.Second)
	save(rc, old, map[string][]FleetHistoryBucket{"a": {bkt(old, ClusterHealthy, 10, 0)}})
	save(rd, now-600, map[string][]FleetHistoryBucket{"a": {bkt(now-int64(23*time.Hour/time.Second), ClusterHealthy, 10, 0)}})
	save(ra, now-60, map[string][]FleetHistoryBucket{
		"a":    {bkt(now-900, ClusterHealthy, 10, 0), bkt(now-600, ClusterHealthy, 10, 0)},
		"gone": {bkt(now-600, ClusterHealthy, 10, 0)},
	})
	save(rb, now-120, map[string][]FleetHistoryBucket{
		"a": {bkt(now-600, ClusterDegraded, 10, 0), bkt(now-300, ClusterHealthy, 10, 0)},
		"b": {bkt(now-300, ClusterUnreachable, 10, 10)},
	})
	// pod-c's newest sample is older than 24 h: its key was dropped by the
	// next save; pod/d (23 h) is kept under a sanitised key.
	if _, ok := blobs.data["history-pod-c"]; ok {
		t.Fatal("stale replica key kept")
	}
	if _, ok := blobs.data["history-pod-d"]; !ok || len(blobs.data) != 3 {
		t.Fatalf("keys %v", len(blobs.data))
	}
	n, err := sp.RestoreFleetHistory(context.Background(), ra)
	if err != nil {
		t.Fatal(err)
	}
	h := sp.fleet().history
	if _, ok := h["gone"]; ok {
		t.Fatal("unregistered cluster restored")
	}
	ta := h["a"]
	if n != 5 || len(ta) != 4 || ta[1].T != now-900 || ta[2].T != now-600 || ta[2].State != ClusterDegraded || ta[3].T != now-300 {
		t.Fatalf("merge: n=%d %+v", n, ta)
	}
	if len(h["b"]) != 1 || h["b"][0].nFailed != 10 {
		t.Fatalf("b %+v", h["b"])
	}
	if got := sp.fleet().restoredFrom.Unix(); got != now-60 {
		t.Fatalf("restoredFrom %d", got)
	}
	// Restoring twice does not duplicate samples.
	if _, err := sp.RestoreFleetHistory(context.Background(), ra); err != nil || len(sp.fleet().history["a"]) != 4 {
		t.Fatalf("second restore: %d %v", len(sp.fleet().history["a"]), err)
	}
	// An unreadable key is skipped, not fatal.
	logs := captureFleetHistoryLog(t)
	blobs.data["history-junk"] = "not base64!"
	if snaps, err := ra.Load(context.Background()); err != nil || len(snaps) != 3 || len(logs()) != 1 {
		t.Fatalf("junk key: %d %v %v", len(snaps), err, logs())
	}
}

func TestFleetHistoryRestartGap(t *testing.T) {
	a := newFake(healthyOv())
	sp, clk := newHistFleet(t, map[string]*fakeCluster{"a": a}, "a")
	now := clk.Now().Unix()
	// 2 h of healthy buckets ending 10 minutes ago, then a 10-minute restart.
	var bs []FleetHistoryBucket
	for i := 24; i >= 2; i-- {
		bs = append(bs, bkt(now-int64(i)*300, ClusterHealthy, 10, 0))
	}
	store := persistentMemStore(&memoryFleetHistoryBlobs{}, "pod-a", clk)
	if err := store.Save(context.Background(), FleetHistorySnapshot{Version: 1, SavedAt: now - 600, Clusters: map[string][]FleetHistoryBucket{"a": bs}}); err != nil {
		t.Fatal(err)
	}
	sp.StartFleetHistoryPersistence(t.Context(), store, time.Hour)
	probeFor(sp, clk, 5*time.Minute, 30*time.Second)
	h, _ := sp.FleetHistory(context.Background(), "a", "24h")
	c := h.Clusters[0]
	if *c.UptimePercent != 100 || len(c.Incidents) != 0 || c.Checks != 23*10+10 {
		t.Fatalf("healthy restart: %+v", c)
	}
	if len(c.Gaps) != 1 || c.Gaps[0].From != time.Unix(now-600, 0).UTC().Format(time.RFC3339) || c.Gaps[0].To != time.Unix(now, 0).UTC().Format(time.RFC3339) {
		t.Fatalf("gap %+v", c.Gaps)
	}
	var restored int
	for _, s := range c.Samples {
		if s.Restored {
			restored++
		}
	}
	if restored != 23 || h.RestoredFrom == nil {
		t.Fatalf("restored samples %d of %d", restored, len(c.Samples))
	}

	// A cluster that was down when the console stopped and is still down
	// after: two incidents, split at the gap; the gap is not downtime.
	iv := 30 * time.Second
	r := func(t int64, s string, restored bool) fleetRecord {
		x := rec(t, s)
		x.Restored = restored
		if restored {
			x.n = 10
		}
		return x
	}
	recs := []fleetRecord{
		r(0, ClusterHealthy, true), r(300, ClusterUnreachable, true), r(600, ClusterUnreachable, true),
		// restart: 15 minutes without samples
		r(1500, ClusterUnreachable, false), r(1530, ClusterHealthy, false),
	}
	inc := fleetIncidents(recs, iv, time.Unix(2000, 0))
	if len(inc) != 2 || *inc[0].To != time.Unix(600, 0).UTC().Format(time.RFC3339) || inc[0].DurationSeconds != 300 ||
		inc[1].From != time.Unix(1500, 0).UTC().Format(time.RFC3339) || inc[1].DurationSeconds != 30 {
		t.Fatalf("incidents across the gap %+v", inc)
	}
	// Weights: 300 up, 300 down, 30 down (the gap counts one interval, not
	// 900 s), 30 down, 30 up => 330 of 690 up.
	if u := fleetUptime(recs, iv); *u != round2(330.0/690*100) {
		t.Fatalf("uptime across the gap %v", *u)
	}
	// Healthy before and after a long restart: no incident at all.
	recs = []fleetRecord{r(0, ClusterHealthy, true), r(300, ClusterHealthy, true), r(4000, ClusterHealthy, false)}
	if inc := fleetIncidents(recs, iv, time.Unix(4100, 0)); len(inc) != 0 {
		t.Fatalf("gap made an incident %+v", inc)
	}
	if u := fleetUptime(recs, iv); *u != 100 {
		t.Fatalf("gap counted %v", *u)
	}
}

// failingBlobs fails (or panics) on every write.
type failingBlobs struct {
	memoryFleetHistoryBlobs
	panics bool
}

func (f *failingBlobs) update(context.Context, func(map[string]string) error) error {
	if f.panics {
		panic("boom")
	}
	return errors.New("configmaps is forbidden")
}

func TestFleetHistorySaveFailureIsRateLimited(t *testing.T) {
	logs := captureFleetHistoryLog(t)
	a := newFake(healthyOv())
	sp, clk := newHistFleet(t, map[string]*fakeCluster{"a": a}, "a")
	sp.ProbeAll(context.Background())
	store := newKeyedFleetHistoryStore(&failingBlobs{}, "pod-a", true)
	sp.fleet().persistent, sp.fleet().persistInterval = true, time.Minute
	for i := 0; i < 5; i++ {
		if err := sp.saveFleetHistory(context.Background(), store); err == nil {
			t.Fatal("expected an error")
		}
		clk.Add(time.Minute)
	}
	if l := logs(); len(l) != 1 || !strings.Contains(l[0], "forbidden") {
		t.Fatalf("logs %v", l)
	}
	clk.Add(6 * time.Minute)
	_ = sp.saveFleetHistory(context.Background(), store)
	if l := logs(); len(l) != 2 {
		t.Fatalf("after 10 min: %v", l)
	}
	panicky := newKeyedFleetHistoryStore(&failingBlobs{panics: true}, "pod-a", true)
	if err := sp.saveFleetHistory(context.Background(), panicky); err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("panic not contained: %v", err)
	}
	h, _ := sp.FleetHistory(context.Background(), "a", "1h")
	if h.Durable {
		t.Fatal("durable after failed saves")
	}
	// A failing load starts empty.
	if n, err := sp.RestoreFleetHistory(context.Background(), &errLoadStore{}); err == nil || n != 0 {
		t.Fatal("load error not returned")
	}
}

type errLoadStore struct{}

func (errLoadStore) Load(context.Context) ([]FleetHistorySnapshot, error) {
	return nil, errors.New("down")
}
func (errLoadStore) Save(context.Context, FleetHistorySnapshot) error { return nil }
func (errLoadStore) Persistent() bool                                 { return true }

func TestFleetHistoryShutdownFlushAndDurable(t *testing.T) {
	_ = captureFleetHistoryLog(t)
	a := newFake(healthyOv())
	sp, clk := newHistFleet(t, map[string]*fakeCluster{"a": a}, "a")
	blobs := &memoryFleetHistoryBlobs{}
	store := persistentMemStore(blobs, "pod-a", clk)
	ctx, cancel := context.WithCancel(context.Background())
	done := sp.StartFleetHistoryPersistence(ctx, store, time.Hour)
	probeFor(sp, clk, 3*time.Minute, 30*time.Second)
	h, _ := sp.FleetHistory(context.Background(), "a", "1h")
	if h.Durable || h.RestoredFrom != nil {
		t.Fatalf("before any save: durable=%v restoredFrom=%v", h.Durable, h.RestoredFrom)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("no flush on shutdown")
	}
	snaps, err := store.Load(context.Background())
	if err != nil || len(snaps) != 1 || snaps[0].Replica == "" || len(snaps[0].Clusters["a"]) == 0 {
		t.Fatalf("flushed %+v %v", snaps, err)
	}
	var checks int
	for _, b := range snaps[0].Clusters["a"] {
		checks += b.Checks
	}
	if checks != 6 {
		t.Fatalf("flushed checks %d", checks)
	}
	// Durable: saved within 3 intervals.
	h, _ = sp.FleetHistory(context.Background(), "a", "1h")
	if !h.Durable {
		t.Fatal("durable after a save")
	}
	clk.Add(3 * time.Hour)
	if h, _ = sp.FleetHistory(context.Background(), "a", "1h"); !h.Durable {
		t.Fatal("durable exactly 3 intervals after a save")
	}
	clk.Add(time.Second)
	if h, _ = sp.FleetHistory(context.Background(), "a", "1h"); h.Durable {
		t.Fatal("durable more than 3 intervals after a save")
	}
	// Disabled persistence: closed channel, nothing restored.
	sp2 := newFleet(t, map[string]*fakeCluster{"a": a}, "a")
	select {
	case <-sp2.StartFleetHistoryPersistence(context.Background(), store, 0):
	default:
		t.Fatal("interval 0 must return a closed channel")
	}
	if len(sp2.fleet().history) != 0 {
		t.Fatal("restored with persistence off")
	}
	select {
	case <-sp2.StartFleetHistoryPersistence(context.Background(), nil, time.Minute):
	default:
		t.Fatal("nil store must return a closed channel")
	}
}

func TestFleetHistoryReplicaKey(t *testing.T) {
	for in, want := range map[string]string{
		"zyvor-axiom-6d4f9-abcde": "history-zyvor-axiom-6d4f9-abcde",
		"host.local":              "history-host.local",
		"a/b:c d":                 "history-a-b-c-d",
		"":                        "history-unknown",
	} {
		if got := FleetHistoryReplicaKey(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if k := FleetHistoryReplicaKey(strings.Repeat("x", 400)); len(k) != 253 {
		t.Errorf("long key %d", len(k))
	}
	t.Setenv("POD_NAME", "axiom-0")
	if FleetHistoryReplicaID() != "axiom-0" {
		t.Error("POD_NAME ignored")
	}
	t.Setenv("POD_NAME", "")
	if FleetHistoryReplicaID() == "" {
		t.Error("no hostname fallback")
	}
}
