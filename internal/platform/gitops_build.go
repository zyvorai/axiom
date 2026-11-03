package platform

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

// GitOps application creation: typed request, validation and rendering.
//
// This file is pure (no cluster access) and compiles in both build modes: the
// preview endpoint calls PreviewGitOps directly, and both providers call
// buildGitOps before creating anything. Every object is assembled field by
// field from the typed request; the client can never hand us raw YAML, so it
// cannot smuggle in arbitrary specs, finalizers, annotations or extra objects.

// GitOps engines accepted by CreateGitOpsApp / PreviewGitOps.
const (
	GitOpsEngineArgoCD        = "argocd"
	GitOpsEngineFluxKustomize = "flux-kustomize"
	GitOpsEngineFluxHelm      = "flux-helm"
)

const (
	gitOpsMaxNameLen    = 53
	gitOpsMaxURLLen     = 512
	gitOpsMaxPathLen    = 256
	gitOpsMaxValuesLen  = 64 << 10
	gitOpsMaxValuesJSON = 256 << 10 // parsed size cap: defeats alias/anchor expansion
	gitOpsMaxValuesDeep = 32
	gitOpsMinInterval   = 30 * time.Second
	gitOpsArgoServer    = "https://kubernetes.default.svc"
)

// GitOpsCreateRequest is the body of POST /api/v1/gitops and /api/v1/gitops/preview.
type GitOpsCreateRequest struct {
	Engine      string            `json:"engine"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Source      GitOpsSource      `json:"source"`
	Destination GitOpsDestination `json:"destination"`
	Project     string            `json:"project,omitempty"`
	Interval    string            `json:"interval,omitempty"`
	Values      string            `json:"values,omitempty"`
	SyncPolicy  GitOpsSyncPolicy  `json:"syncPolicy"`
}

// GitOpsSource is where the application's manifests or chart come from.
type GitOpsSource struct {
	RepoURL      string `json:"repoUrl"`
	Path         string `json:"path,omitempty"`
	Revision     string `json:"revision,omitempty"`
	Chart        string `json:"chart,omitempty"`
	ChartVersion string `json:"chartVersion,omitempty"`
	// SecretRef names an existing Secret in the CR namespace (Flux only). Only
	// the name is ever accepted — never credentials.
	SecretRef string `json:"secretRef,omitempty"`
}

// GitOpsDestination is where the application's workloads are deployed.
type GitOpsDestination struct {
	Namespace string `json:"namespace,omitempty"`
	Server    string `json:"server,omitempty"`
}

// GitOpsSyncPolicy are the sync toggles. For Flux, automated and selfHeal are
// implied (Flux always reconciles and corrects drift).
type GitOpsSyncPolicy struct {
	Automated       bool `json:"automated,omitempty"`
	Prune           bool `json:"prune,omitempty"`
	SelfHeal        bool `json:"selfHeal,omitempty"`
	CreateNamespace bool `json:"createNamespace,omitempty"`
}

// GitOpsObjectRef identifies one rendered or created object.
type GitOpsObjectRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
}

// GitOpsPreview is the response of POST /api/v1/gitops/preview.
type GitOpsPreview struct {
	Objects  []GitOpsObjectRef `json:"objects"`
	YAML     string            `json:"yaml"`
	Warnings []string          `json:"warnings"`
}

// GitOpsCreateResult is the response of POST /api/v1/gitops.
type GitOpsCreateResult struct {
	OK       bool              `json:"ok"`
	Created  []GitOpsObjectRef `json:"created"`
	Warnings []string          `json:"warnings,omitempty"`
}

// gitOpsObject is one object to create, in order. Group/Resource identify the
// CRD; Version is the newest known version, which the Kubernetes provider
// replaces with the one the API server actually serves.
type gitOpsObject struct {
	Group, Version, Resource, Kind string
	Namespace, Name                string
	Obj                            map[string]any
}

func (o gitOpsObject) ref() GitOpsObjectRef {
	return GitOpsObjectRef{APIVersion: o.Group + "/" + o.Version, Kind: o.Kind, Namespace: o.Namespace, Name: o.Name}
}

// withVersion returns the object document with apiVersion set for version.
func (o gitOpsObject) withVersion(version string) map[string]any {
	doc := map[string]any{}
	for k, v := range o.Obj {
		doc[k] = v
	}
	doc["apiVersion"] = o.Group + "/" + version
	return doc
}

type gitOpsPlan struct {
	Engine   string
	Objects  []gitOpsObject
	Warnings []string
}

var (
	gitOpsDNSLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	gitOpsRefChars = regexp.MustCompile(`^[A-Za-z0-9._/+~^*<>=!, -]{1,128}$`)
	gitOpsInterval = regexp.MustCompile(`^[0-9]{1,6}(s|m|h)$`)
	gitOpsCommit   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	gitOpsSCPLike  = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:[A-Za-z0-9._~/-]+$`)
)

