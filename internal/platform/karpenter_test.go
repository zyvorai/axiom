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
