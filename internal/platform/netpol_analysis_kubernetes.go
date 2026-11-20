//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func npSelectorFrom(ls *metav1.LabelSelector) npSelector {
	if ls == nil {
		return npSelector{}
	}
	s := npSelector{MatchLabels: ls.MatchLabels}
	for _, e := range ls.MatchExpressions {
		s.Exprs = append(s.Exprs, npExpr{Key: e.Key, Op: string(e.Operator), Values: e.Values})
	}
	return s
}

func npPortsFrom(ports []netv1.NetworkPolicyPort) []npPort {
	out := make([]npPort, 0, len(ports))
	for _, p := range ports {
		np := npPort{Protocol: "TCP"}
		if p.Protocol != nil {
			np.Protocol = string(*p.Protocol)
		}
		if p.Port != nil {
			np.Port = p.Port.String()
		}
		if p.EndPort != nil {
			np.EndPort = int(*p.EndPort)
		}
		out = append(out, np)
	}
	return out
}

// matchesNothing is a selector no label set satisfies (unknown operator).
var matchesNothing = npSelector{Exprs: []npExpr{{Key: "invalid", Op: "Invalid"}}}

func npPeersFrom(peers []netv1.NetworkPolicyPeer) []npPeer {
	out := make([]npPeer, 0, len(peers))
	for _, p := range peers {
		switch {
		case p.IPBlock != nil:
			out = append(out, npPeer{CIDR: p.IPBlock.CIDR, Except: p.IPBlock.Except})
		case p.PodSelector != nil || p.NamespaceSelector != nil:
			var peer npPeer
			if p.PodSelector != nil {
				s := npSelectorFrom(p.PodSelector)
				peer.Pod = &s
			}
			if p.NamespaceSelector != nil {
				s := npSelectorFrom(p.NamespaceSelector)
				peer.Namespace = &s
			}
			out = append(out, peer)
		default:
			// A peer with neither selector nor ipBlock is invalid. An empty peer
			// list means "any peer", so dropping it would fail OPEN; keep a peer
			// that matches nothing instead.
			ms := matchesNothing
			out = append(out, npPeer{Pod: &ms})
		}
	}
	return out
}

func (k *KubernetesProvider) AnalyzeNetworkPolicies(ctx context.Context, ns string) (NetworkPolicyAnalysis, error) {
	ns = strings.TrimSpace(ns)
	if ns == "" || ns == "all" {
		return NetworkPolicyAnalysis{}, NewStatusError(400, fmt.Errorf("a namespace is required"))
	}
	if _, err := k.client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{}); err != nil {
		return NetworkPolicyAnalysis{}, err
	}
	pods, err := k.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return NetworkPolicyAnalysis{}, err
	}
	nss, err := k.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return NetworkPolicyAnalysis{}, err
	}
	pols, err := k.client.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return NetworkPolicyAnalysis{}, err
	}
	in := npInput{Namespace: ns}
	for _, n := range nss.Items {
		labels := map[string]string{}
		for lk, lv := range n.Labels {
			labels[lk] = lv
		}
		// The API server stamps this label on every namespace; synthesise it
		// so selectors on it resolve identically on clusters/fakes that don't.
		if _, ok := labels["kubernetes.io/metadata.name"]; !ok {
			labels["kubernetes.io/metadata.name"] = n.Name
		}
		in.Namespaces = append(in.Namespaces, npNamespace{Name: n.Name, Labels: labels})
	}
	for _, p := range pods.Items {
		// Pods that already finished never receive traffic.
		if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
			continue
		}
		in.Pods = append(in.Pods, npPod{Name: p.Name, Labels: p.Labels})
	}
	for _, np := range pols.Items {
		pol := npPolicy{Namespace: np.Namespace, Name: np.Name, PodSelector: npSelectorFrom(&np.Spec.PodSelector)}
		for _, t := range np.Spec.PolicyTypes {
			pol.Types = append(pol.Types, string(t))
		}
		for _, r := range np.Spec.Ingress {
			pol.Ingress = append(pol.Ingress, npRule{Peers: npPeersFrom(r.From), Ports: npPortsFrom(r.Ports)})
		}
		for _, r := range np.Spec.Egress {
			pol.Egress = append(pol.Egress, npRule{Peers: npPeersFrom(r.To), Ports: npPortsFrom(r.Ports)})
		}
		in.Policies = append(in.Policies, pol)
	}
	return analyzeNetworkPolicies(in), nil
}
