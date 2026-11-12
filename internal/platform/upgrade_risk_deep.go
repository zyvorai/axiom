package platform

import (
	"context"
	"fmt"
	"strings"
)

// DeepenUpgradeRisk adds cert/secret/GPU/version-skew signals onto a lite report.
func DeepenUpgradeRisk(ctx context.Context, p Provider, rep UpgradeRiskReport) UpgradeRiskReport {
	if p == nil {
		return rep
	}
	// Version skew across nodes.
	versions := map[string]int{}
	if nodes, err := p.ListNodes(ctx); err == nil {
		for _, n := range nodes {
			v := strings.TrimSpace(n.Version)
			if v == "" {
				continue
			}
			versions[v]++
		}
	}
	if len(versions) > 1 {
		var parts []string
		for v, n := range versions {
			parts = append(parts, fmt.Sprintf("%s×%d", v, n))
		}
		rep.Findings = append(rep.Findings, UpgradeFinding{
			Severity: "warning", Code: "version-skew",
			Title:  "Node Kubernetes version skew",
			Detail: strings.Join(parts, ", "),
			Hint:   "Bring all nodes to the same patch before a control-plane upgrade",
		})
	}

	if health, err := p.ClusterHealth(ctx, ""); err == nil {
		for _, f := range health.Findings {
			switch f.Analyzer {
			case AnalyzerCertExpiry:
				rep.Findings = append(rep.Findings, UpgradeFinding{
					Severity: "warning", Code: "cert-expiry",
					Title: f.Title, Detail: f.Message, Hint: f.Suggestion,
				})
			case AnalyzerNodeClockSkew:
				rep.Findings = append(rep.Findings, UpgradeFinding{
					Severity: "warning", Code: "clock-skew",
					Title: f.Title, Detail: f.Message, Hint: f.Suggestion,
				})
			}
		}
	}

	if acc, err := p.ListAccelerators(ctx); err == nil && acc.Status != "" && acc.Status != "missing" {
		if acc.Status != "ready" {
			rep.Findings = append(rep.Findings, UpgradeFinding{
				Severity: "info", Code: "gpu-operator",
				Title:  "Accelerators not fully ready",
				Detail: acc.Summary,
				Hint:   "Confirm GPU Operator / device plugin before draining GPU nodes",
			})
		}
	}

	// Recalculate counts/status.
	rep.Counts = map[string]int{}
	for _, f := range rep.Findings {
		rep.Counts[f.Severity]++
	}
	switch {
	case rep.Counts["critical"] > 0:
		rep.Status = "blocked"
		rep.Summary = "Upgrade blocked until critical findings are cleared"
	case rep.Counts["warning"] > 0:
		rep.Status = "caution"
		rep.Summary = "Upgrade possible with caution — review deepened pre-check warnings"
	default:
		rep.Status = "ready"
		rep.Summary = "Deepened pre-check found no hard blockers"
	}
	return rep
}
