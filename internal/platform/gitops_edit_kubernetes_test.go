//go:build kubernetes

package platform

import (
	"context"
	"net/http"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

func gitOpsPatchActions(dyn *dynamicfake.FakeDynamicClient) []string {
	var out []string
	for _, a := range dyn.Actions() {
		if p, ok := a.(clienttesting.PatchAction); ok {
			if p.GetPatchType() != types.JSONPatchType {
				out = append(out, "WRONG-TYPE")
			}
			out = append(out, p.GetResource().Resource+" "+p.GetNamespace()+"/"+p.GetName())
		}
	}
	return out
}

func liveObj(t *testing.T, dyn *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource, ns, name string) map[string]any {
	t.Helper()
	u, err := dyn.Resource(gvr).Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return u.Object
}

func withExtras(u *unstructured.Unstructured, rv string) *unstructured.Unstructured {
	u.SetResourceVersion(rv)
	u.SetAnnotations(map[string]string{"owner": "team-a"})
	u.SetFinalizers([]string{"example.com/keep"})
	_ = unstructured.SetNestedField(u.Object, "kept", "spec", "unknownField")
	_ = unstructured.SetNestedField(u.Object, "Ready", "status", "phase")
	return u
}

func TestKubeGitOpsEditArgoRoundTrip(t *testing.T) {
	ctx := context.Background()
	app := withExtras(argoApp("shop"), "100")
	_ = unstructured.SetNestedMap(app.Object, map[string]any{"repoURL": "https://github.com/org/shop.git", "path": "deploy", "targetRevision": "main"}, "spec", "source")
	_ = unstructured.SetNestedMap(app.Object, map[string]any{"server": gitOpsArgoServer, "namespace": "shop"}, "spec", "destination")
	_ = unstructured.SetNestedField(app.Object, "default", "spec", "project")
	k, _, dyn := gitOpsProvider(allGitOpsV1, app)
	ref := GitOpsRef{Kind: "Application", Namespace: "argocd", Name: "shop"}

	ed, err := k.GetGitOpsEditable(ctx, ref)
	if err != nil || !ed.Editable || ed.ResourceVersion != "100" || ed.Source.Revision != "main" {
		t.Fatalf("get: %v %+v", err, ed)
	}
	req := editBody(t, ed, func(r *GitOpsEditRequest) { r.Source.Revision = "v2" })
	p, err := k.PreviewGitOpsEdit(ctx, ref, req)
	if err != nil || len(p.Changes) != 1 || p.Changes[0] != (GitOpsEditChange{Object: "Application/argocd/shop", Field: "spec.source.targetRevision", From: "main", To: "v2"}) {
		t.Fatalf("preview: %v %+v", err, p)
	}
	if len(gitOpsPatchActions(dyn)) != 0 {
		t.Fatal("preview patched")
	}
	res, err := k.UpdateGitOpsApp(ctx, ref, req)
	if err != nil || !res.OK || res.Noop {
		t.Fatalf("update: %v %+v", err, res)
	}
	if got := gitOpsPatchActions(dyn); len(got) != 1 || got[0] != "applications argocd/shop" {
		t.Fatalf("patches: %v", got)
	}
	o := liveObj(t, dyn, gvrOf(argoAppGVRs, "v1alpha1"), "argocd", "shop")
	if nestedString(o, "spec", "source", "targetRevision") != "v2" || nestedString(o, "spec", "unknownField") != "kept" ||
		nestedString(o, "status", "phase") != "Ready" || nestedString(o, "metadata", "annotations", "owner") != "team-a" ||
		len(nestedStrings(o, "metadata", "finalizers")) != 1 || nestedString(o, "spec", "source", "path") != "deploy" {
		t.Fatalf("live after update: %+v", o)
	}

	// Stale resourceVersion: 409, nothing patched.
	before := len(gitOpsPatchActions(dyn))
	_, err = k.UpdateGitOpsApp(ctx, ref, editBody(t, ed, func(r *GitOpsEditRequest) { r.ResourceVersion = "99"; r.Source.Revision = "v3" }))
	if HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "changed since you opened it") || len(gitOpsPatchActions(dyn)) != before {
		t.Fatalf("stale: %v", err)
	}
	// Non-editable (multiple sources): GET says so, PUT is a 400.
	multi := argoApp("multi")
	multi.SetResourceVersion("5")
	_ = unstructured.SetNestedSlice(multi.Object, []any{map[string]any{"repoURL": "https://a/b"}, map[string]any{"repoURL": "https://c/d"}}, "spec", "sources")
	k2, _, dyn2 := gitOpsProvider(allGitOpsV1, multi)
	mref := GitOpsRef{Kind: "Application", Namespace: "argocd", Name: "multi"}
	med, _ := k2.GetGitOpsEditable(ctx, mref)
	if med.Editable || len(med.Reasons) == 0 {
		t.Fatalf("multi: %+v", med)
	}
	if _, err := k2.UpdateGitOpsApp(ctx, mref, editBody(t, med, nil)); HTTPStatus(err) != http.StatusBadRequest || len(gitOpsPatchActions(dyn2)) != 0 {
		t.Fatalf("multi put: %v", err)
	}
	if _, err := k.GetGitOpsEditable(ctx, GitOpsRef{Kind: "Application", Namespace: "argocd", Name: "missing"}); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("404: %v", err)
	}
}

