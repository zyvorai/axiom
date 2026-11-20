package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecommendCPUMilli(t *testing.T) {
	cases := []struct{ usage, want int64 }{
		{0, 10}, {1, 10}, {5, 10}, {8, 10}, {9, 20}, // 9*1.2 = 10.8 -> 20
		{100, 120}, {101, 130}, {250, 300}, {1000, 1200}, {-5, 10},
	}
	for _, c := range cases {
		if got := recommendCPUMilli(c.usage); got != c.want {
			t.Errorf("recommendCPUMilli(%d) = %d, want %d", c.usage, got, c.want)
		}
	}
}

func TestRecommendMemoryMiB(t *testing.T) {
	cases := []struct {
		usage int64
		want  int64
	}{
		{0, 16}, {1, 16}, {10 * mib, 16}, {14 * mib, 24}, // 14*1.15 = 16.1 -> 24
		{100 * mib, 120}, // 115 -> 120
		{160 * mib, 184}, // 184 exactly
		{160*mib + 1, 192},
		{1024 * mib, 1184}, // 1177.6 -> 1184
		{-1, 16},
	}
	for _, c := range cases {
		if got := recommendMemoryMiB(c.usage); got != c.want {
			t.Errorf("recommendMemoryMiB(%d) = %d, want %d", c.usage, got, c.want)
		}
	}
}

func rc(cpuReq, memReqMiB int64) rightsizingContainer {
	return rightsizingContainer{Namespace: "ns", Kind: "Deployment", Workload: "w", Container: "c", Replicas: 1, RequestCPUMilli: cpuReq, RequestMemBytes: memReqMiB * mib}
}

func use(cpu, memMiB int64) containerUsage {
	return containerUsage{CPUMilli: cpu, MemBytes: memMiB * mib, HasCPU: true, HasMem: true}
}

var testBasis = rightsizingBasis{CPU: "p95", Memory: "p99"}

func TestRecommendContainerVerdicts(t *testing.T) {
	cases := []struct {
		name    string
		c       rightsizingContainer
		u       containerUsage
		verdict string
		reason  string
	}{
		// CPU rec 120m; 500 > 180 and gap 380 >= 50 -> over.
		{"cpu over", rc(500, 140), use(100, 100), "over", "CPU request 500m"},
		// rec 120m, request 181 > 180 but gap 61 >= 50 -> over.
		{"cpu just over ratio", rc(181, 140), use(100, 100), "over", "CPU request"},
		// rec 120m, request 180 is not > 1.5x -> ok.
		{"cpu exactly 1.5x", rc(180, 140), use(100, 100), "ok", "in line"},
		// rec 10m, request 40m > 15m but gap 30 < 50 -> ok.
		{"cpu ratio but small gap", rc(40, 140), use(5, 100), "ok", ""},
		// memory rec 120Mi; 512 > 180 and gap 392 >= 64 -> over.
		{"memory over", rc(130, 512), use(100, 100), "over", "memory request 512Mi"},
		// memory rec 16Mi; request 64Mi > 24 but gap 48 < 64 -> ok.
		{"memory ratio but small gap", rc(130, 64), use(100, 10), "ok", ""},
		// CPU usage 95 > 0.9*100 -> under (risk wins over memory over).
		{"cpu under", rc(100, 4096), use(95, 100), "under", "throttling"},
		{"cpu at 90% is not under", rc(100, 140), use(90, 100), "ok", ""},
		{"memory under", rc(1000, 1000), use(800, 950), "under", "OOM"},
		{"no request", rc(0, 0), use(50, 50), "under", "no request set"},
		{"no cpu request", rc(0, 200), use(50, 50), "under", "no CPU request set"},
		{"no memory request", rc(100, 0), use(50, 50), "under", "no memory request set"},
	}
	for _, c := range cases {
		it, ok := recommendContainer(c.c, c.u, testBasis)
		if !ok {
			t.Fatalf("%s: no item", c.name)
		}
		if it.Verdict != c.verdict || !strings.Contains(it.Reason, c.reason) {
			t.Errorf("%s: verdict %q reason %q, want %q containing %q", c.name, it.Verdict, it.Reason, c.verdict, c.reason)
		}
		if it.CPU.Basis != "p95" || it.Memory.Basis != "p99" {
			t.Errorf("%s: basis %+v %+v", c.name, it.CPU, it.Memory)
		}
	}
}

