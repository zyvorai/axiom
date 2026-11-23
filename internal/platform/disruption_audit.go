package platform

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Disruption readiness audit (GET /api/v1/disruption/audit). Drains and node
// upgrades go through the Eviction API, which obeys PodDisruptionBudgets: a
// budget that allows no disruption stalls a drain, and a replicated workload
// with no budget at all can lose every replica at once. This audit reads the
// PDBs and workloads the Provider already serves. It cannot match a PDB to a
// workload (the PDB summary carries no selector), so "no budget" is reported
// per namespace, not per workload.

const (
	disruptionMaxFindings = 300

	DisruptionBlocking      = "blocking"
	DisruptionUnsatisfiable = "unsatisfiable"
	DisruptionUnhealthy     = "unhealthy"
	DisruptionNoMatch       = "selects-nothing"
	DisruptionNoBudget      = "no-budget"
)

// DisruptionFinding is one PDB or namespace that needs a look.
type DisruptionFinding struct {
	Kind      string   `json:"kind"`
	Severity  string   `json:"severity"` // high | medium | low
	Namespace string   `json:"namespace"`
	Name      string   `json:"name,omitempty"`
	Workloads []string `json:"workloads,omitempty"`
	Reason    string   `json:"reason"`
	Suggest   string   `json:"suggestion"`
}

type DisruptionReport struct {
	Summary struct {
		Total  int            `json:"total"`
		ByKind map[string]int `json:"byKind"`
		PDBs   int            `json:"pdbs"`
	} `json:"summary"`
	Findings  []DisruptionFinding `json:"findings"`
	Truncated bool                `json:"truncated,omitempty"`
	Note      string              `json:"note"`
}

type DisruptionInput struct {
	PDBs      []PDBSummary
	Workloads []Workload
}

// parsePDBValue reads a PDB minAvailable/maxUnavailable value: a whole number
// or a percentage such as "50%". ok is false for empty or malformed text.
func parsePDBValue(s string) (n int, percent, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 8 {
		return 0, false, false
	}
	if strings.HasSuffix(s, "%") {
		v, err := strconv.Atoi(strings.TrimSuffix(s, "%"))
		if err != nil || v < 0 || v > 100 {
			return 0, false, false
		}
		return v, true, true
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return 0, false, false
	}
	return v, false, true
}

// DetectDisruption is pure. namespace "" or "all" means every namespace.
func DetectDisruption(in DisruptionInput, namespace string) DisruptionReport {
	if namespace == "all" {
		namespace = ""
	}
	inScope := func(ns string) bool { return namespace == "" || ns == namespace }
	var out []DisruptionFinding
	pdbNS := map[string]bool{}
	pdbs := 0
	for _, p := range in.PDBs {
		if !inScope(p.Namespace) {
			continue
		}
		pdbs++
		pdbNS[p.Namespace] = true
		f := DisruptionFinding{Namespace: p.Namespace, Name: p.Name}
		minN, minPct, minOK := parsePDBValue(p.MinAvailable)
		maxN, _, maxOK := parsePDBValue(p.MaxUnavailable)
		switch {
		case p.ExpectedPods == 0:
			f.Kind, f.Severity = DisruptionNoMatch, "medium"
			f.Reason = "The budget currently selects no Pods, so it protects nothing."
			f.Suggest = "Check the selector against the workload's Pod labels, or delete the budget if the workload is gone."
		case (minOK && !minPct && int32(minN) >= p.ExpectedPods && p.ExpectedPods > 0) || (minOK && minPct && minN == 100) ||
			(maxOK && maxN == 0):
			f.Kind, f.Severity = DisruptionUnsatisfiable, "high"
			f.Reason = fmt.Sprintf("minAvailable=%q maxUnavailable=%q leaves no Pod that may be evicted (%d expected), so every drain of a node running one of them will wait.", p.MinAvailable, p.MaxUnavailable, p.ExpectedPods)
			f.Suggest = "Allow at least one disruption (lower minAvailable below the replica count, or set maxUnavailable to 1), or add replicas."
		case p.CurrentHealthy < p.DesiredHealthy:
			f.Kind, f.Severity = DisruptionUnhealthy, "high"
			f.Reason = fmt.Sprintf("Only %d of the required %d Pods are healthy: no further disruption is allowed until they recover.", p.CurrentHealthy, p.DesiredHealthy)
			f.Suggest = "Fix the unhealthy Pods first; a drain started now will be refused with 429."
		case p.CurrentHealthy == p.DesiredHealthy:
			f.Kind, f.Severity = DisruptionBlocking, "medium"
			f.Reason = fmt.Sprintf("%d healthy Pods equal the %d required: zero disruptions are allowed right now.", p.CurrentHealthy, p.DesiredHealthy)
			f.Suggest = "Expected during a rollout; if it persists, a drain of these Pods will stall."
		default:
			continue
		}
		out = append(out, f)
	}

	// Namespaces with replicated workloads and no PDB at all.
	perNS := map[string][]string{}
	for _, w := range in.Workloads {
		if !inScope(w.Namespace) || protectedNamespace(w.Namespace) || strings.HasPrefix(w.Namespace, "kube-") {
			continue
		}
		if (w.Kind == "Deployment" || w.Kind == "StatefulSet") && w.Desired >= 2 && !pdbNS[w.Namespace] {
			perNS[w.Namespace] = append(perNS[w.Namespace], w.Name)
		}
	}
	for ns, names := range perNS {
		sort.Strings(names)
		shown := names
		if len(shown) > 10 {
			shown = shown[:10]
		}
		out = append(out, DisruptionFinding{Kind: DisruptionNoBudget, Severity: "low", Namespace: ns, Workloads: shown,
			Reason:  fmt.Sprintf("%d workload(s) run two or more replicas but the namespace has no PodDisruptionBudget, so a drain may evict every replica at once.", len(names)),
			Suggest: "Add a PodDisruptionBudget (for example maxUnavailable: 1) for each workload that must stay available."})
	}

	rank := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.Slice(out, func(i, j int) bool {
		if rank[out[i].Severity] != rank[out[j].Severity] {
			return rank[out[i].Severity] < rank[out[j].Severity]
		}
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	rep := DisruptionReport{Findings: out}
	if len(rep.Findings) > disruptionMaxFindings {
		rep.Findings = rep.Findings[:disruptionMaxFindings]
		rep.Truncated = true
	}
	if rep.Findings == nil {
		rep.Findings = []DisruptionFinding{}
	}
	rep.Summary.Total = len(out)
	rep.Summary.PDBs = pdbs
	rep.Summary.ByKind = map[string]int{}
	for _, f := range out {
		rep.Summary.ByKind[f.Kind]++
	}
	rep.Note = "Heuristic over PodDisruptionBudget summaries: the budget summary carries no selector, so missing budgets are reported per namespace and a budget is not matched to a workload. A blocking budget during a rollout is normal. Axiom changes nothing from this report."
	return rep
}

// DisruptionAudit collects the snapshot from the provider and runs the audit.
func DisruptionAudit(ctx context.Context, p Provider, namespace string) (DisruptionReport, error) {
	var in DisruptionInput
	var err error
	if in.PDBs, err = p.ListPodDisruptionBudgets(ctx, ""); err != nil {
		return DisruptionReport{}, err
	}
	if in.Workloads, err = p.ListWorkloads(ctx, ""); err != nil {
		return DisruptionReport{}, err
	}
	return DetectDisruption(in, namespace), nil
}
