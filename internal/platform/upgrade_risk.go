package platform

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// UpgradeRiskReport is a Lightspeed-style read-only pre-check for day-2 ops.
type UpgradeRiskReport struct {
	GeneratedAt string             `json:"generatedAt"`
	Status      string             `json:"status"` // ready | caution | blocked
	Summary     string             `json:"summary"`
	Cluster     UpgradeClusterInfo `json:"cluster"`
	Findings    []UpgradeFinding   `json:"findings"`
	Counts      map[string]int     `json:"counts"`
}

type UpgradeClusterInfo struct {
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`
	Platform          string `json:"platform,omitempty"`
	NodeCount         int    `json:"nodeCount"`
	NodesReady        int    `json:"nodesReady"`
	NodesNotReady     int    `json:"nodesNotReady"`
	GoRuntime         string `json:"goRuntime,omitempty"`
}

type UpgradeFinding struct {
	Severity string `json:"severity"` // critical | warning | info
	Code     string `json:"code"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Hint     string `json:"hint,omitempty"`
}

// AssessUpgradeRisk gathers drain/PDB/node/version signals for a safe-to-upgrade hint.
func AssessUpgradeRisk(ctx context.Context, p Provider) (UpgradeRiskReport, error) {
	rep := UpgradeRiskReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Cluster:     UpgradeClusterInfo{GoRuntime: runtime.Version(), Platform: "kubernetes"},
		Findings:    []UpgradeFinding{},
		Counts:      map[string]int{},
	}
	nodes, err := p.ListNodes(ctx)
	if err != nil {
		return rep, err
	}
	rep.Cluster.NodeCount = len(nodes)
	for _, n := range nodes {
		ready := strings.EqualFold(n.Status, "Ready") || strings.Contains(strings.ToLower(n.Status), "ready")
		if ready && !n.Unschedulable {
			rep.Cluster.NodesReady++
		} else {
			rep.Cluster.NodesNotReady++
			title := "Node not fully ready"
			if n.Unschedulable {
				title = "Node cordoned"
			}
			rep.Findings = append(rep.Findings, UpgradeFinding{
				Severity: "warning", Code: "node-not-ready", Title: title,
				Detail: fmt.Sprintf("%s status=%s unschedulable=%v", n.Name, n.Status, n.Unschedulable),
				Hint:   "Uncordon or repair the node before a control-plane upgrade",
			})
		}
		if rep.Cluster.KubernetesVersion == "" && n.Version != "" {
			rep.Cluster.KubernetesVersion = n.Version
		}
	}

	// PDB pressure: any PDB with disruptionsAllowed == 0 and current healthy == desired is a caution.
	if pdbs, err := p.ListPodDisruptionBudgets(ctx, ""); err == nil {
		for _, pdb := range pdbs {
			// Without disruptionsAllowed on the summary, treat tight PDBs
			// (desired healthy with zero room relative to expected) as caution.
			if pdb.DesiredHealthy > 0 && pdb.CurrentHealthy >= pdb.DesiredHealthy && pdb.ExpectedPods > 0 &&
				(pdb.MinAvailable != "" || pdb.MaxUnavailable == "0" || pdb.MaxUnavailable == "0%") {
				rep.Findings = append(rep.Findings, UpgradeFinding{
					Severity: "warning", Code: "pdb-blocks-drain",
					Title:  "PDB may block node drain",
					Detail: fmt.Sprintf("%s/%s current=%d desired=%d minAvailable=%s maxUnavailable=%s",
						pdb.Namespace, pdb.Name, pdb.CurrentHealthy, pdb.DesiredHealthy, pdb.MinAvailable, pdb.MaxUnavailable),
					Hint: "Drain will wait; raise maxUnavailable or scale before upgrade",
				})
			}
		}
	}

	if ov, err := p.Overview(ctx); err == nil {
		if ov.Warnings > 0 {
			rep.Findings = append(rep.Findings, UpgradeFinding{
				Severity: "info", Code: "cluster-warnings",
				Title:  "Active Warning events",
				Detail: fmt.Sprintf("%d Warning events in the overview window", ov.Warnings),
				Hint:   "Clear CrashLoops and ImagePull errors before upgrading",
			})
		}
	}

	if len(nodes) == 0 {
		rep.Findings = append(rep.Findings, UpgradeFinding{
			Severity: "critical", Code: "no-nodes", Title: "No nodes reported",
			Detail: "The cluster returned zero nodes", Hint: "Restore node connectivity before any upgrade",
		})
	}

	rep = DeepenUpgradeRisk(ctx, p, rep)
	if rep.Status == "ready" && len(rep.Findings) == 0 {
		rep.Findings = append(rep.Findings, UpgradeFinding{
			Severity: "info", Code: "ok", Title: "Pre-check passed",
			Detail: "Nodes look ready and no PDB hard-blocks were detected",
			Hint:   "Still run your distribution's official upgrade docs (k3s/OpenShift/…)",
		})
	}
	return rep, nil
}

