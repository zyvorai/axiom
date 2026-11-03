package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func editArgoObj() map[string]any {
	return map[string]any{
		"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
		"metadata": map[string]any{
			"name": "shop", "namespace": "argocd", "resourceVersion": "41",
			"annotations": map[string]any{"team": "payments"},
			"finalizers":  []any{argoResourcesFinalizer},
		},
		"spec": map[string]any{
			"project":              "default",
			"source":               map[string]any{"repoURL": "https://github.com/org/shop.git", "path": "deploy", "targetRevision": "main"},
			"destination":          map[string]any{"server": gitOpsArgoServer, "namespace": "shop"},
			"syncPolicy":           map[string]any{"automated": map[string]any{"prune": true, "selfHeal": false, "allowEmpty": true}, "syncOptions": []any{"ServerSideApply=true"}},
			"revisionHistoryLimit": int64(3), // unknown to the typed model
		},
		"status": map[string]any{"sync": map[string]any{"status": "Synced"}},
	}
}

// editBody turns the GET .../edit response into an edit request, as the UI does.
func editBody(t *testing.T, ed GitOpsEditable, mut func(*GitOpsEditRequest)) GitOpsEditRequest {
	t.Helper()
	r := GitOpsEditRequest{
		Kind: ed.Kind, Engine: ed.Engine, Name: ed.Name, Namespace: ed.Namespace,
		Source: ed.Source, Destination: ed.Destination, Project: ed.Project, Interval: ed.Interval,
		Values: ed.Values, SyncPolicy: ed.SyncPolicy, ResourceVersion: ed.ResourceVersion,
		SourceResourceVersion: ed.SourceResourceVersion,
	}
	if mut != nil {
		mut(&r)
	}
	return r
}

func patchOps(t *testing.T, b []byte) []map[string]any {
	t.Helper()
	var ops []map[string]any
	if err := json.Unmarshal(b, &ops); err != nil {
		t.Fatalf("patch: %v", err)
	}
	return ops
}

func TestGitOpsEditArgoRoundTrip(t *testing.T) {
	tgt := gitOpsEditTarget{Kind: "Application", Obj: editArgoObj()}
	ed := readGitOpsEditable(tgt)
	if !ed.Editable || ed.Engine != "argocd" || ed.Source.Path != "deploy" || ed.Source.Revision != "main" || ed.Project != "default" ||
		!ed.SyncPolicy.Automated || !ed.SyncPolicy.Prune || ed.SyncPolicy.SelfHeal || ed.Destination.Namespace != "shop" || ed.ResourceVersion != "41" {
		t.Fatalf("editable: %+v", ed)
	}
	// Sending the GET body back unchanged is a no-op.
	plan, err := planGitOpsEdit(tgt, editBody(t, ed, nil))
	if err != nil || !plan.Preview.Noop || len(plan.Preview.Changes) != 0 || plan.AppPatch != nil {
		t.Fatalf("noop: %v %+v", err, plan.Preview)
	}
	plan, err = planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) {
		r.Source.Revision = "v2"
		r.SyncPolicy.SelfHeal = true
		r.SyncPolicy.CreateNamespace = true
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []GitOpsEditChange{
		{Object: "Application/argocd/shop", Field: "spec.source.targetRevision", From: "main", To: "v2"},
		{Object: "Application/argocd/shop", Field: "spec.syncPolicy.automated.selfHeal", From: "false", To: "true"},
		{Object: "Application/argocd/shop", Field: "spec.syncPolicy.syncOptions", From: "- ServerSideApply=true", To: "- ServerSideApply=true\n- CreateNamespace=true"},
	}
	if !reflect.DeepEqual(plan.Preview.Changes, want) || plan.Preview.Noop {
		t.Fatalf("changes: %+v", plan.Preview.Changes)
	}
	ops := patchOps(t, plan.AppPatch)
	if len(ops) != 4 || ops[0]["op"] != "test" || ops[0]["path"] != "/metadata/resourceVersion" || ops[0]["value"] != "41" ||
		ops[1]["path"] != "/spec/source/targetRevision" || ops[2]["path"] != "/spec/syncPolicy/automated/selfHeal" {
		t.Fatalf("patch: %s", plan.AppPatch)
	}
	after := plan.AppAfter
	if nestedString(after, "metadata", "annotations", "team") != "payments" || len(nestedStrings(after, "metadata", "finalizers")) != 1 ||
		nestedValue(after, "spec", "revisionHistoryLimit") == nil || nestedString(after, "status", "sync", "status") != "Synced" ||
		nestedValue(after, "spec", "syncPolicy", "automated", "allowEmpty") != true {
		t.Fatalf("unmanaged fields lost: %+v", after)
	}
	if !strings.Contains(plan.Preview.YAML, "targetRevision: v2") || !strings.Contains(plan.Preview.YAML, "team: payments") {
		t.Fatalf("yaml: %s", plan.Preview.YAML)
	}
	if !reflect.DeepEqual(plan.Fields, []string{"spec.source.targetRevision", "spec.syncPolicy.automated.selfHeal", "spec.syncPolicy.syncOptions"}) {
		t.Errorf("fields: %v", plan.Fields)
	}
	// The original is untouched (the plan works on a copy).
	if nestedString(tgt.Obj, "spec", "source", "targetRevision") != "main" {
		t.Fatal("plan mutated the live object")
	}

	// Turning automated sync off removes the whole automated block.
	plan, _ = planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) { r.SyncPolicy = GitOpsSyncPolicy{} }))
	ops = patchOps(t, plan.AppPatch)
	if len(ops) != 2 || ops[1]["op"] != "remove" || ops[1]["path"] != "/spec/syncPolicy/automated" {
		t.Fatalf("automated off: %s", plan.AppPatch)
	}
}

