package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNormalizeCreateHPARequestValidation(t *testing.T) {
	base := func() CreateHPARequest {
		return CreateHPARequest{Namespace: "shop", Name: "web-hpa", TargetKind: "Deployment", TargetName: "web", MinReplicas: 2, MaxReplicas: 5, TargetCPUPercent: 60}
	}
	bad := map[string]func(*CreateHPARequest){
		"missing namespace":      func(r *CreateHPARequest) { r.Namespace = "" },
		"missing name":           func(r *CreateHPARequest) { r.Name = "" },
		"missing target kind":    func(r *CreateHPARequest) { r.TargetKind = "" },
		"missing target name":    func(r *CreateHPARequest) { r.TargetName = "" },
		"namespace not dns":      func(r *CreateHPARequest) { r.Namespace = "Shop_1" },
		"namespace with dot":     func(r *CreateHPARequest) { r.Namespace = "a.b" },
		"name not dns":           func(r *CreateHPARequest) { r.Name = "Web HPA" },
		"name too long":          func(r *CreateHPARequest) { r.Name = strings.Repeat("a", 254) },
		"target name not dns":    func(r *CreateHPARequest) { r.TargetName = "web/../x" },
		"unsupported kind":       func(r *CreateHPARequest) { r.TargetKind = "DaemonSet" },
		"lowercase kind":         func(r *CreateHPARequest) { r.TargetKind = "deployment" },
		"negative min":           func(r *CreateHPARequest) { r.MinReplicas = -1 },
		"max below min":          func(r *CreateHPARequest) { r.MinReplicas, r.MaxReplicas = 4, 2 },
		"max above 1000":         func(r *CreateHPARequest) { r.MaxReplicas = 1001 },
		"negative max":           func(r *CreateHPARequest) { r.MaxReplicas = -3 },
		"cpu above 100":          func(r *CreateHPARequest) { r.TargetCPUPercent = 101 },
		"cpu negative":           func(r *CreateHPARequest) { r.TargetCPUPercent = -5 },
		"memory above 100":       func(r *CreateHPARequest) { r.TargetMemoryPercent = 150 },
		"memory negative":        func(r *CreateHPARequest) { r.TargetMemoryPercent = -1 },
		"stabilization > 3600":   func(r *CreateHPARequest) { r.ScaleDownStabilizationSeconds = 3601 },
		"stabilization negative": func(r *CreateHPARequest) { r.ScaleDownStabilizationSeconds = -1 },
	}
	for name, mut := range bad {
		t.Run(name, func(t *testing.T) {
			r := base()
			mut(&r)
			if _, err := normalizeCreateHPARequest(r); err == nil || HTTPStatus(err) != http.StatusBadRequest {
				t.Fatalf("want a 400, got %v", err)
			}
		})
	}

	good := map[string]struct {
		mut   func(*CreateHPARequest)
		check func(CreateHPARequest) bool
	}{
		"as given": {func(*CreateHPARequest) {}, func(r CreateHPARequest) bool {
			return r.MinReplicas == 2 && r.MaxReplicas == 5 && r.TargetCPUPercent == 60
		}},
		"min defaults to 1":    {func(r *CreateHPARequest) { r.MinReplicas = 0 }, func(r CreateHPARequest) bool { return r.MinReplicas == 1 }},
		"max defaults to min":  {func(r *CreateHPARequest) { r.MaxReplicas = 0 }, func(r CreateHPARequest) bool { return r.MaxReplicas == 2 }},
		"max 1000 ok":          {func(r *CreateHPARequest) { r.MaxReplicas = 1000 }, func(r CreateHPARequest) bool { return r.MaxReplicas == 1000 }},
		"no target → cpu 80":   {func(r *CreateHPARequest) { r.TargetCPUPercent = 0 }, func(r CreateHPARequest) bool { return r.TargetCPUPercent == 80 && r.TargetMemoryPercent == 0 }},
		"memory only":          {func(r *CreateHPARequest) { r.TargetCPUPercent, r.TargetMemoryPercent = 0, 70 }, func(r CreateHPARequest) bool { return r.TargetCPUPercent == 0 && r.TargetMemoryPercent == 70 }},
		"statefulset":          {func(r *CreateHPARequest) { r.TargetKind = "StatefulSet" }, func(r CreateHPARequest) bool { return true }},
		"replicaset":           {func(r *CreateHPARequest) { r.TargetKind = "ReplicaSet" }, func(r CreateHPARequest) bool { return true }},
		"subdomain name":       {func(r *CreateHPARequest) { r.Name = "web.v2" }, func(r CreateHPARequest) bool { return true }},
		"stabilization bounds": {func(r *CreateHPARequest) { r.ScaleDownStabilizationSeconds = 3600 }, func(r CreateHPARequest) bool { return r.ScaleDownStabilizationSeconds == 3600 }},
	}
	for name, tc := range good {
		t.Run(name, func(t *testing.T) {
			r := base()
			tc.mut(&r)
			got, err := normalizeCreateHPARequest(r)
			if err != nil || !tc.check(got) {
				t.Fatalf("got %+v err=%v", got, err)
			}
		})
	}
}

