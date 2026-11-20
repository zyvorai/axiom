//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Create / delete for VerticalPodAutoscalers and KEDA ScaledObjects, and the
// TriggerAuthentication name list. One object per request, so no rollback.

var triggerAuthenticationGVRs = []schema.GroupVersionResource{
	{Group: "keda.sh", Version: "v1alpha1", Resource: "triggerauthentications"},
}

// resolveAutoscalerGVR asks discovery which candidate version the API server
// serves (newest first). A missing CRD is a 422 naming the product.
func (k *KubernetesProvider) resolveAutoscalerGVR(candidates []schema.GroupVersionResource, product string) (schema.GroupVersionResource, error) {
	if k.client == nil {
		return schema.GroupVersionResource{}, fmt.Errorf("kubernetes client unavailable")
	}
	if k.dynamic == nil {
		return schema.GroupVersionResource{}, fmt.Errorf("dynamic client unavailable")
	}
	disc := k.client.Discovery()
	for _, gvr := range candidates {
		list, err := disc.ServerResourcesForGroupVersion(gvr.GroupVersion().String())
		if err != nil {
			if isMissingCRD(err) {
				continue
			}
			return schema.GroupVersionResource{}, err
		}
		for _, r := range list.APIResources {
			if r.Name == gvr.Resource {
				return gvr, nil
			}
		}
	}
	return schema.GroupVersionResource{}, NewStatusError(http.StatusUnprocessableEntity,
		fmt.Errorf("%s is not installed: no %s.%s CRD", product, candidates[0].Resource, candidates[0].Group))
}

// createAutoscaler creates plan's object at the served version.
func (k *KubernetesProvider) createAutoscaler(ctx context.Context, gvr schema.GroupVersionResource, plan autoscalerPlan, warnings []string) (AutoscalerCreateResult, error) {
	o := plan.Object
	obj := &unstructured.Unstructured{Object: o.withVersion(gvr.Version)}
	if _, err := k.dynamic.Resource(gvr).Namespace(o.Namespace).Create(ctx, obj, metav1.CreateOptions{FieldManager: autoscalerManagedByValue}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return AutoscalerCreateResult{}, NewStatusError(http.StatusConflict, fmt.Errorf("%s %s/%s already exists", o.Kind, o.Namespace, o.Name))
		}
		return AutoscalerCreateResult{}, fmt.Errorf("create %s %s/%s: %w", o.Kind, o.Namespace, o.Name, err)
	}
	return AutoscalerCreateResult{OK: true, Created: []AutoscalerObjectRef{o.ref(gvr.Version)}, Warnings: nonNilStrings(warnings)}, nil
}

// namespaceHPAs lists the HPAs in ns for the conflict warnings; a failure only
// turns into a warning that the check could not run.
func (k *KubernetesProvider) namespaceHPAs(ctx context.Context, ns string) ([]HPASummary, []string) {
	hpas, err := k.ListHPAs(ctx, ns)
	if err != nil {
		return nil, []string{"could not check for HorizontalPodAutoscalers on the same workload: " + err.Error()}
	}
	return hpas, nil
}

func (k *KubernetesProvider) CreateVPA(ctx context.Context, req VPACreateRequest) (AutoscalerCreateResult, error) {
	plan, err := buildVPA(req)
	if err != nil {
		return AutoscalerCreateResult{}, err
	}
	gvr, err := k.resolveAutoscalerGVR(vpaGVRs, "The Vertical Pod Autoscaler")
	if err != nil {
		return AutoscalerCreateResult{}, err
	}
	hpas, warn := k.namespaceHPAs(ctx, req.Namespace)
	warnings := append(append(plan.Warnings, warn...), vpaHPAConflictWarnings(plan, hpas)...)
	return k.createAutoscaler(ctx, gvr, plan, warnings)
}

func (k *KubernetesProvider) DeleteVPA(ctx context.Context, ns, name string) error {
	gvr, err := k.resolveAutoscalerGVR(vpaGVRs, "The Vertical Pod Autoscaler")
	if err != nil {
		return err
	}
	return k.dynamic.Resource(gvr).Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *KubernetesProvider) CreateScaledObject(ctx context.Context, req ScaledObjectCreateRequest) (AutoscalerCreateResult, error) {
	plan, err := buildScaledObject(req)
	if err != nil {
		return AutoscalerCreateResult{}, err
	}
	gvr, err := k.resolveAutoscalerGVR(scaledObjectGVRs, "KEDA")
	if err != nil {
		return AutoscalerCreateResult{}, err
	}
	if len(plan.AuthRefs) > 0 {
		taGVR, err := k.resolveAutoscalerGVR(triggerAuthenticationGVRs, "KEDA")
		if err != nil {
			return AutoscalerCreateResult{}, err
		}
		for _, ref := range plan.AuthRefs {
			if _, gerr := k.dynamic.Resource(taGVR).Namespace(req.Namespace).Get(ctx, ref, metav1.GetOptions{}); gerr != nil {
				if apierrors.IsNotFound(gerr) {
					return AutoscalerCreateResult{}, hpaBadRequest("TriggerAuthentication %s not found in namespace %s", ref, req.Namespace)
				}
				return AutoscalerCreateResult{}, fmt.Errorf("check TriggerAuthentication %s/%s: %w", req.Namespace, ref, gerr)
			}
		}
	}
	hpas, warn := k.namespaceHPAs(ctx, req.Namespace)
	warnings := append(append(plan.Warnings, warn...), kedaHPAConflictWarnings(plan, hpas)...)
	return k.createAutoscaler(ctx, gvr, plan, warnings)
}

func (k *KubernetesProvider) DeleteScaledObject(ctx context.Context, ns, name string) error {
	gvr, err := k.resolveAutoscalerGVR(scaledObjectGVRs, "KEDA")
	if err != nil {
		return err
	}
	return k.dynamic.Resource(gvr).Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

// ListTriggerAuthentications returns TriggerAuthentication names only; the
// spec (secret references, pod identity) is never copied out.
func (k *KubernetesProvider) ListTriggerAuthentications(ctx context.Context, ns string) (TriggerAuthenticationList, error) {
	empty := TriggerAuthenticationList{Installed: false, Items: []TriggerAuthenticationSummary{}}
	if k.dynamic == nil {
		return empty, nil
	}
	list, _, err := k.listFirstGVR(ctx, triggerAuthenticationGVRs, ns)
	if err != nil {
		if isMissingCRD(err) {
			return empty, nil
		}
		return TriggerAuthenticationList{}, err
	}
	out := TriggerAuthenticationList{Installed: true, Items: make([]TriggerAuthenticationSummary, 0, len(list.Items))}
	for _, item := range list.Items {
		out.Items = append(out.Items, TriggerAuthenticationSummary{
			Namespace: item.GetNamespace(), Name: item.GetName(), Kind: "TriggerAuthentication",
			Age: age(item.GetCreationTimestamp().Time),
		})
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.Namespace == b.Namespace {
			return a.Name < b.Name
		}
		return a.Namespace < b.Namespace
	})
	return out, nil
}
