package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func gitOpsListed(t *testing.T, e *idEnv, kind, name string) bool {
	t.Helper()
	var items []platform.GitOpsItem
	decodeInto(t, e.do("GET", "/api/v1/gitops", "", asAdmin()), &items)
	for _, it := range items {
		if it.Kind == kind && it.Name == name {
			return true
		}
	}
	return false
}

func TestGitOpsDeletePreviewEndpoint(t *testing.T) {
	e := newGitOpsEnv(t)
	w := e.do("GET", "/api/v1/gitops/Application/argocd/platform-apps/delete-preview", "", asAdmin())
	if w.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var p platform.GitOpsDeletePreview
	decodeInto(t, w, &p)
	if !p.Cascade || p.Engine != "argocd" || p.Kind != "Application" || p.Name != "platform-apps" || p.ManagedByAxiom ||
		!strings.Contains(p.Consequence, "resources-finalizer.argocd.argoproj.io") || len(p.Detail) != 1 {
		t.Fatalf("preview: %+v", p)
	}
	if !strings.Contains(w.Body.String(), `"source":null`) {
		t.Errorf("argo source must be null: %s", w.Body.String())
	}
	w = e.do("GET", "/api/v1/gitops/Application/argocd/docs-site/delete-preview", "", asAdmin())
	decodeInto(t, w, &p)
	if p.Cascade || !strings.Contains(p.Consequence, "keep running") || !strings.Contains(w.Body.String(), `"detail":[]`) {
		t.Fatalf("non-cascading preview: %s", w.Body.String())
	}
	w = e.do("GET", "/api/v1/gitops/Kustomization/flux-system/apps/delete-preview", "", asAdmin())
	if !strings.Contains(w.Body.String(), `"sharedWith":[{"kind":"Kustomization","namespace":"flux-system","name":"infra"}]`) {
		t.Fatalf("sharedWith: %s", w.Body.String())
	}
	if w := e.do("GET", "/api/v1/gitops/Application/argocd/nope/delete-preview", "", asAdmin()); w.Code != http.StatusNotFound {
		t.Errorf("unknown: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/gitops/Pod/argocd/x/delete-preview", "", asAdmin()); w.Code != http.StatusBadRequest {
		t.Errorf("bad kind: %d %s", w.Code, w.Body.String())
	}
}

func TestGitOpsDeleteEndpoint(t *testing.T) {
	e := newGitOpsEnv(t)
	// gitOpsBody has prune:true, so deleting the Kustomization cascades.
	if w := e.do("POST", "/api/v1/gitops", gitOpsBody("doomed-app"), asAdmin()); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	w := e.do("GET", "/api/v1/gitops/Kustomization/flux-system/doomed-app/delete-preview", "", asAdmin())
	var p platform.GitOpsDeletePreview
	decodeInto(t, w, &p)
	if !p.Cascade || !p.ManagedByAxiom || p.Source == nil || !p.Source.ManagedByAxiom || p.Source.Kind != "GitRepository" || len(p.Source.SharedWith) != 0 {
		t.Fatalf("created app preview: %s", w.Body.String())
	}

	base := "/api/v1/gitops/Kustomization/flux-system/doomed-app"
	for _, q := range []string{"", "?confirm=", "?confirm=doomed", "?confirm=Doomed-App", "?confirm=DOOMED-APP"} {
		w := e.do("DELETE", base+q, "", asAdmin())
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "this deletion also removes the application's resources; pass confirm=doomed-app") {
			t.Fatalf("confirm %q: %d %s", q, w.Code, w.Body.String())
		}
	}
	if !gitOpsListed(t, e, "Kustomization", "doomed-app") {
		t.Fatal("deleted without confirmation")
	}
	if w := e.do("DELETE", base+"?confirm=doomed-app&removeSource=maybe", "", asAdmin()); w.Code != http.StatusBadRequest {
		t.Fatalf("bad removeSource: %d", w.Code)
	}
	w = e.do("DELETE", base+"?confirm=doomed-app&removeSource=true", "", asAdmin())
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	var res platform.GitOpsDeleteResult
	decodeInto(t, w, &res)
	if !res.OK || !res.Cascade || len(res.Deleted) != 2 || res.Deleted[0].Kind != "Kustomization" || res.Deleted[1].Kind != "GitRepository" {
		t.Fatalf("result: %s", w.Body.String())
	}
	if gitOpsListed(t, e, "Kustomization", "doomed-app") {
		t.Fatal("still listed")
	}
	audited := false
	for _, a := range platform.ListAxiomAudit() {
		if a.Action == "gitops.delete" && a.Target == "Kustomization/flux-system/doomed-app" && a.Detail == "cascade=true" && a.User == idAdminUser {
			audited = true
		}
	}
	if !audited {
		t.Errorf("no audit entry: %+v", platform.ListAxiomAudit())
	}
	if w := e.do("DELETE", base+"?confirm=doomed-app", "", asAdmin()); w.Code != http.StatusNotFound {
		t.Fatalf("second delete: %d %s", w.Code, w.Body.String())
	}

	// Seed source shared with another Kustomization and not created by Axiom: 409, nothing deleted.
	w = e.do("DELETE", "/api/v1/gitops/Kustomization/flux-system/infra?removeSource=true", "", asAdmin())
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "Kustomization flux-system/apps") {
		t.Fatalf("shared source: %d %s", w.Code, w.Body.String())
	}
	if !gitOpsListed(t, e, "Kustomization", "infra") {
		t.Fatal("deleted despite 409")
	}

	// Non-cascading (Axiom-created Argo apps carry no finalizer): no confirm needed.
	argo := `{"engine":"argocd","name":"quiet-app","source":{"repoUrl":"https://github.com/org/repo.git","path":"deploy","revision":"v1"},"destination":{"namespace":"prod"}}`
	if w := e.do("POST", "/api/v1/gitops", argo, asAdmin()); w.Code != http.StatusCreated {
		t.Fatalf("create argo: %d %s", w.Code, w.Body.String())
	}
	w = e.do("DELETE", "/api/v1/gitops/Application/argocd/quiet-app", "", asAdmin())
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"cascade":false`) {
		t.Fatalf("argo delete: %d %s", w.Code, w.Body.String())
	}
}

func TestGitOpsDeleteAccess(t *testing.T) {
	e := newGitOpsEnv(t)
	if w := e.do("POST", "/api/v1/gitops", gitOpsBody("guarded-app"), asAdmin()); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	del := "/api/v1/gitops/Kustomization/flux-system/guarded-app?confirm=guarded-app"
	preview := "/api/v1/gitops/Kustomization/flux-system/guarded-app/delete-preview"
	creds := map[string][2]string{
		"operator": {"oper1", "oper-password"}, "viewer": {"viewer1", "viewer-password"},
		"scoped": {"scoped", "scoped-password"}, "scopedop": {"scopedop", "scopedop-password"},
	}
	for who, c := range creds {
		if w := e.do("DELETE", del, "", withBasic(c[0], c[1])); w.Code != http.StatusForbidden {
			t.Errorf("%s delete: %d", who, w.Code)
		}
		want := http.StatusOK
		if who == "scoped" || who == "scopedop" {
			want = http.StatusForbidden
		}
		if w := e.do("GET", preview, "", withBasic(c[0], c[1])); w.Code != want {
			t.Errorf("%s preview: %d, want %d", who, w.Code, want)
		}
	}
	if w := e.do("DELETE", del, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
	if !gitOpsListed(t, e, "Kustomization", "guarded-app") {
		t.Fatal("deleted by a non-admin")
	}
	// Operators keep sync.
	if w := e.do("POST", "/api/v1/gitops/Kustomization/flux-system/guarded-app/sync", "", withBasic("oper1", "oper-password")); w.Code != http.StatusOK {
		t.Errorf("operator sync: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("DELETE", del+"&removeSource=true", "", asAdmin()); w.Code != http.StatusOK {
		t.Fatalf("admin delete: %d %s", w.Code, w.Body.String())
	}
}
