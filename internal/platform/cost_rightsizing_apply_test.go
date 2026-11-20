package platform

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func rsItem(verdict string, cpuReq, cpuRec, memReqMiB, memRecMiB int64) RightsizingItem {
	return RightsizingItem{
		Verdict: verdict,
		CPU:     RightsizingCPU{RequestMilli: cpuReq, RecommendedMilli: cpuRec},
		Memory:  RightsizingMemory{RequestMiB: memReqMiB, RecommendedMiB: memRecMiB},
	}
}

func TestPlanRightsizingChanges(t *testing.T) {
	both := []string{"cpu", "memory"}
	cases := []struct {
		name      string
		cur       rightsizingCurrentResources
		item      RightsizingItem
		resources []string
		want      []RightsizingChange
		patch     string
	}{
		{
			name:      "requests only, both lowered",
			cur:       rightsizingCurrentResources{CPURequestMilli: 500, MemRequestBytes: 512 * mib},
			item:      rsItem("over", 500, 250, 512, 288),
			resources: both,
			want: []RightsizingChange{
				{Resource: "cpu", Field: "requests", From: "500m", To: "250m"},
				{Resource: "memory", Field: "requests", From: "512Mi", To: "288Mi"},
			},
			patch: `{"spec":{"template":{"spec":{"containers":[{"name":"api","resources":{"requests":{"cpu":"250m","memory":"288Mi"}}}]}}}}`,
		},
		{
			name:      "limit below the new request is raised",
			cur:       rightsizingCurrentResources{CPURequestMilli: 100, CPULimitMilli: 200, MemRequestBytes: 128 * mib, MemLimitBytes: 256 * mib},
			item:      rsItem("under", 100, 300, 128, 288),
			resources: both,
			want: []RightsizingChange{
				{Resource: "cpu", Field: "requests", From: "100m", To: "300m"},
				{Resource: "cpu", Field: "limits", From: "200m", To: "300m", Note: "limit raised to stay >= the request"},
				{Resource: "memory", Field: "requests", From: "128Mi", To: "288Mi"},
				{Resource: "memory", Field: "limits", From: "256Mi", To: "288Mi", Note: "limit raised to stay >= the request"},
			},
			patch: `{"spec":{"template":{"spec":{"containers":[{"name":"api","resources":{"limits":{"cpu":"300m","memory":"288Mi"},"requests":{"cpu":"300m","memory":"288Mi"}}}]}}}}`,
		},
		{
			name:      "limit already above stays (never lowered)",
			cur:       rightsizingCurrentResources{CPURequestMilli: 2000, CPULimitMilli: 4000, MemRequestBytes: 4096 * mib, MemLimitBytes: 8192 * mib},
			item:      rsItem("over", 2000, 420, 4096, 1040),
			resources: both,
			want: []RightsizingChange{
				{Resource: "cpu", Field: "requests", From: "2", To: "420m"},
				{Resource: "memory", Field: "requests", From: "4Gi", To: "1040Mi"},
			},
		},
		{
			name:      "limit equal to the new request stays",
			cur:       rightsizingCurrentResources{CPURequestMilli: 100, CPULimitMilli: 300},
			item:      rsItem("under", 100, 300, 0, 0),
			resources: []string{"cpu"},
			want:      []RightsizingChange{{Resource: "cpu", Field: "requests", From: "100m", To: "300m"}},
		},
		{
			name:      "no limit stays none when raising",
			cur:       rightsizingCurrentResources{CPURequestMilli: 100, MemRequestBytes: 64 * mib},
			item:      rsItem("under", 100, 1500, 64, 2048),
			resources: both,
			want: []RightsizingChange{
				{Resource: "cpu", Field: "requests", From: "100m", To: "1500m"},
				{Resource: "memory", Field: "requests", From: "64Mi", To: "2Gi"},
			},
			patch: `{"spec":{"template":{"spec":{"containers":[{"name":"api","resources":{"requests":{"cpu":"1500m","memory":"2Gi"}}}]}}}}`,
		},
		{
			name:      "no existing request is set",
			cur:       rightsizingCurrentResources{},
			item:      rsItem("under", 0, 20, 0, 48),
			resources: both,
			want: []RightsizingChange{
				{Resource: "cpu", Field: "requests", From: "none", To: "20m"},
				{Resource: "memory", Field: "requests", From: "none", To: "48Mi"},
			},
		},
		{
			name:      "cpu only",
			cur:       rightsizingCurrentResources{CPURequestMilli: 1000, MemRequestBytes: 4096 * mib, MemLimitBytes: 1024 * mib},
			item:      rsItem("over", 1000, 150, 4096, 8192),
			resources: []string{"cpu"},
			want:      []RightsizingChange{{Resource: "cpu", Field: "requests", From: "1", To: "150m"}},
			patch:     `{"spec":{"template":{"spec":{"containers":[{"name":"api","resources":{"requests":{"cpu":"150m"}}}]}}}}`,
		},
		{
			name:      "memory only",
			cur:       rightsizingCurrentResources{CPURequestMilli: 1000, MemRequestBytes: 4096 * mib},
			item:      rsItem("over", 1000, 150, 4096, 808),
			resources: []string{"memory"},
			want:      []RightsizingChange{{Resource: "memory", Field: "requests", From: "4Gi", To: "808Mi"}},
			patch:     `{"spec":{"template":{"spec":{"containers":[{"name":"api","resources":{"requests":{"memory":"808Mi"}}}]}}}}`,
		},
		{
			name:      "unchanged resource is skipped",
			cur:       rightsizingCurrentResources{CPURequestMilli: 250, MemRequestBytes: 512 * mib},
			item:      rsItem("over", 250, 250, 512, 288),
			resources: both,
			want:      []RightsizingChange{{Resource: "memory", Field: "requests", From: "512Mi", To: "288Mi"}},
		},
		{
			name:      "unknown recommendation (0) leaves the resource alone",
			cur:       rightsizingCurrentResources{},
			item:      rsItem("under", 0, 0, 0, 48),
			resources: both,
			want:      []RightsizingChange{{Resource: "memory", Field: "requests", From: "none", To: "48Mi"}},
		},
		{
			name:      "decimal and odd units render exactly",
			cur:       rightsizingCurrentResources{CPURequestMilli: 2500, MemRequestBytes: 1_000_000_000},
			item:      rsItem("over", 2500, 3000, 954, 1024),
			resources: both,
			want: []RightsizingChange{
				{Resource: "cpu", Field: "requests", From: "2500m", To: "3"},
				{Resource: "memory", Field: "requests", From: "1G", To: "1Gi"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := planRightsizingChanges(tc.cur, tc.item, tc.resources)
			if !reflect.DeepEqual(p.Changes, tc.want) {
				t.Fatalf("changes\n got %+v\nwant %+v", p.Changes, tc.want)
			}
			if tc.patch != "" {
				b, err := p.patchJSON("api")
				if err != nil || string(b) != tc.patch {
					t.Fatalf("patch\n got %s (%v)\nwant %s", b, err, tc.patch)
				}
			}
			// applyTo leaves the new request in place and limits never below it.
			after := p.applyTo(tc.cur)
			for _, c := range p.Changes {
				if c.Field != "requests" {
					continue
				}
				switch c.Resource {
				case "cpu":
					if after.CPURequestMilli != tc.item.CPU.RecommendedMilli || (after.CPULimitMilli != 0 && after.CPULimitMilli < after.CPURequestMilli) {
						t.Fatalf("cpu after apply %+v", after)
					}
					if (tc.cur.CPULimitMilli == 0) != (after.CPULimitMilli == 0) || after.CPULimitMilli < tc.cur.CPULimitMilli {
						t.Fatalf("cpu limit added or lowered: %+v -> %+v", tc.cur, after)
					}
				case "memory":
					if after.MemRequestBytes != tc.item.Memory.RecommendedMiB*mib || (after.MemLimitBytes != 0 && after.MemLimitBytes < after.MemRequestBytes) {
						t.Fatalf("memory after apply %+v", after)
					}
					if (tc.cur.MemLimitBytes == 0) != (after.MemLimitBytes == 0) || after.MemLimitBytes < tc.cur.MemLimitBytes {
						t.Fatalf("memory limit added or lowered: %+v -> %+v", tc.cur, after)
					}
				}
			}
		})
	}
}

func TestFormatRightsizingQuantities(t *testing.T) {
	cpu := map[int64]string{10: "10m", 250: "250m", 1000: "1", 1500: "1500m", 4000: "4"}
	for in, want := range cpu {
		if got := formatCPUMilli(in); got != want {
			t.Errorf("formatCPUMilli(%d) = %q, want %q", in, got, want)
		}
	}
	mem := map[int64]string{16 * mib: "16Mi", 288 * mib: "288Mi", 1024 * mib: "1Gi", 1536 * mib: "1536Mi", 8192 * mib: "8Gi",
		512 * 1024: "512Ki", 500_000_000: "500M", 2_000_000_000: "2G", 1234: "1234"}
	for in, want := range mem {
		if got := formatMemoryBytes(in); got != want {
			t.Errorf("formatMemoryBytes(%d) = %q, want %q", in, got, want)
		}
	}
	if formatCPUMilliOrNone(0) != "none" || formatMemoryBytesOrNone(0) != "none" {
		t.Error("an unset quantity renders as none")
	}
}

func TestNormalizeRightsizingApplyRequest(t *testing.T) {
	good := RightsizingApplyRequest{Namespace: "payments", Kind: "Deployment", Workload: "payments-api", Container: "api", Resources: []string{"memory", "cpu", "memory"}}
	got, err := NormalizeRightsizingApplyRequest(good)
	if err != nil || got.Window != "7d" || !reflect.DeepEqual(got.Resources, []string{"cpu", "memory"}) {
		t.Fatalf("normalized %+v %v", got, err)
	}
	neg := int64(-1)
	bad := map[string]func(*RightsizingApplyRequest){
		"no namespace":      func(r *RightsizingApplyRequest) { r.Namespace = "" },
		"bad namespace":     func(r *RightsizingApplyRequest) { r.Namespace = "Pay_ments" },
		"bad kind":          func(r *RightsizingApplyRequest) { r.Kind = "Job" },
		"lowercase kind":    func(r *RightsizingApplyRequest) { r.Kind = "deployment" },
		"no workload":       func(r *RightsizingApplyRequest) { r.Workload = "" },
		"bad workload":      func(r *RightsizingApplyRequest) { r.Workload = "API/x" },
		"bad container":     func(r *RightsizingApplyRequest) { r.Container = "a.b" },
		"no container":      func(r *RightsizingApplyRequest) { r.Container = "" },
		"no resources":      func(r *RightsizingApplyRequest) { r.Resources = nil },
		"unknown resource":  func(r *RightsizingApplyRequest) { r.Resources = []string{"cpu", "gpu"} },
		"bad window":        func(r *RightsizingApplyRequest) { r.Window = "9d" },
		"negative observed": func(r *RightsizingApplyRequest) { r.Observed = &RightsizingObserved{CPURequestMilli: &neg} },
	}
	for name, mut := range bad {
		r := good
		mut(&r)
		if _, err := NormalizeRightsizingApplyRequest(r); HTTPStatus(err) != 400 {
			t.Errorf("%s: want 400, got %v", name, err)
		}
	}
	r := good
	r.Workload, r.Window = "web.v2", "30d"
	if got, err := NormalizeRightsizingApplyRequest(r); err != nil || got.Window != "30d" {
		t.Fatalf("dotted workload / window: %+v %v", got, err)
	}
}

func TestPrepareRightsizingApply(t *testing.T) {
	req, _ := NormalizeRightsizingApplyRequest(RightsizingApplyRequest{Namespace: "payments", Kind: "Deployment", Workload: "payments-api", Container: "api", Resources: []string{"cpu", "memory"}})
	cur := rightsizingCurrentResources{CPURequestMilli: 500, MemRequestBytes: 512 * mib}
	item := rsItem("over", 500, 250, 512, 288)

	cpu, memMiB := int64(500), int64(512)
	req.Observed = &RightsizingObserved{CPURequestMilli: &cpu, MemoryRequestMiB: &memMiB}
	if p, err := prepareRightsizingApply(req, item, cur); err != nil || len(p.Changes) != 2 {
		t.Fatalf("matching observed: %+v %v", p, err)
	}
	stale := int64(400)
	req.Observed = &RightsizingObserved{CPURequestMilli: &stale}
	_, err := prepareRightsizingApply(req, item, cur)
	if HTTPStatus(err) != 409 || !strings.Contains(err.Error(), "the live requests changed since you looked") || !strings.Contains(err.Error(), "500m") {
		t.Fatalf("stale: %v", err)
	}
	req.Observed = &RightsizingObserved{MemoryRequestMiB: &stale}
	if _, err := prepareRightsizingApply(req, item, cur); HTTPStatus(err) != 409 {
		t.Fatalf("stale memory: %v", err)
	}
	req.Observed = nil
	if _, err := prepareRightsizingApply(req, rsItem("ok", 500, 500, 512, 512), cur); HTTPStatus(err) != 409 || !strings.Contains(err.Error(), "already right-sized") {
		t.Fatalf("ok verdict: %v", err)
	}
	// Over on memory only, but the caller picked cpu: nothing to change.
	req.Resources = []string{"cpu"}
	if _, err := prepareRightsizingApply(req, rsItem("over", 500, 500, 512, 288), cur); HTTPStatus(err) != 409 {
		t.Fatalf("nothing to change: %v", err)
	}
}

func TestRightsizingApplyResultJSONAndSummary(t *testing.T) {
	res := RightsizingApplyResult{OK: true, DryRun: true, Verdict: "over", Source: "metrics-snapshot", Confidence: "low",
		Rollout: rightsizingRolloutNote("Deployment", "", false),
		Changes: []RightsizingChange{
			{Resource: "cpu", Field: "requests", From: "500m", To: "250m"},
			{Resource: "memory", Field: "limits", From: "256Mi", To: "288Mi", Note: rightsizingLimitRaisedNote},
		}}
	b, _ := json.Marshal(res)
	want := `{"ok":true,"dryRun":true,"changes":[{"resource":"cpu","field":"requests","from":"500m","to":"250m"},{"resource":"memory","field":"limits","from":"256Mi","to":"288Mi","note":"limit raised to stay >= the request"}],"rollout":"Deployments restart their pods with the new values; expect a rolling update","verdict":"over","source":"metrics-snapshot","confidence":"low"}`
	// encoding/json escapes '>' as \u003e; clients decode it back.
	if strings.ReplaceAll(string(b), `\u003e`, ">") != want {
		t.Fatalf("json\n got %s\nwant %s", b, want)
	}
	if s := res.Summary(); s != "cpu requests 500m->250m, memory limits 256Mi->288Mi" {
		t.Fatalf("summary %q", s)
	}
	for _, k := range []string{"StatefulSet", "DaemonSet"} {
		if n := rightsizingRolloutNote(k, "OnDelete", false); !strings.Contains(n, "OnDelete") {
			t.Errorf("%s OnDelete note %q", k, n)
		}
		if n := rightsizingRolloutNote(k, "RollingUpdate", false); n == "" || strings.Contains(n, "OnDelete") {
			t.Errorf("%s note %q", k, n)
		}
	}
	if n := rightsizingRolloutNote("Deployment", "", true); !strings.Contains(n, "paused") {
		t.Errorf("paused note %q", n)
	}
}

func demoItem(t *testing.T, d *DemoProvider, ns, workload, container string) RightsizingItem {
	t.Helper()
	rep, err := d.ListRightsizing(context.Background(), ns, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range rep.Items {
		if it.Workload == workload && it.Container == container {
			return it
		}
	}
	t.Fatalf("no item %s/%s/%s in %+v", ns, workload, container, rep.Items)
	return RightsizingItem{}
}

func TestDemoApplyRightsizingRoundTrip(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	before := demoItem(t, d, "payments", "payments-api", "api")
	if before.Verdict != "over" {
		t.Fatalf("seed verdict %+v", before)
	}
	cpu, mem := before.CPU.RequestMilli, before.Memory.RequestMiB
	req := RightsizingApplyRequest{Namespace: "payments", Kind: "Deployment", Workload: "payments-api", Container: "api",
		Resources: []string{"cpu", "memory"}, DryRun: true, Observed: &RightsizingObserved{CPURequestMilli: &cpu, MemoryRequestMiB: &mem}}

	dry, err := d.ApplyRightsizing(ctx, req)
	if err != nil || !dry.OK || !dry.DryRun || len(dry.Changes) != 2 || dry.Verdict != "over" || dry.Source != "metrics-snapshot" || dry.Confidence != "low" {
		t.Fatalf("dry run %+v %v", dry, err)
	}
	if dry.Changes[0] != (RightsizingChange{Resource: "cpu", Field: "requests", From: "2", To: "420m"}) ||
		dry.Changes[1] != (RightsizingChange{Resource: "memory", Field: "requests", From: "4Gi", To: "1040Mi"}) {
		t.Fatalf("dry-run changes (limits stay, never lowered) %+v", dry.Changes)
	}
	if again := demoItem(t, d, "payments", "payments-api", "api"); again.Verdict != "over" || again.CPU.RequestMilli != cpu {
		t.Fatalf("dry run wrote something: %+v", again)
	}

	req.DryRun = false
	res, err := d.ApplyRightsizing(ctx, req)
	if err != nil || res.DryRun || !reflect.DeepEqual(res.Changes, dry.Changes) {
		t.Fatalf("apply %+v %v", res, err)
	}
	after := demoItem(t, d, "payments", "payments-api", "api")
	if after.Verdict != "ok" || after.CPU.RequestMilli != before.CPU.RecommendedMilli || after.Memory.RequestMiB != before.Memory.RecommendedMiB {
		t.Fatalf("after apply %+v", after)
	}
	// Second apply: the view the caller had is stale now.
	if _, err := d.ApplyRightsizing(ctx, req); HTTPStatus(err) != 409 || !strings.Contains(err.Error(), "changed since you looked") {
		t.Fatalf("second apply with old observed: %v", err)
	}
	req.Observed = nil
	if _, err := d.ApplyRightsizing(ctx, req); HTTPStatus(err) != 409 || !strings.Contains(err.Error(), "already right-sized") {
		t.Fatalf("second apply: %v", err)
	}
	// Another provider instance is unaffected.
	if other := demoItem(t, NewDemoProvider(), "payments", "payments-api", "api"); other.Verdict != "over" {
		t.Fatalf("overrides leaked across providers: %+v", other)
	}

	// Under-provisioned with a limit at the request: the limit is raised.
	redis, err := d.ApplyRightsizing(ctx, RightsizingApplyRequest{Namespace: "data", Kind: "StatefulSet", Workload: "redis", Container: "redis", Resources: []string{"memory"}, DryRun: true})
	if err != nil || len(redis.Changes) != 2 || redis.Changes[1].Field != "limits" || redis.Changes[1].Note != rightsizingLimitRaisedNote || redis.Verdict != "under" ||
		!strings.Contains(redis.Rollout, "StatefulSets") {
		t.Fatalf("redis %+v %v", redis, err)
	}
	// No request set at all.
	ne, err := d.ApplyRightsizing(ctx, RightsizingApplyRequest{Namespace: "observability", Kind: "DaemonSet", Workload: "node-exporter", Container: "node-exporter", Resources: []string{"cpu", "memory"}})
	if err != nil || len(ne.Changes) != 2 || ne.Changes[0].From != "none" || ne.Changes[1].From != "none" {
		t.Fatalf("node-exporter %+v %v", ne, err)
	}
	if it := demoItem(t, d, "observability", "node-exporter", "node-exporter"); it.Verdict != "ok" {
		t.Fatalf("node-exporter after apply %+v", it)
	}

	for name, r := range map[string]RightsizingApplyRequest{
		"unknown workload":  {Namespace: "payments", Kind: "Deployment", Workload: "nope", Container: "api", Resources: []string{"cpu"}},
		"unknown container": {Namespace: "payments", Kind: "Deployment", Workload: "payments-api", Container: "nope", Resources: []string{"cpu"}},
		"wrong kind":        {Namespace: "data", Kind: "Deployment", Workload: "redis", Container: "redis", Resources: []string{"cpu"}},
	} {
		if _, err := d.ApplyRightsizing(ctx, r); HTTPStatus(err) != 404 {
			t.Errorf("%s: want 404, got %v", name, err)
		}
	}
	if _, err := d.ApplyRightsizing(ctx, RightsizingApplyRequest{Namespace: "payments", Kind: "Job", Workload: "x", Container: "y", Resources: []string{"cpu"}}); HTTPStatus(err) != 400 {
		t.Errorf("bad kind: %v", err)
	}
}
