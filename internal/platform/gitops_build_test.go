package platform

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func argoReq() GitOpsCreateRequest {
	return GitOpsCreateRequest{
		Engine: GitOpsEngineArgoCD, Name: "web",
		Source:      GitOpsSource{RepoURL: "https://github.com/org/repo.git", Path: "deploy/overlays/prod"},
		Destination: GitOpsDestination{Namespace: "prod"},
		SyncPolicy:  GitOpsSyncPolicy{Automated: true, Prune: true, SelfHeal: true, CreateNamespace: true},
	}
}

func argoHelmReq() GitOpsCreateRequest {
	return GitOpsCreateRequest{
		Engine: GitOpsEngineArgoCD, Name: "nginx",
		Source:      GitOpsSource{RepoURL: "https://charts.bitnami.com/bitnami", Chart: "nginx", ChartVersion: "15.1.0"},
		Destination: GitOpsDestination{Namespace: "web"},
		Values:      "replicaCount: 2\n",
	}
}

func fluxKsReq() GitOpsCreateRequest {
	return GitOpsCreateRequest{
		Engine: GitOpsEngineFluxKustomize, Name: "apps",
		Source:      GitOpsSource{RepoURL: "ssh://git@github.com/org/repo.git", Path: "clusters/prod", Revision: "refs/tags/v1.2.3", SecretRef: "repo-creds"},
		Destination: GitOpsDestination{Namespace: "prod"},
		Interval:    "10m",
		SyncPolicy:  GitOpsSyncPolicy{Prune: true},
	}
}

func fluxHelmReq() GitOpsCreateRequest {
	return GitOpsCreateRequest{
		Engine: GitOpsEngineFluxHelm, Name: "podinfo",
		Source:      GitOpsSource{RepoURL: "oci://ghcr.io/stefanprodan/charts", Chart: "podinfo", ChartVersion: "6.5.0"},
		Destination: GitOpsDestination{Namespace: "podinfo"},
		Values:      "replicaCount: 2\nui:\n  color: blue\n",
		SyncPolicy:  GitOpsSyncPolicy{CreateNamespace: true},
	}
}

const goldenArgo = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  labels:
    app.kubernetes.io/managed-by: zyvor-axiom
  name: web
  namespace: argocd
spec:
  destination:
    namespace: prod
    server: https://kubernetes.default.svc
  project: default
  source:
    path: deploy/overlays/prod
    repoURL: https://github.com/org/repo.git
    targetRevision: HEAD
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
    - CreateNamespace=true
`

const goldenArgoHelm = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  labels:
    app.kubernetes.io/managed-by: zyvor-axiom
  name: nginx
  namespace: argocd
spec:
  destination:
    namespace: web
    server: https://kubernetes.default.svc
  project: default
  source:
    chart: nginx
    helm:
      values: |
        replicaCount: 2
    repoURL: https://charts.bitnami.com/bitnami
    targetRevision: 15.1.0
`

const goldenFluxKs = `apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  labels:
    app.kubernetes.io/managed-by: zyvor-axiom
  name: apps
  namespace: flux-system
spec:
  interval: 10m
  ref:
    tag: v1.2.3
  secretRef:
    name: repo-creds
  url: ssh://git@github.com/org/repo.git
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  labels:
    app.kubernetes.io/managed-by: zyvor-axiom
  name: apps
  namespace: flux-system
spec:
  interval: 10m
  path: ./clusters/prod
  prune: true
  sourceRef:
    kind: GitRepository
    name: apps
  targetNamespace: prod
  timeout: 5m
  wait: false
`

const goldenFluxHelm = `apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  labels:
    app.kubernetes.io/managed-by: zyvor-axiom
  name: podinfo
  namespace: flux-system
spec:
  interval: 5m
  type: oci
  url: oci://ghcr.io/stefanprodan/charts
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  labels:
    app.kubernetes.io/managed-by: zyvor-axiom
  name: podinfo
  namespace: flux-system
spec:
  chart:
    spec:
      chart: podinfo
      sourceRef:
        kind: HelmRepository
        name: podinfo
      version: 6.5.0
  install:
    createNamespace: true
  interval: 5m
  targetNamespace: podinfo
  values:
    replicaCount: 2
    ui:
      color: blue
`

