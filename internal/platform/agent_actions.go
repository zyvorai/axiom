package platform

import (
	"fmt"
	"net/http"
	"strings"
)

// Allowlisted AgentAction.Type values Axiom will propose and execute.
const (
	AgentActionRestartPod      = "restart_pod"
	AgentActionDeletePod       = "delete_pod"
	AgentActionScaleWorkload   = "scale_workload"
	AgentActionRolloutRestart  = "rollout_restart"
	AgentActionCordonNode      = "cordon_node"
	AgentActionUncordonNode    = "uncordon_node"
	AgentActionApplyRightsizing = "apply_rightsizing"
	AgentActionSealWorkload     = "seal_workload"
	AgentActionNoop             = "noop"
)

var agentActionAllowlist = map[string]bool{
	AgentActionRestartPod:       true,
	AgentActionDeletePod:        true,
	AgentActionScaleWorkload:    true,
	AgentActionRolloutRestart:   true,
	AgentActionCordonNode:       true,
	AgentActionUncordonNode:     true,
	AgentActionApplyRightsizing: true,
	AgentActionSealWorkload:     true,
	AgentActionNoop:             true,
}

// AgentOwnedAnnotation marks an AgentRun CR that Axiom planned and will execute.
const AgentOwnedAnnotation = "axiom.zyvor.dev/owned"

// AgentSource values for CreateAgentRunRequest.Source.
const (
	AgentSourceManual = "manual"
	AgentSourceHealth = "health"
	AgentSourceAlert  = "alert"
	AgentSourceMCP    = "mcp"
)

func IsAllowlistedAgentAction(t string) bool {
	return agentActionAllowlist[strings.TrimSpace(t)]
}

// SanitizeAgentPlan drops unknown action types and fills missing IDs.
func SanitizeAgentPlan(p *AgentPlan) *AgentPlan {
	if p == nil {
		return &AgentPlan{Summary: "No plan", Risk: "low", RequiresApproval: true}
	}
	out := &AgentPlan{
		Summary:          strings.TrimSpace(p.Summary),
		RootCause:        strings.TrimSpace(p.RootCause),
		Risk:             strings.ToLower(strings.TrimSpace(p.Risk)),
		RequiresApproval: true,
	}
	if out.Risk == "" {
		out.Risk = "low"
	}
	if out.Summary == "" {
		out.Summary = "Proposed remediation"
	}
	for i, a := range p.Actions {
		typ := strings.TrimSpace(a.Type)
		if !IsAllowlistedAgentAction(typ) {
			continue
		}
		id := strings.TrimSpace(a.ID)
		if id == "" {
			id = fmt.Sprintf("a%d", i+1)
		}
		risk := strings.ToLower(strings.TrimSpace(a.Risk))
		if risk == "" {
			risk = "low"
		}
		out.Actions = append(out.Actions, AgentAction{
			ID:          id,
			Type:        typ,
			Risk:        risk,
			Description: strings.TrimSpace(a.Description),
			Target:      a.Target,
			Params:      a.Params,
		})
	}
	if len(out.Actions) == 0 {
		out.Actions = []AgentAction{{
			ID: "a1", Type: AgentActionNoop, Risk: "low",
			Description: "No safe automated action identified — investigate manually",
		}}
	}
	return out
}

func agentRunCanDecide(r AgentRun) bool {
	if !strings.EqualFold(r.Approval, "Pending") && r.Approval != "" {
		return false
	}
	phase := strings.ToLower(r.Phase)
	return phase == "proposed" || phase == "pending" || phase == ""
}

func errAgentRunNotPending() error {
	return NewStatusError(http.StatusConflict, fmt.Errorf("agent run is not pending approval"))
}