func TestKubeGitOpsEditFluxKustomization(t *testing.T) {
	ctx := context.Background()
	ks := withExtras(axiomLabelled(fluxKs("flux-system", "web", "web", true)), "10")
	_ = unstructured.SetNestedField(ks.Object, "./deploy", "spec", "path")
	_ = unstructured.SetNestedField(ks.Object, "5m", "spec", "interval")
	repo := withExtras(axiomLabelled(existing(gvrOf(fluxGitRepositoryGVRs, "v1"), "GitRepository", "flux-system", "web")), "20")
	_ = unstructured.SetNestedMap(repo.Object, map[string]any{"url": "https://github.com/org/web.git", "interval": "5m", "ref": map[string]any{"branch": "main"}, "unknownField": "kept"}, "spec")
	shared := fluxKs("flux-system", "other", "shared-src", false)
	sharedSrc := axiomLabelled(existing(gvrOf(fluxGitRepositoryGVRs, "v1"), "GitRepository", "flux-system", "shared-src"))
	_ = unstructured.SetNestedField(sharedSrc.Object, "https://github.com/org/shared.git", "spec", "url")
	shared2 := fluxKs("flux-system", "other2", "shared-src", false)
	k, _, dyn := gitOpsProvider(allGitOpsV1, ks, repo, shared, sharedSrc, shared2)
	ref := GitOpsRef{Kind: "Kustomization", Namespace: "flux-system", Name: "web"}

	ed, err := k.GetGitOpsEditable(ctx, ref)
	if err != nil || !ed.Editable || len(ed.ReadOnly) != 0 || ed.SourceResourceVersion != "20" || ed.Source.Path != "deploy" {
		t.Fatalf("get: %v %+v", err, ed)
	}
	res, err := k.UpdateGitOpsApp(ctx, ref, editBody(t, ed, func(r *GitOpsEditRequest) {
		r.Source.RepoURL = "https://github.com/org/web2.git"
		r.Source.Path = "deploy/prod"
	}))
	if err != nil || len(res.Changes) != 2 {
		t.Fatalf("update: %v %+v", err, res)
	}
	if got := gitOpsPatchActions(dyn); len(got) != 2 || got[0] != "gitrepositories flux-system/web" || got[1] != "kustomizations flux-system/web" {
		t.Fatalf("patch order: %v", got)
	}
	src := liveObj(t, dyn, gvrOf(fluxGitRepositoryGVRs, "v1"), "flux-system", "web")
	app := liveObj(t, dyn, gvrOf(fluxKustomizationGVRs, "v1"), "flux-system", "web")
	if nestedString(src, "spec", "url") != "https://github.com/org/web2.git" || nestedString(src, "spec", "unknownField") != "kept" ||
		nestedString(src, "metadata", "annotations", "owner") != "team-a" || nestedString(app, "spec", "path") != "./deploy/prod" ||
		nestedString(app, "spec", "unknownField") != "kept" || nestedString(app, "status", "phase") != "Ready" {
		t.Fatalf("after: %+v / %+v", src, app)
	}

	// Shared source: url is read-only, a change is refused before any patch.
	oref := GitOpsRef{Kind: "Kustomization", Namespace: "flux-system", Name: "other"}
	oed, err := k.GetGitOpsEditable(ctx, oref)
	if err != nil || len(oed.ReadOnly) != 3 || !strings.Contains(strings.Join(oed.Reasons, " "), "shared with Kustomization flux-system/other2") {
		t.Fatalf("shared get: %v %+v", err, oed)
	}
	before := len(gitOpsPatchActions(dyn))
	if _, err := k.UpdateGitOpsApp(ctx, oref, editBody(t, oed, func(r *GitOpsEditRequest) { r.Source.RepoURL = "https://github.com/org/x.git" })); HTTPStatus(err) != http.StatusBadRequest || len(gitOpsPatchActions(dyn)) != before {
		t.Fatalf("shared put: %v", err)
	}
}