func TestRecommendContainerUnknownUsageSkipped(t *testing.T) {
	if _, ok := recommendContainer(rc(100, 100), containerUsage{}, testBasis); ok {
		t.Fatal("a container without usage must be skipped")
	}
	if _, ok := recommendContainer(rc(0, 0), containerUsage{}, testBasis); ok {
		t.Fatal("no request and no usage must be skipped")
	}
	// Only memory known: CPU recommendation keeps the request, verdict from memory.
	it, ok := recommendContainer(rc(500, 1024), containerUsage{MemBytes: 100 * mib, HasMem: true}, testBasis)
	if !ok || it.CPU.RecommendedMilli != 500 || it.Memory.RecommendedMiB != 120 || it.Verdict != "over" {
		t.Fatalf("memory-only: %+v", it)
	}
}

func TestRecommendContainerDeltaSign(t *testing.T) {
	// 3 replicas, CPU 1000m -> 120m (-0.88 core), memory 1024Mi -> 120Mi (-904/1024 GiB).
	c := rc(1000, 1024)
	c.Replicas = 3
	it, _ := recommendContainer(c, use(100, 100), testBasis)
	want := round2((-0.88*costCPURatePerCore + (-904.0/1024)*costMemoryRatePerGi) * 3)
	if it.MonthlyDeltaUSD != want || it.MonthlyDeltaUSD >= 0 {
		t.Fatalf("delta %v, want %v (negative = savings)", it.MonthlyDeltaUSD, want)
	}
	// No request set: recommending adds cost.
	it, _ = recommendContainer(rc(0, 0), use(100, 100), testBasis)
	if it.MonthlyDeltaUSD <= 0 {
		t.Fatalf("no-request delta should be positive: %v", it.MonthlyDeltaUSD)
	}
	if it.CPU.RequestMilli != 0 || it.CPU.RecommendedMilli != 120 || it.Memory.UsageMiB != 100 {
		t.Fatalf("fields: %+v", it)
	}
}

func TestFinishRightsizingItemsSortSummaryCap(t *testing.T) {
	items := []RightsizingItem{
		{Namespace: "b", Workload: "x", Kind: "Deployment", Container: "c", Verdict: "over", MonthlyDeltaUSD: -10},
		{Namespace: "a", Workload: "y", Kind: "Deployment", Container: "c", Verdict: "ok", MonthlyDeltaUSD: -1},
		{Namespace: "a", Workload: "x", Kind: "Deployment", Container: "c", Verdict: "over", MonthlyDeltaUSD: -10},
		{Namespace: "a", Workload: "x", Kind: "Deployment", Container: "b", Verdict: "under", MonthlyDeltaUSD: 5},
		{Namespace: "a", Workload: "x", Kind: "Deployment", Container: "a", Verdict: "over", MonthlyDeltaUSD: -30.5},
	}
	out, sum := finishRightsizingItems(items)
	order := []string{}
	for _, it := range out {
		order = append(order, it.Namespace+"/"+it.Workload+"/"+it.Container)
	}
	if got := strings.Join(order, ","); got != "a/x/a,a/x/c,b/x/c,a/y/c,a/x/b" {
		t.Fatalf("order %s", got)
	}
	if sum.Workloads != 3 || sum.OverProvisioned != 3 || sum.UnderProvisioned != 1 || sum.MonthlySavingsUSD != 50.5 {
		t.Fatalf("summary %+v", sum)
	}

	many := make([]RightsizingItem, 0, 600)
	for i := 0; i < 600; i++ {
		many = append(many, RightsizingItem{Namespace: "n", Workload: fmt.Sprintf("w%03d", i), Verdict: "over", MonthlyDeltaUSD: -1})
	}
	out, sum = finishRightsizingItems(many)
	if len(out) != rightsizingMaxItems || sum.Workloads != 600 || sum.OverProvisioned != 600 || sum.MonthlySavingsUSD != 600 {
		t.Fatalf("cap: %d %+v", len(out), sum)
	}
	if out, _ := finishRightsizingItems(nil); out == nil {
		t.Fatal("empty items must be [] not null")
	}
}

func TestNormalizeRightsizingWindow(t *testing.T) {
	for _, w := range []string{"1d", "3d", "7d", "14d", "30d"} {
		if got, err := NormalizeRightsizingWindow(w); err != nil || got != w {
			t.Errorf("%s: %q %v", w, got, err)
		}
	}
	if got, _ := NormalizeRightsizingWindow(""); got != "7d" {
		t.Errorf("default %q", got)
	}
	for _, w := range []string{"2d", "7", "1w", "7d]", "7d) or vector(1"} {
		if _, err := NormalizeRightsizingWindow(w); err == nil || HTTPStatus(err) != 400 {
			t.Errorf("%q accepted (%v)", w, err)
		}
	}
}

