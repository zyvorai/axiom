package platform

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestKarpenterMapping(t *testing.T) {
	pool := unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "default"},
		"spec": map[string]any{
			"limits":     map[string]any{"cpu": "200"},
			"disruption": map[string]any{"consolidationPolicy": "WhenEmpty"},
			"template":   map[string]any{"spec": map[string]any{"nodeClassRef": map[string]any{"name": "nc"}}},
		},
		"status": map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "True"}}},
	}}
	np := karpenterNodePool(pool, "1d")
	if np.NodeClass != "nc" || np.Policy != "WhenEmpty" || np.CPULimit != "200" || np.Ready != "True" {
		t.Fatalf("pool: %+v", np)
	}

	claim := unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "c1", "labels": map[string]any{
			"karpenter.sh/nodepool": "default", "karpenter.sh/capacity-type": "spot",
			"node.kubernetes.io/instance-type": "m6i.large"}},
		"status": map[string]any{"nodeName": "n1", "conditions": []any{
			map[string]any{"type": "Launched", "status": "True"},
			map[string]any{"type": "Registered", "status": "True"},
			map[string]any{"type": "Initialized", "status": "False"}}},
	}}
	c := karpenterNodeClaim(claim, "5s")
	if c.Phase != "Registered" || c.CapacityType != "spot" || c.NodePool != "default" || c.Node != "n1" {
		t.Fatalf("claim: %+v", c)
	}
	if (karpenterNodeClaim(unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "x"}}}, "")).Phase != "Pending" {
		t.Fatal("empty claim should be Pending")
	}
}

// Karpenter v1 reports the node count as an integer in status.nodes (the CRD's "Nodes" printer
// column is .status.nodes) and only resource quantities under status.resources.
func TestKarpenterNodePoolNodeCount(t *testing.T) {
	mk := func(status map[string]any) unstructured.Unstructured {
		return unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": "p"},
			"status":   status,
		}}
	}
	cases := []struct {
		name   string
		status map[string]any
		want   string
	}{
		{"v1 integer status.nodes", map[string]any{"nodes": int64(3), "resources": map[string]any{"cpu": "24", "memory": "96Gi"}}, "3"},
		{"zero nodes is shown", map[string]any{"nodes": int64(0)}, "0"},
		{"older status.resources.nodes", map[string]any{"resources": map[string]any{"nodes": "5"}}, "5"},
		{"absent", map[string]any{}, ""},
	}
	for _, c := range cases {
		got := karpenterNodePool(mk(c.status), "1d")
		if got.Nodes != c.want {
			t.Errorf("%s: Nodes = %q, want %q", c.name, got.Nodes, c.want)
		}
	}
	if got := karpenterNodePool(mk(map[string]any{"nodes": int64(3), "resources": map[string]any{"cpu": "24"}}), ""); got.CPU != "24" {
		t.Errorf("CPU = %q, want 24", got.CPU)
	}
}
