//go:build kubernetes

package platform

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

var allGitOpsV1 = []schema.GroupVersionResource{
	gvrOf(argoAppGVRs, "v1alpha1"), gvrOf(fluxGitRepositoryGVRs, "v1"), gvrOf(fluxKustomizationGVRs, "v1"),
	gvrOf(fluxHelmRepositoryGVRs, "v1"), gvrOf(fluxHelmReleaseGVRs, "v2"),
}

func axiomLabelled(u *unstructured.Unstructured) *unstructured.Unstructured {
	u.SetLabels(map[string]string{GitOpsManagedByLabel: GitOpsManagedByValue})
	return u
}

func argoApp(name string, finalizers ...string) *unstructured.Unstructured {
	u := existing(gvrOf(argoAppGVRs, "v1alpha1"), "Application", "argocd", name)
	u.SetFinalizers(finalizers)
	return u
}

func fluxKs(ns, name, srcName string, prune bool) *unstructured.Unstructured {
	u := existing(gvrOf(fluxKustomizationGVRs, "v1"), "Kustomization", ns, name)
	_ = unstructured.SetNestedField(u.Object, prune, "spec", "prune")
	_ = unstructured.SetNestedMap(u.Object, map[string]any{"kind": "GitRepository", "name": srcName}, "spec", "sourceRef")
	return u
}

func fluxHR(ns, name, srcName, srcNS string, suspend bool) *unstructured.Unstructured {
	u := existing(gvrOf(fluxHelmReleaseGVRs, "v2"), "HelmRelease", ns, name)
	_ = unstructured.SetNestedField(u.Object, suspend, "spec", "suspend")
	_ = unstructured.SetNestedMap(u.Object, map[string]any{"chart": "c", "sourceRef": map[string]any{"kind": "HelmRepository", "name": srcName, "namespace": srcNS}}, "spec", "chart", "spec")
	return u
}

func deleteActions(dyn *dynamicfake.FakeDynamicClient) []string {
	var out []string
	for _, a := range dyn.Actions() {
		if d, ok := a.(clienttesting.DeleteAction); ok {
			out = append(out, d.GetResource().Resource+" "+d.GetNamespace()+"/"+d.GetName())
		}
	}
	return out
}