func gitOpsBad(format string, a ...any) error {
	return NewStatusError(http.StatusBadRequest, fmt.Errorf(format, a...))
}

func checkDNSLabel(field, v string, max int) error {
	if v == "" {
		return gitOpsBad("%s is required", field)
	}
	if len(v) > max || !gitOpsDNSLabel.MatchString(v) {
		return gitOpsBad("%s must be a DNS-1123 label (lowercase letters, digits and '-', at most %d characters)", field, max)
	}
	return nil
}

func checkRefChars(field, v string) error {
	if !gitOpsRefChars.MatchString(v) {
		return gitOpsBad("%s contains unsupported characters or is longer than 128 characters", field)
	}
	return nil
}

func hasSpaceOrControl(s string) bool {
	for _, r := range s {
		if r <= 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return true
		}
	}
	return false
}

// checkRepoURL validates a repository URL. Error messages never echo the URL:
// it may contain credentials, which must not reach responses or logs.
func checkRepoURL(raw string, helm, flux bool) error {
	if raw == "" {
		return gitOpsBad("source.repoUrl is required")
	}
	if len(raw) > gitOpsMaxURLLen {
		return gitOpsBad("source.repoUrl must be at most %d characters", gitOpsMaxURLLen)
	}
	if hasSpaceOrControl(raw) {
		return gitOpsBad("source.repoUrl must not contain whitespace or control characters")
	}
	if !strings.Contains(raw, "://") {
		if gitOpsSCPLike.MatchString(raw) && !helm {
			if flux {
				return gitOpsBad("source.repoUrl: Flux does not accept scp-like git URLs; use the ssh://git@host/org/repo.git form")
			}
			return nil
		}
		return gitOpsBad("invalid source.repoUrl: expected a URL such as https://host/org/repo.git")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return gitOpsBad("invalid source.repoUrl: not a valid URL")
	}
	scheme := strings.ToLower(u.Scheme)
	var allowed []string
	switch {
	case helm:
		allowed = []string{"https", "http", "oci"}
	case flux:
		allowed = []string{"https", "http", "ssh"}
	default:
		allowed = []string{"https", "http", "ssh", "git"}
	}
	ok := false
	for _, s := range allowed {
		ok = ok || scheme == s
	}
	if !ok {
		return gitOpsBad("source.repoUrl scheme must be one of %s for this source", strings.Join(allowed, ", "))
	}
	if u.User != nil {
		_, hasPass := u.User.Password()
		// A bare username is normal for ssh (git@host). On http(s)/oci a
		// username is usually a token, so any userinfo is refused there.
		if hasPass || (scheme != "ssh" && scheme != "git") {
			return gitOpsBad("source.repoUrl must not contain credentials: credentials are never written into a GitOps resource; " +
				"use source.secretRef (Flux) or configure repository credentials in Argo CD")
		}
	}
	if u.Hostname() == "" {
		return gitOpsBad("source.repoUrl must include a host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return gitOpsBad("source.repoUrl must not contain a query or fragment")
	}
	return nil
}

// cleanGitOpsPath validates and normalizes a repository path ("" means ".").
func cleanGitOpsPath(p string) (string, error) {
	if p == "" {
		return ".", nil
	}
	if len(p) > gitOpsMaxPathLen {
		return "", gitOpsBad("source.path must be at most %d characters", gitOpsMaxPathLen)
	}
	if hasSpaceOrControl(p) || strings.Contains(p, "\\") {
		return "", gitOpsBad("source.path must not contain whitespace, control characters or backslashes")
	}
	if strings.HasPrefix(p, "/") {
		return "", gitOpsBad("source.path must be relative to the repository root (no leading '/')")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", gitOpsBad("source.path must not contain '..'")
		}
	}
	return path.Clean(p), nil
}

