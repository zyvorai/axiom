package platform

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// GitOps application deletion: cascade rules, consequence text and the typed
// confirmation check. Pure (no cluster access), compiles in both build modes;
// both providers feed it the object as a plain map (unstructured content) and
// recompute it at delete time, so a stale UI preview can never downgrade a
// cascading delete into an unconfirmed one.

// Label set on every object Axiom creates (gitops_build.go).
const (
	GitOpsManagedByLabel = "app.kubernetes.io/managed-by"
	GitOpsManagedByValue = "zyvor-axiom"
)

// argoResourcesFinalizer makes Argo CD delete every resource an Application
// manages before the Application itself goes away. Variants such as
// "resources-finalizer.argocd.argoproj.io/background" behave the same way.
const argoResourcesFinalizer = "resources-finalizer.argocd.argoproj.io"

// GitOpsRef identifies one GitOps object in delete previews and results.
type GitOpsRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func (r GitOpsRef) String() string { return r.Kind + " " + r.Namespace + "/" + r.Name }

// GitOpsDeleteSource is the Flux source an object references.
type GitOpsDeleteSource struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// SharedWith lists the OTHER Flux objects referencing the same source. A
	// source that is still in use must not be offered for removal.
	SharedWith []GitOpsRef `json:"sharedWith"`
	// ManagedByAxiom reports whether the source carries the managed-by label;
	// removeSource refuses sources Axiom did not create.
	ManagedByAxiom bool `json:"managedByAxiom"`
}

func (s GitOpsDeleteSource) ref() GitOpsRef {
	return GitOpsRef{Kind: s.Kind, Namespace: s.Namespace, Name: s.Name}
}

// GitOpsDeletePreview is the response of GET .../delete-preview.
type GitOpsDeletePreview struct {
	Kind           string              `json:"kind"`
	Namespace      string              `json:"namespace"`
	Name           string              `json:"name"`
	Engine         string              `json:"engine"` // argocd | flux
	Cascade        bool                `json:"cascade"`
	Consequence    string              `json:"consequence"`
	Detail         []string            `json:"detail"`
	Source         *GitOpsDeleteSource `json:"source"`
	ManagedByAxiom bool                `json:"managedByAxiom"`
}

// GitOpsDeleteRequest is DELETE /api/v1/gitops/{kind}/{namespace}/{name}.
type GitOpsDeleteRequest struct {
	Kind         string `json:"kind"`
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	RemoveSource bool   `json:"removeSource"`
	Confirm      string `json:"confirm"`
}

// GitOpsDeleteResult is the response of a successful delete.
type GitOpsDeleteResult struct {
	OK       bool        `json:"ok"`
	Deleted  []GitOpsRef `json:"deleted"`
	Cascade  bool        `json:"cascade"`
	Warnings []string    `json:"warnings,omitempty"`
}

var gitOpsObjectName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)

// checkGitOpsDeleteTarget validates the path values. Names of objects created
// outside Axiom may be DNS-1123 subdomains, so dots are allowed here.
func checkGitOpsDeleteTarget(kind, ns, name string) error {
	switch kind {
	case "Application", "Kustomization", "HelmRelease":
	default:
		return gitOpsBad("kind must be one of Application, Kustomization, HelmRelease")
	}
	if ns == "" || len(ns) > 63 || !gitOpsDNSLabel.MatchString(ns) {
		return gitOpsBad("invalid namespace")
	}
	if name == "" || len(name) > 253 || !gitOpsObjectName.MatchString(name) {
		return gitOpsBad("invalid name")
	}
	return nil
}

func gitOpsEngineOfKind(kind string) string {
	if kind == "Application" {
		return "argocd"
	}
	return "flux"
}

