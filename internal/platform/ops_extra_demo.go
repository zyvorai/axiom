package platform

import (
	"context"
	"fmt"
	"time"
)

func (d *DemoProvider) ListHelmHistory(ctx context.Context, ns, name string) ([]HelmRelease, error) {
	return []HelmRelease{
		{Name: name, Namespace: ns, Status: "deployed", Chart: "demo-1.0.0", Revision: 3, Age: "2d"},
		{Name: name, Namespace: ns, Status: "superseded", Chart: "demo-0.9.0", Revision: 2, Age: "8d"},
		{Name: name, Namespace: ns, Status: "superseded", Chart: "demo-0.8.0", Revision: 1, Age: "20d"},
	}, nil
}

func (d *DemoProvider) HelmUninstall(ctx context.Context, ns, name string) (HelmActionResult, error) {
	return HelmActionResult{OK: true, Action: "uninstall", Message: fmt.Sprintf("demo uninstall %s/%s", ns, name)}, nil
}

func (d *DemoProvider) HelmRollback(ctx context.Context, ns, name string, revision int) (HelmActionResult, error) {
	return HelmActionResult{OK: true, Action: "rollback", Message: fmt.Sprintf("demo rollback %s/%s to %d", ns, name, revision)}, nil
}

func (d *DemoProvider) InstallHelmChart(ctx context.Context, req InstallHelmRequest) (HelmActionResult, error) {
	if req.Namespace == "" || req.ReleaseName == "" || req.RepoURL == "" || req.ChartName == "" {
		return HelmActionResult{}, fmt.Errorf("namespace, releaseName, repoUrl, and chartName are required")
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

func (d *DemoProvider) ListVMSnapshots(ctx context.Context, ns string) ([]VMSnapshot, error) {
	all := []VMSnapshot{
		{Namespace: "default", Name: "edge-api-vm-snap", VM: "edge-api-vm", Ready: true, Status: "Succeeded", Age: "3d"},
	}
	out := make([]VMSnapshot, 0, len(all))
	for _, s := range all {
		if ns == "" || ns == "all" || s.Namespace == ns {
			out = append(out, s)
		}
	}
	return out, nil
}

func (d *DemoProvider) CreateVMSnapshot(ctx context.Context, req CreateVMSnapshotRequest) (VMSnapshot, error) {
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s-snap-%d", req.VM, time.Now().Unix()%1000)
	}
	return VMSnapshot{Namespace: req.Namespace, Name: name, VM: req.VM, Status: "InProgress", Age: "now"}, nil
}

func (d *DemoProvider) LiveMigrateVM(ctx context.Context, ns, name string) (HelmActionResult, error) {
	return HelmActionResult{OK: true, Action: "migrate", Message: fmt.Sprintf("demo migrate %s/%s", ns, name)}, nil
}
