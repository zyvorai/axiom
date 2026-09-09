package platform

import (
	"context"
	"testing"
)

// --- UpdateConfigMap ---

func TestUpdateConfigMapUpdatesDataAndReflectsInGet(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if _, err := d.CreateConfigMap(ctx, CreateConfigMapRequest{Namespace: "wtest", Name: "cm-update", Data: map[string]string{"A": "1"}}); err != nil {
		t.Fatalf("CreateConfigMap: %v", err)
	}

	updated, err := d.UpdateConfigMap(ctx, CreateConfigMapRequest{
		Namespace: "wtest", Name: "cm-update",
		Data: map[string]string{"A": "2", "B": "3"}, Labels: map[string]string{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("UpdateConfigMap: %v", err)
	}
	if updated.Data["A"] != "2" || updated.Data["B"] != "3" || updated.Labels["env"] != "prod" {
		t.Fatalf("unexpected update result: %+v", updated)
	}

	got, err := d.GetConfigMap(ctx, "wtest", "cm-update")
	if err != nil {
		t.Fatalf("GetConfigMap: %v", err)
	}
	if got.Data["A"] != "2" || got.Data["B"] != "3" {
		t.Fatalf("update not reflected in a subsequent Get: %+v", got)
	}
}

func TestUpdateConfigMapMissingReturnsError(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.UpdateConfigMap(context.Background(), CreateConfigMapRequest{Namespace: "wtest", Name: "does-not-exist", Data: map[string]string{"A": "1"}})
	if err == nil {
		t.Fatal("expected UpdateConfigMap on a missing configmap to fail")
	}
}

func TestUpdateConfigMapRequiresNamespaceAndName(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.UpdateConfigMap(context.Background(), CreateConfigMapRequest{})
	if err == nil {
		t.Fatal("expected UpdateConfigMap with empty namespace/name to fail")
	}
}

// --- UpdateSecret ---

func TestUpdateSecretUpdatesDataAndReflectsInGet(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if _, err := d.CreateSecret(ctx, CreateSecretRequest{Namespace: "wtest", Name: "sec-update", Data: map[string]string{"A": "one"}}); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	updated, err := d.UpdateSecret(ctx, CreateSecretRequest{
		Namespace: "wtest", Name: "sec-update",
		Data: map[string]string{"B": "two"}, Type: "kubernetes.io/tls",
	})
	if err != nil {
		t.Fatalf("UpdateSecret: %v", err)
	}
	if updated.Values != nil {
		t.Fatalf("expected UpdateSecret's return value to scrub Values, got %+v", updated.Values)
	}
	if updated.Type != "kubernetes.io/tls" {
		t.Fatalf("expected updated type, got %q", updated.Type)
	}

	got, err := d.GetSecret(ctx, "wtest", "sec-update", true)
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Values["B"] != "two" {
		t.Fatalf("update not reflected in a subsequent Get: %+v", got.Values)
	}
	if _, stillThere := got.Values["A"]; stillThere {
		t.Fatalf("expected UpdateSecret to replace (not merge) data; old key 'A' survived: %+v", got.Values)
	}
}

func TestUpdateSecretFallsBackToKeysWhenDataEmpty(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if _, err := d.CreateSecret(ctx, CreateSecretRequest{Namespace: "wtest", Name: "sec-keys-fallback", Data: map[string]string{"A": "one"}}); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	if _, err := d.UpdateSecret(ctx, CreateSecretRequest{Namespace: "wtest", Name: "sec-keys-fallback", Keys: map[string]string{"C": "three"}}); err != nil {
		t.Fatalf("UpdateSecret via Keys: %v", err)
	}

	got, err := d.GetSecret(ctx, "wtest", "sec-keys-fallback", true)
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Values["C"] != "three" {
		t.Fatalf("expected the Keys alias to populate data when Data is empty, got %+v", got.Values)
	}
}

func TestUpdateSecretMissingReturnsError(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.UpdateSecret(context.Background(), CreateSecretRequest{Namespace: "wtest", Name: "does-not-exist", Data: map[string]string{"A": "1"}})
	if err == nil {
		t.Fatal("expected UpdateSecret on a missing secret to fail")
	}
}

func TestUpdateSecretRequiresNamespaceAndName(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.UpdateSecret(context.Background(), CreateSecretRequest{})
	if err == nil {
		t.Fatal("expected UpdateSecret with empty namespace/name to fail")
	}
}

// --- CreateService / DeleteService ---

func TestCreateServiceAppliesDefaultsAndReturnsValue(t *testing.T) {
	d := NewDemoProvider()
	svc, err := d.CreateService(context.Background(), CreateServiceRequest{Namespace: "wtest", Name: "svc-1", Selector: map[string]string{"app": "svc-1"}})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if svc.Type != "ClusterIP" {
		t.Fatalf("expected default type ClusterIP, got %q", svc.Type)
	}
	if len(svc.Ports) != 1 || svc.Ports[0].Port != 80 || svc.Ports[0].TargetPort != "80" {
		t.Fatalf("expected default port 80->80, got %+v", svc.Ports)
	}
}

func TestCreateServiceRequiresNamespaceAndName(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.CreateService(context.Background(), CreateServiceRequest{})
	if err == nil {
		t.Fatal("expected CreateService with empty namespace/name to fail")
	}
}

// Regression test for a real bug: ListServices (demo.go) used to synthesize
// its output purely from d.workloads/d.pods and never read the
// demoServicesExtra map that CreateService/DeleteService write to
// (writes_demo.go) — a demo-mode CreateService succeeded and was tracked
// (provable via the duplicate-name rejection below) but never appeared in a
// subsequent ListServices call. The same gap affected CreateIngress/
// ListIngresses and CreateResourceQuota/ListResourceQuotas; only
// CreateVeleroBackup's extra map was already merged into its List method.
// Now ListServices/ListIngresses/ListResourceQuotas all merge their extra
// maps the same way ListVeleroBackups does — see the "ReflectedInList"
// tests below for each.
func TestCreateServiceRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateServiceRequest{Namespace: "wtest", Name: "svc-dup", Selector: map[string]string{"app": "svc-dup"}}
	if _, err := d.CreateService(ctx, req); err != nil {
		t.Fatalf("first CreateService should succeed, got %v", err)
	}
	if _, err := d.CreateService(ctx, req); err == nil {
		t.Fatal("expected second CreateService with the same name to fail")
	}
}

func TestCreateServiceReflectedInListServices(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if _, err := d.CreateService(ctx, CreateServiceRequest{Namespace: "wtest", Name: "svc-listed", Selector: map[string]string{"app": "svc-listed"}}); err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	svcs, err := d.ListServices(ctx, "wtest")
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	found := false
	for _, s := range svcs {
		if s.Name == "svc-listed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created service to appear in ListServices, got %+v", svcs)
	}
}

