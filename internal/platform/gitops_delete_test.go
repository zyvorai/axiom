package platform

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestGitOpsCascadeTable(t *testing.T) {
	argo := func(finalizers ...any) map[string]any {
		return demoGitOpsObj("argocd", "a", nil, finalizers, map[string]any{})
	}
	spec := func(kv ...any) map[string]any {
		s := map[string]any{}
		for i := 0; i < len(kv); i += 2 {
			s[kv[i].(string)] = kv[i+1]
		}
		return demoGitOpsObj("flux-system", "f", nil, nil, s)
	}
	cases := []struct {
		name, kind  string
		obj         map[string]any
		cascade     bool
		consequence string
		detail      string
	}{
		{"argo no finalizers", "Application", argo(), false, "Only the Application object is removed; the workloads it deployed keep running but are no longer managed.", ""},
		{"argo resources finalizer", "Application", argo(argoResourcesFinalizer), true,
			"Deleting this application will also delete the Deployments, Services and every other resource it manages (finalizer resources-finalizer.argocd.argoproj.io).",
			"finalizer: resources-finalizer.argocd.argoproj.io"},
		{"argo background variant", "Application", argo("resources-finalizer.argocd.argoproj.io/background"), true,
			"(finalizer resources-finalizer.argocd.argoproj.io/background)", "finalizer: resources-finalizer.argocd.argoproj.io/background"},
		{"argo foreground variant among others", "Application", argo("example.com/other", "resources-finalizer.argocd.argoproj.io/foreground"), true,
			"resources-finalizer.argocd.argoproj.io/foreground", "finalizer: resources-finalizer.argocd.argoproj.io/foreground"},
		{"argo unrelated finalizer", "Application", argo("example.com/other"), false, "Only the Application object is removed", ""},
		{"argo look-alike finalizer", "Application", argo("resources-finalizer.argocd.argoproj.io.evil"), false, "Only the Application object", ""},
		{"argo typed []string finalizers", "Application", map[string]any{"metadata": map[string]any{"finalizers": []string{argoResourcesFinalizer}}}, true, "finalizer resources-finalizer", ""},
		{"ks prune true", "Kustomization", spec("prune", true), true, "Deleting this Kustomization will also delete every resource it applied", "spec.prune: true"},
		{"ks prune false", "Kustomization", spec("prune", false), false, "Only the Kustomization object is removed; the resources it applied keep running but are no longer reconciled.", "spec.prune: false"},
		{"ks prune absent", "Kustomization", spec(), false, "Only the Kustomization object", "spec.prune: false"},
		{"ks prune true suspended", "Kustomization", spec("prune", true, "suspend", true), true, "spec.prune is true", "spec.suspend: true"},
		{"ks prune false suspended", "Kustomization", spec("prune", false, "suspend", true), false, "Only the Kustomization object", "spec.suspend: true"},
		{"ks prune string is not true", "Kustomization", spec("prune", "true"), false, "Only the Kustomization", ""},
		{"hr default", "HelmRelease", spec(), true, "Deleting this HelmRelease will also uninstall its Helm release", "spec.suspend: false"},
		{"hr not suspended", "HelmRelease", spec("suspend", false), true, "uninstall", "spec.suspend: false"},
		{"hr suspended", "HelmRelease", spec("suspend", true), false, "it is suspended, so Flux does not uninstall the Helm release", "spec.suspend: true"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cascade, consequence, detail := gitOpsCascade(c.kind, c.obj)
			if cascade != c.cascade {
				t.Errorf("cascade = %v, want %v", cascade, c.cascade)
			}
			if !strings.Contains(consequence, c.consequence) {
				t.Errorf("consequence %q does not contain %q", consequence, c.consequence)
			}
			if detail == nil {
				t.Error("detail must be a list, not nil")
			}
			if c.detail != "" && !strings.Contains(strings.Join(detail, "\n"), c.detail) {
				t.Errorf("detail %v does not contain %q", detail, c.detail)
			}
			if c.kind == "Application" && !cascade && len(detail) != 0 {
				t.Errorf("non-cascading Application detail: %v", detail)
			}
		})
	}
}

func TestGitOpsDeleteConfirm(t *testing.T) {
	casc := GitOpsDeletePreview{Name: "web-app", Cascade: true}
	for _, c := range []struct {
		confirm string
		ok      bool
	}{{"", false}, {"web", false}, {"other", false}, {"Web-App", false}, {"WEB-APP", false}, {" web-app", false}, {"web-app ", false}, {"web-app", true}} {
		err := checkGitOpsDeleteConfirm(casc, c.confirm)
		if (err == nil) != c.ok {
			t.Errorf("confirm %q: err=%v", c.confirm, err)
		}
		if err != nil && (HTTPStatus(err) != http.StatusBadRequest || !strings.Contains(err.Error(), "this deletion also removes the application's resources; pass confirm=web-app")) {
			t.Errorf("confirm %q: %d %v", c.confirm, HTTPStatus(err), err)
		}
	}
	plain := GitOpsDeletePreview{Name: "web-app"}
	for _, c := range []string{"", "anything"} {
		if err := checkGitOpsDeleteConfirm(plain, c); err != nil {
			t.Errorf("non-cascading %q: %v", c, err)
		}
	}
}

