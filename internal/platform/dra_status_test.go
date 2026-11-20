package platform

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEnrichDRAClaimAllocated(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{
			"phase": "Allocated",
			"allocation": map[string]any{
				"devices": map[string]any{
					"results": []any{
						map[string]any{"driver": "gpu.nvidia.com", "device": "gpu-0"},
					},
				},
			},
			"reservedFor": []any{
				map[string]any{"resource": "pods", "name": "train-0", "uid": "x"},
			},
		},
	}}
	out := enrichDRAObject("ResourceClaim", *u, DRAObject{Kind: "ResourceClaim", Name: "c"})
	if out.Status != "Allocated" || out.Driver != "gpu.nvidia.com" {
		t.Fatalf("%+v", out)
	}
	if out.Detail != "devices=gpu-0 · pod=train-0" {
		t.Fatalf("detail %q", out.Detail)
	}
}

func TestEnrichDRAClaimPending(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{}}}
	out := enrichDRAObject("ResourceClaim", *u, DRAObject{Name: "p"})
	if out.Status != "Pending" {
		t.Fatalf("%+v", out)
	}
}
