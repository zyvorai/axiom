//go:build kubernetes

package platform

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (k *KubernetesProvider) ListDRA(ctx context.Context) (DRAInventory, error) {
	inv := DRAInventory{Objects: []DRAObject{}, Counts: map[string]int{}}
	type kindGVR struct {
		kind string
		gvr  schema.GroupVersionResource
		ns   bool
	}
	versions := []string{"v1", "v1beta1", "v1alpha3"}
	kinds := []struct {
		kind, resource string
		ns             bool
	}{
		{"DeviceClass", "deviceclasses", false},
		{"ResourceSlice", "resourceslices", false},
		{"ResourceClaim", "resourceclaims", true},
		{"ResourceClaimTemplate", "resourceclaimtemplates", true},
	}
	any := false
	for _, kg := range kinds {
		var list *unstructured.UnstructuredList
		var err error
		for _, ver := range versions {
			gvr := schema.GroupVersionResource{Group: "resource.k8s.io", Version: ver, Resource: kg.resource}
			if kg.ns {
				list, err = k.dynamic.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 200})
			} else {
				list, err = k.dynamic.Resource(gvr).List(ctx, metav1.ListOptions{Limit: 200})
			}
			if err == nil {
				break
			}
		}
		if err != nil || list == nil {
			continue
		}
		any = true
		for _, it := range list.Items {
			driver, _, _ := unstructured.NestedString(it.Object, "spec", "driver")
			inv.Objects = append(inv.Objects, DRAObject{
				Kind: kg.kind, Namespace: it.GetNamespace(), Name: it.GetName(),
				Driver: driver, Age: age(it.GetCreationTimestamp().Time),
				Detail: fmt.Sprintf("%s/%s", kg.kind, it.GetName()),
			})
		}
		inv.Counts[kg.kind] = len(list.Items)
	}
	inv.Available = any
	if !any {
		inv.Note = "resource.k8s.io CRDs not installed"
	}
	return inv, nil
}