func TestKubeGitOpsEditFluxHelmRelease(t *testing.T) {
	ctx := context.Background()
	hr := withExtras(fluxHR("apps", "redis", "bitnami", "flux-system", false), "3")
	_ = unstructured.SetNestedField(hr.Object, "10m", "spec", "interval")
	_ = unstructured.SetNestedField(hr.Object, "redis", "spec", "chart", "spec", "chart")
	_ = unstructured.SetNestedMap(hr.Object, map[string]any{"replicas": int64(2), "old": "drop-me"}, "spec", "values")
	repo := existing(gvrOf(fluxHelmRepositoryGVRs, "v1"), "HelmRepository", "flux-system", "bitnami") // not Axiom's
	repo.SetResourceVersion("4")
	_ = unstructured.SetNestedField(repo.Object, "https://charts.bitnami.com/bitnami", "spec", "url")
	k, _, dyn := gitOpsProvider(allGitOpsV1, hr, repo)
	ref := GitOpsRef{Kind: "HelmRelease", Namespace: "apps", Name: "redis"}
	ed, err := k.GetGitOpsEditable(ctx, ref)
	if err != nil || !ed.Editable || len(ed.ReadOnly) != 2 || ed.Source.Chart != "redis" || !strings.Contains(ed.Values, "old: drop-me") {
		t.Fatalf("get: %v %+v", err, ed)
	}
	_, err = k.UpdateGitOpsApp(ctx, ref, editBody(t, ed, func(r *GitOpsEditRequest) { r.Values = "replicas: 3\n"; r.Source.ChartVersion = "18.1.0" }))
	if err != nil {
		t.Fatal(err)
	}
	if got := gitOpsPatchActions(dyn); len(got) != 1 || got[0] != "helmreleases apps/redis" {
		t.Fatalf("patches: %v", got)
	}
	o := liveObj(t, dyn, gvrOf(fluxHelmReleaseGVRs, "v2"), "apps", "redis")
	v, _ := nestedMap(o, "spec", "values")
	if len(v) != 1 || nestedString(o, "spec", "chart", "spec", "version") != "18.1.0" || nestedString(o, "spec", "unknownField") != "kept" ||
		len(nestedStrings(o, "metadata", "finalizers")) != 1 {
		t.Fatalf("after: %+v", o)
	}
}
