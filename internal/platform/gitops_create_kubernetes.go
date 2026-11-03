//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const gitOpsFieldManager = "zyvor-axiom"

// gitOpsCandidateGVRs are the API versions tried for each kind, newest first.
func gitOpsCandidateGVRs(kind string) []schema.GroupVersionResource {
	switch kind {
	case "Application":
		return argoAppGVRs
	case "GitRepository":
		return fluxGitRepositoryGVRs
	case "Kustomization":
		return fluxKustomizationGVRs
	case "HelmRepository":
		return fluxHelmRepositoryGVRs
	case "HelmRelease":
		return fluxHelmReleaseGVRs
	}
	return nil
}

// resolveGitOpsGVR asks discovery which candidate version the API server
// serves. A missing CRD is a 422 with an engine-specific hint.
func (k *KubernetesProvider) resolveGitOpsGVR(engine string, o gitOpsObject) (schema.GroupVersionResource, error) {
	if k.client == nil {
		return schema.GroupVersionResource{}, fmt.Errorf("kubernetes client unavailable")
	}
	disc := k.client.Discovery()
	for _, gvr := range gitOpsCandidateGVRs(o.Kind) {
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
	product := "Flux"
	if engine == GitOpsEngineArgoCD {
		product = "Argo CD"
	}
	return schema.GroupVersionResource{}, NewStatusError(http.StatusUnprocessableEntity,
		fmt.Errorf("%s is not installed: no %s.%s CRD", product, o.Resource, o.Group))
}

// CreateGitOpsApp creates the rendered objects in order (source first). Every
// version is resolved before anything is created, so a missing CRD leaves the
// cluster untouched; if a later object fails, the ones created by this call
// are deleted again (guarded by UID) and the error says so.
func (k *KubernetesProvider) CreateGitOpsApp(ctx context.Context, req GitOpsCreateRequest) (GitOpsCreateResult, error) {
	plan, err := buildGitOps(req)
	if err != nil {
		return GitOpsCreateResult{}, err
	}
	if k.dynamic == nil {
		return GitOpsCreateResult{}, fmt.Errorf("dynamic client unavailable")
	}
	gvrs := make([]schema.GroupVersionResource, len(plan.Objects))
	for i, o := range plan.Objects {
		if gvrs[i], err = k.resolveGitOpsGVR(plan.Engine, o); err != nil {
			return GitOpsCreateResult{}, err
		}
	}

	type createdObj struct {
		gvr schema.GroupVersionResource
		ref GitOpsObjectRef
		uid string
	}
	var created []createdObj
	for i, o := range plan.Objects {
		gvr := gvrs[i]
		obj := &unstructured.Unstructured{Object: o.withVersion(gvr.Version)}
		ref := o.ref()
		ref.APIVersion = gvr.GroupVersion().String()
		out, cerr := k.dynamic.Resource(gvr).Namespace(o.Namespace).Create(ctx, obj, metav1.CreateOptions{FieldManager: gitOpsFieldManager})
		if cerr != nil {
			what := fmt.Sprintf("create %s %s/%s", o.Kind, o.Namespace, o.Name)
			if apierrors.IsAlreadyExists(cerr) {
				what = fmt.Sprintf("%s %s/%s already exists", o.Kind, o.Namespace, o.Name)
			}
			var notes []string
			for j := len(created) - 1; j >= 0; j-- {
				c := created[j]
				opts := metav1.DeleteOptions{}
				if c.uid != "" {
					uid := types.UID(c.uid)
					opts.Preconditions = &metav1.Preconditions{UID: &uid}
				}
				derr := k.dynamic.Resource(c.gvr).Namespace(c.ref.Namespace).Delete(ctx, c.ref.Name, opts)
				if derr != nil && !apierrors.IsNotFound(derr) {
					notes = append(notes, fmt.Sprintf("rolling back %s %s/%s FAILED (%v): delete it manually", c.ref.Kind, c.ref.Namespace, c.ref.Name, derr))
				} else {
					notes = append(notes, fmt.Sprintf("rolled back %s %s/%s created by this request", c.ref.Kind, c.ref.Namespace, c.ref.Name))
				}
			}
			if len(notes) > 0 {
				what += "; " + strings.Join(notes, "; ")
			}
			return GitOpsCreateResult{}, fmt.Errorf("%s: %w", what, cerr)
		}
		created = append(created, createdObj{gvr: gvr, ref: ref, uid: string(out.GetUID())})
	}
	refs := make([]GitOpsObjectRef, 0, len(created))
	for _, c := range created {
		refs = append(refs, c.ref)
	}
	return GitOpsCreateResult{OK: true, Created: refs, Warnings: plan.Warnings}, nil
}
