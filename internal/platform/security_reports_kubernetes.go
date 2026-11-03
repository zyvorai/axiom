//go:build kubernetes

package platform

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	secPageSize = 200
	secMaxPages = 50 // 10,000 objects per source
)

var (
	trivyVulnGVRs = []schema.GroupVersionResource{
		{Group: "aquasecurity.github.io", Version: "v1alpha1", Resource: "vulnerabilityreports"},
	}
	trivyConfigGVRs = []schema.GroupVersionResource{
		{Group: "aquasecurity.github.io", Version: "v1alpha1", Resource: "configauditreports"},
	}
	policyReportGVRs = []schema.GroupVersionResource{
		{Group: "wgpolicyk8s.io", Version: "v1alpha2", Resource: "policyreports"},
		{Group: "wgpolicyk8s.io", Version: "v1beta1", Resource: "policyreports"},
	}
	clusterPolicyReportGVRs = []schema.GroupVersionResource{
		{Group: "wgpolicyk8s.io", Version: "v1alpha2", Resource: "clusterpolicyreports"},
		{Group: "wgpolicyk8s.io", Version: "v1beta1", Resource: "clusterpolicyreports"},
	}
)

type reportFetch struct {
	items     []map[string]any
	available bool
	message   string
	truncated bool
}

// fetchReports lists the first served version of a report CRD, paging so a
// large cluster does not produce one enormous response. A missing CRD is
// "not installed" (not an error); a Forbidden answer is reported as unavailable
// with an RBAC hint so the page explains rather than failing; anything else is
// a real error.
func (k *KubernetesProvider) fetchReports(ctx context.Context, gvrs []schema.GroupVersionResource, ns string) (reportFetch, error) {
	if k.dynamic == nil {
		return reportFetch{}, fmt.Errorf("dynamic client unavailable")
	}
	for _, gvr := range gvrs {
		res := reportFetch{available: true}
		cont := ""
		failed := false
		for page := 0; ; page++ {
			if page >= secMaxPages {
				res.truncated = true
				break
			}
			opts := metav1.ListOptions{Limit: secPageSize, Continue: cont}
			var list *unstructured.UnstructuredList
			var err error
			if ns != "" {
				list, err = k.dynamic.Resource(gvr).Namespace(ns).List(ctx, opts)
			} else {
				list, err = k.dynamic.Resource(gvr).List(ctx, opts)
			}
			if err == nil {
				for _, it := range list.Items {
					res.items = append(res.items, it.Object)
				}
			} else {
				if isMissingCRD(err) {
					failed = true
					break
				}
				if apierrors.IsForbidden(err) {
					return reportFetch{message: "Axiom's ServiceAccount is not allowed to read " + gvr.Resource + "." + gvr.Group + " (see deploy RBAC)"}, nil
				}
				return reportFetch{}, err
			}
			cont = list.GetContinue()
			if cont == "" {
				break
			}
		}
		if !failed {
			return res, nil
		}
	}
	return reportFetch{message: "CRD not installed"}, nil
}

func (k *KubernetesProvider) loadSecurityReports(ctx context.Context, ns string, needVuln, needConfig, needPolicy bool) (secReportSet, error) {
	nsArg := ""
	if ns != "" && ns != "all" {
		nsArg = ns
	}
	var set secReportSet
	add := func(name, group string, f reportFetch) {
		set.Sources = append(set.Sources, SecuritySource{Name: name, Group: group, Available: f.available, Reports: len(f.items), Message: f.message})
		set.Truncated = set.Truncated || f.truncated
	}
	if needVuln {
		f, err := k.fetchReports(ctx, trivyVulnGVRs, nsArg)
		if err != nil {
			return set, err
		}
		set.Vuln = f.items
		add("Trivy VulnerabilityReports", "aquasecurity.github.io", f)
	}
	if needConfig {
		f, err := k.fetchReports(ctx, trivyConfigGVRs, nsArg)
		if err != nil {
			return set, err
		}
		set.Config = f.items
		add("Trivy ConfigAuditReports", "aquasecurity.github.io", f)
	}
	if needPolicy {
		f, err := k.fetchReports(ctx, policyReportGVRs, nsArg)
		if err != nil {
			return set, err
		}
		if nsArg == "" {
			// Cluster-scoped results (Kyverno ClusterPolicyReport) only appear in
			// the all-namespaces view; a namespace-scoped caller never sees them.
			cf, err := k.fetchReports(ctx, clusterPolicyReportGVRs, "")
			if err != nil {
				return set, err
			}
			f.items = append(f.items, cf.items...)
			f.truncated = f.truncated || cf.truncated
		}
		set.Policy = f.items
		add("PolicyReports", "wgpolicyk8s.io", f)
	}
	return set, nil
}

func (k *KubernetesProvider) ListSecurityReports(ctx context.Context, ns string) (SecurityReportSummary, error) {
	set, err := k.loadSecurityReports(ctx, ns, true, true, true)
	if err != nil {
		return SecurityReportSummary{}, err
	}
	return set.summary(), nil
}

func (k *KubernetesProvider) GetWorkloadSecurity(ctx context.Context, ns, kind, name string) (WorkloadSecurityDetail, error) {
	if err := validateWorkloadRef(ns, kind, name); err != nil {
		return WorkloadSecurityDetail{}, err
	}
	if ns == "all" {
		return WorkloadSecurityDetail{}, NewStatusError(400, fmt.Errorf("a namespace is required"))
	}
	set, err := k.loadSecurityReports(ctx, ns, true, true, true)
	if err != nil {
		return WorkloadSecurityDetail{}, err
	}
	return set.detail(ns, kind, name), nil
}
