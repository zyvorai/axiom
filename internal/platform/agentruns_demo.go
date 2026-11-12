package platform

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

var (
	demoAgentRunsMu sync.Mutex
	demoAgentRuns   = []AgentRun{
		{
			Namespace: "default", Name: "diagnose-broken-pod",
			Goal: "Diagnose why this pod is unhealthy and propose the safest fix.",
			Autonomy: "ProposeOnly", DryRun: true,
			Target: &AgentObjectRef{APIVersion: "v1", Kind: "Pod", Namespace: "default", Name: "broken-demo"},
			Phase: "Proposed", Approval: "Pending", Risk: "medium",
			OwnedByAxiom: true, Source: AgentSourceManual,
			Plan: &AgentPlan{
				Summary: "Pod CrashLoopBackOff — restart recommended",
				RootCause: "container exit 1", Risk: "medium", RequiresApproval: true,
				Actions: []AgentAction{
					{ID: "a1", Type: AgentActionRestartPod, Risk: "low", Description: "Delete pod to force recreate",
						Target: &AgentObjectRef{APIVersion: "v1", Kind: "Pod", Namespace: "default", Name: "broken-demo"}},
				},
			},
			Age: "5m", CanApprove: true, CanDeny: true,
		},
	}
	demoPlatformIntents = []PlatformIntent{
		{Namespace: "default", Name: "need-win-builder", WorkloadKind: "VirtualMachine",
			Replicas: 1, CPU: "4", Memory: "16Gi", Phase: "Pending", Age: "1h"},
	}
)

func (d *DemoProvider) ListAgentRuns(ctx context.Context, ns string) ([]AgentRun, error) {
	demoAgentRunsMu.Lock()
	defer demoAgentRunsMu.Unlock()
	out := make([]AgentRun, 0, len(demoAgentRuns))
	for _, r := range demoAgentRuns {
		cp := r
		agentRunDecideFlags(&cp)
		out = append(out, cp)
	}
	return filterNS(ns, out, func(r AgentRun) string { return r.Namespace }), nil
}

func (d *DemoProvider) GetAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	demoAgentRunsMu.Lock()
	defer demoAgentRunsMu.Unlock()
	for _, r := range demoAgentRuns {
		if r.Namespace == ns && r.Name == name {
			cp := r
			agentRunDecideFlags(&cp)
			return cp, nil
		}
	}
	return AgentRun{}, fmt.Errorf("agent run %s/%s not found", ns, name)
}

func (d *DemoProvider) CreateAgentRun(ctx context.Context, req CreateAgentRunRequest) (AgentRun, error) {
	if strings.TrimSpace(req.Goal) == "" {
		return AgentRun{}, fmt.Errorf("goal is required")
	}
	run := newStoredAgentRun(req)
	run.Plan = SanitizeAgentPlan(deterministicAgentPlan(run, req.ExtraContext+" "+req.Goal))
	run.Risk = run.Plan.Risk
	run.Message = "demo planner"
	demoAgentRunsMu.Lock()
	defer demoAgentRunsMu.Unlock()
	for _, existing := range demoAgentRuns {
		if existing.Namespace == run.Namespace && existing.Name == run.Name {
			return AgentRun{}, fmt.Errorf("agent run %s/%s already exists", run.Namespace, run.Name)
		}
	}
	demoAgentRuns = append(demoAgentRuns, run)
	return run, nil
}

func (d *DemoProvider) UpdateAgentRun(ctx context.Context, run AgentRun) (AgentRun, error) {
	demoAgentRunsMu.Lock()
	defer demoAgentRunsMu.Unlock()
	for i, existing := range demoAgentRuns {
		if existing.Namespace == run.Namespace && existing.Name == run.Name {
			agentRunDecideFlags(&run)
			demoAgentRuns[i] = run
			return run, nil
		}
	}
	return AgentRun{}, fmt.Errorf("agent run %s/%s not found", run.Namespace, run.Name)
}

func (d *DemoProvider) ApproveAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	run, err := d.GetAgentRun(ctx, ns, name)
	if err != nil {
		return AgentRun{}, err
	}
	if !agentRunCanDecide(run) {
		return AgentRun{}, errAgentRunNotPending()
	}
	executed, err := ExecuteAgentRun(ctx, d, run)
	if err != nil {
		return AgentRun{}, err
	}
	return d.UpdateAgentRun(ctx, executed)
}

func (d *DemoProvider) DenyAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	run, err := d.GetAgentRun(ctx, ns, name)
	if err != nil {
		return AgentRun{}, err
	}
	if !agentRunCanDecide(run) {
		return AgentRun{}, errAgentRunNotPending()
	}
	run.Approval = "Denied"
	run.Phase = "Denied"
	run.Message = "demo denied"
	run.CanApprove = false
	run.CanDeny = false
	return d.UpdateAgentRun(ctx, run)
}

func (d *DemoProvider) ListPlatformIntents(ctx context.Context, ns string) ([]PlatformIntent, error) {
	return filterNS(ns, demoPlatformIntents, func(i PlatformIntent) string { return i.Namespace }), nil
}
