//go:build kubernetes

package platform

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func newFakeKubernetesProvider(objects ...runtime.Object) (*KubernetesProvider, *k8sfake.Clientset) {
	cs := k8sfake.NewSimpleClientset(objects...)
	return &KubernetesProvider{client: cs}, cs
}

// newFakeDynamicProvider wires a *KubernetesProvider to a fake dynamic client.
// gvrToListKind must be supplied explicitly (rather than relying on the
// default scheme-guessing in dynamicfake.NewSimpleDynamicClient) because no
// objects exist yet at construction time for List to guess a Kind from —
// see k8s.io/client-go/dynamic/fake/simple.go's NewSimpleDynamicClient.
func newFakeDynamicProvider(gvrToListKind map[schema.GroupVersionResource]string) *KubernetesProvider {
	scheme := runtime.NewScheme()
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind)
	return &KubernetesProvider{dynamic: dyn}
}

// --- UpdateConfigMap ---

func TestKubernetesUpdateConfigMapReflectsInGet(t *testing.T) {
	ctx := context.Background()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-1", Namespace: "wtest"},
		Data:       map[string]string{"A": "1"},
	}
	k, _ := newFakeKubernetesProvider(cm)

	updated, err := k.UpdateConfigMap(ctx, CreateConfigMapRequest{
		Namespace: "wtest", Name: "cm-1",
		Data: map[string]string{"A": "2", "B": "3"}, Labels: map[string]string{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("UpdateConfigMap: %v", err)
	}
	if updated.Data["A"] != "2" || updated.Data["B"] != "3" || updated.Labels["env"] != "prod" {
		t.Fatalf("unexpected update result: %+v", updated)
	}

	got, err := k.GetConfigMap(ctx, "wtest", "cm-1")
	if err != nil {
		t.Fatalf("GetConfigMap: %v", err)
	}
	if got.Data["A"] != "2" || got.Data["B"] != "3" {
		t.Fatalf("update not reflected in a subsequent Get: %+v", got)
	}
}

func TestKubernetesUpdateConfigMapMissingReturnsError(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	_, err := k.UpdateConfigMap(context.Background(), CreateConfigMapRequest{Namespace: "wtest", Name: "does-not-exist", Data: map[string]string{"A": "1"}})
	if err == nil {
		t.Fatal("expected UpdateConfigMap on a missing configmap to fail")
	}
}

func TestKubernetesUpdateConfigMapRequiresNamespaceAndName(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	_, err := k.UpdateConfigMap(context.Background(), CreateConfigMapRequest{})
	if err == nil {
		t.Fatal("expected UpdateConfigMap with empty namespace/name to fail")
	}
}

// --- UpdateSecret ---

func TestKubernetesUpdateSecretReflectsInGet(t *testing.T) {
	ctx := context.Background()
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sec-1", Namespace: "wtest"},
		Data:       map[string][]byte{"A": []byte("one")},
	}
	k, _ := newFakeKubernetesProvider(sec)

	if _, err := k.UpdateSecret(ctx, CreateSecretRequest{Namespace: "wtest", Name: "sec-1", Data: map[string]string{"B": "two"}}); err != nil {
		t.Fatalf("UpdateSecret: %v", err)
	}

	got, err := k.GetSecret(ctx, "wtest", "sec-1", true)
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Values["B"] != "two" {
		t.Fatalf("update not reflected in a subsequent Get: %+v", got.Values)
	}
	if _, stillThere := got.Values["A"]; stillThere {
		t.Fatalf("expected UpdateSecret to replace (not merge) existing data keys: %+v", got.Values)
	}
}

func TestKubernetesUpdateSecretDataTakesPriorityOverKeys(t *testing.T) {
	ctx := context.Background()
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec-2", Namespace: "wtest"}}
	k, _ := newFakeKubernetesProvider(sec)

	_, err := k.UpdateSecret(ctx, CreateSecretRequest{
		Namespace: "wtest", Name: "sec-2",
		Data: map[string]string{"A": "from-data"},
		Keys: map[string]string{"A": "from-keys-should-be-ignored", "C": "from-keys"},
	})
	if err != nil {
		t.Fatalf("UpdateSecret: %v", err)
	}
	got, err := k.GetSecret(ctx, "wtest", "sec-2", true)
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Values["A"] != "from-data" {
		t.Fatalf("expected Data to take priority over Keys for overlapping keys, got %q", got.Values["A"])
	}
	if got.Values["C"] != "from-keys" {
		t.Fatalf("expected Keys to fill in non-overlapping entries, got %+v", got.Values)
	}
}

