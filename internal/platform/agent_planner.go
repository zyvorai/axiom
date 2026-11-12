package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const agentPlanSystemPrompt = `You are Axiom's cluster remediation planner. Given redacted Kubernetes evidence, propose the safest fix.
Respond with ONLY a JSON object (no markdown) of this shape:
{"summary":"...","rootCause":"...","risk":"low|medium|high","actions":[{"id":"a1","type":"TYPE","risk":"low|medium|high","description":"...","target":{"apiVersion":"v1","kind":"Pod","namespace":"...","name":"..."},"params":{}}]}
Allowed action types ONLY: restart_pod, delete_pod, scale_workload, rollout_restart, cordon_node, uncordon_node, apply_rightsizing, noop.
For scale_workload set params.replicas to an integer string.
For cordon_node/uncordon_node set target.kind=Node and target.name to the node name.
For apply_rightsizing set params: kind, namespace, workload, container, resources (comma-separated cpu and/or memory).
Never invent Secret or ConfigMap contents. Prefer the least risky fix. Prefer noop when unsure.`

// PlanAgentRun gathers evidence and fills run.Plan. Uses BYO LLM when AI is
// enabled; otherwise a deterministic plan from the target / health context.
func PlanAgentRun(ctx context.Context, p Provider, in Integrations, demo bool, run AgentRun, extra string) (AgentRun, error) {
	plan, msg, err := buildAgentPlan(ctx, p, in, demo, run, extra)
	if err != nil {
		run.Message = err.Error()
		run.Phase = "Proposed"
		run.Plan = SanitizeAgentPlan(&AgentPlan{
			Summary: "Planning failed — noop",
			Risk:    "low",
			Actions: []AgentAction{{ID: "a1", Type: AgentActionNoop, Risk: "low", Description: "Investigate manually"}},
		})
		run.Risk = "low"
		agentRunDecideFlags(&run)
		return run, nil
	}
	run.Plan = plan
	run.Risk = plan.Risk
	if msg != "" {
		run.Message = msg
	}
	run.Phase = "Proposed"
	run.Approval = "Pending"
	run.OwnedByAxiom = true
	agentRunDecideFlags(&run)
	return run, nil
}

func buildAgentPlan(ctx context.Context, p Provider, in Integrations, demo bool, run AgentRun, extra string) (*AgentPlan, string, error) {
	evidence := gatherAgentEvidence(ctx, p, run, extra)
	st := AIStatusOf(in, demo)
	if st.Enabled {
		client, err := NewAIClient(AIConfigOf(in))
		if err == nil {
			user := "Goal: " + run.Goal + "\n\nEvidence:\n" + evidence
			if len(user) > aiMaxPromptBytes {
				user = user[:aiMaxPromptBytes]
			}
			text, err := client.Complete(ctx, agentPlanSystemPrompt, user)
			if err == nil {
				plan, perr := parseAgentPlanJSON(text)
				if perr == nil {
					return SanitizeAgentPlan(plan), "planned with AI", nil
				}
			}
		}
	}
	return SanitizeAgentPlan(deterministicAgentPlan(run, evidence)), "deterministic plan", nil
}