// parseGitOpsValues checks the Helm values are a YAML mapping of bounded size
// and depth, and returns them as a JSON-compatible map.
func parseGitOpsValues(v string) (map[string]any, error) {
	if len(v) > gitOpsMaxValuesLen {
		return nil, gitOpsBad("values must be at most %d KiB", gitOpsMaxValuesLen>>10)
	}
	// sigs.k8s.io/yaml (yaml.v2 underneath) refuses excessive alias expansion;
	// the JSON size cap below bounds what remains.
	js, err := yaml.YAMLToJSON([]byte(v))
	if err != nil {
		return nil, gitOpsBad("values must be valid YAML: %v", err)
	}
	if len(js) > gitOpsMaxValuesJSON {
		return nil, gitOpsBad("values expand to more than %d KiB (anchors/aliases?)", gitOpsMaxValuesJSON>>10)
	}
	var out any
	if err := json.Unmarshal(js, &out); err != nil {
		return nil, gitOpsBad("values must be valid YAML")
	}
	m, ok := out.(map[string]any)
	if !ok {
		return nil, gitOpsBad("values must be a YAML mapping (key: value)")
	}
	if jsonDepth(m) > gitOpsMaxValuesDeep {
		return nil, gitOpsBad("values are nested deeper than %d levels", gitOpsMaxValuesDeep)
	}
	return m, nil
}

func jsonDepth(v any) int {
	switch t := v.(type) {
	case map[string]any:
		d := 0
		for _, c := range t {
			d = max(d, jsonDepth(c))
		}
		return d + 1
	case []any:
		d := 0
		for _, c := range t {
			d = max(d, jsonDepth(c))
		}
		return d + 1
	}
	return 0
}

