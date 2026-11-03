package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// demoNetPolDetails are the full specs behind the summaries in demoNetPols
// (ops_demo.go). A policy appears in the analysis only while its summary still
// exists, so deleting one in the UI is reflected here.
func demoNetPolDetails() map[string]npPolicy {
	sel := func(kv ...string) *npSelector {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return &npSelector{MatchLabels: m}
	}
	return map[string]npPolicy{
		"payments/deny-all-ingress": {Namespace: "payments", Name: "deny-all-ingress", Types: []string{"Ingress"}},
		"commerce/allow-checkout": {
			Namespace: "commerce", Name: "allow-checkout",
			PodSelector: npSelector{MatchLabels: map[string]string{"app": "checkout"}},
			Types:       []string{"Ingress", "Egress"},
			Ingress: []npRule{
				{Peers: []npPeer{{Namespace: sel("kubernetes.io/metadata.name", "ingress-nginx")}}, Ports: []npPort{{Protocol: "TCP", Port: "8080"}, {Protocol: "TCP", Port: "8443"}}},
				{Peers: []npPeer{{CIDR: "10.42.0.0/16", Except: []string{"10.42.9.0/24"}}}, Ports: []npPort{{Protocol: "TCP", Port: "8080"}}},
			},
			Egress: []npRule{
				{Peers: []npPeer{{Namespace: sel("kubernetes.io/metadata.name", "payments"), Pod: sel("app", "payments-api")}}, Ports: []npPort{{Protocol: "TCP", Port: "8080"}}},
				{Peers: []npPeer{{Namespace: sel("kubernetes.io/metadata.name", "kube-system"), Pod: sel("k8s-app", "kube-dns")}}, Ports: []npPort{{Protocol: "UDP", Port: "53"}, {Protocol: "TCP", Port: "53"}}},
			},
		},
	}
}

func (d *DemoProvider) AnalyzeNetworkPolicies(ctx context.Context, ns string) (NetworkPolicyAnalysis, error) {
	ns = strings.TrimSpace(ns)
	if ns == "" || ns == "all" {
		return NetworkPolicyAnalysis{}, NewStatusError(400, fmt.Errorf("a namespace is required"))
	}
	pods, _ := d.ListPods(ctx, ns)
	in := npInput{Namespace: ns}
	for _, p := range pods {
		in.Pods = append(in.Pods, npPod{Name: p.Name, Labels: p.Labels})
	}
	seen := map[string]bool{}
	nss, _ := d.ListNamespaces(ctx)
	for _, n := range nss {
		seen[n.Name] = true
		in.Namespaces = append(in.Namespaces, npNamespace{Name: n.Name, Labels: map[string]string{"kubernetes.io/metadata.name": n.Name}})
	}
	for _, extra := range []string{"ingress-nginx", "kube-system"} {
		if !seen[extra] {
			in.Namespaces = append(in.Namespaces, npNamespace{Name: extra, Labels: map[string]string{"kubernetes.io/metadata.name": extra}})
		}
	}
	if !seen[ns] && len(in.Pods) == 0 {
		return NetworkPolicyAnalysis{}, notFoundf("namespace %q not found", ns)
	}
	demoNetPolMu.Lock()
	live := make(map[string]bool, len(demoNetPols))
	for k := range demoNetPols {
		live[k] = true
	}
	demoNetPolMu.Unlock()
	details := demoNetPolDetails()
	keys := make([]string, 0, len(details))
	for k := range details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if live[k] && details[k].Namespace == ns {
			in.Policies = append(in.Policies, details[k])
		}
	}
	return analyzeNetworkPolicies(in), nil
}
