package platform

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// NetworkPolicy analyzer. Pure: the Kubernetes provider converts networking/v1
// objects into the np* types below, the demo provider supplies fixed ones.
//
// Kubernetes semantics implemented (https://kubernetes.io/docs/concepts/services-networking/network-policies/):
//   - A pod is "isolated" for a direction when at least one policy in its
//     namespace selects it (spec.podSelector) AND that policy's effective
//     policyTypes include the direction. Un-isolated pods allow everything.
//   - policyTypes defaults to [Ingress] (plus Egress when the policy has egress rules).
//   - For isolated pods, allowed traffic is the UNION of the rules of every
//     selecting policy; an empty rule list allows nothing.
//   - A rule with no `from`/`to` peers matches every peer; one with no `ports`
//     matches every port.
//
// Not modelled (and flagged where relevant): CNI-specific extensions, hostNetwork
// pods, and enforcement - a cluster without a NetworkPolicy-capable CNI ignores
// these objects entirely.

const (
	npMaxPodsPerPeer = 25
	npMaxNamespaces  = 25
)

type npExpr struct {
	Key    string
	Op     string // In | NotIn | Exists | DoesNotExist
	Values []string
}

type npSelector struct {
	MatchLabels map[string]string
	Exprs       []npExpr
}

func (s npSelector) empty() bool { return len(s.MatchLabels) == 0 && len(s.Exprs) == 0 }

func (s npSelector) matches(labels map[string]string) bool {
	for k, v := range s.MatchLabels {
		if got, ok := labels[k]; !ok || got != v {
			return false
		}
	}
	for _, e := range s.Exprs {
		got, has := labels[e.Key]
		in := false
		for _, v := range e.Values {
			if has && got == v {
				in = true
			}
		}
		switch e.Op {
		case "In":
			if !in {
				return false
			}
		case "NotIn":
			if in {
				return false
			}
		case "Exists":
			if !has {
				return false
			}
		case "DoesNotExist":
			if has {
				return false
			}
		default:
			return false // unknown operator: fail closed (select nothing)
		}
	}
	return true
}

func (s npSelector) describe() string {
	if s.empty() {
		return "all"
	}
	parts := make([]string, 0, len(s.MatchLabels)+len(s.Exprs))
	for k, v := range s.MatchLabels {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	for _, e := range s.Exprs {
		switch e.Op {
		case "In":
			parts = append(parts, fmt.Sprintf("%s in (%s)", e.Key, strings.Join(e.Values, ",")))
		case "NotIn":
			parts = append(parts, fmt.Sprintf("%s notin (%s)", e.Key, strings.Join(e.Values, ",")))
		case "Exists":
			parts = append(parts, e.Key)
		case "DoesNotExist":
			parts = append(parts, "!"+e.Key)
		}
	}
	return strings.Join(parts, ", ")
}

type npPeer struct {
	Pod       *npSelector
	Namespace *npSelector
	CIDR      string
	Except    []string
}

type npPort struct {
	Protocol string
	Port     string // number or named port; empty = every port of the protocol
	EndPort  int
}

type npRule struct {
	Peers []npPeer
	Ports []npPort
}

type npPolicy struct {
	Namespace   string
	Name        string
	PodSelector npSelector
	Types       []string
	Ingress     []npRule
	Egress      []npRule
}

func (p npPolicy) hasType(t string) bool {
	if len(p.Types) == 0 {
		if t == "Ingress" {
			return true
		}
		return len(p.Egress) > 0
	}
	for _, x := range p.Types {
		if x == t {
			return true
		}
	}
	return false
}

type npNamespace struct {
	Name   string
	Labels map[string]string
}

type npPod struct {
	Name   string
	Labels map[string]string
}

type npInput struct {
	Namespace  string
	Namespaces []npNamespace // every namespace (for namespaceSelector resolution)
	Pods       []npPod       // pods of Namespace only
	Policies   []npPolicy    // policies of Namespace only
}

func formatPorts(ports []npPort) []string {
	if len(ports) == 0 {
		return []string{"all ports"}
	}
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		proto := p.Protocol
		if proto == "" {
			proto = "TCP"
		}
		switch {
		case p.Port == "":
			out = append(out, proto+"/*")
		case p.EndPort > 0:
			out = append(out, fmt.Sprintf("%s/%s-%d", proto, p.Port, p.EndPort))
		default:
			out = append(out, proto+"/"+p.Port)
		}
	}
	return out
}

func (in npInput) matchNamespaces(sel npSelector) []string {
	var out []string
	for _, ns := range in.Namespaces {
		if sel.matches(ns.Labels) {
			out = append(out, ns.Name)
		}
	}
	sort.Strings(out)
	if len(out) > npMaxNamespaces {
		out = out[:npMaxNamespaces]
	}
	return out
}

func (in npInput) matchPods(sel npSelector) (names []string, total int) {
	for _, p := range in.Pods {
		if sel.matches(p.Labels) {
			total++
			if len(names) < npMaxPodsPerPeer {
				names = append(names, p.Name)
			}
		}
	}
	sort.Strings(names)
	return names, total
}

