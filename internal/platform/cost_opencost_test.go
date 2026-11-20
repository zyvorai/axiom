package platform

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A trimmed but real-shaped OpenCost /allocation/compute answer
// (aggregate=namespace,controllerKind,controller, accumulate=true), with
// fields Axiom does not read left in.
const openCostSample = `{
  "code": 200,
  "status": "success",
  "data": [
    {
      "payments/deployment/payments-api": {
        "name": "payments/deployment/payments-api",
        "properties": {"cluster": "cluster-one", "namespace": "payments", "controllerKind": "deployment", "controller": "payments-api", "labels": {"app": "payments"}},
        "window": {"start": "2026-10-05T00:00:00Z", "end": "2026-11-04T00:00:00Z"},
        "start": "2026-10-05T00:00:00Z", "end": "2026-11-04T00:00:00Z", "minutes": 43200,
        "cpuCores": 1.5, "cpuCoreHours": 1080, "cpuCost": 30.5, "cpuCostAdjustment": 0.5, "cpuEfficiency": 0.4,
        "gpuCount": 0, "gpuCost": 0, "networkCost": 2, "loadBalancerCost": 3,
        "pvByteHours": 1e12, "pvCost": 4, "ramBytes": 1e9, "ramByteHours": 7.2e11, "ramCost": 10,
        "sharedCost": 0, "externalCost": 0, "totalCost": 50, "totalEfficiency": 0.3,
        "rawAllocationOnly": null, "someNewField": {"nested": [1, 2, 3]}
      },
      "payments/__unallocated__/__unallocated__": {
        "name": "payments/__unallocated__/__unallocated__",
        "properties": {"namespace": "payments"},
        "minutes": 43200, "cpuCost": 1, "ramCost": 1, "totalCost": 2
      },
      "data/statefulset/statefulset:redis": {
        "name": "data/statefulset/statefulset:redis",
        "minutes": 43200, "cpuCost": "5.5", "ramCost": 4.5
      },
      "__idle__": {"name": "__idle__", "minutes": 43200, "cpuCost": 20, "ramCost": 10, "totalCost": 30},
      "__unmounted__/__unmounted__/__unmounted__": {"name": "__unmounted__/__unmounted__/__unmounted__", "pvCost": 6, "totalCost": 6},
      "broken/deployment/x": "not an object"
    }
  ]
}`

func TestNewOpenCostClientValidation(t *testing.T) {
	if c, err := NewOpenCostClient("  ", "t"); c != nil || err != nil {
		t.Fatalf("empty: %v %v", c, err)
	}
	for _, bad := range []string{"ftp://x", "http://", "http://user:pw@opencost:9003", "::"} {
		c, err := NewOpenCostClient(bad, "")
		if err == nil || c != nil {
			t.Errorf("%q accepted", bad)
			continue
		}
		if strings.Contains(err.Error(), "pw") {
			t.Errorf("%q: error leaks the password: %v", bad, err)
		}
	}
	c, err := NewOpenCostClient("http://opencost.opencost.svc:9003/base/?x=1#f", " tok ")
	if err != nil || c.base.String() != "http://opencost.opencost.svc:9003/base" || c.token != "tok" {
		t.Fatalf("client %+v %v", c, err)
	}
}

func TestParseOpenCostAllocations(t *testing.T) {
	set, err := parseOpenCostAllocations([]byte(openCostSample), "30d")
	if err != nil {
		t.Fatal(err)
	}
	if set.Idle != 30 || set.Unallocated != 6 || set.Minutes != 43200 {
		t.Fatalf("buckets %+v", set)
	}
	if len(set.Rows) != 3 {
		t.Fatalf("rows %+v", set.Rows)
	}
	byKey := map[string]openCostRow{}
	for _, r := range set.Rows {
		byKey[r.Namespace+"/"+r.Controller] = r
	}
	p := byKey["payments/payments-api"]
	if p.Kind != "deployment" || p.CPU != 31 || p.Memory != 10 || p.Storage != 4 || p.Network != 2 || p.Total != 50 || p.Other != 3 {
		t.Fatalf("payments-api %+v", p)
	}
	// No properties: namespace / kind / controller come from the key and
	// "statefulset:redis" loses its kind prefix; a string number is read; a
	// missing totalCost is the sum of the parts.
	r := byKey["data/redis"]
	if r.Kind != "statefulset" || r.CPU != 5.5 || r.Total != 10 {
		t.Fatalf("redis %+v", r)
	}
	if u := byKey["payments/__unallocated__"]; u.Total != 2 {
		t.Fatalf("unallocated controller %+v", u)
	}
}

