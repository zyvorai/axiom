package platform

import (
	"context"
	"fmt"
	"strings"
)

// demoHelmRevisions builds a realistic history for a demo release: the last
// revision is live, earlier ones are superseded with older chart/app versions.
func demoHelmRevisions(r HelmRelease) map[int]HelmRevisionData {
	out := map[int]HelmRevisionData{}
	chartName := r.Chart
	if i := strings.LastIndex(r.Chart, "-"); i > 0 {
		chartName = r.Chart[:i]
	}
	hooks := 0
	if r.Name == "cert-manager" {
		hooks = 1
	}
	for rev := 1; rev <= r.Revision; rev++ {
		status, desc := "superseded", "Upgrade complete"
		if rev == 1 {
			desc = "Install complete"
		}
		if rev == r.Revision {
			status = "deployed"
		}
		ver := fmt.Sprintf("1.%d.0", rev)
		replicas := 2
		if rev%2 == 0 {
			replicas = 3
		}
		manifest := fmt.Sprintf("kind: Deployment\nmetadata:\n  name: %s\nspec:\n  replicas: %d\n  image: registry.example.com/%s:%s\n---\nkind: Service\nmetadata:\n  name: %s\n", r.Name, replicas, chartName, ver, r.Name)
		if r.Name == "prometheus" {
			manifest += "---\nkind: PersistentVolumeClaim\nmetadata:\n  name: prometheus-data\n"
		}
		values := redactHelmValues(map[string]any{"replicaCount": replicas, "image": map[string]any{"tag": ver}, "auth": map[string]any{"adminPassword": "x"}})
		out[rev] = HelmRevisionData{
			HelmRevision: HelmRevision{
				Revision: rev, Status: status, Chart: chartName + "-" + ver, ChartVersion: ver, AppVersion: "v" + ver,
				Updated: fmt.Sprintf("2026-%02d-%02dT09:30:00Z", 1+rev%9, 10+rev%9), Description: desc,
			},
			Values: values, Manifest: manifest, Hooks: hooks,
		}
	}
	return out
}

func (d *DemoProvider) demoRevisions(ns, name string) (map[int]HelmRevisionData, error) {
	demoHelmMu.Lock()
	r, ok := demoHelmReleases[ns+"/"+name]
	demoHelmMu.Unlock()
	if !ok {
		return nil, NewStatusError(404, fmt.Errorf("release %s/%s not found", ns, name))
	}
	return demoHelmRevisions(r), nil
}

func (d *DemoProvider) ListHelmRevisions(ctx context.Context, ns, name string) ([]HelmRevision, error) {
	revs, err := d.demoRevisions(ns, name)
	if err != nil {
		return nil, err
	}
	return helmRevisionList(revs), nil
}

func (d *DemoProvider) DiffHelmRevisions(ctx context.Context, ns, name string, from, to int) (HelmRevisionDiff, error) {
	revs, err := d.demoRevisions(ns, name)
	if err != nil {
		return HelmRevisionDiff{}, err
	}
	return diffHelmRevisions(ns, name, revs, from, to)
}

func (d *DemoProvider) PreviewHelmRollback(ctx context.Context, ns, name string, revision int) (HelmRollbackPreview, error) {
	revs, err := d.demoRevisions(ns, name)
	if err != nil {
		return HelmRollbackPreview{}, err
	}
	return previewHelmRollback(ns, name, revs, revision)
}

func (d *DemoProvider) RollbackHelm(ctx context.Context, req HelmRollbackRequest) (HelmActionResult, error) {
	p, err := d.PreviewHelmRollback(ctx, req.Namespace, req.Name, req.Revision)
	if err != nil {
		return HelmActionResult{}, err
	}
	if err := checkHelmRollbackConfirm(p, req.Confirm); err != nil {
		return HelmActionResult{}, err
	}
	return d.HelmRollback(ctx, req.Namespace, req.Name, p.Target)
}