func TestGitOpsEditStaleAndIdentity(t *testing.T) {
	tgt := gitOpsEditTarget{Kind: "Application", Obj: editArgoObj()}
	ed := readGitOpsEditable(tgt)
	_, err := planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) { r.ResourceVersion = "40" }))
	if HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "the application changed since you opened it") {
		t.Fatalf("stale: %v", err)
	}
	if _, err := planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) { r.ResourceVersion = "" })); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("missing rv: %v", err)
	}
	for field, mut := range map[string]func(*GitOpsEditRequest){
		"kind":         func(r *GitOpsEditRequest) { r.Kind = "Kustomization" },
		"engine":       func(r *GitOpsEditRequest) { r.Engine = "flux-helm" },
		"name":         func(r *GitOpsEditRequest) { r.Name = "other" },
		"namespace":    func(r *GitOpsEditRequest) { r.Namespace = "default" },
		"source.chart": func(r *GitOpsEditRequest) { r.Source.Chart = "nginx"; r.Source.ChartVersion = "1.0.0" },
	} {
		_, err := planGitOpsEdit(tgt, editBody(t, ed, mut))
		if HTTPStatus(err) != http.StatusBadRequest || !strings.HasPrefix(err.Error(), field) {
			t.Errorf("%s: %v", field, err)
		}
	}
	// Project is editable for Argo CD.
	plan, err := planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) { r.Project = "payments" }))
	if err != nil || len(plan.Preview.Changes) != 1 || plan.Preview.Changes[0].Field != "spec.project" {
		t.Fatalf("project: %v %+v", err, plan.Preview.Changes)
	}
}

func TestGitOpsEditSameValidationAsCreate(t *testing.T) {
	tgt := gitOpsEditTarget{Kind: "Application", Obj: editArgoObj()}
	ed := readGitOpsEditable(tgt)
	for name, mut := range map[string]func(*GitOpsEditRequest){
		"credentials in url": func(r *GitOpsEditRequest) { r.Source.RepoURL = "https://u:s3cr3t@github.com/org/shop.git" },
		"escaping path":      func(r *GitOpsEditRequest) { r.Source.Path = "../x" },
		"interval on argo":   func(r *GitOpsEditRequest) { r.Interval = "5m" },
		"prune w/o auto":     func(r *GitOpsEditRequest) { r.SyncPolicy = GitOpsSyncPolicy{Prune: true} },
		"values on git path": func(r *GitOpsEditRequest) { r.Values = "a: 1" },
	} {
		_, err := planGitOpsEdit(tgt, editBody(t, ed, mut))
		if HTTPStatus(err) != http.StatusBadRequest {
			t.Errorf("%s: %v", name, err)
		}
		if err != nil && strings.Contains(err.Error(), "s3cr3t") {
			t.Errorf("%s: error echoes credentials: %v", name, err)
		}
	}
	// Warnings are the creation warnings.
	plan, _ := planGitOpsEdit(tgt, editBody(t, ed, nil))
	if len(plan.Preview.Warnings) == 0 || !strings.Contains(strings.Join(plan.Preview.Warnings, " "), "moving branch") {
		t.Errorf("warnings: %v", plan.Preview.Warnings)
	}
}

