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

	for _, f := range rep.Findings {
		rep.Counts[f.Severity]++
	}
	switch {
	case rep.Counts["critical"] > 0:
		rep.Status = "blocked"
		rep.Summary = "Upgrade blocked until critical findings are cleared"
	case rep.Counts["warning"] > 0:
		rep.Status = "caution"
		rep.Summary = "Upgrade possible with caution — review warnings first"
	default:
		rep.Status = "ready"
		rep.Summary = "No hard blockers found in the lightweight pre-check"
		rep.Findings = append(rep.Findings, UpgradeFinding{
			Severity: "info", Code: "ok", Title: "Pre-check passed",
			Detail: "Nodes look ready and no PDB hard-blocks were detected",
			Hint:   "Still run your distribution's official upgrade docs (k3s/OpenShift/…)",
		})
	}
	return rep, nil
}
