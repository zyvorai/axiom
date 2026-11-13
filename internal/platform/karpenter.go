package platform

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// KarpenterNodePool is one karpenter.sh NodePool (read-only).
type KarpenterNodePool struct {
	Name             string `json:"name"`
	NodeClass        string `json:"nodeClass,omitempty"`
	Policy           string `json:"policy,omitempty"` // consolidationPolicy
	ConsolidateAfter string `json:"consolidateAfter,omitempty"`
	CPULimit         string `json:"cpuLimit,omitempty"`
	MemoryLimit      string `json:"memoryLimit,omitempty"`
	Nodes            string `json:"nodes,omitempty"` // status.resources.nodes
	CPU              string `json:"cpu,omitempty"`   // status.resources.cpu
	Ready            string `json:"ready"`           // True | False | Unknown
	Age              string `json:"age,omitempty"`
}

// KarpenterNodeClaim is one karpenter.sh NodeClaim (read-only).
type KarpenterNodeClaim struct {
	Name         string `json:"name"`
	NodePool     string `json:"nodePool,omitempty"`
	Node         string `json:"node,omitempty"`
	InstanceType string `json:"instanceType,omitempty"`
	CapacityType string `json:"capacityType,omitempty"` // on-demand | spot
	Zone         string `json:"zone,omitempty"`
	Phase        string `json:"phase"` // Initialized | Registered | Launched | Pending
	Age          string `json:"age,omitempty"`
}

// KarpenterInventory is GET /api/v1/karpenter.
type KarpenterInventory struct {
	Available  bool                 `json:"available"`
	Note       string               `json:"note,omitempty"`
	NodePools  []KarpenterNodePool  `json:"nodePools"`
	NodeClaims []KarpenterNodeClaim `json:"nodeClaims"`
}

// ListKarpenter returns a read-only Karpenter inventory when karpenter.sh is installed.
func ListKarpenter(ctx context.Context, p Provider) (KarpenterInventory, error) {
	if l, ok := p.(interface {
		ListKarpenter(context.Context) (KarpenterInventory, error)
	}); ok {
		return l.ListKarpenter(ctx)
	}
	return emptyKarpenter("Karpenter view not available on this provider"), nil
}

func emptyKarpenter(note string) KarpenterInventory {
	return KarpenterInventory{Note: note, NodePools: []KarpenterNodePool{}, NodeClaims: []KarpenterNodeClaim{}}
}

func karpenterCondStatus(it unstructured.Unstructured, typ string) string {
	conds, _, _ := unstructured.NestedSlice(it.Object, "status", "conditions")
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t == typ {
			if s, _ := m["status"].(string); s != "" {
				return s
			}
		}
	}
	return "Unknown"
}

// karpenterNodePool maps a NodePool object; age is supplied by the caller.
func karpenterNodePool(it unstructured.Unstructured, age string) KarpenterNodePool {
	str := func(path ...string) string { v, _, _ := unstructured.NestedString(it.Object, path...); return v }
	np := KarpenterNodePool{
		Name:             it.GetName(),
		NodeClass:        str("spec", "template", "spec", "nodeClassRef", "name"),
		Policy:           str("spec", "disruption", "consolidationPolicy"),
		ConsolidateAfter: str("spec", "disruption", "consolidateAfter"),
		CPULimit:         str("spec", "limits", "cpu"),
		MemoryLimit:      str("spec", "limits", "memory"),
		Nodes:            karpenterNodeCount(it),
		CPU:              str("status", "resources", "cpu"),
		Ready:            karpenterCondStatus(it, "Ready"),
		Age:              age,
	}
	return np
}

// karpenterNodeCount reads the NodePool's node count. Karpenter v1 keeps it as an integer in
// status.nodes (the "Nodes" printer column); older shapes carried it as status.resources.nodes.
func karpenterNodeCount(it unstructured.Unstructured) string {
	if n, found, err := unstructured.NestedInt64(it.Object, "status", "nodes"); err == nil && found {
		return strconv.FormatInt(n, 10)
	}
	if s, found, err := unstructured.NestedString(it.Object, "status", "resources", "nodes"); err == nil && found {
		return s
	}
	return ""
}

// karpenterNodeClaim maps a NodeClaim; the phase is the furthest lifecycle condition reached.
func karpenterNodeClaim(it unstructured.Unstructured, age string) KarpenterNodeClaim {
	lbl := it.GetLabels()
	node, _, _ := unstructured.NestedString(it.Object, "status", "nodeName")
	phase := "Pending"
	for _, c := range []string{"Launched", "Registered", "Initialized"} {
		if karpenterCondStatus(it, c) == "True" {
			phase = c
		}
	}
	return KarpenterNodeClaim{
		Name:         it.GetName(),
		NodePool:     lbl["karpenter.sh/nodepool"],
		Node:         node,
		InstanceType: lbl["node.kubernetes.io/instance-type"],
		CapacityType: lbl["karpenter.sh/capacity-type"],
		Zone:         lbl["topology.kubernetes.io/zone"],
		Phase:        phase,
		Age:          age,
	}
}

func sortKarpenter(inv *KarpenterInventory) {
	sort.Slice(inv.NodePools, func(i, j int) bool { return inv.NodePools[i].Name < inv.NodePools[j].Name })
	sort.Slice(inv.NodeClaims, func(i, j int) bool {
		a, b := inv.NodeClaims[i], inv.NodeClaims[j]
		if a.NodePool != b.NodePool {
			return strings.Compare(a.NodePool, b.NodePool) < 0
		}
		return a.Name < b.Name
	})
}

func (d *DemoProvider) ListKarpenter(ctx context.Context) (KarpenterInventory, error) {
	return KarpenterInventory{
		Available: true,
		Note:      "Demo Karpenter inventory (simulated)",
		NodePools: []KarpenterNodePool{
			{Name: "default", NodeClass: "default", Policy: "WhenEmptyOrUnderutilized", ConsolidateAfter: "30s",
				CPULimit: "200", Nodes: "3", CPU: "24", Ready: "True", Age: "21d"},
		},
		NodeClaims: []KarpenterNodeClaim{
			{Name: "default-x7k2p", NodePool: "default", Node: "ip-10-0-1-21", InstanceType: "m6i.2xlarge", CapacityType: "on-demand", Zone: "us-east-1a", Phase: "Initialized", Age: "6d"},
			{Name: "default-q9d4r", NodePool: "default", Node: "ip-10-0-2-37", InstanceType: "c6a.4xlarge", CapacityType: "spot", Zone: "us-east-1b", Phase: "Initialized", Age: "2d"},
			{Name: "default-m3w8z", NodePool: "default", InstanceType: "c6a.4xlarge", CapacityType: "spot", Zone: "us-east-1c", Phase: "Launched", Age: "40s"},
		},
	}, nil
}