func TestGitOpsDeleteTargetValidation(t *testing.T) {
	bad := [][3]string{
		{"GitRepository", "flux-system", "x"}, {"application", "argocd", "x"}, {"Pod", "default", "x"},
		{"Application", "", "x"}, {"Application", "Bad_NS", "x"}, {"Application", "argocd", ""}, {"Application", "argocd", "UPPER"},
	}
	for _, b := range bad {
		if err := checkGitOpsDeleteTarget(b[0], b[1], b[2]); HTTPStatus(err) != http.StatusBadRequest {
			t.Errorf("%v: %v", b, err)
		}
	}
	if err := checkGitOpsDeleteTarget("Kustomization", "flux-system", "apps.v2"); err != nil {
		t.Error(err)
	}
}

func TestGitOpsDeletePreviewSourceSharing(t *testing.T) {
	ks := func(ns, name, srcKind, srcName, srcNS string) map[string]any {
		sr := map[string]any{"kind": srcKind, "name": srcName}
		if srcNS != "" {
			sr["namespace"] = srcNS
		}
		return demoGitOpsObj(ns, name, nil, nil, map[string]any{"prune": true, "sourceRef": sr})
	}
	hr := func(ns, name, srcKind, srcName, srcNS string) map[string]any {
		sr := map[string]any{"kind": srcKind, "name": srcName}
		if srcNS != "" {
			sr["namespace"] = srcNS
		}
		return demoGitOpsObj(ns, name, nil, nil, map[string]any{"chart": map[string]any{"spec": map[string]any{"chart": "c", "sourceRef": sr}}})
	}
	self := ks("flux-system", "apps", "GitRepository", "repo", "")
	peers := []gitOpsPeer{
		{"Kustomization", self}, // itself: ignored
		{"Kustomization", ks("flux-system", "infra", "GitRepository", "repo", "")}, // same ns default
		{"Kustomization", ks("team-a", "x", "GitRepository", "repo", "flux-system")},
		{"Kustomization", ks("team-a", "y", "GitRepository", "repo", "")},          // team-a/repo: different
		{"Kustomization", ks("flux-system", "z", "OCIRepository", "repo", "")},     // other kind
		{"HelmRelease", hr("flux-system", "h", "GitRepository", "repo", "")},       // chart from the same git repo
		{"HelmRelease", hr("flux-system", "h2", "HelmRepository", "repo", "")},     // different kind
		{"HelmRelease", map[string]any{"metadata": map[string]any{"name": "bad"}}}, // no sourceRef
	}
	src := demoGitOpsObj("flux-system", "repo", map[string]any{GitOpsManagedByLabel: GitOpsManagedByValue}, nil, map[string]any{})
	p := buildGitOpsDeletePreview("Kustomization", self, peers, src)
	if p.Engine != "flux" || !p.Cascade || p.ManagedByAxiom || p.Source == nil {
		t.Fatalf("preview: %+v", p)
	}
	if p.Source.Kind != "GitRepository" || p.Source.Namespace != "flux-system" || p.Source.Name != "repo" || !p.Source.ManagedByAxiom {
		t.Fatalf("source: %+v", p.Source)
	}
	var got []string
	for _, r := range p.Source.SharedWith {
		got = append(got, r.String())
	}
	want := "Kustomization flux-system/infra,Kustomization team-a/x,HelmRelease flux-system/h"
	if strings.Join(got, ",") != want {
		t.Fatalf("sharedWith = %v, want %s", got, want)
	}
	if err := checkGitOpsRemoveSource(p, true); HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "Kustomization flux-system/infra") {
		t.Fatalf("shared removeSource: %v", err)
	}

	// Unshared but not created by Axiom.
	lone := buildGitOpsDeletePreview("Kustomization", self, nil, demoGitOpsObj("flux-system", "repo", nil, nil, nil))
	if lone.Source == nil || lone.Source.SharedWith == nil || len(lone.Source.SharedWith) != 0 || lone.Source.ManagedByAxiom {
		t.Fatalf("lone: %+v", lone.Source)
	}
	if err := checkGitOpsRemoveSource(lone, true); HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "not created by Axiom") {
		t.Fatalf("unmanaged removeSource: %v", err)
	}
	// Source gone: allowed (warning at the caller).
	if err := checkGitOpsRemoveSource(lone, false); err != nil {
		t.Fatalf("missing source: %v", err)
	}
	// Unshared and ours.
	ours := buildGitOpsDeletePreview("Kustomization", self, nil, src)
	if err := checkGitOpsRemoveSource(ours, true); err != nil {
		t.Fatal(err)
	}
	// Unsupported source kind.
	oci := buildGitOpsDeletePreview("Kustomization", ks("flux-system", "o", "OCIRepository", "r", ""), nil, nil)
	if err := checkGitOpsRemoveSource(oci, false); HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("oci removeSource: %v", err)
	}
	// HelmRelease source with explicit namespace.
	h := buildGitOpsDeletePreview("HelmRelease", hr("monitoring", "prom", "HelmRepository", "charts", "flux-system"), nil, nil)
	if h.Source == nil || h.Source.Namespace != "flux-system" || h.Source.Kind != "HelmRepository" {
		t.Fatalf("hr source: %+v", h.Source)
	}
	// Argo: no source; removeSource is a 400.
	a := buildGitOpsDeletePreview("Application", demoGitOpsObj("argocd", "a", map[string]any{GitOpsManagedByLabel: GitOpsManagedByValue}, nil, nil), nil, nil)
	if a.Source != nil || a.Engine != "argocd" || !a.ManagedByAxiom {
		t.Fatalf("argo: %+v", a)
	}
	if err := checkGitOpsRemoveSource(a, false); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("argo removeSource: %v", err)
	}
}

