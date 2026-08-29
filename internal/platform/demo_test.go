package platform

import (
	"context"
	"testing"
)

func TestDemoLifecycle(t *testing.T) {
	ctx := context.Background()
	p := NewDemoProvider()
	w, err := p.Launch(ctx, LaunchRequest{Namespace: "test", Name: "api", Image: "nginx:1.27", Replicas: 2, Port: 80})
	if err != nil {
		t.Fatal(err)
	}
	if w.Desired != 2 || w.Ready != 2 {
		t.Fatalf("unexpected launch state: %+v", w)
	}
	if err := p.Stop(ctx, "test", "Deployment", "api"); err != nil {
		t.Fatal(err)
	}
	ws, _ := p.ListWorkloads(ctx, "test")
	if len(ws) != 1 || ws[0].Desired != 0 || ws[0].Status != "Stopped" {
		t.Fatalf("stop failed: %+v", ws)
	}
	if err := p.Start(ctx, "test", "Deployment", "api"); err != nil {
		t.Fatal(err)
	}
	ws, _ = p.ListWorkloads(ctx, "test")
	if ws[0].Desired != 2 || ws[0].Status != "Healthy" {
		t.Fatalf("start did not restore previous replica count: %+v", ws[0])
	}
	if err := p.Scale(ctx, "test", "Deployment", "api", 5); err != nil {
		t.Fatal(err)
	}
	pods, _ := p.ListPods(ctx, "test")
	if len(pods) != 5 {
		t.Fatalf("expected 5 pods, got %d", len(pods))
	}
}

func TestDemoExec(t *testing.T) {
	p := NewDemoProvider()
	out, err := p.ExecCommand(context.Background(), "payments", "payments-api-01", "payments-api", []string{"whoami"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "axiom" {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestDemoExecShellWrappedCommand(t *testing.T) {
	p := NewDemoProvider()
	out, err := p.ExecCommand(context.Background(), "payments", "payments-api-01", "payments-api", []string{"/bin/sh", "-lc", "whoami"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "axiom" {
		t.Fatalf("unexpected output %q", out)
	}
}

// A huge replica count used to crash the whole process: rebuildPodsLocked
// did make([]Pod, 0, desired) with an unvalidated caller-supplied value,
// and a multi-gigabyte allocation attempt triggers a fatal (unrecoverable)
// Go runtime out-of-memory error, not something an HTTP handler's
// deferred recover() can catch. Found via randomized/exploratory testing:
// POSTing {"replicas": 999999999} to the scale endpoint took the demo
// server down entirely.
func TestScaleRejectsHugeReplicaCount(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	err := d.Scale(ctx, "commerce", "Deployment", "checkout", 999999999)
	if err == nil {
		t.Fatal("expected Scale to reject a replica count above maxDemoReplicas, got nil error")
	}
}

func TestScaleAllowsMaxReplicaCount(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if err := d.Scale(ctx, "commerce", "Deployment", "checkout", maxDemoReplicas); err != nil {
		t.Fatalf("Scale(maxDemoReplicas) should succeed, got %v", err)
	}
	pods, err := d.ListPods(ctx, "commerce")
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	if len(pods) != maxDemoReplicas {
		t.Fatalf("expected %d pods after scaling to the cap, got %d", maxDemoReplicas, len(pods))
	}
}

// A duplicate Launch for the same namespace/kind/name used to silently
// overwrite the existing workload and report success both times, unlike
// a real cluster's API server which rejects the second create with
// AlreadyExists. Found by rapid-clicking the Launch button three times
// in the browser before the button had a chance to disable — all three
// POSTs returned 201.
func TestLaunchRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := LaunchRequest{Namespace: "default", Name: "race-launch-test", Image: "nginx:latest", Replicas: 1}
	if _, err := d.Launch(ctx, req); err != nil {
		t.Fatalf("first Launch should succeed, got %v", err)
	}
	if _, err := d.Launch(ctx, req); err == nil {
		t.Fatal("expected second Launch with the same name to fail, got nil error")
	}
}

func TestLaunchRejectsHugeReplicaCount(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	_, err := d.Launch(ctx, LaunchRequest{Namespace: "default", Name: "huge", Image: "nginx:latest", Replicas: 999999999})
	if err == nil {
		t.Fatal("expected Launch to reject a replica count above maxDemoReplicas, got nil error")
	}
}

// rebuildPodsLocked itself must never allocate beyond the cap, even if a
// future call site somehow skips the Scale/Launch validation — this is
// the defense-in-depth clamp, tested directly against the internal
// struct field rather than through Scale/Launch's own validation.
func TestRebuildPodsLockedClampsAllocation(t *testing.T) {
	d := NewDemoProvider()
	d.mu.Lock()
	w := Workload{Namespace: "default", Name: "unchecked", Kind: "Deployment", Desired: 999999999}
	d.workloads[key(w.Namespace, w.Kind, w.Name)] = w
	d.rebuildPodsLocked(w)
	d.mu.Unlock()

	pods, err := d.ListPods(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	count := 0
	for _, p := range pods {
		if p.WorkloadID == "unchecked" {
			count++
		}
	}
	if count != maxDemoReplicas {
		t.Fatalf("rebuildPodsLocked should clamp to %d pods even with an unvalidated Desired, got %d", maxDemoReplicas, count)
	}
}