func TestPreviewGitOpsGolden(t *testing.T) {
	cases := []struct {
		name  string
		req   GitOpsCreateRequest
		yaml  string
		kinds []string
	}{
		{"argocd git", argoReq(), goldenArgo, []string{"argoproj.io/v1alpha1 Application"}},
		{"argocd helm", argoHelmReq(), goldenArgoHelm, []string{"argoproj.io/v1alpha1 Application"}},
		{"flux kustomize", fluxKsReq(), goldenFluxKs, []string{"source.toolkit.fluxcd.io/v1 GitRepository", "kustomize.toolkit.fluxcd.io/v1 Kustomization"}},
		{"flux helm", fluxHelmReq(), goldenFluxHelm, []string{"source.toolkit.fluxcd.io/v1 HelmRepository", "helm.toolkit.fluxcd.io/v2 HelmRelease"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := PreviewGitOps(c.req)
			if err != nil {
				t.Fatal(err)
			}
			if p.YAML != c.yaml {
				t.Errorf("yaml mismatch:\n--- got\n%s\n--- want\n%s", p.YAML, c.yaml)
			}
			if len(p.Objects) != len(c.kinds) {
				t.Fatalf("objects: %+v", p.Objects)
			}
			for i, o := range p.Objects {
				if o.APIVersion+" "+o.Kind != c.kinds[i] || o.Name != c.req.Name || o.Namespace == "" {
					t.Errorf("object %d: %+v", i, o)
				}
			}
			if p.Warnings == nil {
				t.Error("warnings must be an empty list, not null")
			}
		})
	}
}

func TestBuildGitOpsDefaultsAndRefs(t *testing.T) {
	// Flux git: revision defaults to branch main, path "." becomes "./".
	r := GitOpsCreateRequest{Engine: GitOpsEngineFluxKustomize, Name: "x", Source: GitOpsSource{RepoURL: "https://h/o/r.git"}}
	p, err := PreviewGitOps(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"branch: main", "path: ./\n", "interval: 5m", "namespace: flux-system", "prune: false"} {
		if !strings.Contains(p.YAML, want) {
			t.Errorf("missing %q in\n%s", want, p.YAML)
		}
	}
	r.Source.Revision = "0123456789abcdef0123456789abcdef01234567"
	p, _ = PreviewGitOps(r)
	if !strings.Contains(p.YAML, "commit: 0123456789abcdef0123456789abcdef01234567") {
		t.Errorf("commit ref: %s", p.YAML)
	}
	r.Source.Revision = "release/1.x"
	p, _ = PreviewGitOps(r)
	if !strings.Contains(p.YAML, "branch: release/1.x") {
		t.Errorf("branch ref: %s", p.YAML)
	}
	// Path is cleaned.
	r.Source.Path = "a/./b/"
	p, _ = PreviewGitOps(r)
	if !strings.Contains(p.YAML, "path: ./a/b\n") {
		t.Errorf("cleaned path: %s", p.YAML)
	}
	// Argo: no syncPolicy at all when manual; no finalizers ever.
	a := GitOpsCreateRequest{Engine: GitOpsEngineArgoCD, Name: "m", Source: GitOpsSource{RepoURL: "git@github.com:org/repo.git"}}
	p, err = PreviewGitOps(a)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.YAML, "syncPolicy") || strings.Contains(p.YAML, "finalizer") || !strings.Contains(p.YAML, "path: .\n") {
		t.Errorf("manual argo app: %s", p.YAML)
	}
	// Explicit destination server and project are kept.
	a.Destination.Server, a.Project = "https://10.0.0.1:6443", "team-a"
	p, _ = PreviewGitOps(a)
	if !strings.Contains(p.YAML, "server: https://10.0.0.1:6443") || !strings.Contains(p.YAML, "project: team-a") {
		t.Errorf("server/project: %s", p.YAML)
	}
	// HelmRepository over https has no type field; no chart version → no version.
	h := GitOpsCreateRequest{Engine: GitOpsEngineFluxHelm, Name: "h", Source: GitOpsSource{RepoURL: "https://charts.example.com", Chart: "c"}}
	p, err = PreviewGitOps(h)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.YAML, "type: oci") || strings.Contains(p.YAML, "version:") {
		t.Errorf("https helm repo: %s", p.YAML)
	}
}