func TestWorkloadFromPodName(t *testing.T) {
	known := map[string]bool{"ns/Deployment/web": true, "ns/StatefulSet/db": true, "ns/DaemonSet/agent": true}
	cases := []struct{ pod, kind, name string }{
		{"web-7f7fc496b4-x2x9q", "Deployment", "web"},
		{"db-0", "StatefulSet", "db"},
		{"db-12", "StatefulSet", "db"},
		{"agent-abcde", "DaemonSet", "agent"},
		{"other-7f7fc496b4-x2x9q", "", ""},
		{"web", "", ""},
	}
	for _, c := range cases {
		k, n, ok := workloadFromPodName("ns", c.pod, known)
		if k != c.kind || n != c.name || ok != (c.kind != "") {
			t.Errorf("%s: %s/%s %v", c.pod, k, n, ok)
		}
	}
	if _, _, ok := workloadFromPodName("other", "db-0", known); ok {
		t.Error("namespace must match")
	}
}

type fakeUsageSource struct {
	snapshotUsageSource
	n       string
	samples []podContainerUsage
	err     error
}

func (f fakeUsageSource) name() string { return f.n }
func (f fakeUsageSource) usage(context.Context, string, string) ([]podContainerUsage, error) {
	return f.samples, f.err
}

func testInputs() rightsizingInputs {
	return rightsizingInputs{
		Containers: []rightsizingContainer{
			{Namespace: "ns", Kind: "Deployment", Workload: "web", Container: "app", Replicas: 2, RequestCPUMilli: 1000, RequestMemBytes: 1024 * mib},
			{Namespace: "ns", Kind: "Deployment", Workload: "idle", Container: "app", Replicas: 1, RequestCPUMilli: 100, RequestMemBytes: 128 * mib},
		},
		Resolve: func(ns, pod string) (string, string, bool) {
			return workloadFromPodName(ns, pod, map[string]bool{"ns/Deployment/web": true, "ns/Deployment/idle": true})
		},
	}
}

func TestRunRightsizingAggregatesMaxAcrossReplicas(t *testing.T) {
	src := fakeUsageSource{n: "metrics-snapshot", samples: []podContainerUsage{
		{Namespace: "ns", Pod: "web-abc12345-aaaaa", Container: "app", CPUMilli: 100, MemBytes: 100 * mib, HasCPU: true, HasMem: true},
		{Namespace: "ns", Pod: "web-abc12345-bbbbb", Container: "app", CPUMilli: 200, MemBytes: 50 * mib, HasCPU: true, HasMem: true},
		{Namespace: "ns", Pod: "web-abc12345-bbbbb", Container: "POD", CPUMilli: 999, HasCPU: true},
		{Namespace: "ns", Pod: "gone-abc12345-ccccc", Container: "app", CPUMilli: 5000, HasCPU: true},
	}}
	rep := runRightsizing(context.Background(), "", "7d", []rightsizingUsageSource{src}, testInputs())
	if rep.Source != "metrics-snapshot" || rep.Confidence != "low" || len(rep.Items) != 1 {
		t.Fatalf("report %+v", rep)
	}
	it := rep.Items[0]
	if it.Workload != "web" || it.CPU.UsageMilli != 200 || it.Memory.UsageMiB != 100 || it.CPU.RecommendedMilli != 240 || it.Replicas != 2 || it.Verdict != "over" {
		t.Fatalf("item %+v", it)
	}
	if !strings.Contains(rep.Note, "One sample is not a usage profile") {
		t.Fatalf("note %q", rep.Note)
	}
}

func TestRunRightsizingFallbackAndNone(t *testing.T) {
	failing := fakeUsageSource{n: "prometheus", err: errors.New("Prometheus is unreachable")}
	empty := fakeUsageSource{n: "prometheus"}
	snap := fakeUsageSource{n: "metrics-snapshot", samples: []podContainerUsage{
		{Namespace: "ns", Pod: "idle-abc12345-aaaaa", Container: "app", CPUMilli: 5, MemBytes: 20 * mib, HasCPU: true, HasMem: true},
	}}
	rep := runRightsizing(context.Background(), "", "7d", []rightsizingUsageSource{failing, snap}, testInputs())
	if rep.Source != "metrics-snapshot" || !strings.Contains(rep.Note, "Prometheus unavailable: Prometheus is unreachable.") || len(rep.Items) != 1 {
		t.Fatalf("fallback: %+v", rep)
	}
	rep = runRightsizing(context.Background(), "", "7d", []rightsizingUsageSource{empty, snap}, testInputs())
	if rep.Source != "metrics-snapshot" || !strings.Contains(rep.Note, "Prometheus returned no container usage") {
		t.Fatalf("empty fallback: %+v", rep)
	}
	rep = runRightsizing(context.Background(), "", "3d", []rightsizingUsageSource{failing, snapshotUsageSource{}}, testInputs())
	if rep.Source != "none" || rep.Window != "3d" || len(rep.Items) != 0 || rep.Items == nil || !strings.Contains(rep.Note, "AXIOM_PROMETHEUS_URL") {
		t.Fatalf("none: %+v", rep)
	}
}

