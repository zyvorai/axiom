//go:build kubernetes

package platform

import (
	"context"
	"net/http"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

var gitOpsListKinds = map[schema.GroupVersionResource]string{}

func init() {
	for _, l := range [][]schema.GroupVersionResource{argoAppGVRs, fluxGitRepositoryGVRs, fluxKustomizationGVRs, fluxHelmRepositoryGVRs, fluxHelmReleaseGVRs} {
		for _, g := range l {
			kind := map[string]string{
				"applications": "ApplicationList", "gitrepositories": "GitRepositoryList", "kustomizations": "KustomizationList",
				"helmrepositories": "HelmRepositoryList", "helmreleases": "HelmReleaseList",
			}[g.Resource]
			gitOpsListKinds[g] = kind
		}
	}
}

// gitOpsProvider builds a provider whose discovery serves exactly the given
// GVRs (the fake dynamic client itself accepts any resource).
func gitOpsProvider(served []schema.GroupVersionResource, dynObjects ...runtime.Object) (*KubernetesProvider, *k8sfake.Clientset, *dynamicfake.FakeDynamicClient) {
	k, cs, dyn := newFakeFullProvider(gitOpsListKinds, dynObjects)
	byGV := map[string]*metav1.APIResourceList{}
	for _, g := range served {
		gv := g.GroupVersion().String()
		if byGV[gv] == nil {
			byGV[gv] = &metav1.APIResourceList{GroupVersion: gv}
			cs.Resources = append(cs.Resources, byGV[gv])
		}
		byGV[gv].APIResources = append(byGV[gv].APIResources, metav1.APIResource{Name: g.Resource, Namespaced: true})
	}
	return k, cs, dyn
}

func gvrOf(l []schema.GroupVersionResource, version string) schema.GroupVersionResource {
	for _, g := range l {
		if g.Version == version {
			return g
		}
	}
	panic("no " + version)
}

func existing(gvr schema.GroupVersionResource, kind, ns, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(gvr.GroupVersion().String())
	u.SetKind(kind)
	u.SetNamespace(ns)
	u.SetName(name)
	return u
}

func createActions(dyn *dynamicfake.FakeDynamicClient) []string {
	var out []string
	for _, a := range dyn.Actions() {
		if a.GetVerb() == "create" || a.GetVerb() == "delete" {
			out = append(out, a.GetVerb()+" "+a.GetResource().Resource+"/"+a.GetResource().Version)
		}
	}
	return out
}

func TestCreateGitOpsAppFluxKustomizeOrder(t *testing.T) {
	k, _, dyn := gitOpsProvider([]schema.GroupVersionResource{
		gvrOf(fluxGitRepositoryGVRs, "v1"), gvrOf(fluxKustomizationGVRs, "v1"),
	})
	res, err := k.CreateGitOpsApp(context.Background(), fluxKsReq())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(createActions(dyn), ","); got != "create gitrepositories/v1,create kustomizations/v1" {
		t.Fatalf("order: %s", got)
	}
	if len(res.Created) != 2 || res.Created[0].APIVersion != "source.toolkit.fluxcd.io/v1" || res.Created[1].APIVersion != "kustomize.toolkit.fluxcd.io/v1" {
		t.Fatalf("result: %+v", res)
	}
	for _, a := range dyn.Actions() {
		if c, ok := a.(clienttesting.CreateAction); ok {
			u := c.GetObject().(*unstructured.Unstructured)
			if u.GetNamespace() != "flux-system" || u.GetName() != "apps" {
				t.Errorf("object: %s/%s", u.GetNamespace(), u.GetName())
			}
			if len(u.GetFinalizers()) != 0 || len(u.GetAnnotations()) != 0 {
				t.Errorf("unexpected metadata: %+v", u.Object["metadata"])
			}
		}
	}
	ks, err := dyn.Resource(gvrOf(fluxKustomizationGVRs, "v1")).Namespace("flux-system").Get(context.Background(), "apps", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p, _, _ := unstructured.NestedString(ks.Object, "spec", "path"); p != "./clusters/prod" {
		t.Errorf("path: %q", p)
	}
}

func TestCreateGitOpsAppVersionFallback(t *testing.T) {
	// Only the older Flux APIs are installed.
	k, _, dyn := gitOpsProvider([]schema.GroupVersionResource{
		gvrOf(fluxHelmRepositoryGVRs, "v1beta2"), gvrOf(fluxHelmReleaseGVRs, "v2beta1"),
		gvrOf(fluxGitRepositoryGVRs, "v1beta2"), gvrOf(fluxKustomizationGVRs, "v1beta2"),
	})
	res, err := k.CreateGitOpsApp(context.Background(), fluxHelmReq())
	if err != nil {
		t.Fatal(err)
	}
	if res.Created[0].APIVersion != "source.toolkit.fluxcd.io/v1beta2" || res.Created[1].APIVersion != "helm.toolkit.fluxcd.io/v2beta1" {
		t.Fatalf("versions: %+v", res.Created)
	}
	hr, err := dyn.Resource(gvrOf(fluxHelmReleaseGVRs, "v2beta1")).Namespace("flux-system").Get(context.Background(), "podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if hr.GetAPIVersion() != "helm.toolkit.fluxcd.io/v2beta1" {
		t.Errorf("apiVersion on the object: %s", hr.GetAPIVersion())
	}
	if _, err := k.CreateGitOpsApp(context.Background(), fluxKsReq()); err != nil {
		t.Fatal(err)
	}
	if got := createActions(dyn); got[len(got)-2] != "create gitrepositories/v1beta2" || got[len(got)-1] != "create kustomizations/v1beta2" {
		t.Fatalf("kustomize versions: %v", got)
	}
}

func TestCreateGitOpsAppRollback(t *testing.T) {
	ks := gvrOf(fluxKustomizationGVRs, "v1")
	gr := gvrOf(fluxGitRepositoryGVRs, "v1")
	k, _, dyn := gitOpsProvider([]schema.GroupVersionResource{gr, ks},
		existing(ks, "Kustomization", "flux-system", "apps"))
	_, err := k.CreateGitOpsApp(context.Background(), fluxKsReq())
	if err == nil {
		t.Fatal("expected a conflict")
	}
	if HTTPStatus(err) != http.StatusConflict {
		t.Errorf("status %d: %v", HTTPStatus(err), err)
	}
	for _, want := range []string{"Kustomization flux-system/apps already exists", "rolled back GitRepository flux-system/apps"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if _, gerr := dyn.Resource(gr).Namespace("flux-system").Get(context.Background(), "apps", metav1.GetOptions{}); gerr == nil {
		t.Fatal("the GitRepository created before the conflict was left behind")
	}
	if got := strings.Join(createActions(dyn), ","); got != "create gitrepositories/v1,create kustomizations/v1,delete gitrepositories/v1" {
		t.Errorf("actions: %s", got)
	}
}

func TestCreateGitOpsAppExisting(t *testing.T) {
	app := argoAppGVRs[0]
	k, _, dyn := gitOpsProvider([]schema.GroupVersionResource{app}, existing(app, "Application", "argocd", "web"))
	_, err := k.CreateGitOpsApp(context.Background(), argoReq())
	if HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "Application argocd/web already exists") {
		t.Fatalf("got %v (%d)", err, HTTPStatus(err))
	}
	if strings.Contains(err.Error(), "rolled back") {
		t.Errorf("nothing was created, nothing to roll back: %v", err)
	}
	for _, a := range createActions(dyn) {
		if strings.HasPrefix(a, "delete") {
			t.Errorf("unexpected delete: %s", a)
		}
	}
}

func TestCreateGitOpsAppCRDMissing(t *testing.T) {
	// No CRDs at all.
	k, _, dyn := gitOpsProvider(nil)
	_, err := k.CreateGitOpsApp(context.Background(), argoReq())
	if HTTPStatus(err) != http.StatusUnprocessableEntity || err.Error() != "Argo CD is not installed: no applications.argoproj.io CRD" {
		t.Fatalf("argo: %v (%d)", err, HTTPStatus(err))
	}
	// Source controller present, kustomize-controller absent: nothing is created.
	k, _, dyn = gitOpsProvider([]schema.GroupVersionResource{gvrOf(fluxGitRepositoryGVRs, "v1")})
	_, err = k.CreateGitOpsApp(context.Background(), fluxKsReq())
	if HTTPStatus(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "Flux is not installed: no kustomizations.kustomize.toolkit.fluxcd.io CRD") {
		t.Fatalf("flux: %v (%d)", err, HTTPStatus(err))
	}
	if got := createActions(dyn); len(got) != 0 {
		t.Errorf("created despite a missing CRD: %v", got)
	}
}

func TestCreateGitOpsAppInvalid(t *testing.T) {
	k, _, dyn := gitOpsProvider([]schema.GroupVersionResource{argoAppGVRs[0]})
	r := argoReq()
	r.Source.RepoURL = "https://u:secret@github.com/o/r.git"
	_, err := k.CreateGitOpsApp(context.Background(), r)
	if HTTPStatus(err) != http.StatusBadRequest || strings.Contains(err.Error(), "secret@") {
		t.Fatalf("got %v", err)
	}
	if len(createActions(dyn)) != 0 {
		t.Error("created an invalid object")
	}
}