func TestBuildGitOpsWarnings(t *testing.T) {
	has := func(ws []string, sub string) bool {
		for _, w := range ws {
			if strings.Contains(w, sub) {
				return true
			}
		}
		return false
	}
	r := argoReq()
	r.Source.Path = ""
	r.Source.RepoURL = "http://git.example.com/org/repo.git"
	r.Namespace = "team-apps"
	p, err := PreviewGitOps(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"automated prune", "moving branch", "plain http", "apps-in-any-namespace"} {
		if !has(p.Warnings, sub) {
			t.Errorf("missing warning %q: %q", sub, p.Warnings)
		}
	}
	for _, w := range p.Warnings {
		if strings.Contains(w, "git.example.com") {
			t.Errorf("warning echoes the repo URL: %q", w)
		}
	}
	// A pinned tag, scoped path, https, default namespace: no warnings.
	r = argoReq()
	r.Source.Revision = "v1.2.3"
	p, _ = PreviewGitOps(r)
	if len(p.Warnings) != 0 {
		t.Errorf("unexpected warnings: %q", p.Warnings)
	}
	// Flux is always automated: tracking main warns.
	k := fluxKsReq()
	k.Source.Revision = ""
	k.Source.Path = ""
	k.Namespace = "apps"
	p, _ = PreviewGitOps(k)
	for _, sub := range []string{"moving branch", "automated prune", "Flux controllers watch"} {
		if !has(p.Warnings, sub) {
			t.Errorf("flux: missing warning %q: %q", sub, p.Warnings)
		}
	}
}