func TestKubePreviewGitOpsDeleteKinds(t *testing.T) {
	gitRepo := existing(gvrOf(fluxGitRepositoryGVRs, "v1"), "GitRepository", "flux-system", "repo")
	k, _, _ := gitOpsProvider(allGitOpsV1,
		argoApp("plain"),
		argoApp("cascading", argoResourcesFinalizer),
		argoApp("variant", "resources-finalizer.argocd.argoproj.io/background"),
		axiomLabelled(argoApp("ours")),
		fluxKs("flux-system", "pruned", "repo", true),
		fluxKs("flux-system", "kept", "repo", false),
		fluxHR("monitoring", "prom", "charts", "flux-system", false),
		fluxHR("monitoring", "paused", "charts", "flux-system", true),
		gitRepo,
	)
	ctx := context.Background()
	for _, c := range []struct {
		kind, ns, name string
		cascade        bool
		engine         string
		detail         string
	}{
		{"Application", "argocd", "plain", false, "argocd", ""},
		{"Application", "argocd", "cascading", true, "argocd", "finalizer: resources-finalizer.argocd.argoproj.io"},
		{"Application", "argocd", "variant", true, "argocd", "finalizer: resources-finalizer.argocd.argoproj.io/background"},
		{"Kustomization", "flux-system", "pruned", true, "flux", "spec.prune: true"},
		{"Kustomization", "flux-system", "kept", false, "flux", "spec.prune: false"},
		{"HelmRelease", "monitoring", "prom", true, "flux", "spec.suspend: false"},
		{"HelmRelease", "monitoring", "paused", false, "flux", "spec.suspend: true"},
	} {
		p, err := k.PreviewGitOpsDelete(ctx, c.kind, c.ns, c.name)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if p.Cascade != c.cascade || p.Engine != c.engine || p.Kind != c.kind || p.Name != c.name || p.Consequence == "" {
			t.Errorf("%s: %+v", c.name, p)
		}
		if c.detail != "" && (len(p.Detail) == 0 || p.Detail[0] != c.detail) {
			t.Errorf("%s detail: %v", c.name, p.Detail)
		}
		if (c.engine == "argocd") != (p.Source == nil) {
			t.Errorf("%s source: %+v", c.name, p.Source)
		}
	}
	p, _ := k.PreviewGitOpsDelete(ctx, "Kustomization", "flux-system", "pruned")
	if p.Source.Kind != "GitRepository" || p.Source.Namespace != "flux-system" || len(p.Source.SharedWith) != 1 || p.Source.SharedWith[0].Name != "kept" || p.Source.ManagedByAxiom {
		t.Errorf("shared git source: %+v", p.Source)
	}
	p, _ = k.PreviewGitOpsDelete(ctx, "HelmRelease", "monitoring", "prom")
	if p.Source.Kind != "HelmRepository" || p.Source.Namespace != "flux-system" || len(p.Source.SharedWith) != 1 || p.Source.SharedWith[0].Name != "paused" {
		t.Errorf("shared helm source: %+v", p.Source)
	}
	if p, _ := k.PreviewGitOpsDelete(ctx, "Application", "argocd", "ours"); !p.ManagedByAxiom {
		t.Errorf("managedByAxiom: %+v", p)
	}
	if _, err := k.PreviewGitOpsDelete(ctx, "Application", "argocd", "missing"); HTTPStatus(err) != http.StatusNotFound {
		t.Errorf("404: %v (%d)", err, HTTPStatus(err))
	}
	if _, err := k.PreviewGitOpsDelete(ctx, "GitRepository", "flux-system", "repo"); HTTPStatus(err) != http.StatusBadRequest {
		t.Errorf("bad kind: %v", err)
	}
}

func TestKubeGitOpsDeleteCRDAbsent(t *testing.T) {
	k, _, dyn := gitOpsProvider([]schema.GroupVersionResource{gvrOf(fluxKustomizationGVRs, "v1")})
	ctx := context.Background()
	_, err := k.PreviewGitOpsDelete(ctx, "Application", "argocd", "x")
	if HTTPStatus(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "Argo CD is not installed") {
		t.Fatalf("argo absent: %v (%d)", err, HTTPStatus(err))
	}
	_, err = k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "HelmRelease", Namespace: "flux-system", Name: "x"})
	if HTTPStatus(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "Flux is not installed") {
		t.Fatalf("helm absent: %v (%d)", err, HTTPStatus(err))
	}
	if len(deleteActions(dyn)) != 0 {
		t.Fatalf("deleted something: %v", deleteActions(dyn))
	}
}