// gitOpsCascade applies the cascade rules to the object's metadata/spec:
//   - Argo CD Application: cascades iff it carries the resources finalizer
//     (or a resources-finalizer.argocd.argoproj.io/* variant);
//   - Flux Kustomization: cascades iff spec.prune is true (the controller
//     garbage-collects its inventory);
//   - Flux HelmRelease: cascades (the release is uninstalled) unless
//     spec.suspend is true.
func gitOpsCascade(kind string, obj map[string]any) (cascade bool, consequence string, detail []string) {
	detail = []string{}
	switch kind {
	case "Application":
		for _, f := range nestedStrings(obj, "metadata", "finalizers") {
			if f == argoResourcesFinalizer || strings.HasPrefix(f, argoResourcesFinalizer+"/") {
				detail = append(detail, "finalizer: "+f)
			}
		}
		if len(detail) > 0 {
			f := strings.TrimPrefix(detail[0], "finalizer: ")
			return true, fmt.Sprintf("Deleting this application will also delete the Deployments, Services and every other resource it manages (finalizer %s).", f), detail
		}
		return false, "Only the Application object is removed; the workloads it deployed keep running but are no longer managed.", detail
	case "Kustomization":
		prune := nestedBool(obj, "spec", "prune")
		suspend := nestedBool(obj, "spec", "suspend")
		detail = append(detail, fmt.Sprintf("spec.prune: %t", prune))
		if suspend {
			detail = append(detail, "spec.suspend: true")
		}
		if prune {
			return true, "Deleting this Kustomization will also delete every resource it applied (the Deployments, Services … in its inventory), because spec.prune is true.", detail
		}
		return false, "Only the Kustomization object is removed; the resources it applied keep running but are no longer reconciled.", detail
	case "HelmRelease":
		suspend := nestedBool(obj, "spec", "suspend")
		detail = append(detail, fmt.Sprintf("spec.suspend: %t", suspend))
		if !suspend {
			return true, "Deleting this HelmRelease will also uninstall its Helm release, deleting the Deployments, Services and every other resource the chart installed.", detail
		}
		return false, "Only the HelmRelease object is removed: it is suspended, so Flux does not uninstall the Helm release; its workloads keep running but are no longer managed.", detail
	}
	return false, "", detail
}

// gitOpsSourceRef returns the Flux source an object references (spec.sourceRef
// for a Kustomization, spec.chart.spec.sourceRef for a HelmRelease), with the
// namespace defaulting to the object's own.
func gitOpsSourceRef(kind string, obj map[string]any) (GitOpsRef, bool) {
	var sr map[string]any
	switch kind {
	case "Kustomization":
		sr, _ = nestedMap(obj, "spec", "sourceRef")
	case "HelmRelease":
		sr, _ = nestedMap(obj, "spec", "chart", "spec", "sourceRef")
	default:
		return GitOpsRef{}, false
	}
	if sr == nil {
		return GitOpsRef{}, false
	}
	ref := GitOpsRef{}
	ref.Kind, _ = sr["kind"].(string)
	ref.Name, _ = sr["name"].(string)
	ref.Namespace, _ = sr["namespace"].(string)
	if ref.Namespace == "" {
		ref.Namespace = nestedString(obj, "metadata", "namespace")
	}
	if ref.Kind == "" || ref.Name == "" {
		return GitOpsRef{}, false
	}
	return ref, true
}

// gitOpsPeer is another Flux object in the cluster, used to detect a shared source.
type gitOpsPeer struct {
	Kind string
	Obj  map[string]any
}

func gitOpsRefOf(kind string, obj map[string]any) GitOpsRef {
	return GitOpsRef{Kind: kind, Namespace: nestedString(obj, "metadata", "namespace"), Name: nestedString(obj, "metadata", "name")}
}

func gitOpsManagedByAxiom(obj map[string]any) bool {
	labels, _ := nestedMap(obj, "metadata", "labels")
	v, _ := labels[GitOpsManagedByLabel].(string)
	return v == GitOpsManagedByValue
}

