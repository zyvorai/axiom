package platform

// AgentObjectRef scopes an AgentRun to a cluster object.
type AgentObjectRef struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
}

type AgentAction struct {
	ID          string            `json:"id,omitempty"`
	Type        string            `json:"type,omitempty"`
	Risk        string            `json:"risk,omitempty"`
	Description string            `json:"description,omitempty"`
	Target      *AgentObjectRef   `json:"target,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
}

type AgentPlan struct {
	Summary          string        `json:"summary,omitempty"`
	RootCause        string        `json:"rootCause,omitempty"`
	Risk             string        `json:"risk,omitempty"`
	RequiresApproval bool          `json:"requiresApproval,omitempty"`
	Actions          []AgentAction `json:"actions,omitempty"`
}

type AgentExecuted struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type,omitempty"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// AgentRun is the Axiom-facing view of an agentic remediation run.
// When the platform.example.com AgentRun CRD is present, runs live there;
// otherwise Axiom persists them in ConfigMap zyvor-axiom-agent-runs.
type AgentRun struct {
	Namespace    string           `json:"namespace"`
	Name         string           `json:"name"`
	Goal         string           `json:"goal"`
	Autonomy     string           `json:"autonomy"`
	DryRun       bool             `json:"dryRun"`
	Target       *AgentObjectRef  `json:"target,omitempty"`
	Phase        string           `json:"phase"`
	Approval     string           `json:"approval"`
	Message      string           `json:"message,omitempty"`
	Risk         string           `json:"risk,omitempty"`
	Plan         *AgentPlan       `json:"plan,omitempty"`
	Executed     []AgentExecuted  `json:"executed,omitempty"`
	Age          string           `json:"age"`
	CanApprove   bool             `json:"canApprove"`
	CanDeny      bool             `json:"canDeny"`
	OwnedByAxiom bool             `json:"ownedByAxiom,omitempty"`
	Source       string           `json:"source,omitempty"`
	CreatedAt    string           `json:"createdAt,omitempty"`
}

type CreateAgentRunRequest struct {
	Namespace    string          `json:"namespace"`
	Name         string          `json:"name"`
	Goal         string          `json:"goal"`
	ExtraContext string          `json:"extraContext,omitempty"`
	DryRun       *bool           `json:"dryRun,omitempty"`
	Target       *AgentObjectRef `json:"target,omitempty"`
	Source       string          `json:"source,omitempty"`
}

// PlatformIntent is a read-only view of platform.example.com/v1 Intent
// (distinct from Axiom NL place intent).
type PlatformIntent struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	WorkloadKind string `json:"workloadKind"`
	Replicas     int32  `json:"replicas,omitempty"`
	Image        string `json:"image,omitempty"`
	CPU          string `json:"cpu,omitempty"`
	Memory       string `json:"memory,omitempty"`
	Phase        string `json:"phase"`
	Message      string `json:"message,omitempty"`
	AppliedTo    string `json:"appliedTo,omitempty"`
	Age          string `json:"age"`
}

func agentRunDecideFlags(r *AgentRun) {
	can := agentRunCanDecide(*r)
	r.CanApprove = can
	r.CanDeny = can
}