func TestKubeGitOpsDeleteConfirmAndOrder(t *testing.T) {
	ctx := context.Background()
	k, _, dyn := gitOpsProvider(allGitOpsV1, argoApp("web", argoResourcesFinalizer), argoApp("plain"))
	for _, confirm := range []string{"", "WEB", "we", "web "} {
		_, err := k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Application", Namespace: "argocd", Name: "web", Confirm: confirm})
		if HTTPStatus(err) != http.StatusBadRequest || !strings.Contains(err.Error(), "pass confirm=web") {
			t.Fatalf("confirm %q: %v", confirm, err)
		}
	}
	if len(deleteActions(dyn)) != 0 {
		t.Fatalf("deleted without confirm: %v", deleteActions(dyn))
	}
	res, err := k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Application", Namespace: "argocd", Name: "web", Confirm: "web"})
	if err != nil || !res.OK || !res.Cascade || len(res.Deleted) != 1 || res.Deleted[0].Name != "web" {
		t.Fatalf("delete: %+v %v", res, err)
	}
	// Default propagation, UID precondition, nothing else.
	for _, a := range dyn.Actions() {
		if d, ok := a.(clienttesting.DeleteActionImpl); ok {
			if d.DeleteOptions.PropagationPolicy != nil {
				t.Errorf("propagation policy set: %v", *d.DeleteOptions.PropagationPolicy)
			}
		}
	}
	// Non-cascading: no confirm needed.
	res, err = k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Application", Namespace: "argocd", Name: "plain"})
	if err != nil || res.Cascade {
		t.Fatalf("plain: %+v %v", res, err)
	}
	if got := strings.Join(deleteActions(dyn), ","); got != "applications argocd/web,applications argocd/plain" {
		t.Fatalf("deletes: %s", got)
	}
	// Argo + removeSource is refused before anything is deleted.
	k2, _, dyn2 := gitOpsProvider(allGitOpsV1, argoApp("x"))
	if _, err := k2.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Application", Namespace: "argocd", Name: "x", RemoveSource: true}); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("argo removeSource: %v", err)
	}
	if len(deleteActions(dyn2)) != 0 {
		t.Fatal("deleted")
	}
	if _, err := k2.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Application", Namespace: "argocd", Name: "gone"}); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("404: %v", err)
	}
}

func TestKubeGitOpsDeleteCascadeRecomputed(t *testing.T) {
	// The app had no finalizer when previewed; one was added before the delete.
	ctx := context.Background()
	k, _, dyn := gitOpsProvider(allGitOpsV1, argoApp("web"))
	if p, _ := k.PreviewGitOpsDelete(ctx, "Application", "argocd", "web"); p.Cascade {
		t.Fatal("preview cascade")
	}
	gvr := gvrOf(argoAppGVRs, "v1alpha1")
	u, _ := dyn.Resource(gvr).Namespace("argocd").Get(ctx, "web", metav1.GetOptions{})
	u.SetFinalizers([]string{argoResourcesFinalizer})
	if _, err := dyn.Resource(gvr).Namespace("argocd").Update(ctx, u, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Application", Namespace: "argocd", Name: "web"}); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("stale preview honoured: %v", err)
	}
}