func TestGitOpsEditNotEditable(t *testing.T) {
	multi := editArgoObj()
	multi["spec"].(map[string]any)["sources"] = []any{map[string]any{"repoURL": "https://a/b"}}
	appset := editArgoObj()
	appset["metadata"].(map[string]any)["ownerReferences"] = []any{map[string]any{"kind": "ApplicationSet", "name": "all"}}
	helmGit := editArgoObj()
	helmGit["spec"].(map[string]any)["source"].(map[string]any)["helm"] = map[string]any{"valueFiles": []any{"v.yaml"}}

	ks := demoGitOpsObj("flux-system", "ks", nil, nil, map[string]any{
		"path": "./x", "interval": "5m", "sourceRef": map[string]any{"kind": "GitRepository", "name": "ks"},
		"patches": []any{map[string]any{"patch": "x"}},
	})
	post := demoGitOpsObj("flux-system", "ks", nil, nil, map[string]any{
		"path": "./x", "sourceRef": map[string]any{"kind": "GitRepository", "name": "ks"}, "postBuild": map[string]any{"substitute": map[string]any{"a": "b"}},
	})
	ociKs := demoGitOpsObj("flux-system", "ks", nil, nil, map[string]any{"path": "./", "sourceRef": map[string]any{"kind": "OCIRepository", "name": "o"}})
	vf := demoGitOpsObj("apps", "hr", nil, nil, map[string]any{
		"chart":      map[string]any{"spec": map[string]any{"chart": "c", "sourceRef": map[string]any{"kind": "HelmRepository", "name": "r"}}},
		"valuesFrom": []any{map[string]any{"kind": "ConfigMap", "name": "v"}},
	})
	chartRef := demoGitOpsObj("apps", "hr", nil, nil, map[string]any{"chartRef": map[string]any{"kind": "OCIRepository", "name": "o"}})

	for _, c := range []struct {
		name, kind string
		obj        map[string]any
		reason     string
	}{
		{"multi-source", "Application", multi, "spec.sources"},
		{"applicationset", "Application", appset, "ApplicationSet"},
		{"helm on git path", "Application", helmGit, "spec.source.helm"},
		{"patches", "Kustomization", ks, "spec.patches"},
		{"postBuild", "Kustomization", post, "spec.postBuild"},
		{"oci source", "Kustomization", ociKs, "OCIRepository"},
		{"valuesFrom", "HelmRelease", vf, "spec.valuesFrom"},
		{"chartRef", "HelmRelease", chartRef, "spec.chartRef"},
	} {
		tgt := gitOpsEditTarget{Kind: c.kind, Obj: c.obj}
		ed := readGitOpsEditable(tgt)
		if ed.Editable || !strings.Contains(strings.Join(ed.Reasons, " "), c.reason) {
			t.Errorf("%s: %+v", c.name, ed)
		}
		_, err := planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) { r.ResourceVersion = "1" }))
		if HTTPStatus(err) != http.StatusBadRequest || !strings.Contains(err.Error(), "cannot be edited here") {
			t.Errorf("%s: plan: %v", c.name, err)
		}
	}
}

