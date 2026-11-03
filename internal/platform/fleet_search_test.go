package platform

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// searchFake is a cluster whose list calls are scripted and counted.
type searchFake struct {
	Provider
	workloads []Workload
	services  []Service
	nss       []Namespace
	err       error
	delay     time.Duration
	ignoreCtx bool // sleep through the delay even when the context is done
	ovErr     error
	listCalls atomic.Int32
}

func (f *searchFake) wait(ctx context.Context) error {
	if f.delay == 0 {
		return nil
	}
	if f.ignoreCtx {
		time.Sleep(f.delay)
		return nil
	}
	select {
	case <-time.After(f.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *searchFake) Overview(ctx context.Context) (Overview, error) {
	if f.ovErr != nil {
		return Overview{}, f.ovErr
	}
	return healthyOv(), nil
}

func (f *searchFake) ListWorkloads(ctx context.Context, ns string) ([]Workload, error) {
	f.listCalls.Add(1)
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.workloads, nil
}

func (f *searchFake) ListServices(ctx context.Context, ns string) ([]Service, error) {
	f.listCalls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return f.services, nil
}

func (f *searchFake) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	f.listCalls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return f.nss, nil
}

func stdSearchFake() *searchFake {
	return &searchFake{
		workloads: []Workload{
			{Namespace: "payments", Name: "payments-api", Kind: "Deployment", Image: "ghcr.io/zyvorai/payments:v2.8.4", Desired: 12, Ready: 12, Status: "Healthy"},
			{Namespace: "payments", Name: "ledger", Kind: "StatefulSet", Image: "ghcr.io/zyvorai/ledger:v1", Desired: 3, Ready: 2, Status: "Degraded"},
			{Namespace: "shop", Name: "checkout", Kind: "Deployment", Image: "ghcr.io/zyvorai/pay-sdk:v1", Desired: 2, Ready: 2, Status: "Healthy"},
			{Namespace: "shop", Name: "pay", Kind: "Deployment", Image: "nginx", Desired: 1, Ready: 1, Status: "Healthy"},
			{Namespace: "web", Name: "frontend", Kind: "Deployment", Image: "nginx", Desired: 1, Ready: 1, Status: "Healthy"},
		},
		services: []Service{
			{Namespace: "payments", Name: "payments-api", Type: "ClusterIP", ClusterIP: "10.0.0.7", Ports: []ServicePort{{Port: 443, Protocol: "TCP"}}, Endpoints: []string{"a", "b"}},
			{Namespace: "web", Name: "frontend", Type: "LoadBalancer"},
		},
		nss: []Namespace{{Name: "payments", Status: "Active", Pods: 15}, {Name: "shop", Status: "Active"}, {Name: "web", Status: "Active"}},
	}
}

func newSearchFleet(t *testing.T, fakes map[string]*searchFake, order ...string) *SwitchableProvider {
	t.Helper()
	sp := NewSwitchableProvider()
	for _, n := range order {
		sp.Register(n, n+"-label", fakes[n], "https://"+n)
	}
	return sp
}

func hitKeys(r FleetSearchResult) []string {
	out := make([]string, 0, len(r.Results))
	for _, h := range r.Results {
		out = append(out, h.Cluster+"/"+h.Kind+"/"+h.Namespace+"/"+h.Name)
	}
	return out
}

func TestSearchFleetRankingAndFilters(t *testing.T) {
	sp := newSearchFleet(t, map[string]*searchFake{"a": stdSearchFake(), "b": stdSearchFake()}, "a", "b")
	cases := []struct {
		name      string
		q         string
		kinds     []string
		limit     int
		want      []string
		total     int
		truncated bool
	}{
		{"ranking: exact, prefix, then namespace/image; cluster order inside a rank", "pay", []string{FleetSearchWorkload}, 0, []string{
			"a/Workload/shop/pay", "b/Workload/shop/pay", // exact name
			"a/Workload/payments/payments-api", "b/Workload/payments/payments-api", // name prefix
			"a/Workload/payments/ledger", "a/Workload/shop/checkout", // namespace / image
			"b/Workload/payments/ledger", "b/Workload/shop/checkout",
		}, 8, false},
		{"name contains ranks before namespace", "ments-a", nil, 0, []string{
			"a/Workload/payments/payments-api", "a/Service/payments/payments-api",
			"b/Workload/payments/payments-api", "b/Service/payments/payments-api",
		}, 4, false},
		{"case-insensitive, trimmed", "  FRONTEND ", []string{FleetSearchService}, 0, []string{"a/Service/web/frontend", "b/Service/web/frontend"}, 2, false},
		{"namespace kind", "pay", []string{FleetSearchNamespace}, 0, []string{"a/Namespace/payments/payments", "b/Namespace/payments/payments"}, 2, false},
		{"limit truncates after sorting", "pay", nil, 3, []string{"a/Workload/shop/pay", "b/Workload/shop/pay", "a/Namespace/payments/payments"}, 12, true},
		{"no match", "zzz", nil, 0, []string{}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := sp.SearchFleet(context.Background(), c.q, c.kinds, c.limit)
			got := hitKeys(r)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("results:\n got %v\nwant %v", got, c.want)
			}
			if r.Total != c.total || r.Truncated != c.truncated {
				t.Fatalf("total=%d truncated=%v, want %d %v", r.Total, r.Truncated, c.total, c.truncated)
			}
			if r.Query != strings.TrimSpace(c.q) || r.Clusters.Searched != 2 || len(r.Clusters.Errors) != 0 || len(r.Clusters.Skipped) != 0 {
				t.Fatalf("meta: %+v", r)
			}
		})
	}
}