func TestKubernetesUpdateSecretRequiresAtLeastOneEntry(t *testing.T) {
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "sec-empty", Namespace: "wtest"}}
	k, _ := newFakeKubernetesProvider(sec)
	_, err := k.UpdateSecret(context.Background(), CreateSecretRequest{Namespace: "wtest", Name: "sec-empty"})
	if err == nil {
		t.Fatal("expected UpdateSecret with no data/keys entries to fail")
	}
}

func TestKubernetesUpdateSecretMissingReturnsError(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	_, err := k.UpdateSecret(context.Background(), CreateSecretRequest{Namespace: "wtest", Name: "does-not-exist", Data: map[string]string{"A": "1"}})
	if err == nil {
		t.Fatal("expected UpdateSecret on a missing secret to fail")
	}
}

// --- CreateService / DeleteService ---

func TestKubernetesCreateServiceReflectedInList(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	svc, err := k.CreateService(ctx, CreateServiceRequest{Namespace: "wtest", Name: "svc-1", Selector: map[string]string{"app": "svc-1"}})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if svc.Type != string(corev1.ServiceTypeClusterIP) {
		t.Fatalf("expected default ClusterIP type, got %q", svc.Type)
	}

	list, err := k.ListServices(ctx, "wtest")
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	found := false
	for _, s := range list {
		if s.Name == "svc-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created service to appear in ListServices, got %+v", list)
	}
}

func TestKubernetesCreateServiceRequiresNamespaceAndName(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	_, err := k.CreateService(context.Background(), CreateServiceRequest{})
	if err == nil {
		t.Fatal("expected CreateService with empty namespace/name to fail")
	}
}

func TestKubernetesCreateServiceRejectsDuplicateName(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	req := CreateServiceRequest{Namespace: "wtest", Name: "svc-dup"}
	if _, err := k.CreateService(ctx, req); err != nil {
		t.Fatalf("first CreateService should succeed, got %v", err)
	}
	if _, err := k.CreateService(ctx, req); err == nil {
		t.Fatal("expected second CreateService with the same name to fail with AlreadyExists")
	}
}

func TestKubernetesDeleteServiceThenSecondDeleteFails(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	req := CreateServiceRequest{Namespace: "wtest", Name: "svc-del"}
	if _, err := k.CreateService(ctx, req); err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if err := k.DeleteService(ctx, "wtest", "svc-del"); err != nil {
		t.Fatalf("first DeleteService should succeed, got %v", err)
	}
	list, _ := k.ListServices(ctx, "wtest")
	for _, s := range list {
		if s.Name == "svc-del" {
			t.Fatalf("expected deleted service to be gone from ListServices, still present: %+v", s)
		}
	}
	if err := k.DeleteService(ctx, "wtest", "svc-del"); err == nil {
		t.Fatal("expected second DeleteService to fail with not-found")
	}
}

// --- CreateVolume / DeleteVolume ---

func TestKubernetesCreateVolumeReflectedInListVolumes(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	vol, err := k.CreateVolume(ctx, CreateVolumeRequest{Namespace: "wtest", Name: "pvc-1", Size: "10Gi"})
	if err != nil {
		t.Fatalf("CreateVolume: %v", err)
	}
	if vol.AccessModes != string(corev1.ReadWriteOnce) {
		t.Fatalf("unexpected access mode: %+v", vol)
	}

	vols, err := k.ListVolumes(ctx, "wtest")
	if err != nil {
		t.Fatalf("ListVolumes: %v", err)
	}
	found := false
	for _, v := range vols {
		if v.Name == "pvc-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created volume to appear in ListVolumes, got %+v", vols)
	}
}