func fluxKsTarget(srcLabels map[string]any, peers ...gitOpsPeer) gitOpsEditTarget {
	ks := demoGitOpsObj("flux-system", "web", map[string]any{GitOpsManagedByLabel: GitOpsManagedByValue}, nil, map[string]any{
		"path": "./deploy", "interval": "5m", "prune": true, "targetNamespace": "web", "wait": false, "timeout": "5m",
		"sourceRef": map[string]any{"kind": "GitRepository", "name": "web"}, "healthChecks": []any{map[string]any{"kind": "Deployment", "name": "web"}},
	})
	ks["metadata"].(map[string]any)["resourceVersion"] = "7"
	src := demoGitOpsObj("flux-system", "web", srcLabels, nil, map[string]any{
		"url": "https://github.com/org/web.git", "interval": "5m", "ref": map[string]any{"branch": "main"},
		"secretRef": map[string]any{"name": "web-creds"}, "ignore": "/docs",
	})
	src["metadata"].(map[string]any)["resourceVersion"] = "3"
	return gitOpsEditTarget{Kind: "Kustomization", Obj: ks, SourceObj: src, Peers: append(peers, gitOpsPeer{Kind: "Kustomization", Obj: ks})}
}

func TestGitOpsEditFluxSourceRules(t *testing.T) {
	managed := map[string]any{GitOpsManagedByLabel: GitOpsManagedByValue}
	tgt := fluxKsTarget(managed)
	ed := readGitOpsEditable(tgt)
	if !ed.Editable || len(ed.ReadOnly) != 0 || ed.Source.RepoURL != "https://github.com/org/web.git" || ed.Source.Revision != "main" ||
		ed.Source.SecretRef != "web-creds" || ed.Source.Path != "deploy" || ed.Interval != "5m" || !ed.SyncPolicy.Prune ||
		ed.SourceObject == nil || ed.SourceObject.Kind != "GitRepository" || ed.SourceResourceVersion != "3" {
		t.Fatalf("editable: %+v", ed)
	}
	plan, err := planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) {
		r.Source.Revision = "refs/tags/v1.2.0"
		r.Source.SecretRef = ""
		r.Source.Path = "deploy/prod"
		r.Interval = "10m"
	}))
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]GitOpsEditChange{}
	for _, c := range plan.Preview.Changes {
		fields[c.Object+" "+c.Field] = c
	}
	for _, f := range []string{
		"GitRepository/flux-system/web spec.ref.branch", "GitRepository/flux-system/web spec.ref.tag", "GitRepository/flux-system/web spec.secretRef",
		"GitRepository/flux-system/web spec.interval", "Kustomization/flux-system/web spec.path", "Kustomization/flux-system/web spec.interval",
	} {
		if _, ok := fields[f]; !ok {
			t.Errorf("missing change %s in %+v", f, plan.Preview.Changes)
		}
	}
	if len(plan.Preview.Changes) != 6 || fields["Kustomization/flux-system/web spec.path"].To != "./deploy/prod" {
		t.Errorf("changes: %+v", plan.Preview.Changes)
	}
	if nestedString(plan.SourceAfter, "spec", "ignore") != "/docs" || nestedString(plan.SourceAfter, "spec", "ref", "tag") != "v1.2.0" ||
		nestedValue(plan.SourceAfter, "spec", "ref", "branch") != nil || nestedValue(plan.SourceAfter, "spec", "secretRef") != nil {
		t.Errorf("source after: %+v", plan.SourceAfter["spec"])
	}
	if nestedValue(plan.AppAfter, "spec", "healthChecks") == nil || nestedString(plan.AppAfter, "spec", "timeout") != "5m" {
		t.Errorf("unmanaged ks fields lost: %+v", plan.AppAfter["spec"])
	}
	if strings.Count(plan.Preview.YAML, "\n---\n") != 1 || ops0(t, plan.SourcePatch)["value"] != "3" {
		t.Errorf("yaml/source patch: %s %s", plan.Preview.YAML, plan.SourcePatch)
	}
	// Only the application changes: the source stays untouched.
	plan, _ = planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) { r.SyncPolicy.Prune = false }))
	if plan.SourcePatch != nil || plan.AppPatch == nil || len(plan.Preview.Changes) != 1 {
		t.Errorf("app-only edit: %+v", plan.Preview.Changes)
	}
	// Stale source resourceVersion.
	if _, err := planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) { r.SourceResourceVersion = "2" })); HTTPStatus(err) != http.StatusConflict {
		t.Errorf("stale source: %v", err)
	}

	// Not Axiom's source: read-only, the app's own fields stay editable.
	other := fluxKsTarget(nil)
	ed = readGitOpsEditable(other)
	if !ed.Editable || !reflect.DeepEqual(ed.ReadOnly, []string{"source.repoUrl", "source.revision", "source.secretRef"}) ||
		!strings.Contains(strings.Join(ed.Reasons, " "), "not created by Axiom") {
		t.Fatalf("unmanaged source: %+v", ed)
	}
	if _, err := planGitOpsEdit(other, editBody(t, ed, func(r *GitOpsEditRequest) { r.Source.RepoURL = "https://github.com/org/evil.git" })); HTTPStatus(err) != http.StatusBadRequest ||
		!strings.HasPrefix(err.Error(), "source.repoUrl cannot be changed") {
		t.Errorf("read-only url: %v", err)
	}
	plan, err = planGitOpsEdit(other, editBody(t, ed, func(r *GitOpsEditRequest) { r.Source.Path = "other"; r.Interval = "1h" }))
	if err != nil || plan.SourcePatch != nil || len(plan.Preview.Changes) != 2 {
		t.Errorf("unmanaged source app edit: %v %+v", err, plan.Preview.Changes)
	}

	// Shared source: read-only too, even though Axiom created it.
	peer := demoGitOpsObj("flux-system", "web-canary", nil, nil, map[string]any{"sourceRef": map[string]any{"kind": "GitRepository", "name": "web"}})
	shared := fluxKsTarget(managed, gitOpsPeer{Kind: "Kustomization", Obj: peer})
	ed = readGitOpsEditable(shared)
	if len(ed.ReadOnly) != 3 || !strings.Contains(strings.Join(ed.Reasons, " "), "shared with Kustomization flux-system/web-canary") {
		t.Fatalf("shared source: %+v", ed)
	}
	if _, err := planGitOpsEdit(shared, editBody(t, ed, func(r *GitOpsEditRequest) { r.Source.Revision = "dev" })); HTTPStatus(err) != http.StatusBadRequest {
		t.Errorf("shared revision: %v", err)
	}
	// Missing source: read-only, app fields still editable.
	missing := fluxKsTarget(managed)
	missing.SourceObj = nil
	ed = readGitOpsEditable(missing)
	if _, err := planGitOpsEdit(missing, editBody(t, ed, func(r *GitOpsEditRequest) { r.Source.Path = "x" })); err != nil || len(ed.ReadOnly) != 3 {
		t.Errorf("missing source: %v %+v", err, ed)
	}
}

