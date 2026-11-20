//go:build kubernetes

package platform

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (k *KubernetesProvider) ListRollouts(ctx context.Context, namespace string) (RolloutsInventory, error) {
	inv := emptyRollouts("")
	if k.dynamic == nil {
		inv.Note = "dynamic client unavailable"
		return inv, nil
	}
	if namespace == "all" {
		namespace = ""
	}
	list := func(group, ver, resource string) (*unstructured.UnstructuredList, bool) {
		gvr := schema.GroupVersionResource{Group: group, Version: ver, Resource: resource}
		l, err := k.dynamic.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{Limit: 500})
		return l, err == nil
	}
	if rs, ok := list("argoproj.io", "v1alpha1", "rollouts"); ok {
		inv.Available = true
		runs := map[string][2]string{}
		if ar, ok := list("argoproj.io", "v1alpha1", "analysisruns"); ok {
			runs = latestAnalysisRuns(ar.Items)
		}
		for _, it := range rs.Items {
			r := runs[it.GetNamespace()+"/"+it.GetName()]
			inv.Rollouts = append(inv.Rollouts, argoRollout(it, r[0], r[1], age(it.GetCreationTimestamp().Time)))
		}
	}
	if cs, ok := list("flagger.app", "v1beta1", "canaries"); ok {
		inv.Available = true
		for _, it := range cs.Items {
			inv.Canaries = append(inv.Canaries, flaggerCanary(it))
		}
	}
	if !inv.Available {
		inv.Note = "Argo Rollouts and Flagger are not installed or not readable"
	}
	sortRollouts(&inv)
	return inv, nil
}
