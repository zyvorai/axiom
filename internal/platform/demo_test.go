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