func TestKubeGitOpsDeleteRemoveSource(t *testing.T) {
	ctx := context.Background()
	ours := func() *unstructured.Unstructured {
		return axiomLabelled(existing(gvrOf(fluxGitRepositoryGVRs, "v1"), "GitRepository", "flux-system", "repo"))
	}
	theirs := func() *unstructured.Unstructured {
		return existing(gvrOf(fluxGitRepositoryGVRs, "v1"), "GitRepository", "flux-system", "repo")
	}

	t.Run("unshared and ours", func(t *testing.T) {
		k, _, dyn := gitOpsProvider(allGitOpsV1, axiomLabelled(fluxKs("flux-system", "app", "repo", true)), ours())
		res, err := k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Kustomization", Namespace: "flux-system", Name: "app", Confirm: "app", RemoveSource: true})
		if err != nil || len(res.Deleted) != 2 || len(res.Warnings) != 0 {
			t.Fatalf("%+v %v", res, err)
		}
		if got := strings.Join(deleteActions(dyn), ","); got != "kustomizations flux-system/app,gitrepositories flux-system/repo" {
			t.Fatalf("order: %s", got)
		}
	})
	t.Run("removeSource false keeps the source", func(t *testing.T) {
		k, _, dyn := gitOpsProvider(allGitOpsV1, fluxKs("flux-system", "app", "repo", false), ours())
		res, err := k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Kustomization", Namespace: "flux-system", Name: "app"})
		if err != nil || len(res.Deleted) != 1 || len(deleteActions(dyn)) != 1 {
			t.Fatalf("%+v %v %v", res, err, deleteActions(dyn))
		}
	})
	t.Run("shared", func(t *testing.T) {
		k, _, dyn := gitOpsProvider(allGitOpsV1, fluxKs("flux-system", "app", "repo", false), fluxKs("flux-system", "other", "repo", true),
			fluxHR("team-b", "chart-from-git", "x", "", false), ours())
		_, err := k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Kustomization", Namespace: "flux-system", Name: "app", RemoveSource: true})
		if HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "Kustomization flux-system/other") {
			t.Fatalf("%v", err)
		}
		if len(deleteActions(dyn)) != 0 {
			t.Fatalf("deleted: %v", deleteActions(dyn))
		}
	})
	t.Run("not created by Axiom", func(t *testing.T) {
		k, _, dyn := gitOpsProvider(allGitOpsV1, fluxKs("flux-system", "app", "repo", false), theirs())
		_, err := k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Kustomization", Namespace: "flux-system", Name: "app", RemoveSource: true})
		if HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "not created by Axiom") {
			t.Fatalf("%v", err)
		}
		if len(deleteActions(dyn)) != 0 {
			t.Fatalf("deleted: %v", deleteActions(dyn))
		}
	})
	t.Run("source delete fails", func(t *testing.T) {
		k, _, dyn := gitOpsProvider(allGitOpsV1, fluxKs("flux-system", "app", "repo", false), ours())
		dyn.PrependReactor("delete", "gitrepositories", func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("etcd unavailable")
		})
		res, err := k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Kustomization", Namespace: "flux-system", Name: "app", RemoveSource: true})
		if err != nil || !res.OK || len(res.Deleted) != 1 || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "etcd unavailable") {
			t.Fatalf("%+v %v", res, err)
		}
		if _, gerr := dyn.Resource(gvrOf(fluxKustomizationGVRs, "v1")).Namespace("flux-system").Get(ctx, "app", metav1.GetOptions{}); gerr == nil {
			t.Fatal("app not deleted")
		}
	})
	t.Run("source already gone", func(t *testing.T) {
		k, _, _ := gitOpsProvider(allGitOpsV1, fluxKs("flux-system", "app", "repo", false))
		res, err := k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Kustomization", Namespace: "flux-system", Name: "app", RemoveSource: true})
		if err != nil || len(res.Deleted) != 1 || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "not found") {
			t.Fatalf("%+v %v", res, err)
		}
	})
	t.Run("helm source, other namespace", func(t *testing.T) {
		repo := axiomLabelled(existing(gvrOf(fluxHelmRepositoryGVRs, "v1"), "HelmRepository", "flux-system", "charts"))
		k, _, dyn := gitOpsProvider(allGitOpsV1, fluxHR("monitoring", "prom", "charts", "flux-system", true), repo)
		res, err := k.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "HelmRelease", Namespace: "monitoring", Name: "prom", RemoveSource: true})
		if err != nil || res.Cascade || len(res.Deleted) != 2 || res.Deleted[1].Namespace != "flux-system" {
			t.Fatalf("%+v %v", res, err)
		}
		if got := strings.Join(deleteActions(dyn), ","); got != "helmreleases monitoring/prom,helmrepositories flux-system/charts" {
			t.Fatalf("order: %s", got)
		}
	})
}

func TestKubeCreateGitOpsSetsManagedByLabel(t *testing.T) {
	k, _, _ := gitOpsProvider([]schema.GroupVersionResource{gvrOf(fluxGitRepositoryGVRs, "v1"), gvrOf(fluxKustomizationGVRs, "v1")})
	req := fluxKsReq()
	if _, err := k.CreateGitOpsApp(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	p, err := k.PreviewGitOpsDelete(context.Background(), "Kustomization", "flux-system", req.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !p.ManagedByAxiom || p.Source == nil || !p.Source.ManagedByAxiom || len(p.Source.SharedWith) != 0 {
		t.Fatalf("%+v %+v", p, p.Source)
	}
}