// buildGitOpsDeletePreview assembles the preview for obj. peers are the
// Kustomizations and HelmReleases in the cluster (obj itself may be among
// them); sourceObj is the referenced source object, nil when absent.
func buildGitOpsDeletePreview(kind string, obj map[string]any, peers []gitOpsPeer, sourceObj map[string]any) GitOpsDeletePreview {
	self := gitOpsRefOf(kind, obj)
	p := GitOpsDeletePreview{
		Kind: kind, Namespace: self.Namespace, Name: self.Name,
		Engine:         gitOpsEngineOfKind(kind),
		ManagedByAxiom: gitOpsManagedByAxiom(obj),
	}
	p.Cascade, p.Consequence, p.Detail = gitOpsCascade(kind, obj)
	if src, ok := gitOpsSourceRef(kind, obj); ok {
		s := &GitOpsDeleteSource{Kind: src.Kind, Namespace: src.Namespace, Name: src.Name, SharedWith: []GitOpsRef{}}
		for _, peer := range peers {
			pr := gitOpsRefOf(peer.Kind, peer.Obj)
			if pr == self {
				continue
			}
			if ps, ok := gitOpsSourceRef(peer.Kind, peer.Obj); ok && ps == src {
				s.SharedWith = append(s.SharedWith, pr)
			}
		}
		s.ManagedByAxiom = sourceObj != nil && gitOpsManagedByAxiom(sourceObj)
		p.Source = s
	}
	return p
}

// checkGitOpsDeleteConfirm enforces the server-side typed confirmation: a
// cascading delete requires confirm to equal the object's name exactly
// (case-sensitive). Non-cascading deletes need no confirmation.
func checkGitOpsDeleteConfirm(p GitOpsDeletePreview, confirm string) error {
	if p.Cascade && confirm != p.Name {
		return gitOpsBad("this deletion also removes the application's resources; pass confirm=%s", p.Name)
	}
	return nil
}

// checkGitOpsRemoveSource decides whether removeSource may delete the source:
// Flux only, never a source shared with another object, never one Axiom did
// not create, and only kinds Axiom knows how to delete. A supported source
// that no longer exists is allowed (the caller reports a warning). Every
// refusal happens before anything is deleted.
func checkGitOpsRemoveSource(p GitOpsDeletePreview, sourceExists bool) error {
	if p.Source == nil {
		if p.Engine == "argocd" {
			return gitOpsBad("removeSource is only supported for Flux objects")
		}
		return gitOpsBad("removeSource: %s %s/%s references no source", p.Kind, p.Namespace, p.Name)
	}
	s := p.Source
	if len(s.SharedWith) > 0 {
		names := make([]string, 0, len(s.SharedWith))
		for _, r := range s.SharedWith {
			names = append(names, r.String())
		}
		return NewStatusError(http.StatusConflict, fmt.Errorf("source %s %s/%s is still used by %s; it was not removed and nothing was deleted",
			s.Kind, s.Namespace, s.Name, strings.Join(names, ", ")))
	}
	if s.Kind != "GitRepository" && s.Kind != "HelmRepository" {
		return NewStatusError(http.StatusConflict, fmt.Errorf("source kind %s cannot be removed from here; nothing was deleted", s.Kind))
	}
	if !sourceExists {
		return nil
	}
	if !s.ManagedByAxiom {
		return NewStatusError(http.StatusConflict, fmt.Errorf("source %s %s/%s was not created by Axiom (no %s=%s label); it is never removed from here and nothing was deleted",
			s.Kind, s.Namespace, s.Name, GitOpsManagedByLabel, GitOpsManagedByValue))
	}
	return nil
}

// --- small nested accessors over unstructured-style maps ---

func nestedMap(obj map[string]any, fields ...string) (map[string]any, bool) {
	cur := obj
	for _, f := range fields {
		next, ok := cur[f].(map[string]any)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func nestedValue(obj map[string]any, fields ...string) any {
	if len(fields) == 0 {
		return nil
	}
	m, ok := nestedMap(obj, fields[:len(fields)-1]...)
	if !ok {
		return nil
	}
	return m[fields[len(fields)-1]]
}

func nestedString(obj map[string]any, fields ...string) string {
	s, _ := nestedValue(obj, fields...).(string)
	return s
}

func nestedBool(obj map[string]any, fields ...string) bool {
	b, _ := nestedValue(obj, fields...).(bool)
	return b
}

func nestedStrings(obj map[string]any, fields ...string) []string {
	switch v := nestedValue(obj, fields...).(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