func TestSearchFleetHitFields(t *testing.T) {
	sp := newSearchFleet(t, map[string]*searchFake{"eu": stdSearchFake()}, "eu")
	r := sp.SearchFleet(context.Background(), "payments-api", nil, 0)
	if len(r.Results) != 2 {
		t.Fatalf("results: %+v", r.Results)
	}
	w, s := r.Results[0], r.Results[1]
	if w.Cluster != "eu" || w.ClusterLabel != "eu-label" || w.ClusterState != ClusterHealthy || w.Kind != FleetSearchWorkload ||
		w.WorkloadKind != "Deployment" || w.Status != "Healthy" || w.Detail != "12/12 ready · ghcr.io/zyvorai/payments:v2.8.4" {
		t.Fatalf("workload hit: %+v", w)
	}
	if s.Kind != FleetSearchService || s.WorkloadKind != "" || s.Status != "2 endpoints" || s.Detail != "ClusterIP · 10.0.0.7 · 443/TCP" {
		t.Fatalf("service hit: %+v", s)
	}
}

func TestSearchFleetLimitClamp(t *testing.T) {
	f := stdSearchFake()
	for i := 0; i < 300; i++ {
		f.workloads = append(f.workloads, Workload{Namespace: "bulk", Name: "pay-" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + time.Duration(i).String()})
	}
	sp := newSearchFleet(t, map[string]*searchFake{"a": f}, "a")
	r := sp.SearchFleet(context.Background(), "pay", []string{FleetSearchWorkload}, 10000)
	if len(r.Results) != FleetSearchMaxLimit || !r.Truncated || r.Total <= FleetSearchMaxLimit {
		t.Fatalf("clamp: %d results, total %d, truncated %v", len(r.Results), r.Total, r.Truncated)
	}
	if r = sp.SearchFleet(context.Background(), "pay", []string{FleetSearchWorkload}, 0); len(r.Results) != FleetSearchDefaultLimit {
		t.Fatalf("default limit: %d", len(r.Results))
	}
}

func TestParseFleetSearchKinds(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		err  bool
	}{
		{"", "Workload,Service,Namespace", false},
		{"Workload", "Workload", false},
		{"service", "Service", false},
		{"Namespace,workload,Namespace", "Namespace,Workload", false},
		{"Pod", "", true},
		{"Workload,", "", true},
	}
	for _, c := range cases {
		got, err := ParseFleetSearchKinds(c.raw)
		if (err != nil) != c.err || (!c.err && strings.Join(got, ",") != c.want) {
			t.Errorf("%q: %v %v", c.raw, got, err)
		}
	}
}

func TestSearchFleetSkipsUnreachable(t *testing.T) {
	dead := stdSearchFake()
	dead.ovErr = errors.New("dial tcp: connection refused")
	sp := newSearchFleet(t, map[string]*searchFake{"a": stdSearchFake(), "dead": dead}, "a", "dead")
	// Never probed yet: the search probes first, so the dead cluster is skipped, not searched.
	r := sp.SearchFleet(context.Background(), "pay", nil, 0)
	if dead.listCalls.Load() != 0 {
		t.Fatalf("unreachable cluster was searched (%d calls)", dead.listCalls.Load())
	}
	if r.Clusters.Searched != 1 || len(r.Clusters.Skipped) != 1 || r.Clusters.Skipped[0] != (FleetSearchSkip{Cluster: "dead", Reason: "unreachable"}) {
		t.Fatalf("clusters: %+v", r.Clusters)
	}
	for _, h := range r.Results {
		if h.Cluster != "a" {
			t.Fatalf("hit from skipped cluster: %+v", h)
		}
	}
}