// buildGitOps validates req and renders the objects to create, in order.
func buildGitOps(req GitOpsCreateRequest) (gitOpsPlan, error) {
	var plan gitOpsPlan
	engine := req.Engine
	argo := engine == GitOpsEngineArgoCD
	flux := engine == GitOpsEngineFluxKustomize || engine == GitOpsEngineFluxHelm
	helmSource := engine == GitOpsEngineFluxHelm || (argo && req.Source.Chart != "")
	if !argo && !flux {
		return plan, gitOpsBad("engine must be one of argocd, flux-kustomize, flux-helm")
	}
	plan.Engine = engine
	defaultNS := "argocd"
	if flux {
		defaultNS = "flux-system"
	}
	ns := req.Namespace
	if ns == "" {
		ns = defaultNS
	}
	src, dst, sp := req.Source, req.Destination, req.SyncPolicy

	if err := checkDNSLabel("name", req.Name, gitOpsMaxNameLen); err != nil {
		return plan, err
	}
	if err := checkDNSLabel("namespace", ns, 63); err != nil {
		return plan, err
	}
	if dst.Namespace != "" {
		if err := checkDNSLabel("destination.namespace", dst.Namespace, 63); err != nil {
			return plan, err
		}
	}
	if err := checkRepoURL(src.RepoURL, helmSource, flux); err != nil {
		return plan, err
	}

	// Engine/field combinations.
	if argo {
		if src.SecretRef != "" {
			return plan, gitOpsBad("source.secretRef is not supported for argocd: Argo CD repository credentials are configured in Argo CD, not here")
		}
		if req.Interval != "" {
			return plan, gitOpsBad("interval is not supported for argocd (Flux only)")
		}
		if (sp.Prune || sp.SelfHeal) && !sp.Automated {
			return plan, gitOpsBad("syncPolicy.prune and syncPolicy.selfHeal require syncPolicy.automated for argocd")
		}
	} else {
		if req.Project != "" {
			return plan, gitOpsBad("project is not supported for Flux (argocd only)")
		}
		if dst.Server != "" {
			return plan, gitOpsBad("destination.server is not supported for Flux (argocd only)")
		}
		if src.SecretRef != "" {
			if err := checkDNSLabel("source.secretRef", src.SecretRef, 63); err != nil {
				return plan, err
			}
		}
	}
	if helmSource {
		if engine == GitOpsEngineFluxHelm && src.Chart == "" {
			return plan, gitOpsBad("source.chart is required for flux-helm")
		}
		if src.Path != "" {
			return plan, gitOpsBad("source.path must not be set for a Helm chart source (set either path or chart)")
		}
		if src.Revision != "" {
			return plan, gitOpsBad("source.revision must not be set for a Helm chart source; use source.chartVersion")
		}
		if err := checkRefChars("source.chart", src.Chart); err != nil {
			return plan, err
		}
		if argo && src.ChartVersion == "" {
			return plan, gitOpsBad("source.chartVersion is required for an Argo CD Helm chart source (\"*\" tracks the latest)")
		}
		if src.ChartVersion != "" {
			if err := checkRefChars("source.chartVersion", src.ChartVersion); err != nil {
				return plan, err
			}
		}
	} else {
		if src.Chart != "" {
			return plan, gitOpsBad("source.chart is not supported for %s", engine)
		}
		if src.ChartVersion != "" {
			return plan, gitOpsBad("source.chartVersion requires source.chart")
		}
		if req.Values != "" {
			return plan, gitOpsBad("values are only supported for Helm chart sources")
		}
		if src.Revision != "" {
			if err := checkRefChars("source.revision", src.Revision); err != nil {
				return plan, err
			}
		}
	}
	cleanPath, err := cleanGitOpsPath(src.Path)
	if err != nil {
		return plan, err
	}
	var values map[string]any
	if strings.TrimSpace(req.Values) != "" {
		if values, err = parseGitOpsValues(req.Values); err != nil {
			return plan, err
		}
	}
	if sp.CreateNamespace && dst.Namespace == "" {
		return plan, gitOpsBad("syncPolicy.createNamespace requires destination.namespace")
	}
	if engine == GitOpsEngineFluxKustomize && sp.CreateNamespace {
		return plan, gitOpsBad("syncPolicy.createNamespace is not supported for flux-kustomize: include a Namespace manifest in the repository path")
	}
	if engine == GitOpsEngineFluxHelm && sp.Prune {
		return plan, gitOpsBad("syncPolicy.prune is not supported for flux-helm (Helm removes what a release no longer renders)")
	}
	interval := "5m"
	if flux && req.Interval != "" {
		if !gitOpsInterval.MatchString(req.Interval) {
			return plan, gitOpsBad("interval must look like 30s, 5m or 1h")
		}
		d, perr := time.ParseDuration(req.Interval)
		if perr != nil || d < gitOpsMinInterval {
			return plan, gitOpsBad("interval must be at least 30s")
		}
		interval = req.Interval
	}

	meta := func() map[string]any { return map[string]any{"name": req.Name, "namespace": ns} }
	automated := sp.Automated || flux
	revision := src.Revision

	switch engine {
	case GitOpsEngineArgoCD:
		server := dst.Server
		if server == "" {
			server = gitOpsArgoServer
		} else if err := checkArgoServer(server); err != nil {
			return plan, err
		}
		project := req.Project
		if project == "" {
			project = "default"
		}
		if err := checkDNSLabel("project", project, 63); err != nil {
			return plan, err
		}
		source := map[string]any{"repoURL": src.RepoURL}
		if helmSource {
			source["chart"] = src.Chart
			source["targetRevision"] = src.ChartVersion
			if values != nil {
				source["helm"] = map[string]any{"values": req.Values}
			}
		} else {
			if revision == "" {
				revision = "HEAD"
			}
			source["path"] = cleanPath
			source["targetRevision"] = revision
		}
		destination := map[string]any{"server": server}
		if dst.Namespace != "" {
			destination["namespace"] = dst.Namespace
		}
		syncPolicy := map[string]any{}
		if sp.Automated {
			syncPolicy["automated"] = map[string]any{"prune": sp.Prune, "selfHeal": sp.SelfHeal}
		}
		if sp.CreateNamespace {
			syncPolicy["syncOptions"] = []any{"CreateNamespace=true"}
		}
		spec := map[string]any{"project": project, "source": source, "destination": destination}
		if len(syncPolicy) > 0 {
			spec["syncPolicy"] = syncPolicy
		}
		plan.Objects = append(plan.Objects, gitOpsObject{
			Group: "argoproj.io", Version: "v1alpha1", Resource: "applications", Kind: "Application",
			Namespace: ns, Name: req.Name,
			Obj: map[string]any{"kind": "Application", "metadata": meta(), "spec": spec},
		})

	case GitOpsEngineFluxKustomize:
		if revision == "" {
			revision = "main"
		}
		ref := map[string]any{}
		switch {
		case strings.HasPrefix(revision, "refs/tags/") && len(revision) > len("refs/tags/"):
			ref["tag"] = strings.TrimPrefix(revision, "refs/tags/")
		case gitOpsCommit.MatchString(revision):
			ref["commit"] = revision
		default:
			ref["branch"] = revision
		}
		repoSpec := map[string]any{"url": src.RepoURL, "interval": interval, "ref": ref}
		if src.SecretRef != "" {
			repoSpec["secretRef"] = map[string]any{"name": src.SecretRef}
		}
		ksPath := "./" + cleanPath
		if cleanPath == "." {
			ksPath = "./"
		}
		ksSpec := map[string]any{
			"interval":  interval,
			"path":      ksPath,
			"prune":     sp.Prune,
			"sourceRef": map[string]any{"kind": "GitRepository", "name": req.Name},
			"wait":      false,
			"timeout":   "5m",
		}
		if dst.Namespace != "" {
			ksSpec["targetNamespace"] = dst.Namespace
		}
		plan.Objects = append(plan.Objects,
			gitOpsObject{
				Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories", Kind: "GitRepository",
				Namespace: ns, Name: req.Name,
				Obj: map[string]any{"kind": "GitRepository", "metadata": meta(), "spec": repoSpec},
			},
			gitOpsObject{
				Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations", Kind: "Kustomization",
				Namespace: ns, Name: req.Name,
				Obj: map[string]any{"kind": "Kustomization", "metadata": meta(), "spec": ksSpec},
			})

	case GitOpsEngineFluxHelm:
		repoSpec := map[string]any{"url": src.RepoURL, "interval": interval}
		if strings.HasPrefix(strings.ToLower(src.RepoURL), "oci://") {
			repoSpec["type"] = "oci"
		}
		if src.SecretRef != "" {
			repoSpec["secretRef"] = map[string]any{"name": src.SecretRef}
		}
		chartSpec := map[string]any{
			"chart":     src.Chart,
			"sourceRef": map[string]any{"kind": "HelmRepository", "name": req.Name},
		}
		if src.ChartVersion != "" {
			chartSpec["version"] = src.ChartVersion
		}
		hrSpec := map[string]any{
			"interval": interval,
			"chart":    map[string]any{"spec": chartSpec},
		}
		if dst.Namespace != "" {
			hrSpec["targetNamespace"] = dst.Namespace
		}
		if sp.CreateNamespace {
			hrSpec["install"] = map[string]any{"createNamespace": true}
		}
		if values != nil {
			hrSpec["values"] = values
		}
		plan.Objects = append(plan.Objects,
			gitOpsObject{
				Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories", Kind: "HelmRepository",
				Namespace: ns, Name: req.Name,
				Obj: map[string]any{"kind": "HelmRepository", "metadata": meta(), "spec": repoSpec},
			},
			gitOpsObject{
				Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases", Kind: "HelmRelease",
				Namespace: ns, Name: req.Name,
				Obj: map[string]any{"kind": "HelmRelease", "metadata": meta(), "spec": hrSpec},
			})
	}

	// Warnings (never echo the repo URL).
	if sp.Prune && automated && !helmSource && cleanPath == "." {
		plan.Warnings = append(plan.Warnings, "automated prune is on for the whole repository (path \".\"): anything removed from the repository is deleted from the cluster")
	}
	if automated && !helmSource && (revision == "HEAD" || revision == "main") {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("revision %q tracks a moving branch: every commit is deployed automatically", revision))
	}
	if strings.HasPrefix(strings.ToLower(src.RepoURL), "http://") {
		plan.Warnings = append(plan.Warnings, "the repository URL uses plain http: content can be tampered with in transit")
	}
	if ns != defaultNS {
		if argo {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("namespace %q is not %q: Argo CD only reconciles Applications outside its own namespace when apps-in-any-namespace is enabled for it", ns, defaultNS))
		} else {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("namespace %q is not %q: make sure the Flux controllers watch it", ns, defaultNS))
		}
	}
	return plan, nil
}

