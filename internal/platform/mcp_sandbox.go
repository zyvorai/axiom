package platform

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MCPSandboxGrant is a short-lived HITL execution grant for MCP/agent apply.
// Full ClusterRole minting is cluster-admin sensitive; Axiom records a sandbox
// grant that gates ApproveAgentRun execution for a TTL instead of minting
// unbounded ClusterRoles in the console process.
type MCPSandboxGrant struct {
	ID        string    `json:"id"`
	User      string    `json:"user"`
	Namespace string    `json:"namespace,omitempty"`
	Actions   []string  `json:"actions"` // allowlisted agent action types
	ExpiresAt time.Time `json:"expiresAt"`
	Note      string    `json:"note,omitempty"`
}

type mcpSandboxStore struct {
	mu     sync.Mutex
	grants map[string]MCPSandboxGrant
}

var mcpSandbox = &mcpSandboxStore{grants: map[string]MCPSandboxGrant{}}

// MintMCPSandbox creates a short-lived allowlist for agent execution (HITL).
// Default TTL 15m; actions default to the full agent allowlist.
func MintMCPSandbox(user, namespace string, actions []string, ttl time.Duration) MCPSandboxGrant {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if len(actions) == 0 {
		actions = []string{
			AgentActionRestartPod, AgentActionDeletePod, AgentActionScaleWorkload,
			AgentActionRolloutRestart, AgentActionCordonNode, AgentActionUncordonNode,
			AgentActionApplyRightsizing, AgentActionSealWorkload, AgentActionNoop,
		}
	}
	g := MCPSandboxGrant{
		ID: fmt.Sprintf("sbx-%d", time.Now().UnixNano()),
		User: user, Namespace: namespace, Actions: actions,
		ExpiresAt: time.Now().UTC().Add(ttl),
		Note:      "Ephemeral HITL sandbox — does not mint ClusterRoles; gates allowlisted agent apply",
	}
	mcpSandbox.mu.Lock()
	mcpSandbox.grants[g.ID] = g
	mcpSandbox.mu.Unlock()
	RecordAxiomAudit("mcp.sandbox_mint", g.ID, fmt.Sprintf("user=%s ttl=%s actions=%d", user, ttl, len(actions)), user)
	return g
}

// MCPSandboxAllows reports whether grantID is live and permits actionType.
func MCPSandboxAllows(grantID, actionType string) bool {
	grantID = strings.TrimSpace(grantID)
	if grantID == "" {
		return true // no sandbox required when unset (classic approve path)
	}
	mcpSandbox.mu.Lock()
	defer mcpSandbox.mu.Unlock()
	g, ok := mcpSandbox.grants[grantID]
	if !ok || time.Now().After(g.ExpiresAt) {
		if ok {
			delete(mcpSandbox.grants, grantID)
		}
		return false
	}
	for _, a := range g.Actions {
		if a == actionType {
			return true
		}
	}
	return false
}

// ListMCPSandboxes returns live grants (for MCP/tools introspection).
func ListMCPSandboxes(_ context.Context) []MCPSandboxGrant {
	mcpSandbox.mu.Lock()
	defer mcpSandbox.mu.Unlock()
	now := time.Now()
	var out []MCPSandboxGrant
	for id, g := range mcpSandbox.grants {
		if now.After(g.ExpiresAt) {
			delete(mcpSandbox.grants, id)
			continue
		}
		out = append(out, g)
	}
	return out
}
