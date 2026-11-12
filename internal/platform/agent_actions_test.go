package platform

import (
	"context"
	"testing"
)

func TestSanitizeAgentPlanDropsUnknownActions(t *testing.T) {
	p := SanitizeAgentPlan(&AgentPlan{
		Summary: "x",
		Actions: []AgentAction{
			{Type: "rm -rf /", Description: "bad"},
			{Type: AgentActionRestartPod, Description: "ok"},
		},
	})
	if len(p.Actions) != 1 || p.Actions[0].Type != AgentActionRestartPod {
		t.Fatalf("got %#v", p.Actions)
	}
}

func TestDemoApproveRequiresPending(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	run, err := d.CreateAgentRun(ctx, CreateAgentRunRequest{Namespace: "default", Name: "once", Goal: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ApproveAgentRun(ctx, run.Namespace, run.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ApproveAgentRun(ctx, run.Namespace, run.Name); err == nil {
		t.Fatal("expected conflict on second approve")
	}
}

func TestDemoCreatePlansDeterministically(t *testing.T) {
	d := NewDemoProvider()
	run, err := d.CreateAgentRun(context.Background(), CreateAgentRunRequest{
		Namespace: "default", Name: "crash-1", Goal: "fix CrashLoopBackOff",
		Target: &AgentObjectRef{Kind: "Pod", Name: "x", Namespace: "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.Plan == nil || len(run.Plan.Actions) == 0 {
		t.Fatal("expected plan")
	}
	if !run.OwnedByAxiom {
		t.Fatal("expected OwnedByAxiom")
	}
}