func TestKubernetesCreateVolumeRejectsInvalidSize(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	_, err := k.CreateVolume(context.Background(), CreateVolumeRequest{Namespace: "wtest", Name: "pvc-bad", Size: "not-a-quantity"})
	if err == nil {
		t.Fatal("expected CreateVolume with an invalid size to fail")
	}
}

func TestKubernetesCreateVolumeRequiresFields(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	_, err := k.CreateVolume(context.Background(), CreateVolumeRequest{Namespace: "wtest", Name: "pvc-missing-size"})
	if err == nil {
		t.Fatal("expected CreateVolume without a size to fail")
	}
}

func TestKubernetesDeleteVolumeThenSecondDeleteFails(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	if _, err := k.CreateVolume(ctx, CreateVolumeRequest{Namespace: "wtest", Name: "pvc-del", Size: "5Gi"}); err != nil {
		t.Fatalf("CreateVolume: %v", err)
	}
	if err := k.DeleteVolume(ctx, "wtest", "pvc-del"); err != nil {
		t.Fatalf("first DeleteVolume should succeed, got %v", err)
	}
	if err := k.DeleteVolume(ctx, "wtest", "pvc-del"); err == nil {
		t.Fatal("expected second DeleteVolume to fail with not-found")
	}
}

// --- CreateIngress / DeleteIngress ---

func TestKubernetesCreateIngressReflectedInList(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	ing, err := k.CreateIngress(ctx, CreateIngressRequest{Namespace: "wtest", Name: "ing-1", Host: "wtest.example.com", Service: "svc-1", TLSSecret: "tls-secret"})
	if err != nil {
		t.Fatalf("CreateIngress: %v", err)
	}
	if !ing.TLS {
		t.Fatalf("expected TLS true when TLSSecret is set, got %+v", ing)
	}

	list, err := k.ListIngresses(ctx, "wtest")
	if err != nil {
		t.Fatalf("ListIngresses: %v", err)
	}
	found := false
	for _, i := range list {
		if i.Name == "ing-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created ingress to appear in ListIngresses, got %+v", list)
	}
}

func TestKubernetesCreateIngressRequiresFields(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	_, err := k.CreateIngress(context.Background(), CreateIngressRequest{Namespace: "wtest", Name: "ing-missing"})
	if err == nil {
		t.Fatal("expected CreateIngress without host/service to fail")
	}
}

func TestKubernetesDeleteIngressThenSecondDeleteFails(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	if _, err := k.CreateIngress(ctx, CreateIngressRequest{Namespace: "wtest", Name: "ing-del", Host: "del.example.com", Service: "svc-1"}); err != nil {
		t.Fatalf("CreateIngress: %v", err)
	}
	if err := k.DeleteIngress(ctx, "wtest", "ing-del"); err != nil {
		t.Fatalf("first DeleteIngress should succeed, got %v", err)
	}
	if err := k.DeleteIngress(ctx, "wtest", "ing-del"); err == nil {
		t.Fatal("expected second DeleteIngress to fail with not-found")
	}
}

// --- CreateResourceQuota / DeleteResourceQuota ---