func TestParseOpenCostAllocationsEdgeCases(t *testing.T) {
	cases := []struct {
		name, body string
		wantErr    string
	}{
		{"non-200 code", `{"code":500,"message":"boom http://secret"}`, "code 500"},
		{"empty data", `{"code":200,"data":[]}`, "no allocation data"},
		{"null set", `{"code":200,"data":[null]}`, "no allocation data"},
		{"empty set", `{"code":200,"data":[{}]}`, "no allocation data"},
		{"not json", `<html>`, "malformed"},
	}
	for _, c := range cases {
		_, err := parseOpenCostAllocations([]byte(c.body), "30d")
		if err == nil || !strings.Contains(err.Error(), c.wantErr) || strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// Partial rows and huge / non-finite numbers.
	set, err := parseOpenCostAllocations([]byte(`{"data":[{
		"a/deployment/x": {"properties": {"namespace": "a"}},
		"b/deployment/y": {"properties": {"namespace": "b"}, "cpuCost": 1e308, "ramCost": 1e308, "totalCost": 1e999},
		"c/deployment/z": {"properties": {"namespace": "c"}, "cpuCost": "NaN", "ramCost": null, "totalCost": 3},
		"Bad_NS/deployment/q": {"totalCost": 9}
	}]}`), "7d")
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Rows) != 3 || set.Unallocated != 9 {
		t.Fatalf("rows %+v unallocated %v", set.Rows, set.Unallocated)
	}
	est := costFromAllocations(CostEstimate{}, set, "7d")
	for _, n := range est.Namespaces {
		if math.IsInf(n.MonthlyUSD, 0) || math.IsNaN(n.MonthlyUSD) || n.MonthlyUSD > costMaxRowUSD {
			t.Fatalf("unbounded %+v", n)
		}
	}
	if est.Namespaces[0].MonthlyUSD != 0 || est.Namespaces[1].MonthlyUSD != costMaxRowUSD {
		t.Fatalf("namespaces %+v", est.Namespaces)
	}
}

func TestOpenCostWindowScaling(t *testing.T) {
	if s := openCostScale(10080, "7d"); math.Abs(s-30.0/7) > 1e-9 {
		t.Fatalf("7d scale %v", s)
	}
	if s := openCostScale(0, "14d"); math.Abs(s-30.0/14) > 1e-9 {
		t.Fatalf("unknown minutes use the window: %v", s)
	}
	if s := openCostScale(43200, "30d"); s != 1 {
		t.Fatalf("30d scale %v", s)
	}
	// Only 2 days of data in a 7-day window: scale by the 2 days covered.
	set := openCostAllocations{Minutes: 2 * 1440, Rows: []openCostRow{{Namespace: "a", CPU: 2, Memory: 1, Total: 3}}}
	est := costFromAllocations(CostEstimate{Namespaces: []NamespaceCost{{Namespace: "a", CPUCores: 1.5, MemoryGi: 3}, {Namespace: "gone", CPUCores: 1}}}, set, "7d")
	if est.Namespaces[0].MonthlyUSD != 45 || *est.Namespaces[0].CPUUSD != 30 || est.Namespaces[0].CPUCores != 1.5 || est.Namespaces[0].MemoryGi != 3 {
		t.Fatalf("scaled row %+v", est.Namespaces[0])
	}
	if est.Namespaces[1].Namespace != "gone" || est.Namespaces[1].MonthlyUSD != 0 || est.TotalMonthlyUSD != 45 {
		t.Fatalf("estimate-only namespace %+v total %v", est.Namespaces, est.TotalMonthlyUSD)
	}
	if !strings.Contains(est.Note, "2d") && !strings.Contains(est.Note, "2 days") && !strings.Contains(est.Note, "48h") {
		t.Errorf("note should say how much data there is: %q", est.Note)
	}
	// Minutes beyond the window are capped at the window.
	big, _ := parseOpenCostAllocations([]byte(`{"data":[{"a/x/y":{"properties":{"namespace":"a"},"minutes":99999,"totalCost":1}}]}`), "7d")
	if big.Minutes != 10080 {
		t.Fatalf("minutes cap %v", big.Minutes)
	}
	// Start/end when minutes are missing.
	se, _ := parseOpenCostAllocations([]byte(`{"data":[{"a/x/y":{"properties":{"namespace":"a"},"start":"2026-11-01T00:00:00Z","end":"2026-11-04T00:00:00Z","totalCost":1}}]}`), "7d")
	if se.Minutes != 3*1440 {
		t.Fatalf("minutes from start/end %v", se.Minutes)
	}
}