func TestBuildGitOpsSetsManagedByLabel(t *testing.T) {
	for _, req := range []GitOpsCreateRequest{argoReq(), argoHelmReq(), fluxKsReq(), fluxHelmReq()} {
		plan, err := buildGitOps(req)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range plan.Objects {
			if !gitOpsManagedByAxiom(o.Obj) {
				t.Errorf("%s %s has no managed-by label: %+v", o.Kind, o.Name, o.Obj["metadata"])
			}
		}
	}
}

func TestDemoGitOpsDeleteRoundTrip(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()

	// Seed rows exercise both cascade outcomes.
	for _, c := range []struct {
		kind, ns, name string
		cascade        bool
	}{
		{"Application", "argocd", "platform-apps", true},
		{"Application", "argocd", "docs-site", false},
		{"Kustomization", "flux-system", "apps", true},
		{"Kustomization", "flux-system", "infra", false},
		{"HelmRelease", "monitoring", "prometheus", true},
	} {
		p, err := d.PreviewGitOpsDelete(ctx, c.kind, c.ns, c.name)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if p.Cascade != c.cascade || p.ManagedByAxiom {
			t.Errorf("%s: %+v", c.name, p)
		}
	}
	p, _ := d.PreviewGitOpsDelete(ctx, "Kustomization", "flux-system", "apps")
	if p.Source == nil || len(p.Source.SharedWith) != 1 || p.Source.SharedWith[0].Name != "infra" || p.Source.ManagedByAxiom {
		t.Fatalf("shared seed source: %+v", p.Source)
	}
	if _, err := d.PreviewGitOpsDelete(ctx, "HelmRelease", "argocd", "platform-apps"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("kind mismatch: %v", err)
	}
	if _, err := d.PreviewGitOpsDelete(ctx, "Application", "argocd", "nope"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("unknown: %v", err)
	}
	// A seed source we did not create is never removed.
	if _, err := d.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Kustomization", Namespace: "flux-system", Name: "apps", Confirm: "apps", RemoveSource: true}); HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("shared seed removeSource: %v", err)
	}

	// Create a Flux app (prune on: cascades), then delete it with its source.
	req := fluxKsReq()
	req.Name = "demo-delete-roundtrip"
	if _, err := d.CreateGitOpsApp(ctx, req); err != nil {
		t.Fatal(err)
	}
	p, err := d.PreviewGitOpsDelete(ctx, "Kustomization", "flux-system", req.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Cascade || !p.ManagedByAxiom || p.Source == nil || !p.Source.ManagedByAxiom || len(p.Source.SharedWith) != 0 || p.Source.Name != req.Name {
		t.Fatalf("created preview: %+v %+v", p, p.Source)
	}
	if _, err := d.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Kustomization", Namespace: "flux-system", Name: req.Name}); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("missing confirm: %v", err)
	}
	res, err := d.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Kustomization", Namespace: "flux-system", Name: req.Name, Confirm: req.Name, RemoveSource: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || !res.Cascade || len(res.Deleted) != 2 || res.Deleted[0].Kind != "Kustomization" || res.Deleted[1].Kind != "GitRepository" {
		t.Fatalf("result: %+v", res)
	}
	items, _ := d.ListGitOps(ctx)
	for _, it := range items {
		if it.Name == req.Name {
			t.Fatalf("still listed: %+v", it)
		}
	}
	if _, err := d.PreviewGitOpsDelete(ctx, "Kustomization", "flux-system", req.Name); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("after delete: %v", err)
	}
	// Re-creating works (source was removed too).
	if _, err := d.CreateGitOpsApp(ctx, req); err != nil {
		t.Fatal(err)
	}
	// Argo app created by Axiom: no finalizer, so no confirm needed.
	a := argoReq()
	a.Name = "demo-delete-argo"
	if _, err := d.CreateGitOpsApp(ctx, a); err != nil {
		t.Fatal(err)
	}
	res, err = d.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Application", Namespace: "argocd", Name: a.Name})
	if err != nil || res.Cascade || len(res.Deleted) != 1 {
		t.Fatalf("argo delete: %+v %v", res, err)
	}
	// Clean up the re-created Flux app.
	if _, err := d.DeleteGitOpsApp(ctx, GitOpsDeleteRequest{Kind: "Kustomization", Namespace: "flux-system", Name: req.Name, Confirm: req.Name, RemoveSource: true}); err != nil {
		t.Fatal(err)
	}
}