func (in npInput) allowsFromRules(policy string, rules []npRule) []NetworkPolicyAllow {
	var out []NetworkPolicyAllow
	for _, r := range rules {
		ports := formatPorts(r.Ports)
		if len(r.Peers) == 0 {
			out = append(out, NetworkPolicyAllow{Policy: policy, Kind: "any", Peer: "any peer", Ports: ports})
			continue
		}
		for _, peer := range r.Peers {
			a := NetworkPolicyAllow{Policy: policy, Ports: ports}
			switch {
			case peer.CIDR != "":
				a.Kind, a.CIDR, a.Except = "cidr", peer.CIDR, peer.Except
				a.Peer = "CIDR " + peer.CIDR
				if len(peer.Except) > 0 {
					a.Peer += " except " + strings.Join(peer.Except, ", ")
				}
			case peer.Pod != nil && peer.Namespace == nil:
				a.Kind = "pods"
				a.Pods, a.PodCount = in.matchPods(*peer.Pod)
				a.Peer = "pods (" + peer.Pod.describe() + ") in " + in.Namespace
			case peer.Pod == nil && peer.Namespace != nil:
				a.Kind = "namespace"
				a.Namespaces = in.matchNamespaces(*peer.Namespace)
				a.Peer = "all pods in namespaces (" + peer.Namespace.describe() + ")"
			case peer.Pod != nil && peer.Namespace != nil:
				a.Kind = "pods"
				a.Namespaces = in.matchNamespaces(*peer.Namespace)
				a.Peer = "pods (" + peer.Pod.describe() + ") in namespaces (" + peer.Namespace.describe() + ")"
				for _, n := range a.Namespaces {
					if n == in.Namespace {
						a.Pods, a.PodCount = in.matchPods(*peer.Pod)
					}
				}
			default:
				a.Kind, a.Peer = "any", "any peer"
			}
			out = append(out, a)
		}
	}
	return out
}

// analyzeNetworkPolicies computes the effective ingress/egress position of every
// pod in in.Namespace.
func analyzeNetworkPolicies(in npInput) NetworkPolicyAnalysis {
	res := NetworkPolicyAnalysis{
		Namespace: in.Namespace, Policies: len(in.Policies),
		Pods: []PodNetworkAnalysis{}, Uncovered: []string{}, Warnings: []string{}, Notes: []string{},
	}
	policies := append([]npPolicy(nil), in.Policies...)
	sort.Slice(policies, func(i, j int) bool { return policies[i].Name < policies[j].Name })

	for _, p := range policies {
		if p.PodSelector.empty() && len(p.Ingress) == 0 && p.hasType("Ingress") {
			res.DefaultDenyIngress = true
		}
		if p.PodSelector.empty() && len(p.Egress) == 0 && p.hasType("Egress") {
			res.DefaultDenyEgress = true
		}
	}

	pods := append([]npPod(nil), in.Pods...)
	sort.Slice(pods, func(i, j int) bool { return pods[i].Name < pods[j].Name })
	for _, pod := range pods {
		pa := PodNetworkAnalysis{Pod: pod.Name, Labels: pod.Labels, Policies: []string{}, Ingress: []NetworkPolicyAllow{}, Egress: []NetworkPolicyAllow{}}
		for _, pol := range policies {
			if !pol.PodSelector.matches(pod.Labels) {
				continue
			}
			pa.Policies = append(pa.Policies, pol.Name)
			if pol.hasType("Ingress") {
				pa.IngressIsolated = true
				pa.Ingress = append(pa.Ingress, in.allowsFromRules(pol.Name, pol.Ingress)...)
			}
			if pol.hasType("Egress") {
				pa.EgressIsolated = true
				pa.Egress = append(pa.Egress, in.allowsFromRules(pol.Name, pol.Egress)...)
			}
		}
		if len(pa.Policies) == 0 {
			res.Uncovered = append(res.Uncovered, pod.Name)
		}
		res.Pods = append(res.Pods, pa)
	}

	switch {
	case len(in.Policies) == 0 && len(in.Pods) > 0:
		res.Warnings = append(res.Warnings, "No NetworkPolicies in this namespace: every pod accepts and initiates traffic from/to anywhere.")
	case len(res.Uncovered) > 0:
		res.Warnings = append(res.Warnings, strconv.Itoa(len(res.Uncovered))+" pod(s) are not selected by any NetworkPolicy and are fully open in both directions.")
	}
	ingOnly := 0
	for _, p := range res.Pods {
		if p.IngressIsolated && !p.EgressIsolated {
			ingOnly++
		}
	}
	if ingOnly > 0 {
		res.Warnings = append(res.Warnings, strconv.Itoa(ingOnly)+" pod(s) are isolated for ingress only; their egress is unrestricted.")
	}
	res.Notes = append(res.Notes, "Policies only take effect when the cluster CNI enforces NetworkPolicy.")
	return res
}