func TestCostFromAllocationsRows(t *testing.T) {
	set, err := parseOpenCostAllocations([]byte(openCostSample), "30d")
	if err != nil {
		t.Fatal(err)
	}
	est := costFromAllocations(CostEstimate{CPURatePerCore: 20, MemoryRatePerGi: 5,
		Workloads: []WorkloadCost{{Namespace: "payments", Workload: "payments-api", CPUCores: 2, MemoryGi: 4}}}, set, "30d")
	if est.Source != CostSourceOpenCost || est.Window != "30d" || est.IdleMonthlyUSD != 30 || est.UnallocatedMonthlyUSD != 6 || est.CPURatePerCore != 20 {
		t.Fatalf("estimate %+v", est)
	}
	// Idle and unmounted are excluded from the total: payments 52 + data 10.
	if est.TotalMonthlyUSD != 62 || len(est.Namespaces) != 2 || est.Namespaces[1].Namespace != "payments" || est.Namespaces[1].MonthlyUSD != 52 {
		t.Fatalf("namespaces %+v total %v", est.Namespaces, est.TotalMonthlyUSD)
	}
	if !strings.Contains(est.Note, "OpenCost") {
		t.Errorf("note %q", est.Note)
	}
	var api, unowned, redis *WorkloadCost
	for i := range est.Workloads {
		switch est.Workloads[i].Workload {
		case "payments-api":
			api = &est.Workloads[i]
		case costUnownedWorkload:
			unowned = &est.Workloads[i]
		case "redis":
			redis = &est.Workloads[i]
		}
	}
	if api == nil || api.Kind != "Deployment" || api.MonthlyUSD != 50 || api.CPUCores != 2 || *api.OtherUSD != 3 || *api.StorageUSD != 4 {
		t.Fatalf("payments-api %+v", api)
	}
	if unowned == nil || unowned.Kind != "Pod" || unowned.MonthlyUSD != 2 {
		t.Fatalf("unowned %+v", unowned)
	}
	if redis == nil || redis.Kind != "StatefulSet" {
		t.Fatalf("redis %+v", redis)
	}
}

func newTestOpenCost(t *testing.T, h http.HandlerFunc) (*OpenCostClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewOpenCostClient(srv.URL+"/oc", "s3cret-token")
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func TestOpenCostClientRequestAndToken(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	c, _ := newTestOpenCost(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotQuery = r.Header.Get("Authorization"), r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(openCostSample))
	})
	set, err := c.allocations(context.Background(), "14d")
	if err != nil || len(set.Rows) != 3 {
		t.Fatalf("%+v %v", set, err)
	}
	if gotAuth != "Bearer s3cret-token" || gotPath != "/oc/allocation/compute" {
		t.Fatalf("auth %q path %q", gotAuth, gotPath)
	}
	if !strings.Contains(gotQuery, "window=14d") || !strings.Contains(gotQuery, "accumulate=true") || !strings.Contains(gotQuery, "aggregate=namespace%2CcontrollerKind%2Ccontroller") {
		t.Fatalf("query %q", gotQuery)
	}
}

func TestOpenCostClientFailuresNeverLeakURL(t *testing.T) {
	cases := []struct {
		name string
		h    http.HandlerFunc
		want string
	}{
		{"http 500", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) }, "HTTP 500"},
		{"code 401", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"code":401}`)) }, "code 401"},
		{"empty", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"code":200,"data":[]}`)) }, "no allocation data"},
		{"too big", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat(" ", openCostMaxBody+10)))
		}, "exceeds"},
		{"cross-host redirect", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://elsewhere.invalid/x", http.StatusFound)
		}, "another host"},
	}
	for _, tc := range cases {
		c, srv := newTestOpenCost(t, tc.h)
		_, err := c.fetch(context.Background(), "30d")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "s3cret") {
			t.Errorf("%s: error leaks the URL or token: %v", tc.name, err)
		}
	}
	// Unreachable.
	c, err := NewOpenCostClient("http://127.0.0.1:1", "s3cret-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.fetch(context.Background(), "30d"); err == nil || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("unreachable: %v", err)
	}
}