func TestKubernetesCreateResourceQuotaPersistsSpecHard(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	q, err := k.CreateResourceQuota(ctx, CreateResourceQuotaRequest{Namespace: "wtest", Name: "quota-1", Hard: map[string]string{"pods": "10"}})
	if err != nil {
		t.Fatalf("CreateResourceQuota: %v", err)
	}
	if q.Hard["pods"] != "10" {
		t.Fatalf("unexpected return value: %+v", q)
	}

	// ListResourceQuotas (ops_kubernetes.go) reports quota.Status.Hard, which
	// on a real cluster the resourcequota admission controller populates
	// from Spec.Hard. The fake clientset never runs that controller, so
	// Status stays empty here — verify persistence against the raw stored
	// object's Spec instead of going through ListResourceQuotas.
	stored, err := cs.CoreV1().ResourceQuotas("wtest").Get(ctx, "quota-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	qty := stored.Spec.Hard[corev1.ResourcePods]
	if qty.String() != "10" {
		t.Fatalf("expected persisted spec.hard.pods=10, got %v", stored.Spec.Hard)
	}
}

func TestKubernetesCreateResourceQuotaRejectsInvalidQuantity(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	_, err := k.CreateResourceQuota(context.Background(), CreateResourceQuotaRequest{Namespace: "wtest", Name: "quota-bad", Hard: map[string]string{"pods": "not-a-quantity"}})
	if err == nil {
		t.Fatal("expected CreateResourceQuota with an invalid hard value to fail")
	}
}

func TestKubernetesCreateResourceQuotaRequiresFields(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	_, err := k.CreateResourceQuota(context.Background(), CreateResourceQuotaRequest{Namespace: "wtest", Name: "quota-missing"})
	if err == nil {
		t.Fatal("expected CreateResourceQuota without hard limits to fail")
	}
}

func TestKubernetesDeleteResourceQuotaThenSecondDeleteFails(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	if _, err := k.CreateResourceQuota(ctx, CreateResourceQuotaRequest{Namespace: "wtest", Name: "quota-del", Hard: map[string]string{"pods": "5"}}); err != nil {
		t.Fatalf("CreateResourceQuota: %v", err)
	}
	if err := k.DeleteResourceQuota(ctx, "wtest", "quota-del"); err != nil {
		t.Fatalf("first DeleteResourceQuota should succeed, got %v", err)
	}
	if err := k.DeleteResourceQuota(ctx, "wtest", "quota-del"); err == nil {
		t.Fatal("expected second DeleteResourceQuota to fail with not-found")
	}
}

// --- CreateVirtualMachine / DeleteVirtualMachine (dynamic/kubevirt CRD) ---

func TestKubernetesCreateVirtualMachineReflectedInList(t *testing.T) {
	ctx := context.Background()
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{
		kubevirtVMGVR:  "VirtualMachineList",
		kubevirtVMIGVR: "VirtualMachineInstanceList",
	})
	vm, err := k.CreateVirtualMachine(ctx, CreateVMRequest{Namespace: "wtest", Name: "vm-1", Running: true})
	if err != nil {
		t.Fatalf("CreateVirtualMachine: %v", err)
	}
	if vm.Name != "vm-1" {
		t.Fatalf("unexpected vm: %+v", vm)
	}

	list, err := k.ListVirtualMachines(ctx, "wtest")
	if err != nil {
		t.Fatalf("ListVirtualMachines: %v", err)
	}
	found := false
	for _, v := range list {
		if v.Name == "vm-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created VM to appear in ListVirtualMachines, got %+v", list)
	}
}

func TestKubernetesCreateVirtualMachineRequiresNamespaceAndName(t *testing.T) {
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{kubevirtVMGVR: "VirtualMachineList"})
	_, err := k.CreateVirtualMachine(context.Background(), CreateVMRequest{})
	if err == nil {
		t.Fatal("expected CreateVirtualMachine with empty namespace/name to fail")
	}
}

func TestKubernetesCreateVirtualMachineRequiresDynamicClient(t *testing.T) {
	k := &KubernetesProvider{}
	_, err := k.CreateVirtualMachine(context.Background(), CreateVMRequest{Namespace: "wtest", Name: "vm-nodyn"})
	if err == nil {
		t.Fatal("expected CreateVirtualMachine without a dynamic client to fail")
	}
}

func TestKubernetesDeleteVirtualMachineThenSecondDeleteFails(t *testing.T) {
	ctx := context.Background()
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{
		kubevirtVMGVR:  "VirtualMachineList",
		kubevirtVMIGVR: "VirtualMachineInstanceList",
	})
	if _, err := k.CreateVirtualMachine(ctx, CreateVMRequest{Namespace: "wtest", Name: "vm-del"}); err != nil {
		t.Fatalf("CreateVirtualMachine: %v", err)
	}
	if err := k.DeleteVirtualMachine(ctx, "wtest", "vm-del"); err != nil {
		t.Fatalf("first DeleteVirtualMachine should succeed, got %v", err)
	}
	if err := k.DeleteVirtualMachine(ctx, "wtest", "vm-del"); err == nil {
		t.Fatal("expected second DeleteVirtualMachine to fail with not-found")
	}
}