func TestDeleteServiceThenSecondDeleteFails(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateServiceRequest{Namespace: "wtest", Name: "svc-del", Selector: map[string]string{"app": "svc-del"}}
	if _, err := d.CreateService(ctx, req); err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if err := d.DeleteService(ctx, "wtest", "svc-del"); err != nil {
		t.Fatalf("first DeleteService should succeed, got %v", err)
	}
	if err := d.DeleteService(ctx, "wtest", "svc-del"); err == nil {
		t.Fatal("expected second DeleteService to fail with not-found")
	}
}

// --- CreateVolume / DeleteVolume ---

func TestCreateVolumeReflectedInListVolumes(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	vol, err := d.CreateVolume(ctx, CreateVolumeRequest{Namespace: "wtest", Name: "pvc-1", Size: "10Gi"})
	if err != nil {
		t.Fatalf("CreateVolume: %v", err)
	}
	if vol.AccessModes != "ReadWriteOnce" || vol.Status != "Bound" {
		t.Fatalf("unexpected defaults: %+v", vol)
	}

	vols, err := d.ListVolumes(ctx, "wtest")
	if err != nil {
		t.Fatalf("ListVolumes: %v", err)
	}
	found := false
	for _, v := range vols {
		if v.Name == "pvc-1" && v.Capacity == "10Gi" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created volume to appear in ListVolumes, got %+v", vols)
	}
}

func TestCreateVolumeRequiresFields(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.CreateVolume(context.Background(), CreateVolumeRequest{Namespace: "wtest", Name: "pvc-missing-size"})
	if err == nil {
		t.Fatal("expected CreateVolume without a size to fail")
	}
}

func TestCreateVolumeRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateVolumeRequest{Namespace: "wtest", Name: "pvc-dup", Size: "5Gi"}
	if _, err := d.CreateVolume(ctx, req); err != nil {
		t.Fatalf("first CreateVolume should succeed, got %v", err)
	}
	if _, err := d.CreateVolume(ctx, req); err == nil {
		t.Fatal("expected second CreateVolume with the same name to fail")
	}
}

func TestDeleteVolumeRemovesItAndSecondDeleteFails(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateVolumeRequest{Namespace: "wtest", Name: "pvc-del", Size: "5Gi"}
	if _, err := d.CreateVolume(ctx, req); err != nil {
		t.Fatalf("CreateVolume: %v", err)
	}
	if err := d.DeleteVolume(ctx, "wtest", "pvc-del"); err != nil {
		t.Fatalf("first DeleteVolume should succeed, got %v", err)
	}
	vols, _ := d.ListVolumes(ctx, "wtest")
	for _, v := range vols {
		if v.Name == "pvc-del" {
			t.Fatalf("expected deleted volume to be gone from ListVolumes, still present: %+v", v)
		}
	}
	if err := d.DeleteVolume(ctx, "wtest", "pvc-del"); err == nil {
		t.Fatal("expected second DeleteVolume to fail with not-found")
	}
}

// --- CreateVirtualMachine / DeleteVirtualMachine ---

func TestCreateVirtualMachineReflectedInListAndAppliesDefaults(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	vm, err := d.CreateVirtualMachine(ctx, CreateVMRequest{Namespace: "wtest", Name: "vm-1", Running: true})
	if err != nil {
		t.Fatalf("CreateVirtualMachine: %v", err)
	}
	if vm.CPU != "1" || vm.Memory != "2Gi" || vm.Status != "Running" || !vm.Ready {
		t.Fatalf("unexpected defaults/state: %+v", vm)
	}

	vms, err := d.ListVirtualMachines(ctx, "wtest")
	if err != nil {
		t.Fatalf("ListVirtualMachines: %v", err)
	}
	found := false
	for _, v := range vms {
		if v.Name == "vm-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created VM to appear in ListVirtualMachines, got %+v", vms)
	}
}

func TestCreateVirtualMachineRequiresNamespaceAndName(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.CreateVirtualMachine(context.Background(), CreateVMRequest{})
	if err == nil {
		t.Fatal("expected CreateVirtualMachine with empty namespace/name to fail")
	}
}

func TestCreateVirtualMachineRejectsDuplicateAgainstSeededVM(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.CreateVirtualMachine(context.Background(), CreateVMRequest{Namespace: "default", Name: "edge-api-vm"})
	if err == nil {
		t.Fatal("expected CreateVirtualMachine to reject a name colliding with a seeded demo VM")
	}
}

func TestCreateVirtualMachineRejectsDuplicateExtraVM(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateVMRequest{Namespace: "wtest", Name: "vm-dup"}
	if _, err := d.CreateVirtualMachine(ctx, req); err != nil {
		t.Fatalf("first CreateVirtualMachine should succeed, got %v", err)
	}
	if _, err := d.CreateVirtualMachine(ctx, req); err == nil {
		t.Fatal("expected second CreateVirtualMachine with the same name to fail")
	}
}

func TestDeleteVirtualMachineRemovesExtraVMAndSecondDeleteFails(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateVMRequest{Namespace: "wtest", Name: "vm-del"}
	if _, err := d.CreateVirtualMachine(ctx, req); err != nil {
		t.Fatalf("CreateVirtualMachine: %v", err)
	}
	if err := d.DeleteVirtualMachine(ctx, "wtest", "vm-del"); err != nil {
		t.Fatalf("first DeleteVirtualMachine should succeed, got %v", err)
	}
	vms, _ := d.ListVirtualMachines(ctx, "wtest")
	for _, v := range vms {
		if v.Name == "vm-del" {
			t.Fatalf("expected deleted VM to be gone from ListVirtualMachines, still present: %+v", v)
		}
	}
	if err := d.DeleteVirtualMachine(ctx, "wtest", "vm-del"); err == nil {
		t.Fatal("expected second DeleteVirtualMachine to fail with not-found")
	}
}

