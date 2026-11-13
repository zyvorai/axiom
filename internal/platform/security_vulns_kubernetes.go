//go:build kubernetes

package platform

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	trivySbomGVRs        = []schema.GroupVersionResource{{Group: "aquasecurity.github.io", Version: "v1alpha1", Resource: "sbomreports"}}
	trivyClusterSbomGVRs = []schema.GroupVersionResource{{Group: "aquasecurity.github.io", Version: "v1alpha1", Resource: "clustersbomreports"}}
)

// ListVulnerabilities reads Trivy Operator VulnerabilityReports and SbomReports
// (plus ClusterSbomReports for the all-namespaces view). Read-only.
func (k *KubernetesProvider) ListVulnerabilities(ctx context.Context, ns, severity string) (VulnerabilityView, error) {
	sev, err := parseSeverityFilter(severity)
	if err != nil {
		return VulnerabilityView{}, err
	}
	nsArg := ""
	if ns != "" && ns != "all" {
		nsArg = ns
	}
	var set vulnSet
	vf, err := k.fetchReports(ctx, trivyVulnGVRs, nsArg)
	if err != nil {
		return VulnerabilityView{}, err
	}
	set.Vuln = vf.items
	set.Truncated = vf.truncated
	set.Sources = append(set.Sources, SecuritySource{Name: "Trivy VulnerabilityReports", Group: "aquasecurity.github.io", Available: vf.available, Reports: len(vf.items), Message: vf.message})
	sf, err := k.fetchReports(ctx, trivySbomGVRs, nsArg)
	if err != nil {
		return VulnerabilityView{}, err
	}
	if nsArg == "" {
		if cf, cerr := k.fetchReports(ctx, trivyClusterSbomGVRs, ""); cerr == nil && cf.available {
			sf.items = append(sf.items, cf.items...)
			sf.available = true
		}
	}
	set.Sbom = sf.items
	set.Truncated = set.Truncated || sf.truncated
	set.Sources = append(set.Sources, SecuritySource{Name: "Trivy SbomReports", Group: "aquasecurity.github.io", Available: sf.available, Reports: len(sf.items), Message: sf.message})
	return set.view(sev), nil
}
