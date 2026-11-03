//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// gitOpsDeleteCRD returns the group/resource of a deletable or source kind,
// shaped for resolveGitOpsGVR (which only reads Kind, Group and Resource).
func gitOpsDeleteCRD(kind string) gitOpsObject {
	l := gitOpsCandidateGVRs(kind)
	if len(l) == 0 {
		return gitOpsObject{Kind: kind}
	}
	return gitOpsObject{Kind: kind, Group: l[0].Group, Resource: l[0].Resource}
}

func gitOpsEngineForCRD(kind string) string {
	if kind == "Application" {
		return GitOpsEngineArgoCD
	}
	return GitOpsEngineFluxKustomize // any Flux engine: only the "Flux" hint matters
}

// gitOpsDeletePlan is the preview plus what the delete needs to act on it.
type gitOpsDeletePlan struct {
	Preview      GitOpsDeletePreview
	GVR          schema.GroupVersionResource
	UID          types.UID
	SourceGVR    schema.GroupVersionResource
	SourceUID    types.UID
	SourceExists bool
}

func (k *KubernetesProvider) planGitOpsDelete(ctx context.Context, kind, ns, name string) (gitOpsDeletePlan, error) {
	var plan gitOpsDeletePlan
	if err := checkGitOpsDeleteTarget(kind, ns, name); err != nil {
		return plan, err
	}
	if k.dynamic == nil {
		return plan, fmt.Errorf("dynamic client unavailable")
	}
	gvr, err := k.resolveGitOpsGVR(gitOpsEngineForCRD(kind), gitOpsDeleteCRD(kind))
	if err != nil {
		return plan, err
	}
	obj, err := k.dynamic.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return plan, notFoundf("%s %s/%s not found", kind, ns, name)
		}
		return plan, err
	}
	plan.GVR, plan.UID = gvr, obj.GetUID()

	var peers []gitOpsPeer
	var sourceObj map[string]any
	if src, ok := gitOpsSourceRef(kind, obj.Object); ok {
		// Every Flux object that could share the source. A listing error is
		// returned rather than skipped: "unshared" must never be a guess.
		for _, pk := range []string{"Kustomization", "HelmRelease"} {
			pgvr, perr := k.resolveGitOpsGVR(GitOpsEngineFluxKustomize, gitOpsDeleteCRD(pk))
			if perr != nil {
				if HTTPStatus(perr) == http.StatusUnprocessableEntity {
					continue // CRD absent: nothing of that kind can share it
				}
				return plan, perr
			}
			list, lerr := k.dynamic.Resource(pgvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
			if lerr != nil {
				return plan, fmt.Errorf("list %s to check whether the source is shared: %w", pk, lerr)
			}
			for i := range list.Items {
				peers = append(peers, gitOpsPeer{Kind: pk, Obj: list.Items[i].Object})
			}
		}
		if gitOpsCandidateGVRs(src.Kind) != nil {
			sgvr, serr := k.resolveGitOpsGVR(GitOpsEngineFluxKustomize, gitOpsDeleteCRD(src.Kind))
			if serr == nil {
				s, gerr := k.dynamic.Resource(sgvr).Namespace(src.Namespace).Get(ctx, src.Name, metav1.GetOptions{})
				switch {
				case gerr == nil:
					sourceObj = s.Object
					plan.SourceGVR, plan.SourceUID, plan.SourceExists = sgvr, s.GetUID(), true
				case !apierrors.IsNotFound(gerr):
					return plan, fmt.Errorf("get source %s %s/%s: %w", src.Kind, src.Namespace, src.Name, gerr)
				}
			} else if HTTPStatus(serr) != http.StatusUnprocessableEntity {
				return plan, serr
			}
		}
	}
	plan.Preview = buildGitOpsDeletePreview(kind, obj.Object, peers, sourceObj)
	return plan, nil
}

// PreviewGitOpsDelete reports what deleting the object would do.
func (k *KubernetesProvider) PreviewGitOpsDelete(ctx context.Context, kind, ns, name string) (GitOpsDeletePreview, error) {
	plan, err := k.planGitOpsDelete(ctx, kind, ns, name)
	return plan.Preview, err
}

// DeleteGitOpsApp recomputes the preview, enforces the typed confirmation and
// the removeSource rules, then deletes the object (default propagation: what
// happens to the managed resources is the controller's finalizer semantics)
// and, if asked and allowed, its Flux source.
func (k *KubernetesProvider) DeleteGitOpsApp(ctx context.Context, req GitOpsDeleteRequest) (GitOpsDeleteResult, error) {
	plan, err := k.planGitOpsDelete(ctx, req.Kind, req.Namespace, req.Name)
	if err != nil {
		return GitOpsDeleteResult{}, err
	}
	p := plan.Preview
	if err := checkGitOpsDeleteConfirm(p, req.Confirm); err != nil {
		return GitOpsDeleteResult{}, err
	}
	if req.RemoveSource {
		if err := checkGitOpsRemoveSource(p, plan.SourceExists); err != nil {
			return GitOpsDeleteResult{}, err
		}
	}

	opts := metav1.DeleteOptions{}
	if plan.UID != "" {
		uid := plan.UID
		opts.Preconditions = &metav1.Preconditions{UID: &uid}
	}
	if err := k.dynamic.Resource(plan.GVR).Namespace(p.Namespace).Delete(ctx, p.Name, opts); err != nil {
		if apierrors.IsNotFound(err) {
			return GitOpsDeleteResult{}, notFoundf("%s %s/%s not found", p.Kind, p.Namespace, p.Name)
		}
		return GitOpsDeleteResult{}, fmt.Errorf("delete %s %s/%s: %w", p.Kind, p.Namespace, p.Name, err)
	}
	res := GitOpsDeleteResult{OK: true, Cascade: p.Cascade, Deleted: []GitOpsRef{{Kind: p.Kind, Namespace: p.Namespace, Name: p.Name}}}
	if req.RemoveSource && p.Source != nil {
		src := p.Source.ref()
		if !plan.SourceExists {
			res.Warnings = append(res.Warnings, fmt.Sprintf("source %s was not found; nothing else to remove", src))
			return res, nil
		}
		sopts := metav1.DeleteOptions{}
		if plan.SourceUID != "" {
			uid := plan.SourceUID
			sopts.Preconditions = &metav1.Preconditions{UID: &uid}
		}
		if derr := k.dynamic.Resource(plan.SourceGVR).Namespace(src.Namespace).Delete(ctx, src.Name, sopts); derr != nil && !apierrors.IsNotFound(derr) {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s %s/%s was deleted, but removing source %s failed (%v): delete it manually", p.Kind, p.Namespace, p.Name, src, derr))
		} else {
			res.Deleted = append(res.Deleted, src)
		}
	}
	return res, nil
}
