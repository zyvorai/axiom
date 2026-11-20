//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func (k *KubernetesProvider) SuspendGitOpsApp(ctx context.Context, kind, ns, name string, req GitOpsSuspendRequest) (GitOpsSuspendResult, error) {
	if k.dynamic == nil {
		return GitOpsSuspendResult{}, fmt.Errorf("dynamic client unavailable")
	}
	if req.ResourceVersion == "" {
		return GitOpsSuspendResult{}, NewStatusError(http.StatusBadRequest, fmt.Errorf("resourceVersion is required"))
	}
	switch kind {
	case "Kustomization":
		return k.suspendFlux(ctx, fluxKustomizationGVRs, ns, name, req)
	case "HelmRelease":
		return k.suspendFlux(ctx, fluxHelmReleaseGVRs, ns, name, req)
	case "Application":
		return k.suspendArgo(ctx, ns, name, req)
	case "ApplicationSet":
		return GitOpsSuspendResult{}, NewStatusError(http.StatusUnprocessableEntity, fmt.Errorf("ApplicationSet cannot be suspended here; edit the ApplicationSet instead"))
	default:
		return GitOpsSuspendResult{}, NewStatusError(http.StatusBadRequest, fmt.Errorf("unsupported gitops kind %q", kind))
	}
}

func (k *KubernetesProvider) suspendFlux(ctx context.Context, gvrs []schema.GroupVersionResource, ns, name string, req GitOpsSuspendRequest) (GitOpsSuspendResult, error) {
	obj, gvr, err := k.getFirstGVR(ctx, gvrs, ns, name)
	if err != nil {
		return GitOpsSuspendResult{}, err
	}
	if obj.GetResourceVersion() != req.ResourceVersion {
		return GitOpsSuspendResult{}, NewStatusError(http.StatusConflict, fmt.Errorf("resourceVersion conflict; refresh and retry"))
	}
	cur, _, _ := unstructured.NestedBool(obj.Object, "spec", "suspend")
	if cur == req.Suspend {
		return GitOpsSuspendResult{OK: true, Changed: false}, nil
	}
	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"suspend": req.Suspend}})
	_, err = k.dynamic.Resource(gvr).Namespace(ns).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return GitOpsSuspendResult{}, err
	}
	return GitOpsSuspendResult{OK: true, Changed: true}, nil
}

func (k *KubernetesProvider) suspendArgo(ctx context.Context, ns, name string, req GitOpsSuspendRequest) (GitOpsSuspendResult, error) {
	obj, gvr, err := k.getFirstGVR(ctx, argoAppGVRs, ns, name)
	if err != nil {
		return GitOpsSuspendResult{}, err
	}
	if obj.GetResourceVersion() != req.ResourceVersion {
		return GitOpsSuspendResult{}, NewStatusError(http.StatusConflict, fmt.Errorf("resourceVersion conflict; refresh and retry"))
	}
	_, hasAuto, _ := unstructured.NestedMap(obj.Object, "spec", "syncPolicy", "automated")
	wantAuto := !req.Suspend
	if hasAuto == wantAuto {
		return GitOpsSuspendResult{OK: true, Changed: false}, nil
	}
	var patch []byte
	if req.Suspend {
		policy, _, _ := unstructured.NestedMap(obj.Object, "spec", "syncPolicy")
		if policy == nil {
			policy = map[string]any{}
		}
		delete(policy, "automated")
		patch, _ = json.Marshal(map[string]any{"spec": map[string]any{"syncPolicy": policy}})
	} else {
		patch, _ = json.Marshal(map[string]any{
			"spec": map[string]any{
				"syncPolicy": map[string]any{"automated": map[string]any{}},
			},
		})
	}
	_, err = k.dynamic.Resource(gvr).Namespace(ns).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return GitOpsSuspendResult{}, err
	}
	return GitOpsSuspendResult{OK: true, Changed: true}, nil
}

func (k *KubernetesProvider) getFirstGVR(ctx context.Context, gvrs []schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, schema.GroupVersionResource, error) {
	var last error
	for _, gvr := range gvrs {
		obj, err := k.dynamic.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			return obj, gvr, nil
		}
		last = err
		if apierrors.IsNotFound(err) {
			return nil, gvr, notFoundf("%s/%s not found", ns, name)
		}
		if !isMissingCRD(err) {
			return nil, gvr, err
		}
	}
	if last == nil {
		last = notFoundf("%s/%s not found", ns, name)
	}
	return nil, schema.GroupVersionResource{}, last
}