func gatherAgentEvidence(ctx context.Context, p Provider, run AgentRun, extra string) string {
	var b strings.Builder
	b.WriteString("goal: " + run.Goal + "\n")
	if extra != "" {
		b.WriteString("context: " + truncateStr(extra, 800) + "\n")
	}
	if run.Target != nil && run.Target.Name != "" {
		ns := run.Target.Namespace
		if ns == "" {
			ns = run.Namespace
		}
		kind := strings.ToLower(run.Target.Kind)
		b.WriteString(fmt.Sprintf("target: %s/%s/%s\n", kind, ns, run.Target.Name))
		if kind == "pod" || kind == "" {
			if pods, err := p.ListPods(ctx, ns); err == nil {
				for _, pod := range pods {
					if pod.Name == run.Target.Name {
						b.WriteString(fmt.Sprintf("pod status=%s ready=%s restarts=%d\n", pod.Status, pod.Ready, pod.Restarts))
						break
					}
				}
			}
			if diag, err := p.DiagnosePod(ctx, ns, run.Target.Name); err == nil {
				b.WriteString("diagnosis: " + truncateStr(fmt.Sprintf("%v", diag), 1200) + "\n")
			}
		}
		if evs, err := p.ListEvents(ctx, ns); err == nil {
			n := 0
			for _, e := range evs {
				if e.Type != "Warning" {
					continue
				}
				if run.Target.Name != "" && !strings.Contains(e.Object, run.Target.Name) {
					continue
				}
				b.WriteString(fmt.Sprintf("event: %s %s\n", e.Reason, truncateStr(e.Message, 200)))
				n++
				if n >= 8 {
					break
				}
			}
		}
	}
	if report, err := p.ClusterHealth(ctx, run.Namespace); err == nil {
		n := 0
		for _, f := range report.Findings {
			if run.Target != nil && run.Target.Name != "" && f.Name != run.Target.Name {
				continue
			}
			b.WriteString(fmt.Sprintf("finding[%s]: %s — %s (fix: %s)\n", f.Severity, f.Title, truncateStr(f.Message, 160), truncateStr(f.Suggestion, 120)))
			n++
			if n >= 5 {
				break
			}
		}
	}
	out := b.String()
	if len(out) > aiMaxPromptBytes {
		return out[:aiMaxPromptBytes]
	}
	return out
}

func deterministicAgentPlan(run AgentRun, evidence string) *AgentPlan {
	low := strings.ToLower(evidence + " " + run.Goal)
	plan := &AgentPlan{
		Summary:          "Deterministic remediation proposal",
		RootCause:        "Derived from cluster signals without an LLM",
		Risk:             "low",
		RequiresApproval: true,
	}
	tgt := run.Target
	switch {
	case strings.Contains(low, "crashloop") || strings.Contains(low, "backoff") || strings.Contains(low, "oomkilled"):
		plan.Summary = "Pod is crash-looping — restart recommended"
		plan.RootCause = "Container repeatedly exiting"
		plan.Risk = "medium"
		if tgt != nil {
			plan.Actions = []AgentAction{{
				ID: "a1", Type: AgentActionRestartPod, Risk: "low",
				Description: "Delete the pod so its controller recreates it",
				Target:      tgt,
			}}
		}
	case strings.Contains(low, "imagepull") || strings.Contains(low, "errimage"):
		plan.Summary = "Image pull failure — no safe automated fix"
		plan.RootCause = "Image or pull secret issue"
		plan.Actions = []AgentAction{{ID: "a1", Type: AgentActionNoop, Risk: "low", Description: "Fix image name or pull secrets manually"}}
	case strings.Contains(low, "scale") || strings.Contains(low, "replica"):
		plan.Summary = "Scale adjustment proposed"
		plan.Actions = []AgentAction{{
			ID: "a1", Type: AgentActionScaleWorkload, Risk: "medium",
			Description: "Scale the workload",
			Target:      tgt,
			Params:      map[string]string{"replicas": "2"},
		}}
	case tgt != nil && (strings.EqualFold(tgt.Kind, "Deployment") || strings.EqualFold(tgt.Kind, "StatefulSet") || strings.EqualFold(tgt.Kind, "DaemonSet")):
		plan.Summary = "Rollout restart proposed"
		plan.Actions = []AgentAction{{
			ID: "a1", Type: AgentActionRolloutRestart, Risk: "medium",
			Description: "Rollout restart the workload",
			Target:      tgt,
		}}
	case tgt != nil && strings.EqualFold(tgt.Kind, "Pod"):
		plan.Summary = "Restart pod proposed"
		plan.Actions = []AgentAction{{
			ID: "a1", Type: AgentActionRestartPod, Risk: "low",
			Description: "Delete the pod to force recreate",
			Target:      tgt,
		}}
	default:
		plan.Summary = "No automated remediation — review evidence"
		plan.Actions = []AgentAction{{ID: "a1", Type: AgentActionNoop, Risk: "low", Description: "Investigate manually"}}
	}
	return plan
}

func parseAgentPlanJSON(text string) (*AgentPlan, error) {
	text = strings.TrimSpace(text)
	if i := strings.Index(text, "{"); i >= 0 {
		text = text[i:]
	}
	if j := strings.LastIndex(text, "}"); j >= 0 {
		text = text[:j+1]
	}
	var plan AgentPlan
	if err := json.Unmarshal([]byte(text), &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func truncateStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
