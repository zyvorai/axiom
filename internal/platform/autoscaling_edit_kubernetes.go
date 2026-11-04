//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// VPA / ScaledObject editing against the cluster. The pure logic is in
// autoscaling_edit.go; this file only loads the live object and sends the
// JSON Patch (whose first op tests metadata.resourceVersion).

func (k *KubernetesProvider) loadAutoscaler(ctx context.Context, gvrs []schema.GroupVersionResource, product, kind, ns, name string) (schema.GroupVersionResource, map[string]any, error) {
	gvr, err := k.resolveAutoscalerGVR(gvrs, product)
	if err != nil {
		return gvr, nil, err
	}
	obj, err := k.dynamic.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return gvr, nil, notFoundf("%s %s/%s not found", kind, ns, name)
		}
		return gvr, nil, err
	}
	return gvr, obj.Object, nil
}

// patchAutoscaler sends plan.Patch (if any) and returns the new resourceVersion.
func (k *KubernetesProvider) patchAutoscaler(ctx context.Context, gvr schema.GroupVersionResource, kind, ns, name string, plan autoscalerEditPlan) (string, error) {
	if plan.Patch == nil {
		return plan.RV, nil
	}
	out, err := k.dynamic.Resource(gvr).Namespace(ns).Patch(ctx, name, types.JSONPatchType, plan.Patch, metav1.PatchOptions{FieldManager: autoscalerManagedByValue})
	if err != nil {
		what := fmt.Sprintf("update %s %s/%s", kind, ns, name)
		// A failed JSON Patch "test" op surfaces as 422/400/500 depending on
		// the server version; its message always names the failed test.
		testFailed := strings.Contains(err.Error(), "test") && strings.Contains(err.Error(), "/metadata/resourceVersion") ||
			strings.Contains(err.Error(), "test failed")
		switch {
		case apierrors.IsConflict(err) || testFailed:
			return "", NewStatusError(http.StatusConflict, fmt.Errorf("%s: %s (%v)", what, autoscalerConflictText, err))
		case apierrors.IsNotFound(err):
			return "", notFoundf("%s: not found", what)
		}
		return "", fmt.Errorf("%s: %w", what, err)
	}
	return out.GetResourceVersion(), nil
}

func (k *KubernetesProvider) GetVPAEditable(ctx context.Context, ns, name string) (VPAEditable, error) {
	_, obj, err := k.loadAutoscaler(ctx, vpaGVRs, "The Vertical Pod Autoscaler", "VerticalPodAutoscaler", ns, name)
	if err != nil {
		return VPAEditable{}, err
	}
	return readVPAEditable(obj), nil
}

func (k *KubernetesProvider) PreviewVPAEdit(ctx context.Context, ns, name string, req VPAEditRequest) (AutoscalerEditPreview, error) {
	_, obj, err := k.loadAutoscaler(ctx, vpaGVRs, "The Vertical Pod Autoscaler", "VerticalPodAutoscaler", ns, name)
	if err != nil {
		return AutoscalerEditPreview{}, err
	}
	plan, err := planVPAEdit(obj, req)
	return plan.Preview, err
}

// UpdateVPA re-reads the object, refuses a stale resourceVersion, and patches
// only the managed fields.
func (k *KubernetesProvider) UpdateVPA(ctx context.Context, ns, name string, req VPAEditRequest) (AutoscalerEditResult, error) {
	gvr, obj, err := k.loadAutoscaler(ctx, vpaGVRs, "The Vertical Pod Autoscaler", "VerticalPodAutoscaler", ns, name)
	if err != nil {
		return AutoscalerEditResult{}, err
	}
	plan, err := planVPAEdit(obj, req)
	if err != nil {
		return AutoscalerEditResult{}, err
	}
	rv, err := k.patchAutoscaler(ctx, gvr, "VerticalPodAutoscaler", ns, name, plan)
	if err != nil {
		return AutoscalerEditResult{}, err
	}
	res := plan.result(rv)
	hpas, warn := k.namespaceHPAs(ctx, ns)
	res.Warnings = append(append(res.Warnings, warn...), vpaHPAConflictWarnings(plan.Build, hpas)...)
	return res, nil
}

func (k *KubernetesProvider) GetScaledObjectEditable(ctx context.Context, ns, name string) (ScaledObjectEditable, error) {
	_, obj, err := k.loadAutoscaler(ctx, scaledObjectGVRs, "KEDA", "ScaledObject", ns, name)
	if err != nil {
		return ScaledObjectEditable{}, err
	}
	return readScaledObjectEditable(obj), nil
}

func (k *KubernetesProvider) PreviewScaledObjectEdit(ctx context.Context, ns, name string, req ScaledObjectEditRequest) (AutoscalerEditPreview, error) {
	_, obj, err := k.loadAutoscaler(ctx, scaledObjectGVRs, "KEDA", "ScaledObject", ns, name)
	if err != nil {
		return AutoscalerEditPreview{}, err
	}
	plan, err := planScaledObjectEdit(obj, req)
	return plan.Preview, err
}

// UpdateScaledObject re-reads the object, refuses a stale resourceVersion,
// checks every referenced TriggerAuthentication exists (as create does) and
// patches only the managed fields.
func (k *KubernetesProvider) UpdateScaledObject(ctx context.Context, ns, name string, req ScaledObjectEditRequest) (AutoscalerEditResult, error) {
	gvr, obj, err := k.loadAutoscaler(ctx, scaledObjectGVRs, "KEDA", "ScaledObject", ns, name)
	if err != nil {
		return AutoscalerEditResult{}, err
	}
	plan, err := planScaledObjectEdit(obj, req)
	if err != nil {
		return AutoscalerEditResult{}, err
	}
	if len(plan.Build.AuthRefs) > 0 && plan.Patch != nil {
		taGVR, err := k.resolveAutoscalerGVR(triggerAuthenticationGVRs, "KEDA")
		if err != nil {
			return AutoscalerEditResult{}, err
		}
		for _, ref := range plan.Build.AuthRefs {
			if _, gerr := k.dynamic.Resource(taGVR).Namespace(ns).Get(ctx, ref, metav1.GetOptions{}); gerr != nil {
				if apierrors.IsNotFound(gerr) {
					return AutoscalerEditResult{}, hpaBadRequest("TriggerAuthentication %s not found in namespace %s", ref, ns)
				}
				return AutoscalerEditResult{}, fmt.Errorf("check TriggerAuthentication %s/%s: %w", ns, ref, gerr)
			}
		}
	}
	rv, err := k.patchAutoscaler(ctx, gvr, "ScaledObject", ns, name, plan)
	if err != nil {
		return AutoscalerEditResult{}, err
	}
	res := plan.result(rv)
	hpas, warn := k.namespaceHPAs(ctx, ns)
	res.Warnings = append(append(res.Warnings, warn...), kedaHPAConflictWarnings(plan.Build, hpas)...)
	return res, nil
}