func TestSearchFleetSlowClusterTimesOut(t *testing.T) {
	old := fleetSearchTimeout
	fleetSearchTimeout = 80 * time.Millisecond
	t.Cleanup(func() { fleetSearchTimeout = old })

	slow := stdSearchFake()
	slow.delay, slow.ignoreCtx = 5*time.Second, true // even a provider that ignores its context cannot stall the search
	sp := newSearchFleet(t, map[string]*searchFake{"a": stdSearchFake(), "slow": slow, "c": stdSearchFake()}, "a", "slow", "c")
	start := time.Now()
	r := sp.SearchFleet(context.Background(), "pay", nil, 0)
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("search took %s", el)
	}
	if r.Clusters.Searched != 3 || len(r.Clusters.Errors) != 1 || r.Clusters.Errors[0].Cluster != "slow" || !strings.Contains(r.Clusters.Errors[0].Error, "timed out") {
		t.Fatalf("clusters: %+v", r.Clusters)
	}
	clusters := map[string]bool{}
	for _, h := range r.Results {
		clusters[h.Cluster] = true
	}
	if !clusters["a"] || !clusters["c"] || clusters["slow"] {
		t.Fatalf("hits came from %v", clusters)
	}
}

func TestSearchFleetErrorReported(t *testing.T) {
	bad := stdSearchFake()
	bad.err = errors.New("Get \"https://admin:s3cret@10.0.0.5:6443/apis/apps/v1/deployments\":\n  forbidden")
	sp := newSearchFleet(t, map[string]*searchFake{"a": stdSearchFake(), "bad": bad}, "a", "bad")
	r := sp.SearchFleet(context.Background(), "pay", nil, 0)
	if len(r.Clusters.Errors) != 1 || r.Clusters.Errors[0].Cluster != "bad" || r.Clusters.Searched != 2 {
		t.Fatalf("clusters: %+v", r.Clusters)
	}
	msg := r.Clusters.Errors[0].Error
	if strings.Contains(msg, "s3cret") || strings.Contains(msg, "admin:") || strings.Contains(msg, "\n") || !strings.Contains(msg, "https://10.0.0.5:6443") {
		t.Fatalf("error not sanitized: %q", msg)
	}
	if len(r.Results) == 0 {
		t.Fatal("the healthy cluster's hits are missing")
	}
}

func TestSearchFleetUsesEachClustersOwnProvider(t *testing.T) {
	a, b, c := stdSearchFake(), stdSearchFake(), stdSearchFake()
	b.workloads = []Workload{{Namespace: "payments", Name: "only-in-b", Kind: "Deployment", Image: "x"}}
	sp := newSearchFleet(t, map[string]*searchFake{"a": a, "b": b, "c": c}, "a", "b", "c")
	// The session's active cluster must not matter.
	ctx := WithClusterSession(context.Background(), "sess-1")
	if err := sp.SetActive(ctx, "c"); err != nil {
		t.Fatal(err)
	}
	r := sp.SearchFleet(ctx, "only-in", []string{FleetSearchWorkload}, 0)
	if got := strings.Join(hitKeys(r), ","); got != "b/Workload/payments/only-in-b" {
		t.Fatalf("results: %s", got)
	}
	for n, f := range map[string]*searchFake{"a": a, "b": b, "c": c} {
		if f.listCalls.Load() != 1 { // one ListWorkloads each: only the requested kind
			t.Errorf("cluster %s: %d list calls", n, f.listCalls.Load())
		}
	}
}

func TestSearchFleetDemo(t *testing.T) {
	t.Setenv("AXIOM_DEMO_FLEET_DOWN", "ap-south-1")
	sp := NewDemoSwitchableProvider()
	r := sp.SearchFleet(context.Background(), "payments-api", []string{FleetSearchWorkload}, 0)
	if got := strings.Join(hitKeys(r), ","); got != "eu-west-1/Workload/payments/payments-api,us-east-1/Workload/payments/payments-api" {
		t.Fatalf("results: %s", got)
	}
	if r.Results[0].ClusterLabel != "zyvor-demo" || r.Results[0].Detail != "12/12 ready · ghcr.io/zyvorai/payments:v2.8.4" {
		t.Fatalf("hit: %+v", r.Results[0])
	}
	if len(r.Clusters.Skipped) != 1 || r.Clusters.Skipped[0].Cluster != "ap-south-1" {
		t.Fatalf("clusters: %+v", r.Clusters)
	}
}
