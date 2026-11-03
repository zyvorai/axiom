package platform

import (
	"context"
	"strconv"
)

// Demo GitOps editing over the demoGitOpsObjects side map (gitops_delete_demo.go).
// Objects there carry no resourceVersion; one is assigned on first read and
// bumped on every update so the stale-edit 409 behaves as on a real cluster.

// demoGitOpsRV returns obj's resourceVersion, assigning "1" if it has none.
// Caller holds demoGitOpsMu.
func demoGitOpsRV(obj map[string]any) string {
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		obj["metadata"] = meta
	}
	rv, _ := meta["resourceVersion"].(string)
	if rv == "" {
		rv = "1"
		meta["resourceVersion"] = rv
	}
	return rv
}

func demoBumpRV(obj map[string]any) string {
	n, _ := strconv.Atoi(nestedString(obj, "metadata", "resourceVersion"))
	rv := strconv.Itoa(n + 1)
	if meta, ok := obj["metadata"].(map[string]any); ok {
		meta["resourceVersion"] = rv
	}
	return rv
}

// demoGitOpsEditTarget loads the live state. Caller holds demoGitOpsMu.
func demoGitOpsEditTarget(ref GitOpsRef) (gitOpsEditTarget, error) {
	if err := checkGitOpsDeleteTarget(ref.Kind, ref.Namespace, ref.Name); err != nil {
		return gitOpsEditTarget{}, err
	}
	item, ok := demoGitOps[ref.Namespace+"/"+ref.Name]
	if !ok || item.Kind != ref.Kind {
		return gitOpsEditTarget{}, notFoundf("%s %s/%s not found", ref.Kind, ref.Namespace, ref.Name)
	}
	key := demoGitOpsKey(ref.Kind, ref.Namespace, ref.Name)
	obj := demoGitOpsObjects[key]
	if obj == nil {
		obj = demoGitOpsObj(ref.Namespace, ref.Name, nil, nil, map[string]any{})
		demoGitOpsObjects[key] = obj
	}
	demoGitOpsRV(obj)
	t := gitOpsEditTarget{Kind: ref.Kind, Obj: obj}
	if src, ok := gitOpsSourceRef(ref.Kind, obj); ok {
		if s := demoGitOpsObjects[demoGitOpsKey(src.Kind, src.Namespace, src.Name)]; s != nil {
			demoGitOpsRV(s)
			t.SourceObj = s
		}
		for _, it := range demoGitOps {
			if it.Kind != "Kustomization" && it.Kind != "HelmRelease" {
				continue
			}
			if o := demoGitOpsObjects[demoGitOpsKey(it.Kind, it.Namespace, it.Name)]; o != nil {
				t.Peers = append(t.Peers, gitOpsPeer{Kind: it.Kind, Obj: o})
			}
		}
	}
	return t, nil
}

func (d *DemoProvider) GetGitOpsEditable(ctx context.Context, ref GitOpsRef) (GitOpsEditable, error) {
	demoGitOpsMu.Lock()
	defer demoGitOpsMu.Unlock()
	t, err := demoGitOpsEditTarget(ref)
	if err != nil {
		return GitOpsEditable{}, err
	}
	return readGitOpsEditable(t), nil
}

func (d *DemoProvider) PreviewGitOpsEdit(ctx context.Context, ref GitOpsRef, req GitOpsEditRequest) (GitOpsEditPreview, error) {
	demoGitOpsMu.Lock()
	defer demoGitOpsMu.Unlock()
	t, err := demoGitOpsEditTarget(ref)
	if err != nil {
		return GitOpsEditPreview{}, err
	}
	plan, err := planGitOpsEdit(t, req)
	return plan.Preview, err
}

func (d *DemoProvider) UpdateGitOpsApp(ctx context.Context, ref GitOpsRef, req GitOpsEditRequest) (GitOpsEditResult, error) {
	demoGitOpsMu.Lock()
	defer demoGitOpsMu.Unlock()
	t, err := demoGitOpsEditTarget(ref)
	if err != nil {
		return GitOpsEditResult{}, err
	}
	plan, err := planGitOpsEdit(t, req)
	if err != nil {
		return GitOpsEditResult{}, err
	}
	res := GitOpsEditResult{OK: true, Noop: plan.Preview.Noop, Changes: plan.Preview.Changes, Warnings: plan.Preview.Warnings,
		ResourceVersion: plan.Editable.ResourceVersion}
	if plan.SourceAfter != nil {
		demoBumpRV(plan.SourceAfter)
		demoGitOpsObjects[demoGitOpsKey(plan.SourceRef.Kind, plan.SourceRef.Namespace, plan.SourceRef.Name)] = plan.SourceAfter
	}
	if plan.AppAfter != nil {
		res.ResourceVersion = demoBumpRV(plan.AppAfter)
		demoGitOpsObjects[demoGitOpsKey(ref.Kind, ref.Namespace, ref.Name)] = plan.AppAfter
		if ref.Kind == "Application" {
			key := ref.Namespace + "/" + ref.Name
			item := demoGitOps[key]
			item.Source = nestedString(plan.AppAfter, "spec", "source", "repoURL")
			demoGitOps[key] = item
		}
	}
	return res, nil
}