func TestDeleteVirtualMachineRejectsSeededVM(t *testing.T) {
	d := NewDemoProvider()
	if err := d.DeleteVirtualMachine(context.Background(), "default", "edge-api-vm"); err == nil {
		t.Fatal("expected DeleteVirtualMachine to refuse deleting a built-in seeded demo VM")
	}
}

// --- CreateIngress / DeleteIngress ---

func TestCreateIngressReturnsExpectedValue(t *testing.T) {
	d := NewDemoProvider()
	ing, err := d.CreateIngress(context.Background(), CreateIngressRequest{Namespace: "wtest", Name: "ing-1", Host: "wtest.example.com", Service: "svc-1", TLSSecret: "tls-secret"})
	if err != nil {
		t.Fatalf("CreateIngress: %v", err)
	}
	if len(ing.Hosts) != 1 || ing.Hosts[0] != "wtest.example.com" || !ing.TLS {
		t.Fatalf("unexpected ingress: %+v", ing)
	}
}

func TestCreateIngressRequiresFields(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.CreateIngress(context.Background(), CreateIngressRequest{Namespace: "wtest", Name: "ing-missing"})
	if err == nil {
		t.Fatal("expected CreateIngress without host/service to fail")
	}
}

func TestCreateIngressReflectedInListIngresses(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if _, err := d.CreateIngress(ctx, CreateIngressRequest{Namespace: "wtest", Name: "ing-listed", Host: "listed.example.com", Service: "svc-1"}); err != nil {
		t.Fatalf("CreateIngress: %v", err)
	}
	ings, err := d.ListIngresses(ctx, "wtest")
	if err != nil {
		t.Fatalf("ListIngresses: %v", err)
	}
	found := false
	for _, i := range ings {
		if i.Name == "ing-listed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created ingress to appear in ListIngresses, got %+v", ings)
	}
}

func TestCreateIngressRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateIngressRequest{Namespace: "wtest", Name: "ing-dup", Host: "dup.example.com", Service: "svc-1"}
	if _, err := d.CreateIngress(ctx, req); err != nil {
		t.Fatalf("first CreateIngress should succeed, got %v", err)
	}
	if _, err := d.CreateIngress(ctx, req); err == nil {
		t.Fatal("expected second CreateIngress with the same name to fail")
	}
}

func TestDeleteIngressThenSecondDeleteFails(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateIngressRequest{Namespace: "wtest", Name: "ing-del", Host: "del.example.com", Service: "svc-1"}
	if _, err := d.CreateIngress(ctx, req); err != nil {
		t.Fatalf("CreateIngress: %v", err)
	}
	if err := d.DeleteIngress(ctx, "wtest", "ing-del"); err != nil {
		t.Fatalf("first DeleteIngress should succeed, got %v", err)
	}
	if err := d.DeleteIngress(ctx, "wtest", "ing-del"); err == nil {
		t.Fatal("expected second DeleteIngress to fail with not-found")
	}
}

// --- CreateResourceQuota / DeleteResourceQuota ---

func TestCreateResourceQuotaReturnsExpectedValue(t *testing.T) {
	d := NewDemoProvider()
	q, err := d.CreateResourceQuota(context.Background(), CreateResourceQuotaRequest{Namespace: "wtest", Name: "quota-1", Hard: map[string]string{"pods": "10"}})
	if err != nil {
		t.Fatalf("CreateResourceQuota: %v", err)
	}
	if q.Hard["pods"] != "10" {
		t.Fatalf("unexpected quota: %+v", q)
	}
}

func TestCreateResourceQuotaRequiresFields(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.CreateResourceQuota(context.Background(), CreateResourceQuotaRequest{Namespace: "wtest", Name: "quota-missing"})
	if err == nil {
		t.Fatal("expected CreateResourceQuota without hard limits to fail")
	}
}

func TestCreateResourceQuotaReflectedInListResourceQuotas(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if _, err := d.CreateResourceQuota(ctx, CreateResourceQuotaRequest{Namespace: "wtest", Name: "quota-listed", Hard: map[string]string{"pods": "5"}}); err != nil {
		t.Fatalf("CreateResourceQuota: %v", err)
	}
	quotas, err := d.ListResourceQuotas(ctx, "wtest")
	if err != nil {
		t.Fatalf("ListResourceQuotas: %v", err)
	}
	found := false
	for _, q := range quotas {
		if q.Name == "quota-listed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created quota to appear in ListResourceQuotas, got %+v", quotas)
	}
}

func TestCreateResourceQuotaRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateResourceQuotaRequest{Namespace: "wtest", Name: "quota-dup", Hard: map[string]string{"pods": "5"}}
	if _, err := d.CreateResourceQuota(ctx, req); err != nil {
		t.Fatalf("first CreateResourceQuota should succeed, got %v", err)
	}
	if _, err := d.CreateResourceQuota(ctx, req); err == nil {
		t.Fatal("expected second CreateResourceQuota with the same name to fail")
	}
}

func TestDeleteResourceQuotaThenSecondDeleteFails(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateResourceQuotaRequest{Namespace: "wtest", Name: "quota-del", Hard: map[string]string{"pods": "5"}}
	if _, err := d.CreateResourceQuota(ctx, req); err != nil {
		t.Fatalf("CreateResourceQuota: %v", err)
	}
	if err := d.DeleteResourceQuota(ctx, "wtest", "quota-del"); err != nil {
		t.Fatalf("first DeleteResourceQuota should succeed, got %v", err)
	}
	if err := d.DeleteResourceQuota(ctx, "wtest", "quota-del"); err == nil {
		t.Fatal("expected second DeleteResourceQuota to fail with not-found")
	}
}

// --- CreateVeleroBackup (its extra map IS correctly merged into ListVeleroBackups) ---

func TestCreateVeleroBackupReflectedInListVeleroBackups(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	b, err := d.CreateVeleroBackup(ctx, CreateVeleroBackupRequest{Namespace: "velero", Name: "backup-1", IncludedNamespaces: []string{"wtest"}})
	if err != nil {
		t.Fatalf("CreateVeleroBackup: %v", err)
	}
	if b.Phase != "New" || b.Namespaces != "wtest" {
		t.Fatalf("unexpected backup: %+v", b)
	}

	backups, err := d.ListVeleroBackups(ctx, "velero")
	if err != nil {
		t.Fatalf("ListVeleroBackups: %v", err)
	}
	found := false
	for _, bk := range backups {
		if bk.Name == "backup-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created backup to appear in ListVeleroBackups, got %+v", backups)
	}
}

func TestCreateVeleroBackupDefaultsNamespacesToWildcard(t *testing.T) {
	d := NewDemoProvider()
	b, err := d.CreateVeleroBackup(context.Background(), CreateVeleroBackupRequest{Namespace: "velero", Name: "backup-wild"})
	if err != nil {
		t.Fatalf("CreateVeleroBackup: %v", err)
	}
	if b.Namespaces != "*" {
		t.Fatalf("expected wildcard namespaces default, got %q", b.Namespaces)
	}
}

func TestCreateVeleroBackupRequiresFields(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.CreateVeleroBackup(context.Background(), CreateVeleroBackupRequest{})
	if err == nil {
		t.Fatal("expected CreateVeleroBackup with empty namespace/name to fail")
	}
}

func TestCreateVeleroBackupRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateVeleroBackupRequest{Namespace: "velero", Name: "backup-dup"}
	if _, err := d.CreateVeleroBackup(ctx, req); err != nil {
		t.Fatalf("first CreateVeleroBackup should succeed, got %v", err)
	}
	if _, err := d.CreateVeleroBackup(ctx, req); err == nil {
		t.Fatal("expected second CreateVeleroBackup with the same name to fail")
	}
}

// --- UpgradeHelmChart ---

func TestUpgradeHelmChartSucceeds(t *testing.T) {
	d := NewDemoProvider()
	res, err := d.UpgradeHelmChart(context.Background(), InstallHelmRequest{Namespace: "wtest", ReleaseName: "release-1"})
	if err != nil {
		t.Fatalf("UpgradeHelmChart: %v", err)
	}
	if !res.OK || res.Action != "upgrade" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestUpgradeHelmChartRequiresNamespaceAndReleaseName(t *testing.T) {
	d := NewDemoProvider()
	_, err := d.UpgradeHelmChart(context.Background(), InstallHelmRequest{})
	if err == nil {
		t.Fatal("expected UpgradeHelmChart with empty namespace/releaseName to fail")
	}
}
