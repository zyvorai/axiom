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

// GitOps editing against the cluster. The pure logic is in gitops_edit.go;
// this file only loads the live objects and sends the JSON Patches.

type gitOpsEditLive struct {
	target    gitOpsEditTarget
	gvr       schema.GroupVersionResource
	sourceGVR schema.GroupVersionResource
}

func (k *KubernetesProvider) loadGitOpsEdit(ctx context.Context, ref GitOpsRef) (gitOpsEditLive, error) {
	var live gitOpsEditLive
	if err := checkGitOpsDeleteTarget(ref.Kind, ref.Namespace, ref.Name); err != nil {
		return live, err
	}
	if k.dynamic == nil {
		return live, fmt.Errorf("dynamic client unavailable")
	}
	gvr, err := k.resolveGitOpsGVR(gitOpsEngineForCRD(ref.Kind), gitOpsDeleteCRD(ref.Kind))
	if err != nil {
		return live, err
	}
	obj, err := k.dynamic.Resource(gvr).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return live, notFoundf("%s %s/%s not found", ref.Kind, ref.Namespace, ref.Name)
		}
		return live, err
	}
	live.gvr = gvr
	live.target = gitOpsEditTarget{Kind: ref.Kind, Obj: obj.Object}
	src, ok := gitOpsSourceRef(ref.Kind, obj.Object)
	if !ok || gitOpsCandidateGVRs(src.Kind) == nil {
		return live, nil
	}
	// Peers decide whether the source is shared; a listing error is returned,
	// never guessed (the source fields would wrongly become editable).
	for _, pk := range []string{"Kustomization", "HelmRelease"} {
		pgvr, perr := k.resolveGitOpsGVR(GitOpsEngineFluxKustomize, gitOpsDeleteCRD(pk))
		if perr != nil {
			if HTTPStatus(perr) == http.StatusUnprocessableEntity {
				continue
			}
			return live, perr
		}
		list, lerr := k.dynamic.Resource(pgvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if lerr != nil {
			return live, fmt.Errorf("list %s to check whether the source is shared: %w", pk, lerr)
		}
		for i := range list.Items {
			live.target.Peers = append(live.target.Peers, gitOpsPeer{Kind: pk, Obj: list.Items[i].Object})
		}
	}
	sgvr, serr := k.resolveGitOpsGVR(GitOpsEngineFluxKustomize, gitOpsDeleteCRD(src.Kind))
	if serr != nil {
		if HTTPStatus(serr) == http.StatusUnprocessableEntity {
			return live, nil
		}
		return live, serr
	}
	s, gerr := k.dynamic.Resource(sgvr).Namespace(src.Namespace).Get(ctx, src.Name, metav1.GetOptions{})
	switch {
	case gerr == nil:
		live.target.SourceObj = s.Object
		live.sourceGVR = sgvr
	case !apierrors.IsNotFound(gerr):
		return live, fmt.Errorf("get source %s: %w", src, gerr)
	}
	return live, nil
}

// GetGitOpsEditable returns the object's current typed fields.
func (k *KubernetesProvider) GetGitOpsEditable(ctx context.Context, ref GitOpsRef) (GitOpsEditable, error) {
	live, err := k.loadGitOpsEdit(ctx, ref)
	if err != nil {
		return GitOpsEditable{}, err
	}
	return readGitOpsEditable(live.target), nil
}

// PreviewGitOpsEdit computes the managed-field diff without changing anything.
func (k *KubernetesProvider) PreviewGitOpsEdit(ctx context.Context, ref GitOpsRef, req GitOpsEditRequest) (GitOpsEditPreview, error) {
	live, err := k.loadGitOpsEdit(ctx, ref)
	if err != nil {
		return GitOpsEditPreview{}, err
	}
	plan, err := planGitOpsEdit(live.target, req)
	return plan.Preview, err
}

// UpdateGitOpsApp re-reads the object, refuses a stale resourceVersion, and
// applies only the managed fields as JSON Patches whose first op tests the
// resourceVersion (so a concurrent change between read and patch also fails).
// The Flux source is patched first; if the application patch then fails the
// error says the source was already updated.
func (k *KubernetesProvider) UpdateGitOpsApp(ctx context.Context, ref GitOpsRef, req GitOpsEditRequest) (GitOpsEditResult, error) {
	live, err := k.loadGitOpsEdit(ctx, ref)
	if err != nil {
		return GitOpsEditResult{}, err
	}
	plan, err := planGitOpsEdit(live.target, req)
	if err != nil {
		return GitOpsEditResult{}, err
	}
	res := GitOpsEditResult{OK: true, Noop: plan.Preview.Noop, Changes: plan.Preview.Changes, Warnings: plan.Preview.Warnings,
		ResourceVersion: plan.Editable.ResourceVersion}
	opts := metav1.PatchOptions{FieldManager: gitOpsFieldManager}
	if plan.SourcePatch != nil {
		s := plan.SourceRef
		if _, perr := k.dynamic.Resource(live.sourceGVR).Namespace(s.Namespace).Patch(ctx, s.Name, types.JSONPatchType, plan.SourcePatch, opts); perr != nil {
			return GitOpsEditResult{}, k.gitOpsPatchError(perr, fmt.Sprintf("update source %s", s))
		}
	}
	if plan.AppPatch != nil {
		out, perr := k.dynamic.Resource(live.gvr).Namespace(ref.Namespace).Patch(ctx, ref.Name, types.JSONPatchType, plan.AppPatch, opts)
		if perr != nil {
			what := fmt.Sprintf("update %s", ref)
			if plan.SourcePatch != nil {
				what += fmt.Sprintf(" (its source %s WAS already updated)", plan.SourceRef)
			}
			return GitOpsEditResult{}, k.gitOpsPatchError(perr, what)
		}
		res.ResourceVersion = out.GetResourceVersion()
	}
	return res, nil
}

// gitOpsPatchError turns a failed resourceVersion test / conflict into the 409.
func (k *KubernetesProvider) gitOpsPatchError(err error, what string) error {
	// A failed JSON Patch "test" op surfaces as 422/400/500 depending on the
	// server version; its message always names the failed test.
	testFailed := strings.Contains(err.Error(), "test") && strings.Contains(err.Error(), "/metadata/resourceVersion") ||
		strings.Contains(err.Error(), "test failed")
	if apierrors.IsConflict(err) || testFailed {
		return NewStatusError(http.StatusConflict, fmt.Errorf("%s: the application changed since you opened it: reload it and apply your edit again (%v)", what, err))
	}
	if apierrors.IsNotFound(err) {
		return notFoundf("%s: not found", what)
	}
	return fmt.Errorf("%s: %w", what, err)
}
