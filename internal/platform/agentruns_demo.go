package platform

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
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
			Plan: &AgentPlan{
				Summary: "Pod CrashLoopBackOff — restart recommended",
				RootCause: "container exit 1", Risk: "medium", RequiresApproval: true,
				Actions: []AgentAction{
					{ID: "a1", Type: "restart_pod", Risk: "low", Description: "Delete pod to force recreate",
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
	return filterNS(ns, append([]AgentRun(nil), demoAgentRuns...), func(r AgentRun) string { return r.Namespace }), nil
}

func (d *DemoProvider) GetAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	demoAgentRunsMu.Lock()
	defer demoAgentRunsMu.Unlock()
	for _, r := range demoAgentRuns {
		if r.Namespace == ns && r.Name == name {
			return r, nil
		}
	}
	return AgentRun{}, fmt.Errorf("agent run %s/%s not found", ns, name)
}

func (d *DemoProvider) CreateAgentRun(ctx context.Context, req CreateAgentRunRequest) (AgentRun, error) {
	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = "default"
	}
	if strings.TrimSpace(req.Goal) == "" {
		return AgentRun{}, fmt.Errorf("goal is required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("run-%d", time.Now().Unix()%1000000)
	}
	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	run := AgentRun{
		Namespace: ns, Name: name, Goal: req.Goal, Autonomy: "ProposeOnly", DryRun: dryRun,
		Target: req.Target, Phase: "Proposed", Approval: "Pending", Risk: "low",
		Plan: &AgentPlan{
			Summary: "Demo plan — install platform CRDs for live Hermes planning",
			RootCause: "demo mode", Risk: "low", RequiresApproval: true,
			Actions: []AgentAction{{ID: "a1", Type: "noop", Risk: "low", Description: "No-op in demo"}},
		},
		Age: "now", CanApprove: true, CanDeny: true,
	}
	demoAgentRunsMu.Lock()
	demoAgentRuns = append(demoAgentRuns, run)
	demoAgentRunsMu.Unlock()
	return run, nil
}

func (d *DemoProvider) ApproveAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	return d.setDemoAgentApproval(ns, name, "Approved", "Succeeded")
}

func (d *DemoProvider) DenyAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	return d.setDemoAgentApproval(ns, name, "Denied", "Denied")
}

func (d *DemoProvider) setDemoAgentApproval(ns, name, approval, phase string) (AgentRun, error) {
	demoAgentRunsMu.Lock()
	defer demoAgentRunsMu.Unlock()
	for i, r := range demoAgentRuns {
		if r.Namespace == ns && r.Name == name {
			demoAgentRuns[i].Approval = approval
			demoAgentRuns[i].Phase = phase
			demoAgentRuns[i].CanApprove = false
			demoAgentRuns[i].CanDeny = false
			demoAgentRuns[i].Message = "demo " + strings.ToLower(approval)
			return demoAgentRuns[i], nil
		}
	}
	return AgentRun{}, fmt.Errorf("agent run %s/%s not found", ns, name)
}

func (d *DemoProvider) ListPlatformIntents(ctx context.Context, ns string) ([]PlatformIntent, error) {
	return filterNS(ns, demoPlatformIntents, func(i PlatformIntent) string { return i.Namespace }), nil
}