func ops0(t *testing.T, b []byte) map[string]any {
	ops := patchOps(t, b)
	if len(ops) == 0 {
		t.Fatal("empty patch")
	}
	return ops[0]
}

func TestGitOpsEditHelmReleaseValues(t *testing.T) {
	hr := demoGitOpsObj("apps", "redis", nil, nil, map[string]any{
		"interval": "10m", "targetNamespace": "cache",
		"chart":   map[string]any{"spec": map[string]any{"chart": "redis", "version": "18.0.0", "sourceRef": map[string]any{"kind": "HelmRepository", "name": "bitnami", "namespace": "flux-system"}}},
		"values":  map[string]any{"replicas": int64(2), "auth": map[string]any{"enabled": true}},
		"upgrade": map[string]any{"remediation": map[string]any{"retries": int64(3)}},
	})
	hr["metadata"].(map[string]any)["resourceVersion"] = "9"
	repo := demoGitOpsObj("flux-system", "bitnami", map[string]any{GitOpsManagedByLabel: GitOpsManagedByValue}, nil, map[string]any{
		"url": "https://charts.bitnami.com/bitnami", "interval": "1h",
	})
	tgt := gitOpsEditTarget{Kind: "HelmRelease", Obj: hr, SourceObj: repo}
	ed := readGitOpsEditable(tgt)
	if !ed.Editable || ed.Engine != "flux-helm" || ed.Source.Chart != "redis" || ed.Source.ChartVersion != "18.0.0" ||
		!strings.Contains(ed.Values, "replicas: 2") || ed.Source.RepoURL != "https://charts.bitnami.com/bitnami" || len(ed.ReadOnly) != 0 {
		t.Fatalf("editable: %+v", ed)
	}
	if plan, err := planGitOpsEdit(tgt, editBody(t, ed, nil)); err != nil || !plan.Preview.Noop {
		t.Fatalf("noop: %v %+v", err, plan.Preview.Changes)
	}
	plan, err := planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) {
		r.Values = "replicas: 3\n"
		r.Source.ChartVersion = "18.1.0"
		r.Source.RepoURL = "oci://registry-1.docker.io/bitnamicharts"
	}))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := nestedMap(plan.AppAfter, "spec", "values")
	if len(v) != 1 || v["replicas"] != float64(3) {
		t.Fatalf("values must be replaced, not merged: %+v", v)
	}
	if nestedValue(plan.AppAfter, "spec", "upgrade", "remediation", "retries") == nil || nestedString(plan.SourceAfter, "spec", "type") != "oci" {
		t.Errorf("after: %+v / %+v", plan.AppAfter["spec"], plan.SourceAfter["spec"])
	}
	if _, err := planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) { r.Values = "- a" })); HTTPStatus(err) != http.StatusBadRequest {
		t.Errorf("list values: %v", err)
	}
	if _, err := planGitOpsEdit(tgt, editBody(t, ed, func(r *GitOpsEditRequest) { r.Interval = "10s" })); HTTPStatus(err) != http.StatusBadRequest {
		t.Errorf("short interval: %v", err)
	}
}

