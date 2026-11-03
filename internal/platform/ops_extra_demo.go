package platform

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

func (d *DemoProvider) ListHelmHistory(ctx context.Context, ns, name string) ([]HelmRelease, error) {
	demoHelmMu.Lock()
	r, ok := demoHelmReleases[ns+"/"+name]
	demoHelmMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("release %s/%s not found", ns, name)
	}
	history := []HelmRelease{r}
	for rev := r.Revision - 1; rev >= 1 && len(history) < 3; rev-- {
		history = append(history, HelmRelease{
			Name: name, Namespace: ns, Status: "superseded",
			Chart: fmt.Sprintf("demo-0.%d.0", rev), Revision: rev,
			Age: fmt.Sprintf("%dd", (r.Revision-rev)*7),
		})
	}
	return history, nil
}

func (d *DemoProvider) HelmUninstall(ctx context.Context, ns, name string) (HelmActionResult, error) {
	demoHelmMu.Lock()
	defer demoHelmMu.Unlock()
	key := ns + "/" + name
	if _, ok := demoHelmReleases[key]; !ok {
		return HelmActionResult{}, fmt.Errorf("release %s/%s not found", ns, name)
	}
	delete(demoHelmReleases, key)
	return HelmActionResult{OK: true, Action: "uninstall", Message: fmt.Sprintf("demo uninstall %s/%s", ns, name)}, nil
}

func (d *DemoProvider) HelmRollback(ctx context.Context, ns, name string, revision int) (HelmActionResult, error) {
	demoHelmMu.Lock()
	defer demoHelmMu.Unlock()
	key := ns + "/" + name
	r, ok := demoHelmReleases[key]
	if !ok {
		return HelmActionResult{}, fmt.Errorf("release %s/%s not found", ns, name)
	}
	if revision <= 0 { // like `helm rollback` with no revision: go to the previous one
		revision = r.Revision - 1
		if revision < 1 {
			return HelmActionResult{}, NewStatusError(http.StatusConflict, fmt.Errorf("release %s/%s has no previous revision", ns, name))
		}
	}
	r.Revision = revision
	r.Status = "deployed"
	demoHelmReleases[key] = r
	return HelmActionResult{OK: true, Action: "rollback", Message: fmt.Sprintf("demo rollback %s/%s to %d", ns, name, revision)}, nil
}

func (d *DemoProvider) InstallHelmChart(ctx context.Context, req InstallHelmRequest) (HelmActionResult, error) {
	if req.OCIRef != "" {
		if err := ValidateHelmOCIInstall(req); err != nil {
			return HelmActionResult{}, err
		}
		chart := req.OCIRef[strings.LastIndexByte(req.OCIRef, '/')+1:]
		req.RepoURL, req.ChartName = strings.TrimSuffix(req.OCIRef, "/"+chart), chart
	}
	if req.Namespace == "" || req.ReleaseName == "" || req.RepoURL == "" || req.ChartName == "" {
		return HelmActionResult{}, fmt.Errorf("namespace, releaseName, repoUrl, and chartName are required")
	}
	demoHelmMu.Lock()
	defer demoHelmMu.Unlock()
	// A real `helm install` rejects a release name already in use in the
	// namespace ("cannot re-use a name that is still in use") — mirror
	// that instead of silently overwriting the existing release's state.
	key := req.Namespace + "/" + req.ReleaseName
	if _, exists := demoHelmReleases[key]; exists {
		return HelmActionResult{}, fmt.Errorf("release %s/%s already exists", req.Namespace, req.ReleaseName)
	}
	demoHelmReleases[key] = HelmRelease{
		Name: req.ReleaseName, Namespace: req.Namespace, Status: "deployed", Chart: req.ChartName, Revision: 1, Age: "now",
	}
	return HelmActionResult{OK: true, Action: "install", Message: fmt.Sprintf("demo install %s/%s from %s (%s)", req.Namespace, req.ReleaseName, req.RepoURL, req.ChartName)}, nil
}

func (d *DemoProvider) ListExternalSecrets(ctx context.Context, ns string) ([]ExternalSecretSummary, error) {
	all := []ExternalSecretSummary{
		{Namespace: "external-secrets", Name: "atlas-demo-gateway-auth", Store: "vault-lab", Refresh: "1h", Status: "SecretSyncedError", Ready: false, Age: "11d"},
		{Namespace: "default", Name: "app-credentials", Store: "vault-lab", Refresh: "15m", Status: "SecretSynced", Ready: true, Age: "4d"},
	}
	out := make([]ExternalSecretSummary, 0, len(all))
	for _, e := range all {
		if ns == "" || ns == "all" || e.Namespace == ns {
			out = append(out, e)
		}
	}
	return out, nil
}

var (
	demoVMSnapshotsMu sync.Mutex
	demoVMSnapshots   = map[string]VMSnapshot{
		"default/edge-api-vm-snap": {Namespace: "default", Name: "edge-api-vm-snap", VM: "edge-api-vm", Ready: true, Status: "Succeeded", Age: "3d"},
	}
)

func (d *DemoProvider) ListVMSnapshots(ctx context.Context, ns string) ([]VMSnapshot, error) {
	demoVMSnapshotsMu.Lock()
	defer demoVMSnapshotsMu.Unlock()
	out := make([]VMSnapshot, 0, len(demoVMSnapshots))
	for _, s := range demoVMSnapshots {
		if ns == "" || ns == "all" || s.Namespace == ns {
			out = append(out, s)
		}
	}
	return out, nil
}

func (d *DemoProvider) CreateVMSnapshot(ctx context.Context, req CreateVMSnapshotRequest) (VMSnapshot, error) {
	if req.Namespace == "" || req.VM == "" {
		return VMSnapshot{}, fmt.Errorf("namespace and vm are required")
	}
	if !d.vmExists(req.Namespace, req.VM) {
		return VMSnapshot{}, fmt.Errorf("vm %s/%s not found", req.Namespace, req.VM)
	}
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s-snap-%d", req.VM, time.Now().Unix()%1000)
	}
	snap := VMSnapshot{Namespace: req.Namespace, Name: name, VM: req.VM, Ready: true, Status: "Succeeded", Age: "now"}
	demoVMSnapshotsMu.Lock()
	defer demoVMSnapshotsMu.Unlock()
	key := req.Namespace + "/" + name
	if _, exists := demoVMSnapshots[key]; exists {
		return VMSnapshot{}, fmt.Errorf("vmsnapshot %s/%s already exists", req.Namespace, name)
	}
	demoVMSnapshots[key] = snap
	return snap, nil
}

func (d *DemoProvider) LiveMigrateVM(ctx context.Context, ns, name string) (HelmActionResult, error) {
	if !d.vmExists(ns, name) {
		return HelmActionResult{}, fmt.Errorf("vm %s/%s not found", ns, name)
	}
	return HelmActionResult{OK: true, Action: "migrate", Message: fmt.Sprintf("demo migrate %s/%s", ns, name)}, nil
}
