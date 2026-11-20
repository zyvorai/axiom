package platform

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// OutageNarrative stitches fleet incident context with audit presence + AgentRuns.
type OutageNarrative struct {
	Cluster   string   `json:"cluster"`
	From      string   `json:"from,omitempty"`
	State     string   `json:"state,omitempty"`
	Summary   string   `json:"summary"`
	WhoPoked  []string `json:"whoPoked,omitempty"`
	AgentRuns []string `json:"agentRuns,omitempty"`
	Tips      []string `json:"tips,omitempty"`
}

// BuildOutageNarrative composes a short ops-theatre story for a cluster window.
func BuildOutageNarrative(ctx context.Context, p Provider, cluster, fromISO, state string) OutageNarrative {
	n := OutageNarrative{
		Cluster: cluster, From: fromISO, State: state,
		Tips: []string{
			"Open Timeline filtered to this window for who poked it",
			"Ops → Agents for ProposeOnly runs overlapping the outage",
		},
	}
	switch state {
	case "unreachable":
		n.Summary = fmt.Sprintf("Cluster %s became unreachable", cluster)
	case "degraded":
		n.Summary = fmt.Sprintf("Cluster %s reported degraded", cluster)
	default:
		n.Summary = fmt.Sprintf("Cluster %s incident", cluster)
	}

	from := time.Now().Add(-6 * time.Hour)
	if t, err := time.Parse(time.RFC3339, fromISO); err == nil {
		from = t
	}

	for _, a := range ListAxiomAudit() {
		if a.Time.Before(from) {
			continue
		}
		act := strings.ToLower(a.Action)
		if strings.Contains(act, "pod.terminal") || strings.Contains(act, "pod.window") ||
			strings.Contains(act, "pod.exec") || strings.Contains(act, "seal") {
			who := a.User
			if who == "" {
				who = a.Target
			}
			n.WhoPoked = append(n.WhoPoked, fmt.Sprintf("%s · %s", a.Action, who))
			if len(n.WhoPoked) >= 8 {
				break
			}
		}
	}
	if p != nil {
		if runs, err := p.ListAgentRuns(ctx, "all"); err == nil {
			for _, r := range runs {
				n.AgentRuns = append(n.AgentRuns, fmt.Sprintf("%s/%s · %s · %s", r.Namespace, r.Name, r.Phase, truncate(r.Goal, 80)))
				if len(n.AgentRuns) >= 5 {
					break
				}
			}
		}
	}
	if len(n.WhoPoked) > 0 {
		n.Summary += fmt.Sprintf(" — %d presence/seal events in window", len(n.WhoPoked))
	}
	if len(n.AgentRuns) > 0 {
		n.Summary += fmt.Sprintf("; %d recent agent runs", len(n.AgentRuns))
	}
	return n
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