func TestOpenCostClientTimeout(t *testing.T) {
	release := make(chan struct{})
	c, _ := newTestOpenCost(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	c.timeout = 50 * time.Millisecond
	if _, err := c.fetch(context.Background(), "30d"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout: %v", err)
	}
}

func TestOpenCostClientCacheAndSingleFlight(t *testing.T) {
	var hits atomic.Int32
	gate := make(chan struct{})
	c, _ := newTestOpenCost(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-gate
		_, _ = w.Write([]byte(openCostSample))
	})
	now := time.Unix(1_800_000_000, 0)
	var nowMu sync.Mutex
	c.now = func() time.Time { nowMu.Lock(); defer nowMu.Unlock(); return now }
	advance := func(d time.Duration) { nowMu.Lock(); now = now.Add(d); nowMu.Unlock() }

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.allocations(context.Background(), "30d"); err != nil {
				t.Error(err)
			}
		}()
	}
	// Let the first fetch start, then release it.
	for hits.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(gate)
	wg.Wait()
	if n := hits.Load(); n != 1 {
		t.Fatalf("single flight: %d fetches", n)
	}
	// Cached within the TTL; refetched after it; another window is separate.
	_, _ = c.allocations(context.Background(), "30d")
	advance(openCostCacheTTL - time.Second)
	_, _ = c.allocations(context.Background(), "30d")
	if hits.Load() != 1 {
		t.Fatalf("cache: %d fetches", hits.Load())
	}
	advance(2 * time.Second)
	_, _ = c.allocations(context.Background(), "30d")
	_, _ = c.allocations(context.Background(), "7d")
	if hits.Load() != 3 {
		t.Fatalf("expiry/window: %d fetches", hits.Load())
	}
	// A cancelled waiter returns without the result but does not break the fetch.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	advance(openCostCacheTTL + time.Second)
	if _, err := c.allocations(ctx, "30d"); err != nil {
		t.Fatalf("the leader's fetch must not inherit its cancellation: %v", err)
	}
}

// stubCostProvider is a demo provider with an injected allocation source, so
// ActiveCost can be tested in both builds.
type stubCostProvider struct {
	Provider
	src costAllocationSource
}

func (s *stubCostProvider) openCostSource() costAllocationSource { return s.src }

type failingCostSource struct{ err error }

func (f failingCostSource) allocations(context.Context, string) (openCostAllocations, error) {
	return openCostAllocations{}, f.err
}

func TestActiveCostEstimateAndFallback(t *testing.T) {
	t.Setenv("AXIOM_DEMO_OPENCOST", "")
	est, err := ActiveCost(context.Background(), NewDemoProvider(), "7d")
	if err != nil || est.Source != CostSourceEstimate || est.Window != "7d" || est.Note != "" || est.TotalMonthlyUSD != 720 || est.Namespaces[0].CPUUSD != nil || est.GPURatePerGpu != 100 || est.OpenCostConfigured {
		t.Fatalf("estimate %+v %v", est, err)
	}
	p := &stubCostProvider{Provider: NewDemoProvider(), src: failingCostSource{openCostErrorf("OpenCost returned HTTP 503")}}
	est, err = ActiveCost(context.Background(), p, "30d")
	if err != nil || est.Source != CostSourceEstimate || est.TotalMonthlyUSD != 720 || !est.OpenCostConfigured || est.OpenCostReachable || !strings.Contains(est.Note, "HTTP 503") || !strings.Contains(est.Note, "estimate") {
		t.Fatalf("fallback %+v %v", est, err)
	}
	// A foreign error (which might carry a URL) is never echoed.
	p.src = failingCostSource{errors.New("Get http://opencost.internal:9003/allocation?token=x: refused")}
	est, _ = ActiveCost(context.Background(), p, "30d")
	if strings.Contains(est.Note, "opencost.internal") || strings.Contains(est.Note, "token") {
		t.Fatalf("note leaks: %q", est.Note)
	}
	// A real client that fails: the note carries neither URL nor token.
	c, _ := newTestOpenCost(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 502) })
	p.src = c
	est, _ = ActiveCost(context.Background(), p, "30d")
	if est.Source != CostSourceEstimate || !strings.Contains(est.Note, "HTTP 502") || strings.Contains(est.Note, "127.0.0.1") || strings.Contains(est.Note, "s3cret") {
		t.Fatalf("client fallback %+v", est)
	}
	// Through a SwitchableProvider.
	sp := NewSwitchableProvider()
	sp.Register("one", "one", &clusterLabeledProvider{Provider: &stubCostProvider{Provider: NewDemoProvider(), src: demoOpenCostSource{}}, label: "x"}, "demo://one")
	est, _ = ActiveCost(context.Background(), sp, "30d")
	if est.Source != CostSourceOpenCost {
		t.Fatalf("switchable %+v", est)
	}
}