// EnrichUpgradeRiskPlan maps finding codes to allowlisted prep actions (ProposeOnly).
func EnrichUpgradeRiskPlan(run AgentRun, rep UpgradeRiskReport) AgentRun {
	if run.Plan == nil {
		run.Plan = &AgentPlan{Summary: rep.Summary, Risk: "medium", RequiresApproval: true}
	}
	seen := map[string]bool{}
	for _, a := range run.Plan.Actions {
		seen[a.Type+"|"+a.Description] = true
	}
	id := len(run.Plan.Actions) + 1
	add := func(a AgentAction) {
		key := a.Type + "|" + a.Description
		if seen[key] {
			return
		}
		seen[key] = true
		if a.ID == "" {
			a.ID = fmt.Sprintf("u%d", id)
			id++
		}
		run.Plan.Actions = append(run.Plan.Actions, a)
	}
	for _, f := range rep.Findings {
		switch f.Code {
		case "node-not-ready":
			// Detail starts with node name.
			name := strings.Fields(f.Detail)
			node := ""
			if len(name) > 0 {
				node = name[0]
			}
			if node != "" && strings.Contains(f.Title, "cordon") {
				add(AgentAction{
					Type: AgentActionUncordonNode, Risk: "medium",
					Description: "Uncordon " + node + " before upgrade",
					Target:      &AgentObjectRef{Kind: "Node", Name: node, APIVersion: "v1"},
				})
			} else if node != "" {
				add(AgentAction{
					Type: AgentActionCordonNode, Risk: "medium",
					Description: "Cordon unhealthy node " + node + " during prep",
					Target:      &AgentObjectRef{Kind: "Node", Name: node, APIVersion: "v1"},
				})
			}
		case "pdb-blocks-drain":
			add(AgentAction{
				Type: AgentActionNoop, Risk: "low",
				Description: "Review PDB " + f.Detail + " — raise maxUnavailable before drain",
			})
		case "cluster-warnings":
			add(AgentAction{
				Type: AgentActionNoop, Risk: "low",
				Description: "Clear Warning events / CrashLoops before upgrading",
			})
		case "version-skew", "control-plane-skew":
			add(AgentAction{
				Type: AgentActionNoop, Risk: "medium",
				Description: "Resolve version skew before upgrade: " + f.Detail,
			})
		case "cert-expiry", "certificate-expiring":
			add(AgentAction{
				Type: AgentActionNoop, Risk: "medium",
				Description: "Renew expiring certificates before upgrade: " + f.Detail,
			})
		case "drain-blocked", "pod-disruption":
			add(AgentAction{
				Type: AgentActionNoop, Risk: "medium",
				Description: "Unblock drain path: " + f.Title + " — " + f.Detail,
			})
		case "gpu-operator", "accelerator-not-ready":
			add(AgentAction{
				Type: AgentActionNoop, Risk: "low",
				Description: "Confirm GPU Operator / device plugin healthy before node drains",
			})
		}
	}
	run.Plan = SanitizeAgentPlan(run.Plan)
	if run.Plan.Summary == "" {
		run.Plan.Summary = rep.Summary
	}
	return run
}
