package platform

import (
	"context"
	"strings"
	"testing"
)

func TestPlanInPlacePod(t *testing.T) {
	both := []string{"cpu", "memory"}
	item := rsItem("over", 500, 250, 512, 288)

	// Burstable pod: same plan as the template apply, limits never lowered.
	cur := rightsizingCurrentResources{CPURequestMilli: 500, CPULimitMilli: 2000, MemRequestBytes: 512 * mib, MemLimitBytes: 1024 * mib}
	p, err := planInPlacePod(cur, item, both, false)
	if err != nil || len(p.Changes) != 2 || p.NewCPULimitMilli != 0 || p.NewMemLimitBytes != 0 {
		t.Fatalf("burstable: %+v %v", p, err)
	}
	b, _ := p.podPatchJSON("api")
	if string(b) != `{"spec":{"containers":[{"name":"api","resources":{"requests":{"cpu":"250m","memory":"288Mi"}}}]}}` {
		t.Fatalf("pod patch %s", b)
	}

	// Guaranteed pod: lowering a request would change the QoS class.
	g := rightsizingCurrentResources{CPURequestMilli: 500, CPULimitMilli: 500, MemRequestBytes: 512 * mib, MemLimitBytes: 512 * mib}
	if _, err := planInPlacePod(g, item, both, true); err == nil || !strings.Contains(err.Error(), "Guaranteed") {
		t.Fatalf("guaranteed decrease must be refused: %v", err)
	}
	// ... but raising it raises the limit with it and stays Guaranteed.
	up := rsItem("under", 500, 800, 512, 600)
	if p, err := planInPlacePod(g, up, both, true); err != nil || p.NewCPULimitMilli != 800 || p.NewMemLimitBytes != 600*mib {
		t.Fatalf("guaranteed increase: %+v %v", p, err)
	}

	// Already right: empty plan.
	same := rightsizingCurrentResources{CPURequestMilli: 250, MemRequestBytes: 288 * mib}
	if p, err := planInPlacePod(same, item, both, false); err != nil || len(p.Changes) != 0 {
		t.Fatalf("unchanged: %+v %v", p, err)
	}
}

func TestInPlaceRestartsContainerAndSummary(t *testing.T) {
	p := rightsizingPlan{NewCPURequestMilli: 250}
	if !inPlaceRestartsContainer(p, map[string]string{"cpu": "RestartContainer"}) {
		t.Fatal("cpu restart policy not detected")
	}
	if inPlaceRestartsContainer(p, map[string]string{"memory": "RestartContainer", "cpu": "NotRequired"}) {
		t.Fatal("memory policy must not matter for a cpu-only change")
	}
	got := InPlaceSummary([]RightsizingPodResize{{Status: inPlaceResized}, {Status: inPlaceResized}, {Status: inPlaceSkipped}})
	if got != "2 resized, 1 skipped" {
		t.Fatalf("summary %q", got)
	}
}

func TestDemoApplyRightsizingInPlace(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := RightsizingApplyRequest{Namespace: "payments", Kind: "Deployment", Workload: "payments-api", Container: "api",
		Resources: []string{"cpu", "memory"}, DryRun: true, InPlace: true}
	dry, err := d.ApplyRightsizing(ctx, req)
	if err != nil || !dry.InPlace || !dry.DryRun || len(dry.Pods) == 0 || !strings.Contains(dry.Rollout, "without a restart") {
		t.Fatalf("dry run %+v %v", dry, err)
	}
	if demoItem(t, d, "payments", "payments-api", "api").Verdict != "over" {
		t.Fatal("dry run wrote something")
	}
	if st := d.demoPodResizeStatus("payments", "payments-api", 1); st != nil {
		t.Fatal("no resize status before a real apply")
	}
	req.DryRun = false
	res, err := d.ApplyRightsizing(ctx, req)
	if err != nil || !res.InPlace || res.Pods[0].Status != inPlaceResized {
		t.Fatalf("apply %+v %v", res, err)
	}
	if demoItem(t, d, "payments", "payments-api", "api").Verdict != "ok" {
		t.Fatal("requests not updated")
	}
	if st := d.demoPodResizeStatus("payments", "payments-api", 1); st == nil || len(st["conditions"].([]any)) != 2 {
		t.Fatalf("resize status %+v", st)
	}
}