// A real Prometheus source against a failing server falls back to the snapshot.
func TestRunRightsizingPrometheusFailureFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, err := NewPrometheusClient(srv.URL, "s3cret-token")
	if err != nil {
		t.Fatal(err)
	}
	snap := fakeUsageSource{n: "metrics-snapshot", samples: []podContainerUsage{
		{Namespace: "ns", Pod: "idle-abc12345-aaaaa", Container: "app", CPUMilli: 5, MemBytes: 20 * mib, HasCPU: true, HasMem: true},
	}}
	rep := runRightsizing(context.Background(), "", "7d", []rightsizingUsageSource{promUsageSource{client: c}, snap}, testInputs())
	if rep.Source != "metrics-snapshot" || !strings.Contains(rep.Note, "HTTP 500") {
		t.Fatalf("report %+v", rep)
	}
	if strings.Contains(rep.Note, srv.URL) || strings.Contains(rep.Note, "s3cret") {
		t.Fatalf("note leaks the URL or token: %q", rep.Note)
	}
}

func TestPrometheusSourceSuccess(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		queries = append(queries, q)
		if strings.Contains(q, "container_cpu_usage_seconds_total") {
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
				{"metric":{"namespace":"ns","pod":"web-abc12345-aaaaa","container":"app"},"value":[1,"0.1"]},
				{"metric":{"namespace":"ns","pod":"web-abc12345-bbbbb","container":"app"},"value":[1,"0.25"]}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
			{"metric":{"namespace":"ns","pod":"web-abc12345-aaaaa","container":"app"},"value":[1,"104857600"]}]}}`))
	}))
	defer srv.Close()
	c, _ := NewPrometheusClient(srv.URL+"/", "")
	rep := runRightsizing(context.Background(), "ns", "14d", []rightsizingUsageSource{promUsageSource{client: c}}, testInputs())
	if rep.Source != "prometheus" || rep.Confidence != "high" || len(rep.Items) != 1 {
		t.Fatalf("report %+v", rep)
	}
	it := rep.Items[0]
	if it.CPU.UsageMilli != 250 || it.CPU.Basis != "p95" || it.Memory.UsageMiB != 100 || it.Memory.Basis != "p99" || it.CPU.RecommendedMilli != 300 {
		t.Fatalf("item %+v", it)
	}
	if len(queries) != 2 || len(queries) > rightsizingMaxPromQueries {
		t.Fatalf("queries %d", len(queries))
	}
	for _, q := range queries {
		if !strings.Contains(q, `namespace="ns"`) || !strings.Contains(q, "14d") {
			t.Errorf("query %s", q)
		}
	}
	if (promUsageSource{}).confidence("3d") != "medium" || (promUsageSource{}).confidence("7d") != "high" {
		t.Error("confidence by window")
	}
}

func TestDemoRightsizing(t *testing.T) {
	d := NewDemoProvider()
	rep, err := d.ListRightsizing(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Source != "metrics-snapshot" || rep.Window != "7d" || rep.Confidence != "low" || len(rep.Items) != len(demoRightsizing) {
		t.Fatalf("demo %+v", rep)
	}
	verdicts := map[string]int{}
	for _, it := range rep.Items {
		verdicts[it.Verdict]++
	}
	if verdicts["over"] == 0 || verdicts["under"] == 0 || verdicts["ok"] == 0 || rep.Summary.MonthlySavingsUSD <= 0 {
		t.Fatalf("demo must show over, under and ok rows with savings: %v %+v", verdicts, rep.Summary)
	}
	again, _ := d.ListRightsizing(context.Background(), "", "7d")
	if fmt.Sprint(again) != fmt.Sprint(rep) {
		t.Fatal("demo data must be deterministic")
	}
	one, _ := d.ListRightsizing(context.Background(), "payments", "30d")
	for _, it := range one.Items {
		if it.Namespace != "payments" {
			t.Fatalf("namespace filter: %+v", it)
		}
	}
	if len(one.Items) != 2 || one.Window != "30d" {
		t.Fatalf("payments: %+v", one)
	}
	if _, err := d.ListRightsizing(context.Background(), "", "2d"); HTTPStatus(err) != 400 {
		t.Fatalf("bad window: %v", err)
	}
}