func TestBuildGitOpsRejections(t *testing.T) {
	type mut func(*GitOpsCreateRequest)
	cases := []struct {
		name string
		base func() GitOpsCreateRequest
		mut  mut
		want string
	}{
		{"unknown engine", argoReq, func(r *GitOpsCreateRequest) { r.Engine = "jenkins" }, "engine must be"},
		{"empty engine", argoReq, func(r *GitOpsCreateRequest) { r.Engine = "" }, "engine must be"},
		{"missing name", argoReq, func(r *GitOpsCreateRequest) { r.Name = "" }, "name is required"},
		{"uppercase name", argoReq, func(r *GitOpsCreateRequest) { r.Name = "Web" }, "DNS-1123"},
		{"name too long", argoReq, func(r *GitOpsCreateRequest) { r.Name = strings.Repeat("a", 54) }, "at most 53"},
		{"name with dot", argoReq, func(r *GitOpsCreateRequest) { r.Name = "a.b" }, "DNS-1123"},
		{"bad namespace", argoReq, func(r *GitOpsCreateRequest) { r.Namespace = "Argo_CD" }, "namespace must be"},
		{"bad destination namespace", argoReq, func(r *GitOpsCreateRequest) { r.Destination.Namespace = "-prod" }, "destination.namespace"},
		{"bad project", argoReq, func(r *GitOpsCreateRequest) { r.Project = "My Project" }, "project must be"},
		{"bad secretRef", fluxKsReq, func(r *GitOpsCreateRequest) { r.Source.SecretRef = "creds/x" }, "source.secretRef must be"},
		{"http server", argoReq, func(r *GitOpsCreateRequest) { r.Destination.Server = "http://k8s:6443" }, "https URL"},
		{"server with userinfo", argoReq, func(r *GitOpsCreateRequest) { r.Destination.Server = "https://u:p@k8s:6443" }, "credentials"},
		{"server no host", argoReq, func(r *GitOpsCreateRequest) { r.Destination.Server = "https://" }, "https URL"},
		{"missing repo", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "" }, "repoUrl is required"},
		{"repo with password", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "https://user:hunter2@github.com/o/r.git" }, "must not contain credentials"},
		{"repo with token user", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "https://ghp_token@github.com/o/r.git" }, "must not contain credentials"},
		{"ssh repo with password", fluxKsReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "ssh://git:pw@github.com/o/r.git" }, "must not contain credentials"},
		{"repo whitespace", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "https://github.com/o/r .git" }, "whitespace"},
		{"repo control char", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "https://github.com/o/r\n.git" }, "whitespace"},
		{"repo no host", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "https:///o/r.git" }, "host"},
		{"repo too long", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "https://h/" + strings.Repeat("a", 510) }, "at most 512"},
		{"repo file scheme", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "file:///etc/passwd" }, "scheme must be"},
		{"repo query", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "https://h/o/r.git?x=1" }, "query"},
		{"oci for git source", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "oci://ghcr.io/o/r" }, "scheme must be"},
		{"ssh for helm source", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "ssh://git@h/o/r" }, "scheme must be"},
		{"scp-like for helm", argoHelmReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "git@github.com:o/r.git" }, "invalid source.repoUrl"},
		{"scp-like for flux", fluxKsReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "git@github.com:o/r.git" }, "ssh://"},
		{"git scheme for flux", fluxKsReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "git://github.com/o/r.git" }, "scheme must be"},
		{"no scheme", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "github.com/o/r" }, "invalid source.repoUrl"},
		{"path escapes", argoReq, func(r *GitOpsCreateRequest) { r.Source.Path = "../other" }, "'..'"},
		{"path inner dotdot", argoReq, func(r *GitOpsCreateRequest) { r.Source.Path = "a/../../b" }, "'..'"},
		{"absolute path", argoReq, func(r *GitOpsCreateRequest) { r.Source.Path = "/etc" }, "leading '/'"},
		{"backslash path", fluxKsReq, func(r *GitOpsCreateRequest) { r.Source.Path = `a\b` }, "backslash"},
		{"path too long", argoReq, func(r *GitOpsCreateRequest) { r.Source.Path = strings.Repeat("a", 257) }, "at most 256"},
		{"bad revision", argoReq, func(r *GitOpsCreateRequest) { r.Source.Revision = "main;rm -rf" }, "source.revision"},
		{"revision too long", argoReq, func(r *GitOpsCreateRequest) { r.Source.Revision = strings.Repeat("a", 129) }, "source.revision"},
		{"bad chart", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Source.Chart = "nginx$(x)" }, "source.chart"},
		{"bad chart version", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Source.ChartVersion = "1.0\n" }, "source.chartVersion"},
		{"chart and path", argoHelmReq, func(r *GitOpsCreateRequest) { r.Source.Path = "x" }, "either path or chart"},
		{"chart with revision", argoHelmReq, func(r *GitOpsCreateRequest) { r.Source.Revision = "main" }, "use source.chartVersion"},
		{"argo chart without version", argoHelmReq, func(r *GitOpsCreateRequest) { r.Source.ChartVersion = "" }, "chartVersion is required"},
		{"flux helm without chart", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Source.Chart = "" }, "chart is required"},
		{"chart on kustomize", fluxKsReq, func(r *GitOpsCreateRequest) { r.Source.Chart = "x" }, "not supported for flux-kustomize"},
		{"chartVersion without chart", argoReq, func(r *GitOpsCreateRequest) { r.Source.ChartVersion = "1.0.0" }, "requires source.chart"},
		{"values on git source", argoReq, func(r *GitOpsCreateRequest) { r.Values = "a: 1" }, "only supported for Helm"},
		{"values not a mapping", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Values = "- a\n- b\n" }, "YAML mapping"},
		{"values scalar", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Values = "hello" }, "YAML mapping"},
		{"values invalid yaml", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Values = "a: [1, 2\n" }, "valid YAML"},
		{"values too large", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Values = "a: \"" + strings.Repeat("x", 64<<10) + "\"" }, "at most 64 KiB"},
		{"values alias bomb", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Values = aliasBomb() }, "excessive aliasing"},
		{"values too deep", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Values = deepYAML(40) }, "deeper than"},
		{"secretRef on argocd", argoReq, func(r *GitOpsCreateRequest) { r.Source.SecretRef = "creds" }, "Argo CD repository credentials are configured in Argo CD, not here"},
		{"interval on argocd", argoReq, func(r *GitOpsCreateRequest) { r.Interval = "5m" }, "interval is not supported"},
		{"prune without automated (argo)", argoReq, func(r *GitOpsCreateRequest) { r.SyncPolicy.Automated = false }, "require syncPolicy.automated"},
		{"project on flux", fluxKsReq, func(r *GitOpsCreateRequest) { r.Project = "default" }, "project is not supported"},
		{"server on flux", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Destination.Server = "https://k" }, "destination.server is not supported"},
		{"bad interval", fluxKsReq, func(r *GitOpsCreateRequest) { r.Interval = "5 minutes" }, "interval must look like"},
		{"interval ms", fluxKsReq, func(r *GitOpsCreateRequest) { r.Interval = "500ms" }, "interval must look like"},
		{"interval compound", fluxKsReq, func(r *GitOpsCreateRequest) { r.Interval = "1h30m" }, "interval must look like"},
		{"interval too short", fluxKsReq, func(r *GitOpsCreateRequest) { r.Interval = "10s" }, "at least 30s"},
		{"createNamespace without destination", argoReq, func(r *GitOpsCreateRequest) { r.Destination.Namespace = "" }, "requires destination.namespace"},
		{"createNamespace on kustomize", fluxKsReq, func(r *GitOpsCreateRequest) { r.SyncPolicy.CreateNamespace = true }, "not supported for flux-kustomize"},
		{"prune on flux helm", fluxHelmReq, func(r *GitOpsCreateRequest) { r.SyncPolicy.Prune = true }, "prune is not supported for flux-helm"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := c.base()
			c.mut(&r)
			_, err := PreviewGitOps(r)
			if err == nil {
				t.Fatalf("accepted: %+v", r)
			}
			if HTTPStatus(err) != http.StatusBadRequest {
				t.Errorf("status %d for %v", HTTPStatus(err), err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err, c.want)
			}
			if strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "ghp_token") {
				t.Errorf("error echoes credentials: %v", err)
			}
		})
	}
	// Boundary values that must be accepted.
	ok := []struct {
		name string
		base func() GitOpsCreateRequest
		mut  mut
	}{
		{"53-char name", argoReq, func(r *GitOpsCreateRequest) { r.Name = strings.Repeat("a", 53) }},
		{"30s interval", fluxKsReq, func(r *GitOpsCreateRequest) { r.Interval = "30s" }},
		{"1h interval", fluxKsReq, func(r *GitOpsCreateRequest) { r.Interval = "1h" }},
		{"semver range", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Source.ChartVersion = ">=1.0.0 <2.0.0" }},
		{"argo star version", argoHelmReq, func(r *GitOpsCreateRequest) { r.Source.ChartVersion = "*" }},
		{"argo oci chart", argoHelmReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "oci://ghcr.io/o/charts" }},
		{"argo git scheme", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "git://git.example.com/o/r.git" }},
		{"argo ssh with user", argoReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "ssh://git@github.com/o/r.git" }},
		{"flux http repo", fluxKsReq, func(r *GitOpsCreateRequest) { r.Source.RepoURL = "http://git.local/o/r.git" }},
		{"empty values", fluxHelmReq, func(r *GitOpsCreateRequest) { r.Values = "  \n" }},
		{"flux selfHeal implied", fluxKsReq, func(r *GitOpsCreateRequest) { r.SyncPolicy.Automated, r.SyncPolicy.SelfHeal = true, true }},
	}
	for _, c := range ok {
		r := c.base()
		c.mut(&r)
		if _, err := PreviewGitOps(r); err != nil {
			t.Errorf("%s: rejected: %v", c.name, err)
		}
	}
}