func TestKedaTriggerName(t *testing.T) {
	cases := []struct {
		name string
		md   map[string]any
		want string
	}{
		{"explicit", map[string]any{"metricName": "m"}, "explicit"},
		{"", map[string]any{"metricName": "rps", "queueName": "q"}, "rps"},
		{"", map[string]any{"queueName": "orders", "host": "amqp://u:p@h"}, "orders"},
		{"", map[string]any{"topic": "events", "bootstrapServers": "k:9092"}, "events"},
		{"", map[string]any{"password": "hunter2", "connectionString": "secret"}, ""},
		{"", map[string]any{"metricName": 42}, ""},
		{"", nil, ""},
	}
	for _, c := range cases {
		if got := kedaTriggerName(c.name, c.md); got != c.want {
			t.Errorf("kedaTriggerName(%q, %v) = %q, want %q", c.name, c.md, got, c.want)
		}
	}
}

func TestDemoHPAsRenderBeyondCPU(t *testing.T) {
	d := NewDemoProvider()
	list, err := d.ListHPAs(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]HPASummary{}
	for _, h := range list {
		by[h.Namespace+"/"+h.Name] = h
		if h.Metrics == nil || h.Conditions == nil {
			t.Errorf("%s/%s: metrics/conditions must be non-nil arrays", h.Namespace, h.Name)
		}
	}
	co := by["commerce/checkout"]
	if len(co.Metrics) != 2 || co.Metrics[1].Name != "memory" || co.Metrics[1].Target != "80%" || co.TargetCPUPercent != 75 || !co.Behavior || len(co.Conditions) != 3 {
		t.Fatalf("checkout: %+v", co)
	}
	otel := by["observability/otel-collector"]
	var ext, pods bool
	for _, m := range otel.Metrics {
		ext = ext || (m.Type == "External" && m.Current != "")
		pods = pods || (m.Type == "Pods" && m.Current == "")
	}
	if !ext || !pods || otel.TargetCPUPercent != 0 {
		t.Fatalf("otel-collector: %+v", otel)
	}
	// The new HPA must not add health findings (its target exists and has headroom).
	r, _ := d.ClusterHealth(context.Background(), "observability")
	for _, f := range r.Findings {
		if f.Analyzer == AnalyzerHPA {
			t.Errorf("unexpected HPA finding: %+v", f)
		}
	}
	if _, err := d.ListHPAs(context.Background(), "commerce"); err != nil {
		t.Fatal(err)
	}
}

func TestDemoCreateHPAWithMemoryAndStabilization(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateHPARequest{Namespace: "commerce", Name: "mem-cpu-hpa", TargetKind: "Deployment", TargetName: "checkout",
		MinReplicas: 2, MaxReplicas: 6, TargetCPUPercent: 60, TargetMemoryPercent: 75, ScaleDownStabilizationSeconds: 300}
	h, err := d.CreateHPA(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteHPA(ctx, "commerce", "mem-cpu-hpa") })
	if len(h.Metrics) != 2 || h.Metrics[0].Name != "cpu" || h.Metrics[1].Name != "memory" || h.Metrics[1].Target != "75%" || !h.Behavior || h.TargetCPUPercent != 60 {
		t.Fatalf("summary: %+v", h)
	}
	if _, err := d.CreateHPA(ctx, CreateHPARequest{Namespace: "commerce", Name: "x", TargetKind: "Deployment", TargetName: "checkout", TargetMemoryPercent: 101}); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("invalid memory target: %v", err)
	}
	memOnly, err := d.CreateHPA(ctx, CreateHPARequest{Namespace: "commerce", Name: "mem-only-hpa", TargetKind: "Deployment", TargetName: "checkout", TargetMemoryPercent: 70})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteHPA(ctx, "commerce", "mem-only-hpa") })
	if memOnly.TargetCPUPercent != 0 || len(memOnly.Metrics) != 1 || memOnly.Metrics[0].Name != "memory" || memOnly.Behavior {
		t.Fatalf("memory-only: %+v", memOnly)
	}
}

func TestDemoVPAsAndScaledObjects(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	v, err := d.ListVPAs(ctx, "")
	if err != nil || !v.Installed || len(v.Items) != 2 {
		t.Fatalf("vpas: %+v err=%v", v, err)
	}
	var auto bool
	for _, it := range v.Items {
		if it.UpdateMode == "Auto" && len(it.Containers) == 1 && it.Containers[0].Target.CPU == "1200m" {
			auto = true
		}
	}
	if !auto {
		t.Fatalf("expected an Auto-mode VPA with a large recommendation: %+v", v.Items)
	}
	if v, _ := d.ListVPAs(ctx, "data"); len(v.Items) != 1 || v.Items[0].Name != "redis-vpa" {
		t.Fatalf("namespace filter: %+v", v)
	}
	if v, _ := d.ListVPAs(ctx, "nowhere"); !v.Installed || v.Items == nil || len(v.Items) != 0 {
		t.Fatalf("empty namespace: %+v", v)
	}

	s, err := d.ListScaledObjects(ctx, "all")
	if err != nil || !s.Installed || len(s.Items) != 2 {
		t.Fatalf("scaledobjects: %+v err=%v", s, err)
	}
	var active, inactive int
	for _, it := range s.Items {
		if it.Active {
			active++
		} else {
			inactive++
		}
	}
	if active != 1 || inactive != 1 {
		t.Fatalf("want one active and one inactive ScaledObject: %+v", s.Items)
	}
	if s, _ := d.ListScaledObjects(ctx, "payments"); len(s.Items) != 1 || s.Items[0].Triggers[0].Name != "orders-lag" {
		t.Fatalf("namespace filter: %+v", s)
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "metadata") || strings.Contains(string(b), "authenticationRef") {
		t.Fatalf("demo ScaledObject JSON leaks trigger detail: %s", b)
	}
}
