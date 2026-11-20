//go:build kubernetes

package platform

import (
	"context"
	"strings"
)

func (k *KubernetesProvider) ListPolicyCoverage(ctx context.Context, ns string) (PolicyCoverageReport, error) {
	set, err := k.loadSecurityReports(ctx, ns, false, false, true)
	if err != nil {
		return PolicyCoverageReport{}, err
	}
	rep := PolicyCoverageReport{Policies: []PolicyCoverageRow{}, Note: "Aggregated from PolicyReport / ClusterPolicyReport results"}
	for _, src := range set.Sources {
		if src.Available {
			rep.Available = true
		}
	}
	if !rep.Available {
		rep.InstallHint = secInstallHint
		return rep, nil
	}
	rows := map[string]*PolicyCoverageRow{}
	for _, obj := range set.Policy {
		for _, r := range uslice(obj["results"]) {
			policy := ustr(r["policy"])
			if policy == "" {
				continue
			}
			rule := ustr(r["rule"])
			key := strings.ToLower(policy) + "|" + strings.ToLower(rule)
			row := rows[key]
			if row == nil {
				row = &PolicyCoverageRow{Policy: policy, Rule: rule}
				rows[key] = row
			}
			result := ustr(r["result"])
			row.Counts.add(result)
			rep.Totals.add(result)
		}
	}
	for _, r := range rows {
		r.FailRate = failRate(r.Counts)
		rep.Policies = append(rep.Policies, *r)
	}
	sortPolicyCoverage(rep.Policies)
	return rep, nil
}