func TestActiveCostDemoSwitch(t *testing.T) {
	t.Setenv("AXIOM_DEMO_OPENCOST", "true")
	est, err := ActiveCost(context.Background(), NewDemoProvider(), "30d")
	if err != nil || est.Source != CostSourceOpenCost || est.Window != "30d" || est.IdleMonthlyUSD != demoOpenCostIdleMonthly {
		t.Fatalf("demo opencost %+v %v", est, err)
	}
	var sum float64
	for _, n := range est.Namespaces {
		sum += n.MonthlyUSD
		if n.CPUUSD == nil || n.MemoryUSD == nil || n.StorageUSD == nil || n.NetworkUSD == nil || n.OtherUSD == nil {
			t.Fatalf("missing breakdown %+v", n)
		}
		if n.Namespace == "payments" && (n.MonthlyUSD == 180 || n.CPUCores != 6) {
			t.Fatalf("payments should be actual cost with request columns: %+v", n)
		}
	}
	if math.Abs(sum-est.TotalMonthlyUSD) > 0.011 || len(est.Namespaces) != 4 {
		t.Fatalf("total %v vs %v (%d rows)", est.TotalMonthlyUSD, sum, len(est.Namespaces))
	}
	// The monthly figure does not depend on the window (fixed daily rate).
	est7, _ := ActiveCost(context.Background(), NewDemoProvider(), "7d")
	if est7.Window != "7d" || math.Abs(est7.TotalMonthlyUSD-est.TotalMonthlyUSD) > 0.05 {
		t.Fatalf("7d %v vs 30d %v", est7.TotalMonthlyUSD, est.TotalMonthlyUSD)
	}
	// Demo clusters behind a SwitchableProvider follow the switch too.
	est, _ = ActiveCost(context.Background(), NewDemoSwitchableProvider(), "30d")
	if est.Source != CostSourceOpenCost {
		t.Fatalf("demo switchable %+v", est.Source)
	}
}

func TestCostBudgetsUseActiveSource(t *testing.T) {
	store := NewMemoryCostBudgetStore()
	_ = store.Set(context.Background(), CostBudget{Namespace: "payments", MonthlyUSD: 200, WarnPercent: 80})
	t.Setenv("AXIOM_DEMO_OPENCOST", "")
	rep, err := CostBudgetReportFor(context.Background(), NewDemoProvider(), store)
	if err != nil || rep.Source != CostSourceEstimate || rep.Budgets[0].CurrentUSD != 180 {
		t.Fatalf("estimate budgets %+v %v", rep, err)
	}
	t.Setenv("AXIOM_DEMO_OPENCOST", "true")
	rep, err = CostBudgetReportFor(context.Background(), NewDemoProvider(), store)
	if err != nil || rep.Source != CostSourceOpenCost || rep.Budgets[0].CurrentUSD != 147 {
		t.Fatalf("opencost budgets %+v %v", rep, err)
	}
}

func TestNormalizeCostWindow(t *testing.T) {
	for in, want := range map[string]string{"": "30d", " 7d ": "7d", "14d": "14d", "30d": "30d"} {
		if got, err := NormalizeCostWindow(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"1d", "30", "90d", "7D", "-7d"} {
		if _, err := NormalizeCostWindow(bad); err == nil || HTTPStatus(err) != http.StatusBadRequest {
			t.Errorf("%q accepted (%v)", bad, err)
		}
	}
}