func checkArgoServer(s string) error {
	if len(s) > gitOpsMaxURLLen || hasSpaceOrControl(s) {
		return gitOpsBad("destination.server must be an https URL")
	}
	u, err := url.Parse(s)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
		return gitOpsBad("destination.server must be empty or an https URL")
	}
	if u.User != nil {
		return gitOpsBad("destination.server must not contain credentials")
	}
	return nil
}

func (p gitOpsPlan) refs() []GitOpsObjectRef {
	out := make([]GitOpsObjectRef, 0, len(p.Objects))
	for _, o := range p.Objects {
		out = append(out, o.ref())
	}
	return out
}

func (p gitOpsPlan) warnings() []string {
	if p.Warnings == nil {
		return []string{}
	}
	return p.Warnings
}

// PreviewGitOps validates req and renders what CreateGitOpsApp would create,
// using the newest API version of each kind. It never talks to a cluster.
func PreviewGitOps(req GitOpsCreateRequest) (GitOpsPreview, error) {
	plan, err := buildGitOps(req)
	if err != nil {
		return GitOpsPreview{}, err
	}
	docs := make([]string, 0, len(plan.Objects))
	for _, o := range plan.Objects {
		b, err := yaml.Marshal(o.withVersion(o.Version))
		if err != nil {
			return GitOpsPreview{}, err
		}
		docs = append(docs, string(b))
	}
	return GitOpsPreview{Objects: plan.refs(), YAML: strings.Join(docs, "---\n"), Warnings: plan.warnings()}, nil
}