func TestDemoGitOpsEditParity(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if _, err := d.CreateGitOpsApp(ctx, GitOpsCreateRequest{Engine: GitOpsEngineFluxKustomize, Name: "edit-demo",
		Source: GitOpsSource{RepoURL: "https://github.com/org/repo.git", Path: "deploy", Revision: "v1"}, Interval: "5m"}); err != nil {
		t.Fatal(err)
	}
	ref := GitOpsRef{Kind: "Kustomization", Namespace: "flux-system", Name: "edit-demo"}
	ed, err := d.GetGitOpsEditable(ctx, ref)
	if err != nil || !ed.Editable || len(ed.ReadOnly) != 0 || ed.Source.Revision != "v1" || ed.ResourceVersion == "" {
		t.Fatalf("get: %v %+v", err, ed)
	}
	req := editBody(t, ed, func(r *GitOpsEditRequest) { r.Source.Revision = "v2"; r.Source.Path = "deploy/prod" })
	p, err := d.PreviewGitOpsEdit(ctx, ref, req)
	if err != nil || p.Noop || len(p.Changes) != 2 {
		t.Fatalf("preview: %v %+v", err, p)
	}
	res, err := d.UpdateGitOpsApp(ctx, ref, req)
	if err != nil || !res.OK || res.ResourceVersion == ed.ResourceVersion {
		t.Fatalf("update: %v %+v", err, res)
	}
	if _, err := d.UpdateGitOpsApp(ctx, ref, req); HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("stale: %v", err)
	}
	ed2, _ := d.GetGitOpsEditable(ctx, ref)
	if ed2.Source.Revision != "v2" || ed2.Source.Path != "deploy/prod" {
		t.Fatalf("after: %+v", ed2)
	}
	// Seeds: the shared, hand-made flux-system GitRepository is read-only.
	seed, err := d.GetGitOpsEditable(ctx, GitOpsRef{Kind: "Kustomization", Namespace: "flux-system", Name: "apps"})
	if err != nil || !seed.Editable || len(seed.ReadOnly) != 3 || seed.Source.RepoURL == "" {
		t.Fatalf("seed: %v %+v", err, seed)
	}
	if a, err := d.GetGitOpsEditable(ctx, GitOpsRef{Kind: "Application", Namespace: "argocd", Name: "platform-apps"}); err != nil || !a.Editable || a.Source.Path != "apps" {
		t.Fatalf("argo seed: %v %+v", err, a)
	}
	if _, err := d.GetGitOpsEditable(ctx, GitOpsRef{Kind: "Application", Namespace: "argocd", Name: "nope"}); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("404: %v", err)
	}
}
