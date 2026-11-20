//go:build kubernetes

package platform

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (k *KubernetesProvider) ListKarpenter(ctx context.Context) (KarpenterInventory, error) {
	inv := emptyKarpenter("")
	if k.dynamic == nil {
		inv.Note = "dynamic client unavailable"
		return inv, nil
	}
	list := func(resource string) (*unstructured.UnstructuredList, bool) {
		for _, ver := range []string{"v1", "v1beta1"} {
			gvr := schema.GroupVersionResource{Group: "karpenter.sh", Version: ver, Resource: resource}
			if l, err := k.dynamic.Resource(gvr).List(ctx, metav1.ListOptions{Limit: 500}); err == nil {
				return l, true
			}
		}
		return nil, false
	}
	pools, ok := list("nodepools")
	if !ok {
		inv.Note = "Karpenter (karpenter.sh) is not installed or not readable"
		return inv, nil
	}
	inv.Available = true
	for _, it := range pools.Items {
		inv.NodePools = append(inv.NodePools, karpenterNodePool(it, age(it.GetCreationTimestamp().Time)))
	}
	if claims, ok := list("nodeclaims"); ok {
		for _, it := range claims.Items {
			inv.NodeClaims = append(inv.NodeClaims, karpenterNodeClaim(it, age(it.GetCreationTimestamp().Time)))
		}
	}
	sortKarpenter(&inv)
	return inv, nil
}