func aliasBomb() string {
	var b strings.Builder
	b.WriteString("a: &a [\"x\",\"x\",\"x\",\"x\",\"x\",\"x\",\"x\",\"x\",\"x\"]\n")
	prev := "a"
	for _, n := range []string{"b", "c", "d", "e", "f", "g", "h", "i"} {
		b.WriteString(n + ": &" + n + " [*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + "]\n")
		prev = n
	}
	return b.String()
}

func deepYAML(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(strings.Repeat(" ", i) + "k:\n")
	}
	b.WriteString(strings.Repeat(" ", n) + "k: v\n")
	return b.String()
}

func TestDemoCreateGitOpsApp(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := fluxHelmReq()
	req.Name = "demo-create-roundtrip"
	res, err := d.CreateGitOpsApp(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || len(res.Created) != 2 || res.Created[0].Kind != "HelmRepository" || res.Created[1].Kind != "HelmRelease" {
		t.Fatalf("result: %+v", res)
	}
	items, _ := d.ListGitOps(ctx)
	found := false
	for _, it := range items {
		if it.Name == req.Name && it.Namespace == "flux-system" {
			found = it.Kind == "HelmRelease" && it.Status == "Ready" && it.Source == "HelmRepository/"+req.Name
		}
	}
	if !found {
		t.Fatalf("created app not listed correctly: %+v", items)
	}
	if _, err := d.CreateGitOpsApp(ctx, req); HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate: %v (%d)", err, HTTPStatus(err))
	}
	a := argoReq()
	a.Name = "demo-create-argo"
	if _, err := d.CreateGitOpsApp(ctx, a); err != nil {
		t.Fatal(err)
	}
	items, _ = d.ListGitOps(ctx)
	for _, it := range items {
		if it.Name == a.Name && (it.Kind != "Application" || it.Status != "Synced" || it.Source != a.Source.RepoURL+":deploy/overlays/prod") {
			t.Errorf("argo item: %+v", it)
		}
	}
	// Invalid input is a 400 and creates nothing.
	bad := argoReq()
	bad.Name = "demo-bad"
	bad.Source.RepoURL = "https://u:p@h/o/r"
	if _, err := d.CreateGitOpsApp(ctx, bad); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("bad: %v", err)
	}
}
