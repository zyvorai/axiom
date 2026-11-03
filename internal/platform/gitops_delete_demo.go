package platform

import (
	"context"
	"fmt"
)

// Demo GitOps deletion. demoGitOps (ops_demo.go) only holds list summaries, so
// the finalizers / spec.prune / sourceRef / labels the cascade rules need live
// in this side map, keyed "Kind/namespace/name" and guarded by demoGitOpsMu.
// Created applications record their rendered objects here (sources included).
var demoGitOpsObjects = map[string]map[string]any{
	// Seed specs carry the fields GET .../edit reads back (gitops_edit_demo.go).
	"Application/argocd/platform-apps": demoGitOpsObj("argocd", "platform-apps", nil,
		[]any{argoResourcesFinalizer}, map[string]any{"project": "default",
			"source":      map[string]any{"repoURL": "https://github.com/zyvorai/platform.git", "path": "apps", "targetRevision": "main"},
			"destination": map[string]any{"server": gitOpsArgoServer, "namespace": "platform"},
			"syncPolicy":  map[string]any{"automated": map[string]any{"prune": true, "selfHeal": true}}}),
	"Application/argocd/docs-site": demoGitOpsObj("argocd", "docs-site", nil, nil, map[string]any{"project": "default",
		"source":      map[string]any{"repoURL": "https://github.com/zyvorai/docs.git", "path": ".", "targetRevision": "HEAD"},
		"destination": map[string]any{"server": gitOpsArgoServer, "namespace": "docs"}}),
	"Kustomization/flux-system/apps": demoGitOpsObj("flux-system", "apps", nil, nil, map[string]any{
		"prune": true, "sourceRef": map[string]any{"kind": "GitRepository", "name": "flux-system"},
		"path": "./apps", "interval": "10m",
	}),
	"Kustomization/flux-system/infra": demoGitOpsObj("flux-system", "infra", nil, nil, map[string]any{
		"prune": false, "sourceRef": map[string]any{"kind": "GitRepository", "name": "flux-system"},
		"path": "./infrastructure", "interval": "10m",
	}),
	"GitRepository/flux-system/flux-system": demoGitOpsObj("flux-system", "flux-system", nil, nil, map[string]any{
		"url": "https://github.com/zyvorai/fleet.git", "interval": "1m", "ref": map[string]any{"branch": "main"},
	}),
	"HelmRelease/monitoring/prometheus": demoGitOpsObj("monitoring", "prometheus", nil, nil, map[string]any{
		"interval": "30m",
		"chart": map[string]any{"spec": map[string]any{"chart": "kube-prometheus-stack", "version": "65.x",
			"sourceRef": map[string]any{"kind": "HelmRepository", "name": "prometheus-community", "namespace": "flux-system"}}},
	}),
	"HelmRepository/flux-system/prometheus-community": demoGitOpsObj("flux-system", "prometheus-community", nil, nil, map[string]any{
		"url": "https://prometheus-community.github.io/helm-charts", "interval": "1h",
	}),
}

func demoGitOpsObj(ns, name string, labels map[string]any, finalizers []any, spec map[string]any) map[string]any {
	meta := map[string]any{"name": name, "namespace": ns}
	if labels != nil {
		meta["labels"] = labels
	}
	if finalizers != nil {
		meta["finalizers"] = finalizers
	}
	return map[string]any{"metadata": meta, "spec": spec}
}

func demoGitOpsKey(kind, ns, name string) string { return kind + "/" + ns + "/" + name }

// demoRecordGitOpsObjects stores what CreateGitOpsApp rendered. Caller holds demoGitOpsMu.
func demoRecordGitOpsObjects(objs []gitOpsObject) {
	for _, o := range objs {
		demoGitOpsObjects[demoGitOpsKey(o.Kind, o.Namespace, o.Name)] = o.Obj
	}
}

// demoPlanGitOpsDelete builds the preview. Caller holds demoGitOpsMu.
func demoPlanGitOpsDelete(kind, ns, name string) (GitOpsDeletePreview, bool, error) {
	if err := checkGitOpsDeleteTarget(kind, ns, name); err != nil {
		return GitOpsDeletePreview{}, false, err
	}
	item, ok := demoGitOps[ns+"/"+name]
	if !ok || item.Kind != kind {
		return GitOpsDeletePreview{}, false, notFoundf("%s %s/%s not found", kind, ns, name)
	}
	obj := demoGitOpsObjects[demoGitOpsKey(kind, ns, name)]
	if obj == nil {
		obj = demoGitOpsObj(ns, name, nil, nil, map[string]any{})
	}
	var peers []gitOpsPeer
	for _, it := range demoGitOps {
		if it.Kind != "Kustomization" && it.Kind != "HelmRelease" {
			continue
		}
		if o := demoGitOpsObjects[demoGitOpsKey(it.Kind, it.Namespace, it.Name)]; o != nil {
			peers = append(peers, gitOpsPeer{Kind: it.Kind, Obj: o})
		}
	}
	var sourceObj map[string]any
	if src, ok := gitOpsSourceRef(kind, obj); ok {
		sourceObj = demoGitOpsObjects[demoGitOpsKey(src.Kind, src.Namespace, src.Name)]
	}
	return buildGitOpsDeletePreview(kind, obj, peers, sourceObj), sourceObj != nil, nil
}

func (d *DemoProvider) PreviewGitOpsDelete(ctx context.Context, kind, ns, name string) (GitOpsDeletePreview, error) {
	demoGitOpsMu.Lock()
	defer demoGitOpsMu.Unlock()
	p, _, err := demoPlanGitOpsDelete(kind, ns, name)
	return p, err
}

func (d *DemoProvider) DeleteGitOpsApp(ctx context.Context, req GitOpsDeleteRequest) (GitOpsDeleteResult, error) {
	demoGitOpsMu.Lock()
	defer demoGitOpsMu.Unlock()
	p, sourceExists, err := demoPlanGitOpsDelete(req.Kind, req.Namespace, req.Name)
	if err != nil {
		return GitOpsDeleteResult{}, err
	}
	if err := checkGitOpsDeleteConfirm(p, req.Confirm); err != nil {
		return GitOpsDeleteResult{}, err
	}
	if req.RemoveSource {
		if err := checkGitOpsRemoveSource(p, sourceExists); err != nil {
			return GitOpsDeleteResult{}, err
		}
	}
	delete(demoGitOps, p.Namespace+"/"+p.Name)
	delete(demoGitOpsObjects, demoGitOpsKey(p.Kind, p.Namespace, p.Name))
	res := GitOpsDeleteResult{OK: true, Cascade: p.Cascade, Deleted: []GitOpsRef{{Kind: p.Kind, Namespace: p.Namespace, Name: p.Name}}}
	if req.RemoveSource && p.Source != nil {
		src := p.Source.ref()
		if !sourceExists {
			res.Warnings = append(res.Warnings, fmt.Sprintf("source %s was not found; nothing else to remove", src))
		} else {
			delete(demoGitOpsObjects, demoGitOpsKey(src.Kind, src.Namespace, src.Name))
			res.Deleted = append(res.Deleted, src)
		}
	}
	return res, nil
}