func TestKubernetesDeleteVirtualMachineRequiresDynamicClient(t *testing.T) {
	k := &KubernetesProvider{}
	if err := k.DeleteVirtualMachine(context.Background(), "wtest", "vm-x"); err == nil {
		t.Fatal("expected DeleteVirtualMachine without a dynamic client to fail")
	}
}

// --- CreateVeleroBackup (dynamic/velero CRD) ---

func TestKubernetesCreateVeleroBackupReflectedInList(t *testing.T) {
	ctx := context.Background()
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{veleroBackupGVR: "BackupList"})
	b, err := k.CreateVeleroBackup(ctx, CreateVeleroBackupRequest{Namespace: "velero", Name: "backup-1", IncludedNamespaces: []string{"wtest"}})
	if err != nil {
		t.Fatalf("CreateVeleroBackup: %v", err)
	}
	if b.Namespaces != "wtest" {
		t.Fatalf("unexpected backup: %+v", b)
	}

	list, err := k.ListVeleroBackups(ctx, "velero")
	if err != nil {
		t.Fatalf("ListVeleroBackups: %v", err)
	}
	found := false
	for _, bk := range list {
		if bk.Name == "backup-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created backup to appear in ListVeleroBackups, got %+v", list)
	}
}

func TestKubernetesCreateVeleroBackupDefaultsNamespacesToWildcard(t *testing.T) {
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{veleroBackupGVR: "BackupList"})
	b, err := k.CreateVeleroBackup(context.Background(), CreateVeleroBackupRequest{Namespace: "velero", Name: "backup-wild"})
	if err != nil {
		t.Fatalf("CreateVeleroBackup: %v", err)
	}
	if b.Namespaces != "*" {
		t.Fatalf("expected wildcard namespaces default, got %q", b.Namespaces)
	}
}

func TestKubernetesCreateVeleroBackupRequiresFields(t *testing.T) {
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{veleroBackupGVR: "BackupList"})
	_, err := k.CreateVeleroBackup(context.Background(), CreateVeleroBackupRequest{})
	if err == nil {
		t.Fatal("expected CreateVeleroBackup with empty namespace/name to fail")
	}
}

func TestKubernetesCreateVeleroBackupRequiresDynamicClient(t *testing.T) {
	k := &KubernetesProvider{}
	_, err := k.CreateVeleroBackup(context.Background(), CreateVeleroBackupRequest{Namespace: "velero", Name: "backup-nodyn"})
	if err == nil {
		t.Fatal("expected CreateVeleroBackup without a dynamic client to fail")
	}
}

// --- UpgradeHelmChart ---

func TestKubernetesUpgradeHelmChartCreatesJobAndValuesConfigMap(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	res, err := k.UpgradeHelmChart(ctx, InstallHelmRequest{
		Namespace: "wtest", ReleaseName: "release-1",
		RepoURL: "https://charts.example/repo", ChartName: "nginx", Values: "key: value",
	})
	if err != nil {
		t.Fatalf("UpgradeHelmChart: %v", err)
	}
	if !res.OK || res.JobName == "" {
		t.Fatalf("unexpected result: %+v", res)
	}

	jobs, err := cs.BatchV1().Jobs("axiom-system").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Jobs List: %v", err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("expected exactly 1 helm-upgrade job to be created, got %d", len(jobs.Items))
	}
	if jobs.Items[0].Labels["axiom.zyvor.dev/release"] != "release-1" {
		t.Fatalf("expected job labeled with the release name, got %+v", jobs.Items[0].Labels)
	}

	cms, err := cs.CoreV1().ConfigMaps("axiom-system").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("ConfigMaps List: %v", err)
	}
	if len(cms.Items) != 1 {
		t.Fatalf("expected a values configmap to be created when Values is set, got %d", len(cms.Items))
	}
}

func TestKubernetesUpgradeHelmChartRequiresFields(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	_, err := k.UpgradeHelmChart(context.Background(), InstallHelmRequest{Namespace: "wtest", ReleaseName: "release-1"})
	if err == nil {
		t.Fatal("expected UpgradeHelmChart without repoUrl/chartName to fail")
	}
}
