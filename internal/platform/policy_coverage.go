package platform

import (
	"context"
	"sort"
	"strings"
)

// PolicyCoverageRow is one Kyverno/Gatekeeper policy's pass/fail rollup.
type PolicyCoverageRow struct {
	Policy   string               `json:"policy"`
	Rule     string               `json:"rule,omitempty"`
	Counts   SecurityPolicyCounts `json:"counts"`
	FailRate float64              `json:"failRate"` // fail / (pass+fail+warn+error); skip excluded
}

// PolicyCoverageReport is GET /api/v1/security/policy-coverage.
type PolicyCoverageReport struct {
	Available   bool                 `json:"available"`
	Policies    []PolicyCoverageRow  `json:"policies"`
	Totals      SecurityPolicyCounts `json:"totals"`
	InstallHint string               `json:"installHint,omitempty"`
	Note        string               `json:"note,omitempty"`
}

// ListPolicyCoverage aggregates PolicyReport results by policy (+ rule).
func ListPolicyCoverage(ctx context.Context, p Provider, ns string) (PolicyCoverageReport, error) {
	if k, ok := p.(interface {
		ListPolicyCoverage(context.Context, string) (PolicyCoverageReport, error)
	}); ok {
		return k.ListPolicyCoverage(ctx, ns)
	}
	if sp, ok := p.(*SwitchableProvider); ok {
		return ListPolicyCoverage(ctx, sp.current(ctx), ns)
	}
	sum, err := p.ListSecurityReports(ctx, ns)
	if err != nil {
		return PolicyCoverageReport{}, err
	}
	rep := PolicyCoverageReport{
		Available: sum.Available,
		Policies:  []PolicyCoverageRow{},
		Totals:    sum.Policy,
	}
	if !sum.Available {
		rep.InstallHint = sum.InstallHint
		return rep, nil
	}
	rep.Note = "Per-policy breakdown needs PolicyReport CRDs; totals from security reports"
	return rep, nil
}

func (d *DemoProvider) ListPolicyCoverage(ctx context.Context, ns string) (PolicyCoverageReport, error) {
	sum, err := d.ListSecurityReports(ctx, ns)
	if err != nil {
		return PolicyCoverageReport{}, err
	}
	rows := map[string]*PolicyCoverageRow{}
	add := func(policy, rule, result string) {
		key := strings.ToLower(policy) + "|" + strings.ToLower(rule)
		r := rows[key]
		if r == nil {
			r = &PolicyCoverageRow{Policy: policy, Rule: rule}
			rows[key] = r
		}
		r.Counts.add(result)
	}
	includeCommerce := ns == "" || ns == "all" || ns == "commerce"
	includePayments := ns == "" || ns == "all" || ns == "payments"
	if includeCommerce {
		add("require-requests", "check-cpu", "pass")
		add("require-requests", "check-memory", "fail")
		add("disallow-latest", "validate-image", "fail")
	}
	if includePayments {
		add("require-requests", "check-cpu", "pass")
		add("disallow-latest", "validate-image", "pass")
	}
	out := PolicyCoverageReport{
		Available: sum.Available,
		Policies:  make([]PolicyCoverageRow, 0, len(rows)),
		Totals:    sum.Policy,
		Note:      "Demo Kyverno-style policy coverage",
	}
	for _, r := range rows {
		r.FailRate = failRate(r.Counts)
		out.Policies = append(out.Policies, *r)
	}
	sortPolicyCoverage(out.Policies)
	return out, nil
}

func failRate(c SecurityPolicyCounts) float64 {
	den := c.Pass + c.Fail + c.Warn + c.Error
	if den == 0 {
		return 0
	}
	return float64(c.Fail) / float64(den)
}

func sortPolicyCoverage(rows []PolicyCoverageRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Counts.Fail != rows[j].Counts.Fail {
			return rows[i].Counts.Fail > rows[j].Counts.Fail
		}
		if rows[i].Policy != rows[j].Policy {
			return rows[i].Policy < rows[j].Policy
		}
		return rows[i].Rule < rows[j].Rule
	})
}
