package platform

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"reflect"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"
)

// GitOps application editing: reading the typed fields back out of a live
// object, and turning an edited typed request into a precise patch.
//
// This file is pure (no cluster access) and compiles in both build modes.
// The design mirrors creation (gitops_build.go): the request is the same
// typed body, it is validated by buildGitOps, and only the fields the typed
// model manages are ever written. Everything else on the object — labels,
// annotations, finalizers, status, unknown spec keys — is left exactly as it
// is, because the patch is a JSON Patch (RFC 6902) naming only the managed
// paths, guarded by a "test" of metadata.resourceVersion.

// GitOpsEditable is the response of GET /api/v1/gitops/{kind}/{ns}/{name}/edit.
type GitOpsEditable struct {
	Kind        string            `json:"kind"`
	Engine      string            `json:"engine"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Source      GitOpsSource      `json:"source"`
	Destination GitOpsDestination `json:"destination"`
	Project     string            `json:"project,omitempty"`
	Interval    string            `json:"interval,omitempty"`
	Values      string            `json:"values,omitempty"`
	SyncPolicy  GitOpsSyncPolicy  `json:"syncPolicy"`
	// ResourceVersion must be echoed back in the edit body; a mismatch is a 409.
	ResourceVersion string `json:"resourceVersion"`
	// SourceObject is the Flux source the object references (nil for Argo CD).
	SourceObject          *GitOpsRef `json:"sourceObject"`
	SourceResourceVersion string     `json:"sourceResourceVersion,omitempty"`
	// Editable is false when the object uses something the typed model cannot
	// represent; Reasons says what. The UI then shows it read-only.
	Editable bool     `json:"editable"`
	Reasons  []string `json:"reasons"`
	// ReadOnly lists typed fields that cannot be changed on this object (the
	// Flux source fields when the source is not Axiom's or is shared).
	ReadOnly []string `json:"readOnly"`
}

// GitOpsEditRequest is the body of POST .../edit/preview and PUT .../{name}:
// the creation body without the fixed identity fields, plus the
// resourceVersion read from GET .../edit. kind/engine/name/namespace may be
// sent (the UI often echoes them) but must then match the object.
type GitOpsEditRequest struct {
	Kind        string            `json:"kind,omitempty"`
	Engine      string            `json:"engine,omitempty"`
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Source      GitOpsSource      `json:"source"`
	Destination GitOpsDestination `json:"destination"`
	Project     string            `json:"project,omitempty"`
	Interval    string            `json:"interval,omitempty"`
	Values      string            `json:"values,omitempty"`
	SyncPolicy  GitOpsSyncPolicy  `json:"syncPolicy"`

	ResourceVersion       string `json:"resourceVersion"`
	SourceResourceVersion string `json:"sourceResourceVersion,omitempty"`
}

// GitOpsEditChange is one managed field that the edit changes.
type GitOpsEditChange struct {
	Object string `json:"object"` // Kind/namespace/name
	Field  string `json:"field"`  // e.g. spec.source.targetRevision
	From   string `json:"from"`
	To     string `json:"to"`
}

// GitOpsEditPreview is the response of POST .../edit/preview.
type GitOpsEditPreview struct {
	Changes  []GitOpsEditChange `json:"changes"`
	YAML     string             `json:"yaml"`
	Warnings []string           `json:"warnings"`
	Noop     bool               `json:"noop"`
}

// GitOpsEditResult is the response of PUT /api/v1/gitops/{kind}/{ns}/{name}.
type GitOpsEditResult struct {
	OK              bool               `json:"ok"`
	Noop            bool               `json:"noop"`
	Changes         []GitOpsEditChange `json:"changes"`
	Warnings        []string           `json:"warnings"`
	ResourceVersion string             `json:"resourceVersion"`
}

// gitOpsEditTarget is the live state a provider hands to the pure logic.
type gitOpsEditTarget struct {
	Kind      string
	Obj       map[string]any
	SourceObj map[string]any // nil when absent or not applicable
	Peers     []gitOpsPeer   // Kustomizations and HelmReleases, for sharing
}

// gitOpsEditPlan is the computed edit.
type gitOpsEditPlan struct {
	Preview     GitOpsEditPreview
	Editable    GitOpsEditable
	AppPatch    []byte // JSON Patch; nil when the application is unchanged
	SourcePatch []byte // JSON Patch for the Flux source; nil when unchanged
	SourceRef   GitOpsRef
	AppAfter    map[string]any
	SourceAfter map[string]any
	Fields      []string // changed field names, for the audit entry
}

// Typed field names that may be read-only (Flux source fields).
const (
	gitOpsFieldRepoURL   = "source.repoUrl"
	gitOpsFieldRevision  = "source.revision"
	gitOpsFieldSecretRef = "source.secretRef"
)

// Spec keys whose presence makes an object non-editable: the typed model
// cannot represent them, so the form would show an incomplete picture.
var (
	argoSourceBlockKeys = []string{"kustomize", "directory", "plugin"}
	argoHelmBlockKeys   = []string{"valuesObject", "parameters", "fileParameters", "valueFiles"}
	fluxKsBlockKeys     = []string{"patches", "patchesStrategicMerge", "patchesJson6902", "postBuild", "components", "images", "namePrefix", "nameSuffix", "kubeConfig"}
	fluxHRBlockKeys     = []string{"chartRef", "valuesFrom", "postRenderers", "kubeConfig"}
)

func nonEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return t != ""
	case bool:
		return t
	case map[string]any:
		return len(t) > 0
	case []any:
		return len(t) > 0
	}
	return true
}

// readGitOpsEditable extracts the typed fields from the live object.
func readGitOpsEditable(t gitOpsEditTarget) GitOpsEditable {
	obj := t.Obj
	self := gitOpsRefOf(t.Kind, obj)
	ed := GitOpsEditable{
		Kind: t.Kind, Name: self.Name, Namespace: self.Namespace,
		ResourceVersion: nestedString(obj, "metadata", "resourceVersion"),
		Editable:        true, Reasons: []string{}, ReadOnly: []string{},
	}
	block := func(format string, a ...any) {
		ed.Editable = false
		ed.Reasons = append(ed.Reasons, fmt.Sprintf(format, a...))
	}
	spec, _ := nestedMap(obj, "spec")

	switch t.Kind {
	case "Application":
		ed.Engine = GitOpsEngineArgoCD
		if refs, ok := nestedValue(obj, "metadata", "ownerReferences").([]any); ok {
			for _, r := range refs {
				if m, ok := r.(map[string]any); ok && m["kind"] == "ApplicationSet" {
					block("owned by ApplicationSet %v: edit the ApplicationSet instead (changes here would be reverted)", m["name"])
				}
			}
		}
		if nonEmpty(spec["sources"]) {
			block("uses multiple sources (spec.sources)")
		}
		src, _ := nestedMap(obj, "spec", "source")
		if src == nil && !nonEmpty(spec["sources"]) {
			block("has no spec.source")
		}
		for _, k := range argoSourceBlockKeys {
			if nonEmpty(src[k]) {
				block("uses spec.source.%s", k)
			}
		}
		ed.Source.RepoURL, _ = src["repoURL"].(string)
		chart, _ := src["chart"].(string)
		rev, _ := src["targetRevision"].(string)
		helm, _ := src["helm"].(map[string]any)
		if chart != "" {
			ed.Source.Chart, ed.Source.ChartVersion = chart, rev
			if p, _ := src["path"].(string); p != "" {
				block("sets both spec.source.chart and spec.source.path")
			}
			for _, k := range argoHelmBlockKeys {
				if nonEmpty(helm[k]) {
					block("uses spec.source.helm.%s", k)
				}
			}
			ed.Values, _ = helm["values"].(string)
		} else {
			ed.Source.Path, _ = src["path"].(string)
			ed.Source.Revision = rev
			if nonEmpty(helm) {
				block("uses Helm parameters on a git path source (spec.source.helm)")
			}
		}
		ed.Destination.Server = nestedString(obj, "spec", "destination", "server")
		ed.Destination.Namespace = nestedString(obj, "spec", "destination", "namespace")
		if nestedString(obj, "spec", "destination", "name") != "" {
			block("selects its cluster by spec.destination.name")
		}
		ed.Project, _ = spec["project"].(string)
		if auto, ok := nestedMap(obj, "spec", "syncPolicy", "automated"); ok {
			ed.SyncPolicy.Automated = true
			ed.SyncPolicy.Prune, _ = auto["prune"].(bool)
			ed.SyncPolicy.SelfHeal, _ = auto["selfHeal"].(bool)
		}
		for _, o := range nestedStrings(obj, "spec", "syncPolicy", "syncOptions") {
			if o == "CreateNamespace=true" {
				ed.SyncPolicy.CreateNamespace = true
			}
		}

	case "Kustomization":
		ed.Engine = GitOpsEngineFluxKustomize
		for _, k := range fluxKsBlockKeys {
			if nonEmpty(spec[k]) {
				block("uses spec.%s", k)
			}
		}
		ed.Source.Path = fluxPathToTyped(nestedString(obj, "spec", "path"))
		ed.Interval, _ = spec["interval"].(string)
		ed.SyncPolicy.Prune, _ = spec["prune"].(bool)
		ed.Destination.Namespace, _ = spec["targetNamespace"].(string)
		readFluxSource(t, &ed, "GitRepository", block)

	case "HelmRelease":
		ed.Engine = GitOpsEngineFluxHelm
		for _, k := range fluxHRBlockKeys {
			if nonEmpty(spec[k]) {
				block("uses spec.%s", k)
			}
		}
		cs, _ := nestedMap(obj, "spec", "chart", "spec")
		if cs == nil && !nonEmpty(spec["chartRef"]) {
			block("has no spec.chart.spec")
		}
		if nonEmpty(cs["valuesFiles"]) {
			block("uses spec.chart.spec.valuesFiles")
		}
		ed.Source.Chart, _ = cs["chart"].(string)
		ed.Source.ChartVersion, _ = cs["version"].(string)
		ed.Interval, _ = spec["interval"].(string)
		ed.Destination.Namespace, _ = spec["targetNamespace"].(string)
		ed.SyncPolicy.CreateNamespace = nestedBool(obj, "spec", "install", "createNamespace")
		if v, ok := spec["values"].(map[string]any); ok && len(v) > 0 {
			b, err := yaml.Marshal(v)
			if err != nil {
				block("spec.values cannot be rendered as YAML")
			} else {
				ed.Values = string(b)
			}
		}
		readFluxSource(t, &ed, "HelmRepository", block)
	}
	return ed
}

// fluxPathToTyped turns "./deploy/prod" into "deploy/prod" and "./" into ".".
func fluxPathToTyped(p string) string {
	p = strings.TrimPrefix(p, "./")
	if p == "" {
		return "."
	}
	return p
}

// readFluxSource fills the source fields from the referenced GitRepository /
// HelmRepository and marks them read-only unless the source is Axiom's own
// and not shared.
func readFluxSource(t gitOpsEditTarget, ed *GitOpsEditable, wantKind string, block func(string, ...any)) {
	ref, ok := gitOpsSourceRef(t.Kind, t.Obj)
	if !ok {
		block("references no source")
		return
	}
	ed.SourceObject = &GitOpsRef{Kind: ref.Kind, Namespace: ref.Namespace, Name: ref.Name}
	if ref.Kind != wantKind {
		block("references a %s source; only %s sources can be edited here", ref.Kind, wantKind)
		return
	}
	fields := []string{gitOpsFieldRepoURL, gitOpsFieldSecretRef}
	if wantKind == "GitRepository" {
		fields = []string{gitOpsFieldRepoURL, gitOpsFieldRevision, gitOpsFieldSecretRef}
	}
	readOnly := func(format string, a ...any) {
		ed.Reasons = append(ed.Reasons, fmt.Sprintf(format, a...))
		for _, f := range fields {
			if !containsString(ed.ReadOnly, f) {
				ed.ReadOnly = append(ed.ReadOnly, f)
			}
		}
	}
	src := t.SourceObj
	if src == nil {
		readOnly("source %s was not found: its fields are read-only", ref)
		return
	}
	ed.SourceResourceVersion = nestedString(src, "metadata", "resourceVersion")
	ed.Source.RepoURL = nestedString(src, "spec", "url")
	ed.Source.SecretRef = nestedString(src, "spec", "secretRef", "name")
	if wantKind == "GitRepository" {
		r, _ := nestedMap(src, "spec", "ref")
		branch, _ := r["branch"].(string)
		tag, _ := r["tag"].(string)
		commit, _ := r["commit"].(string)
		set := 0
		for _, k := range []string{"branch", "tag", "commit", "semver", "name"} {
			if nonEmpty(r[k]) {
				set++
			}
		}
		switch {
		case nonEmpty(r["semver"]) || nonEmpty(r["name"]) || set > 1:
			readOnly("source %s uses a ref (semver, name or several keys) the typed model cannot represent: its fields are read-only", ref)
		case tag != "":
			ed.Source.Revision = "refs/tags/" + tag
		case commit != "":
			ed.Source.Revision = commit
		default:
			ed.Source.Revision = branch
		}
	}
	p := buildGitOpsDeletePreview(t.Kind, t.Obj, t.Peers, src)
	if p.Source != nil && len(p.Source.SharedWith) > 0 {
		names := make([]string, 0, len(p.Source.SharedWith))
		for _, r := range p.Source.SharedWith {
			names = append(names, r.String())
		}
		readOnly("source %s is shared with %s: its fields are read-only", ref, strings.Join(names, ", "))
	}
	if !gitOpsManagedByAxiom(src) {
		readOnly("source %s was not created by Axiom (no %s=%s label): its fields are read-only", ref, GitOpsManagedByLabel, GitOpsManagedByValue)
	}
}

func containsString(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

func gitOpsConflict() error {
	return NewStatusError(http.StatusConflict, fmt.Errorf("the application changed since you opened it: reload it and apply your edit again"))
}

// planGitOpsEdit validates req against the live target and computes the patch.
func planGitOpsEdit(t gitOpsEditTarget, req GitOpsEditRequest) (gitOpsEditPlan, error) {
	var plan gitOpsEditPlan
	ed := readGitOpsEditable(t)
	plan.Editable = ed
	if !ed.Editable {
		return plan, gitOpsBad("%s %s/%s cannot be edited here: %s", ed.Kind, ed.Namespace, ed.Name, strings.Join(ed.Reasons, "; "))
	}
	// Identity fields are fixed.
	for _, f := range []struct{ field, got, want string }{
		{"kind", req.Kind, ed.Kind}, {"engine", req.Engine, ed.Engine},
		{"name", req.Name, ed.Name}, {"namespace", req.Namespace, ed.Namespace},
	} {
		if f.got != "" && f.got != f.want {
			return plan, gitOpsBad("%s cannot be changed (it is %q); create a new application instead", f.field, f.want)
		}
	}
	if req.ResourceVersion == "" {
		return plan, gitOpsBad("resourceVersion is required: send back the resourceVersion from GET .../edit")
	}
	if req.ResourceVersion != ed.ResourceVersion {
		return plan, gitOpsConflict()
	}
	if req.SourceResourceVersion != "" && req.SourceResourceVersion != ed.SourceResourceVersion {
		return plan, gitOpsConflict()
	}
	if ed.Engine == GitOpsEngineArgoCD && (req.Source.Chart != "") != (ed.Source.Chart != "") {
		return plan, gitOpsBad("source.chart: switching an application between a git path and a Helm chart source is not supported; create a new application instead")
	}

	// Omitted fields that creation would default keep their current value.
	r := req
	if r.Project == "" {
		r.Project = ed.Project
	}
	if r.Destination.Server == "" {
		r.Destination.Server = ed.Destination.Server
	}
	if r.Interval == "" {
		r.Interval = ed.Interval
	}
	if ed.Source.Chart == "" && r.Source.Revision == "" {
		r.Source.Revision = ed.Source.Revision
	}
	// Read-only fields: omitted or unchanged means "keep".
	for _, f := range ed.ReadOnly {
		got, cur := typedSourceField(&r.Source, f), typedSourceField(&ed.Source, f)
		if *got == "" {
			*got = *cur
		} else if *got != *cur {
			return plan, gitOpsBad("%s cannot be changed on this application: %s", f, strings.Join(ed.Reasons, "; "))
		}
	}

	cr := GitOpsCreateRequest{
		Engine: ed.Engine, Name: ed.Name, Namespace: ed.Namespace,
		Source: r.Source, Destination: r.Destination, Project: r.Project,
		Interval: r.Interval, Values: r.Values, SyncPolicy: r.SyncPolicy,
	}
	if ed.Engine == GitOpsEngineArgoCD && cr.Destination.Server == gitOpsArgoServer {
		cr.Destination.Server = "" // the default needs no https-URL check
	}
	if checkDNSLabel("name", cr.Name, gitOpsMaxNameLen) != nil {
		cr.Name = "app" // objects created elsewhere may have longer / dotted names
	}
	built, err := buildGitOps(cr)
	if err != nil && len(ed.ReadOnly) > 0 {
		// A read-only source field that creation would not accept (or that is
		// missing) must not block editing the application's own fields: they
		// are never written, so validate with placeholders instead.
		alt := cr
		for _, f := range ed.ReadOnly {
			v := typedSourceField(&alt.Source, f)
			switch f {
			case gitOpsFieldRepoURL:
				*v = "https://placeholder.invalid/repository"
			default:
				*v = ""
			}
		}
		if b2, err2 := buildGitOps(alt); err2 == nil {
			built, err = b2, nil
		}
	}
	if err != nil {
		return plan, err
	}
	app := built.Objects[len(built.Objects)-1]

	e := &gitOpsEditor{}
	e.app = e.begin(t.Kind, t.Obj)
	switch t.Kind {
	case "Application":
		argoEditOps(e, app.Obj, ed, r)
	case "Kustomization":
		fluxKsEditOps(e, app.Obj)
	case "HelmRelease":
		fluxHREditOps(e, app.Obj)
	}
	if ed.SourceObject != nil && t.SourceObj != nil && len(ed.ReadOnly) == 0 {
		plan.SourceRef = *ed.SourceObject
		e.src = e.begin(ed.SourceObject.Kind, t.SourceObj)
		fluxSourceEditOps(e, built.Objects[0].Obj, ed, r)
	}

	plan.Preview.Warnings = append(built.warnings(), gitOpsEditWarnings(t.Obj)...)
	plan.Preview.Changes = []GitOpsEditChange{}
	var docs []string
	for _, o := range []*gitOpsEditObj{e.src, e.app} {
		if o == nil {
			continue
		}
		plan.Preview.Changes = append(plan.Preview.Changes, o.changes...)
		for _, c := range o.changes {
			plan.Fields = append(plan.Fields, c.Field)
		}
		docs = append(docs, o.yaml())
	}
	plan.Preview.YAML = strings.Join(docs, "---\n")
	plan.Preview.Noop = len(plan.Preview.Changes) == 0
	if e.app.changed() {
		plan.AppPatch, plan.AppAfter = e.app.patch(), e.app.cur
	}
	if e.src != nil && e.src.changed() {
		plan.SourcePatch, plan.SourceAfter = e.src.patch(), e.src.cur
	}
	return plan, nil
}

func typedSourceField(s *GitOpsSource, f string) *string {
	switch f {
	case gitOpsFieldRepoURL:
		return &s.RepoURL
	case gitOpsFieldRevision:
		return &s.Revision
	default:
		return &s.SecretRef
	}
}

// gitOpsEditWarnings flags objects whose parent reconciles them: an edit
// would be reverted by the next sync of the parent.
func gitOpsEditWarnings(obj map[string]any) []string {
	labels, _ := nestedMap(obj, "metadata", "labels")
	var out []string
	if v, _ := labels["argocd.argoproj.io/instance"].(string); v != "" {
		out = append(out, fmt.Sprintf("this object is managed by the Argo CD application %q: its next sync may revert this edit; change it in that application's repository", v))
	}
	if v, _ := labels["kustomize.toolkit.fluxcd.io/name"].(string); v != "" {
		out = append(out, fmt.Sprintf("this object is applied by the Flux Kustomization %q: its next reconciliation may revert this edit; change it in that repository", v))
	}
	return out
}

// --- managed-field operations ---

func argoEditOps(e *gitOpsEditor, rendered map[string]any, ed GitOpsEditable, r GitOpsEditRequest) {
	o := e.app
	rs, _ := rendered["spec"].(map[string]any)
	o.set(rs["project"], "spec", "project")
	rsrc, _ := rs["source"].(map[string]any)
	o.set(rsrc["repoURL"], "spec", "source", "repoURL")
	if ed.Source.Chart != "" {
		o.set(rsrc["chart"], "spec", "source", "chart")
		o.set(rsrc["targetRevision"], "spec", "source", "targetRevision")
		var want any
		if h, ok := rsrc["helm"].(map[string]any); ok {
			want = h["values"]
		}
		if !sameYAMLValues(nestedValue(o.cur, "spec", "source", "helm", "values"), want) {
			o.set(want, "spec", "source", "helm", "values")
		}
	} else {
		if live := nestedString(o.cur, "spec", "source", "path"); path.Clean(orDot(live)) != rsrc["path"] {
			o.set(rsrc["path"], "spec", "source", "path")
		}
		o.set(rsrc["targetRevision"], "spec", "source", "targetRevision")
	}
	rdst, _ := rs["destination"].(map[string]any)
	o.set(rdst["server"], "spec", "destination", "server")
	o.set(rdst["namespace"], "spec", "destination", "namespace")

	if r.SyncPolicy.Automated {
		if _, ok := nestedMap(o.cur, "spec", "syncPolicy", "automated"); ok {
			o.set(r.SyncPolicy.Prune, "spec", "syncPolicy", "automated", "prune")
			o.set(r.SyncPolicy.SelfHeal, "spec", "syncPolicy", "automated", "selfHeal")
		} else {
			o.set(map[string]any{"prune": r.SyncPolicy.Prune, "selfHeal": r.SyncPolicy.SelfHeal}, "spec", "syncPolicy", "automated")
		}
	} else {
		o.set(nil, "spec", "syncPolicy", "automated")
	}
	// syncOptions: only the CreateNamespace entry is managed; others are kept.
	live := nestedStrings(o.cur, "spec", "syncPolicy", "syncOptions")
	var opts []any
	for _, s := range live {
		if !strings.HasPrefix(s, "CreateNamespace=") {
			opts = append(opts, s)
		}
	}
	if r.SyncPolicy.CreateNamespace {
		opts = append(opts, "CreateNamespace=true")
	}
	if len(opts) != len(live) || (r.SyncPolicy.CreateNamespace != ed.SyncPolicy.CreateNamespace) {
		var want any
		if len(opts) > 0 {
			want = opts
		}
		o.set(want, "spec", "syncPolicy", "syncOptions")
	}
}

func orDot(p string) string {
	if p == "" {
		return "."
	}
	return p
}

func fluxKsEditOps(e *gitOpsEditor, rendered map[string]any) {
	o := e.app
	rs, _ := rendered["spec"].(map[string]any)
	if fluxPathToTyped(nestedString(o.cur, "spec", "path")) != fluxPathToTyped(rs["path"].(string)) {
		o.set(rs["path"], "spec", "path")
	}
	o.set(rs["interval"], "spec", "interval")
	o.set(rs["prune"], "spec", "prune")
	o.set(rs["targetNamespace"], "spec", "targetNamespace")
}

func fluxHREditOps(e *gitOpsEditor, rendered map[string]any) {
	o := e.app
	rs, _ := rendered["spec"].(map[string]any)
	cs, _ := nestedMap(rs, "chart", "spec")
	o.set(rs["interval"], "spec", "interval")
	o.set(cs["chart"], "spec", "chart", "spec", "chart")
	o.set(cs["version"], "spec", "chart", "spec", "version")
	o.set(rs["targetNamespace"], "spec", "targetNamespace")
	if nestedBool(rs, "install", "createNamespace") {
		o.set(true, "spec", "install", "createNamespace")
	} else {
		o.set(nil, "spec", "install", "createNamespace")
	}
	o.set(rs["values"], "spec", "values")
}

func fluxSourceEditOps(e *gitOpsEditor, rendered map[string]any, ed GitOpsEditable, r GitOpsEditRequest) {
	o := e.src
	rs, _ := rendered["spec"].(map[string]any)
	o.set(rs["url"], "spec", "url")
	if o.kind == "GitRepository" {
		ref, _ := rs["ref"].(map[string]any)
		for _, k := range []string{"branch", "tag", "commit"} {
			o.set(ref[k], "spec", "ref", k)
		}
	} else {
		want := rs["type"]
		if live := nestedString(o.cur, "spec", "type"); want == nil && live == "default" {
			want = live
		}
		o.set(want, "spec", "type")
	}
	if sr, ok := rs["secretRef"].(map[string]any); ok {
		o.set(sr["name"], "spec", "secretRef", "name")
	} else {
		o.set(nil, "spec", "secretRef")
	}
	// The source interval follows only an explicit interval change.
	if r.Interval != "" && r.Interval != ed.Interval {
		o.set(rs["interval"], "spec", "interval")
	}
}

// sameYAMLValues reports whether two Argo helm.values strings parse equal.
func sameYAMLValues(a, b any) bool {
	as, _ := a.(string)
	bs, _ := b.(string)
	if strings.TrimSpace(as) == "" || strings.TrimSpace(bs) == "" {
		return strings.TrimSpace(as) == strings.TrimSpace(bs)
	}
	var av, bv any
	if yaml.Unmarshal([]byte(as), &av) != nil || yaml.Unmarshal([]byte(bs), &bv) != nil {
		return as == bs
	}
	return reflect.DeepEqual(jsonNormalize(av), jsonNormalize(bv))
}

// --- editor: applies managed-field ops to a copy and records JSON Patch ops ---

type gitOpsEditor struct {
	app, src *gitOpsEditObj
}

type gitOpsEditObj struct {
	kind, label string
	rv          string
	cur         map[string]any
	ops         []map[string]any
	changes     []GitOpsEditChange
}

func (e *gitOpsEditor) begin(kind string, obj map[string]any) *gitOpsEditObj {
	cp, _ := jsonNormalize(obj).(map[string]any)
	return &gitOpsEditObj{
		kind:  kind,
		label: kind + "/" + nestedString(obj, "metadata", "namespace") + "/" + nestedString(obj, "metadata", "name"),
		rv:    nestedString(obj, "metadata", "resourceVersion"),
		cur:   cp,
	}
}

func (o *gitOpsEditObj) changed() bool { return len(o.ops) > 0 }

// set makes fields hold want (nil removes it). Values that are equal, or both
// "empty" (absent, "", false, {}), are left alone so the diff stays minimal.
func (o *gitOpsEditObj) set(want any, fields ...string) {
	want = jsonNormalize(want)
	live := nestedValue(o.cur, fields...)
	if !nonEmpty(live) && !nonEmpty(want) {
		return
	}
	if reflect.DeepEqual(live, want) {
		return
	}
	o.changes = append(o.changes, GitOpsEditChange{
		Object: o.label, Field: strings.Join(fields, "."), From: fmtEditVal(live), To: fmtEditVal(want),
	})
	if want == nil {
		o.ops = append(o.ops, map[string]any{"op": "remove", "path": jsonPointer(fields)})
		parent, _ := nestedMap(o.cur, fields[:len(fields)-1]...)
		delete(parent, fields[len(fields)-1])
		return
	}
	// Find the deepest existing ancestor map; add the rest as one subtree.
	m := o.cur
	i := 0
	for ; i < len(fields)-1; i++ {
		next, ok := m[fields[i]].(map[string]any)
		if !ok {
			break
		}
		m = next
	}
	val := want
	for j := len(fields) - 1; j > i; j-- {
		val = map[string]any{fields[j]: val}
	}
	o.ops = append(o.ops, map[string]any{"op": "add", "path": jsonPointer(fields[:i+1]), "value": val})
	m[fields[i]] = jsonNormalize(val)
}

// patch returns the JSON Patch: a resourceVersion test, then the ops.
func (o *gitOpsEditObj) patch() []byte {
	ops := append([]map[string]any{{"op": "test", "path": "/metadata/resourceVersion", "value": o.rv}}, o.ops...)
	b, _ := json.Marshal(ops)
	return b
}

// yaml renders the object after the change, without managedFields noise.
func (o *gitOpsEditObj) yaml() string {
	cp, _ := jsonNormalize(o.cur).(map[string]any)
	if meta, ok := cp["metadata"].(map[string]any); ok {
		delete(meta, "managedFields")
	}
	b, err := yaml.Marshal(cp)
	if err != nil {
		return ""
	}
	return string(b)
}

func jsonPointer(fields []string) string {
	var sb strings.Builder
	for _, f := range fields {
		sb.WriteByte('/')
		sb.WriteString(strings.ReplaceAll(strings.ReplaceAll(f, "~", "~0"), "/", "~1"))
	}
	return sb.String()
}

// jsonNormalize deep-copies v through JSON so ints/float64s and typed slices
// compare equal regardless of where they came from.
func jsonNormalize(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if json.Unmarshal(b, &out) != nil {
		return v
	}
	return out
}

func fmtEditVal(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case map[string]any:
		if len(t) == 0 {
			return "{}"
		}
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSpace(string(b))
}
