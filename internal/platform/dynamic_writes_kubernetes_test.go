//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

// Tests for the write operations that go through the dynamic client
// (server-side apply, KubeVirt, Velero, GitOps, CRD instances, AgentRuns).

var (
	networkPolicyGVR = schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	namespaceGVR     = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
)

// --- ApplyManifest ---

// applyRecorder emulates the API server's server-side apply on top of the fake
// dynamic client (whose object tracker cannot create objects from an apply
// patch): it creates the object when absent, otherwise deep-merges, and keeps
// every apply request for inspection. A non-nil failWith makes it reject.
type applyRecorder struct {
	requests []clienttesting.PatchActionImpl
	failWith error
}

func installServerSideApply(dyn *dynamicfake.FakeDynamicClient) *applyRecorder {
	rec := &applyRecorder{}
	dyn.PrependReactor("patch", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		pa, ok := action.(clienttesting.PatchActionImpl)
		if !ok || pa.GetPatchType() != types.ApplyPatchType {
			return false, nil, nil
		}
		rec.requests = append(rec.requests, pa)
		if rec.failWith != nil {
			return true, nil, rec.failWith
		}
		var body map[string]any
		if err := json.Unmarshal(pa.GetPatch(), &body); err != nil {
			return true, nil, apierrors.NewBadRequest(err.Error())
		}
		gvr, ns := pa.GetResource(), pa.GetNamespace()
		existing, err := dyn.Tracker().Get(gvr, ns, pa.GetName())
		if apierrors.IsNotFound(err) {
			obj := &unstructured.Unstructured{Object: body}
			if err := dyn.Tracker().Create(gvr, obj, ns); err != nil {
				return true, nil, err
			}
			return true, obj, nil
		}
		if err != nil {
			return true, nil, err
		}
		cur := existing.(*unstructured.Unstructured)
		deepMerge(cur.Object, body)
		if err := dyn.Tracker().Update(gvr, cur, ns); err != nil {
			return true, nil, err
		}
		return true, cur, nil
	})
	return rec
}

func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				deepMerge(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}

func newApplyProvider(t *testing.T) (*KubernetesProvider, *dynamicfake.FakeDynamicClient, *applyRecorder) {
	t.Helper()
	k, cs, dyn := newFakeFullProvider(map[schema.GroupVersionResource]string{
		networkPolicyGVR: "NetworkPolicyList", namespaceGVR: "NamespaceList",
	}, nil)
	cs.Discovery().(*fakediscovery.FakeDiscovery).Resources = []*metav1.APIResourceList{
		{GroupVersion: "networking.k8s.io/v1", APIResources: []metav1.APIResource{{Name: "networkpolicies", Kind: "NetworkPolicy", Namespaced: true}}},
		{GroupVersion: "v1", APIResources: []metav1.APIResource{{Name: "namespaces", Kind: "Namespace", Namespaced: false}}},
	}
	return k, dyn, installServerSideApply(dyn)
}

const networkPolicyYAML = `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all
  namespace: wtest
  labels: {app: web}
spec:
  podSelector: {}
  policyTypes: [Ingress]
`

func getDyn(t *testing.T, dyn *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
	t.Helper()
	if ns == "" {
		return dyn.Resource(gvr).Get(context.Background(), name, metav1.GetOptions{})
	}
	return dyn.Resource(gvr).Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
}

func TestKubernetesApplyManifestServerSideAppliesNetworkPolicy(t *testing.T) {
	ctx := context.Background()
	k, dyn, rec := newApplyProvider(t)

	res, err := k.ApplyManifest(ctx, networkPolicyYAML)
	if err != nil {
		t.Fatalf("ApplyManifest: %v", err)
	}
	if !res.Valid || res.Message != "applied" || res.Kind != "NetworkPolicy" || res.Name != "deny-all" || res.Namespace != "wtest" ||
		res.APIVersion != "networking.k8s.io/v1" {
		t.Fatalf("unexpected result: %+v", res)
	}
	obj, err := getDyn(t, dyn, networkPolicyGVR, "wtest", "deny-all")
	if err != nil {
		t.Fatalf("object was not applied: %v", err)
	}
	if obj.GetLabels()["app"] != "web" {
		t.Fatalf("labels not applied: %v", obj.GetLabels())
	}
	policyTypes, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "policyTypes")
	if len(policyTypes) != 1 || policyTypes[0] != "Ingress" {
		t.Fatalf("spec not applied: %+v", obj.Object["spec"])
	}

	if len(rec.requests) != 1 {
		t.Fatalf("expected exactly one apply request, got %d", len(rec.requests))
	}
	req := rec.requests[0]
	if req.PatchOptions.FieldManager != "zyvor-axiom" || req.PatchOptions.Force == nil || !*req.PatchOptions.Force {
		t.Fatalf("apply must use fieldManager=zyvor-axiom with force=true: %+v", req.PatchOptions)
	}
	if req.GetNamespace() != "wtest" || req.GetName() != "deny-all" || req.GetResource() != networkPolicyGVR {
		t.Fatalf("apply sent to the wrong target: %+v", req)
	}
	for _, act := range dyn.Actions() {
		if act.GetVerb() == "create" {
			t.Fatalf("apply must not use a create call (would fail with AlreadyExists on re-apply): %v", act)
		}
	}
}

func TestKubernetesApplyManifestIsIdempotentAndUpdatesInPlace(t *testing.T) {
	ctx := context.Background()
	k, dyn, _ := newApplyProvider(t)
	if res, _ := k.ApplyManifest(ctx, networkPolicyYAML); !res.Valid {
		t.Fatalf("first apply failed: %+v", res)
	}
	changed := strings.Replace(networkPolicyYAML, "policyTypes: [Ingress]", "policyTypes: [Ingress, Egress]", 1)
	res, err := k.ApplyManifest(ctx, changed)
	if err != nil || !res.Valid {
		t.Fatalf("re-apply must succeed (no AlreadyExists), got %+v err=%v", res, err)
	}
	obj, _ := getDyn(t, dyn, networkPolicyGVR, "wtest", "deny-all")
	policyTypes, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "policyTypes")
	if len(policyTypes) != 2 {
		t.Fatalf("re-apply did not update the object: %+v", obj.Object["spec"])
	}
}

func TestKubernetesApplyManifestAcceptsJSON(t *testing.T) {
	ctx := context.Background()
	k, dyn, _ := newApplyProvider(t)
	res, err := k.ApplyManifest(ctx, `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"from-json","namespace":"wtest"},"spec":{"podSelector":{}}}`)
	if err != nil || !res.Valid {
		t.Fatalf("JSON manifest should apply: %+v err=%v", res, err)
	}
	if _, err := getDyn(t, dyn, networkPolicyGVR, "wtest", "from-json"); err != nil {
		t.Fatalf("object not stored: %v", err)
	}
}

func TestKubernetesApplyManifestNamespacedResourceDefaultsToDefaultNamespace(t *testing.T) {
	ctx := context.Background()
	k, dyn, _ := newApplyProvider(t)
	res, err := k.ApplyManifest(ctx, "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: nons\nspec:\n  podSelector: {}\n")
	if err != nil || !res.Valid || res.Namespace != "default" {
		t.Fatalf("expected apply into the default namespace, got %+v err=%v", res, err)
	}
	if _, err := getDyn(t, dyn, networkPolicyGVR, "default", "nons"); err != nil {
		t.Fatalf("object should be stored in default: %v", err)
	}
}

func TestKubernetesApplyManifestClusterScopedResourceHasNoNamespace(t *testing.T) {
	ctx := context.Background()
	k, dyn, rec := newApplyProvider(t)
	res, err := k.ApplyManifest(ctx, "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: created-by-apply\n")
	if err != nil || !res.Valid {
		t.Fatalf("cluster-scoped apply failed: %+v err=%v", res, err)
	}
	if rec.requests[0].GetNamespace() != "" {
		t.Fatalf("cluster-scoped objects must be applied without a namespace, got %q", rec.requests[0].GetNamespace())
	}
	if _, err := getDyn(t, dyn, namespaceGVR, "", "created-by-apply"); err != nil {
		t.Fatalf("namespace object not stored: %v", err)
	}
}

func TestKubernetesApplyManifestInvalidInputsReportErrorsAndApplyNothing(t *testing.T) {
	ctx := context.Background()
	k, dyn, rec := newApplyProvider(t)
	cases := map[string]string{
		"empty":            "",
		"whitespace only":  "  \n\t ",
		"not yaml":         "{{{{ definitely: [not valid",
		"scalar":           "just a string",
		"no kind":          "apiVersion: v1\nmetadata:\n  name: x\n",
		"no apiVersion":    "kind: NetworkPolicy\nmetadata:\n  name: x\n",
		"no name":          "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  namespace: wtest\nspec:\n  podSelector: {}\n",
		"unknown kind":     "apiVersion: example.com/v1\nkind: Widget\nmetadata:\n  name: w\n",
		"unknown version":  "apiVersion: networking.k8s.io/v9\nkind: NetworkPolicy\nmetadata:\n  name: w\n",
		"json array":       `[{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"x"}}]`,
		"kind mismatching": "apiVersion: v1\nkind: NetworkPolicy\nmetadata:\n  name: w\n",
	}
	for name, manifest := range cases {
		res, err := k.ApplyManifest(ctx, manifest)
		if err != nil {
			t.Fatalf("%s: invalid manifests are reported in the result, not as an error (API returns 200 valid=false); got err=%v", name, err)
		}
		if res.Valid || len(res.Errors) == 0 {
			t.Fatalf("%s: expected valid=false with error messages, got %+v", name, res)
		}
	}
	if len(rec.requests) != 0 {
		t.Fatalf("invalid manifests must never reach the API server, got %d apply requests", len(rec.requests))
	}
	for _, act := range dyn.Actions() {
		if act.GetVerb() != "list" {
			t.Fatalf("no mutation may be attempted for invalid manifests: %v", act)
		}
	}
}

func TestKubernetesApplyManifestNameErrorNamesTheField(t *testing.T) {
	k, _, _ := newApplyProvider(t)
	res, _ := k.ApplyManifest(context.Background(), "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  namespace: x\n")
	if res.Valid || len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "metadata.name") {
		t.Fatalf("expected a metadata.name error, got %+v", res)
	}
}

func TestKubernetesApplyManifestServerRejectionIsReportedNotSwallowed(t *testing.T) {
	ctx := context.Background()
	k, dyn, rec := newApplyProvider(t)
	rec.failWith = apierrors.NewForbidden(schema.GroupResource{Group: "networking.k8s.io", Resource: "networkpolicies"}, "deny-all", errors.New("rbac: no apply"))
	res, err := k.ApplyManifest(ctx, networkPolicyYAML)
	if err != nil {
		t.Fatalf("server rejection is reported in the result: %v", err)
	}
	if res.Valid || res.Message != "apply failed" || len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "rbac: no apply") {
		t.Fatalf("expected apply failed with the server's message, got %+v", res)
	}
	if _, err := getDyn(t, dyn, networkPolicyGVR, "wtest", "deny-all"); !apierrors.IsNotFound(err) {
		t.Fatalf("a rejected apply must not leave the object behind, err=%v", err)
	}
}

func TestKubernetesApplyManifestWithoutClientsIsInvalidNotPanic(t *testing.T) {
	k := &KubernetesProvider{}
	res, err := k.ApplyManifest(context.Background(), networkPolicyYAML)
	if err != nil || res.Valid || len(res.Errors) == 0 {
		t.Fatalf("expected valid=false without clients, got %+v err=%v", res, err)
	}
}

func TestKubernetesApplyManifestMultiDocumentYAMLAppliesAllOrRejects(t *testing.T) {
	ctx := context.Background()
	k, dyn, _ := newApplyProvider(t)
	res, err := k.ApplyManifest(ctx, networkPolicyYAML+"\n---\napiVersion: v1\nkind: Namespace\nmetadata:\n  name: second-doc\n")
	if err != nil {
		t.Fatalf("ApplyManifest: %v", err)
	}
	if !res.Valid {
		return // rejecting multi-document input is acceptable
	}
	if _, err := getDyn(t, dyn, namespaceGVR, "", "second-doc"); err != nil {
		t.Fatalf("result says applied, but the second document was silently dropped: %v", err)
	}
}

// --- DeleteCRDInstance ---

func TestKubernetesDeleteCRDInstanceNamespacedAndClusterScoped(t *testing.T) {
	ctx := context.Background()
	widgets := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}
	gadgets := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "gadgets"}
	k, _, dyn := newFakeFullProvider(
		map[schema.GroupVersionResource]string{widgets: "WidgetList", gadgets: "GadgetList"},
		[]runtime.Object{
			newUnstructured("example.com/v1", "Widget", "a", "w1", nil),
			newUnstructured("example.com/v1", "Widget", "b", "w1", nil),
			newUnstructured("example.com/v1", "Gadget", "", "g1", nil),
		})

	if err := k.DeleteCRDInstance(ctx, "example.com", "v1", "widgets", "a", "w1"); err != nil {
		t.Fatalf("delete namespaced: %v", err)
	}
	if _, err := getDyn(t, dyn, widgets, "a", "w1"); !apierrors.IsNotFound(err) {
		t.Fatalf("a/w1 should be gone, err=%v", err)
	}
	if _, err := getDyn(t, dyn, widgets, "b", "w1"); err != nil {
		t.Fatalf("b/w1 must survive (cross-namespace leak?): %v", err)
	}
	if err := k.DeleteCRDInstance(ctx, "example.com", "v1", "gadgets", "", "g1"); err != nil {
		t.Fatalf("delete cluster-scoped: %v", err)
	}
	if _, err := getDyn(t, dyn, gadgets, "", "g1"); !apierrors.IsNotFound(err) {
		t.Fatalf("g1 should be gone, err=%v", err)
	}
	wantStatus(t, k.DeleteCRDInstance(ctx, "example.com", "v1", "widgets", "a", "w1"), http.StatusNotFound)
}

func TestKubernetesDeleteCRDInstanceRequiresDynamicClient(t *testing.T) {
	k := &KubernetesProvider{}
	if err := k.DeleteCRDInstance(context.Background(), "g", "v1", "things", "ns", "x"); err == nil {
		t.Fatal("expected an error without a dynamic client")
	}
}

// --- KubeVirt VM actions ---

func kubevirtProvider(vms ...runtime.Object) (*KubernetesProvider, *dynamicfake.FakeDynamicClient) {
	k, _, dyn := newFakeFullProvider(map[schema.GroupVersionResource]string{
		kubevirtVMGVR: "VirtualMachineList", kubevirtVMIGVR: "VirtualMachineInstanceList", vmimGVR: "VirtualMachineInstanceMigrationList",
	}, vms)
	return k, dyn
}

func newVM(ns, name string, running bool) *unstructured.Unstructured {
	return newUnstructured("kubevirt.io/v1", "VirtualMachine", ns, name, map[string]any{"spec": map[string]any{"running": running}})
}

func TestKubernetesStopAndStartVirtualMachineSetRunStrategy(t *testing.T) {
	ctx := context.Background()
	k, dyn := kubevirtProvider(newVM("a", "vm1", true), newVM("b", "vm1", true))

	if err := k.StopVirtualMachine(ctx, "a", "vm1"); err != nil {
		t.Fatalf("StopVirtualMachine: %v", err)
	}
	vm, _ := getDyn(t, dyn, kubevirtVMGVR, "a", "vm1")
	strategy, _, _ := unstructured.NestedString(vm.Object, "spec", "runStrategy")
	if strategy != "Halted" {
		t.Fatalf("expected runStrategy Halted, got %q (%v)", strategy, vm.Object["spec"])
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(vm.Object, "spec", "running"); found {
		t.Fatal("spec.running must be removed: KubeVirt rejects a VM that sets both running and runStrategy")
	}
	other, _ := getDyn(t, dyn, kubevirtVMGVR, "b", "vm1")
	if running, _, _ := unstructured.NestedBool(other.Object, "spec", "running"); !running {
		t.Fatalf("b/vm1 must be untouched (cross-namespace leak?): %v", other.Object["spec"])
	}
	list, _ := k.ListVirtualMachines(ctx, "a")
	if len(list) != 1 || list[0].Running {
		t.Fatalf("ListVirtualMachines should show the VM as not running: %+v", list)
	}

	if err := k.StartVirtualMachine(ctx, "a", "vm1"); err != nil {
		t.Fatalf("StartVirtualMachine: %v", err)
	}
	vm, _ = getDyn(t, dyn, kubevirtVMGVR, "a", "vm1")
	if strategy, _, _ := unstructured.NestedString(vm.Object, "spec", "runStrategy"); strategy != "Always" {
		t.Fatalf("expected runStrategy Always, got %q", strategy)
	}
	list, _ = k.ListVirtualMachines(ctx, "a")
	if len(list) != 1 || !list[0].Running {
		t.Fatalf("ListVirtualMachines should show the VM as running: %+v", list)
	}
}

func TestKubernetesStartStopVirtualMachineMissingAndNoClient(t *testing.T) {
	ctx := context.Background()
	k, _ := kubevirtProvider()
	wantStatus(t, k.StartVirtualMachine(ctx, "a", "ghost"), http.StatusNotFound)
	wantStatus(t, k.StopVirtualMachine(ctx, "a", "ghost"), http.StatusNotFound)
	if err := (&KubernetesProvider{}).StartVirtualMachine(ctx, "a", "vm"); err == nil {
		t.Fatal("expected an error without a dynamic client")
	}
}

func TestKubernetesLiveMigrateVMCreatesMigrationObject(t *testing.T) {
	ctx := context.Background()
	k, dyn := kubevirtProvider()
	res, err := k.LiveMigrateVM(ctx, "a", "vm1")
	if err != nil {
		t.Fatalf("LiveMigrateVM: %v", err)
	}
	if !res.OK || res.Action != "migrate" || !strings.HasPrefix(res.JobName, "a/vm1-migrate-") {
		t.Fatalf("unexpected result: %+v", res)
	}
	list, err := dyn.Resource(vmimGVR).Namespace("a").List(ctx, metav1.ListOptions{})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("expected one VirtualMachineInstanceMigration in a, got %v err=%v", list, err)
	}
	m := list.Items[0]
	if vmi, _, _ := unstructured.NestedString(m.Object, "spec", "vmiName"); vmi != "vm1" {
		t.Fatalf("migration must target vmi vm1: %+v", m.Object)
	}
	if m.GetNamespace() != "a" || m.GetKind() != "VirtualMachineInstanceMigration" {
		t.Fatalf("unexpected migration object: %+v", m.Object)
	}
	if other, _ := dyn.Resource(vmimGVR).Namespace("b").List(ctx, metav1.ListOptions{}); len(other.Items) != 0 {
		t.Fatal("no migration may be created in another namespace")
	}
	if _, err := (&KubernetesProvider{}).LiveMigrateVM(ctx, "a", "vm"); err == nil {
		t.Fatal("expected an error without a dynamic client")
	}
}

func TestKubernetesCreateVMSnapshotCloneExportRestoreStoreSpecs(t *testing.T) {
	ctx := context.Background()
	k, _, dyn := newFakeFullProvider(map[schema.GroupVersionResource]string{
		vmSnapshotGVRs[0]: "VirtualMachineSnapshotList", vmCloneGVRs[0]: "VirtualMachineCloneList",
		vmExportGVRs[0]: "VirtualMachineExportList", vmRestoreGVRs[0]: "VirtualMachineRestoreList",
	}, nil)

	if _, err := k.CreateVMSnapshot(ctx, CreateVMSnapshotRequest{Namespace: "a", VM: "vm1", Name: "snap1"}); err != nil {
		t.Fatalf("CreateVMSnapshot: %v", err)
	}
	snap, err := getDyn(t, dyn, vmSnapshotGVRs[0], "a", "snap1")
	if err != nil {
		t.Fatalf("snapshot not stored: %v", err)
	}
	if src, _, _ := unstructured.NestedString(snap.Object, "spec", "source", "name"); src != "vm1" {
		t.Fatalf("snapshot must reference vm1: %+v", snap.Object["spec"])
	}

	if _, err := k.CreateVMClone(ctx, CreateVMCloneRequest{Namespace: "a", SourceVM: "vm1", TargetVM: "vm1-copy", Name: "clone1"}); err != nil {
		t.Fatalf("CreateVMClone: %v", err)
	}
	clone, err := getDyn(t, dyn, vmCloneGVRs[0], "a", "clone1")
	if err != nil {
		t.Fatalf("clone not stored: %v", err)
	}
	if s, _, _ := unstructured.NestedString(clone.Object, "spec", "source", "name"); s != "vm1" {
		t.Fatalf("clone source wrong: %+v", clone.Object["spec"])
	}
	if s, _, _ := unstructured.NestedString(clone.Object, "spec", "target", "name"); s != "vm1-copy" {
		t.Fatalf("clone target wrong: %+v", clone.Object["spec"])
	}

	if _, err := k.CreateVMExport(ctx, CreateVMExportRequest{Namespace: "a", VM: "vm1", Name: "exp1"}); err != nil {
		t.Fatalf("CreateVMExport: %v", err)
	}
	exp, err := getDyn(t, dyn, vmExportGVRs[0], "a", "exp1")
	if err != nil {
		t.Fatalf("export not stored: %v", err)
	}
	if s, _, _ := unstructured.NestedString(exp.Object, "spec", "source", "name"); s != "vm1" {
		t.Fatalf("export source wrong: %+v", exp.Object["spec"])
	}

	if _, err := k.CreateVMRestore(ctx, CreateVMRestoreRequest{Namespace: "a", Snapshot: "snap1", TargetVM: "vm1", Name: "rest1"}); err != nil {
		t.Fatalf("CreateVMRestore: %v", err)
	}
	rest, err := getDyn(t, dyn, vmRestoreGVRs[0], "a", "rest1")
	if err != nil {
		t.Fatalf("restore not stored: %v", err)
	}
	if s, _, _ := unstructured.NestedString(rest.Object, "spec", "virtualMachineSnapshotName"); s != "snap1" {
		t.Fatalf("restore snapshot wrong: %+v", rest.Object["spec"])
	}
	if s, _, _ := unstructured.NestedString(rest.Object, "spec", "target", "name"); s != "vm1" {
		t.Fatalf("restore target wrong: %+v", rest.Object["spec"])
	}
}

func TestKubernetesVMSnapshotCloneFallBackToOlderAPIVersion(t *testing.T) {
	ctx := context.Background()
	k, _, dyn := newFakeFullProvider(map[schema.GroupVersionResource]string{
		vmSnapshotGVRs[1]: "VirtualMachineSnapshotList", vmCloneGVRs[1]: "VirtualMachineCloneList",
	}, nil)
	// Emulate a cluster that only serves the v1alpha1 CRDs.
	dyn.PrependReactor("create", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetResource().Version == "v1beta1" {
			return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), "")
		}
		return false, nil, nil
	})
	if _, err := k.CreateVMSnapshot(ctx, CreateVMSnapshotRequest{Namespace: "a", VM: "vm1", Name: "snap1"}); err != nil {
		t.Fatalf("CreateVMSnapshot fallback: %v", err)
	}
	if _, err := getDyn(t, dyn, vmSnapshotGVRs[1], "a", "snap1"); err != nil {
		t.Fatalf("snapshot should be created via v1alpha1: %v", err)
	}
	if _, err := k.CreateVMClone(ctx, CreateVMCloneRequest{Namespace: "a", SourceVM: "vm1", TargetVM: "c", Name: "clone1"}); err != nil {
		t.Fatalf("CreateVMClone fallback: %v", err)
	}
	if _, err := getDyn(t, dyn, vmCloneGVRs[1], "a", "clone1"); err != nil {
		t.Fatalf("clone should be created via v1alpha1: %v", err)
	}
}

func TestKubernetesVMSnapshotCloneExportRestoreValidateInput(t *testing.T) {
	ctx := context.Background()
	k, _, dyn := newFakeFullProvider(map[schema.GroupVersionResource]string{vmSnapshotGVRs[0]: "VirtualMachineSnapshotList"}, nil)
	if _, err := k.CreateVMSnapshot(ctx, CreateVMSnapshotRequest{Namespace: "a"}); err == nil {
		t.Fatal("snapshot without vm must fail")
	}
	if _, err := k.CreateVMClone(ctx, CreateVMCloneRequest{Namespace: "a", SourceVM: "x"}); err == nil {
		t.Fatal("clone without target must fail")
	}
	if _, err := k.CreateVMExport(ctx, CreateVMExportRequest{Namespace: "a"}); err == nil {
		t.Fatal("export without vm must fail")
	}
	if _, err := k.CreateVMRestore(ctx, CreateVMRestoreRequest{Namespace: "a", Snapshot: "s"}); err == nil {
		t.Fatal("restore without target must fail")
	}
	for _, act := range dyn.Actions() {
		if act.GetVerb() == "create" {
			t.Fatalf("invalid requests must not create anything: %v", act)
		}
	}
	nk := &KubernetesProvider{}
	if _, err := nk.CreateVMSnapshot(ctx, CreateVMSnapshotRequest{Namespace: "a", VM: "v"}); err == nil {
		t.Fatal("snapshot without dynamic client must fail")
	}
}

func TestKubernetesCreateVMSnapshotDuplicateIsConflict(t *testing.T) {
	ctx := context.Background()
	k, _, _ := newFakeFullProvider(map[schema.GroupVersionResource]string{vmSnapshotGVRs[0]: "VirtualMachineSnapshotList"}, nil)
	req := CreateVMSnapshotRequest{Namespace: "a", VM: "vm1", Name: "dup"}
	if _, err := k.CreateVMSnapshot(ctx, req); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := k.CreateVMSnapshot(ctx, req)
	wantStatus(t, err, http.StatusConflict)
}

// --- Velero schedule ---

func TestKubernetesCreateVeleroSchedulePersistsSpec(t *testing.T) {
	ctx := context.Background()
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{veleroSchedGVR: "ScheduleList"})

	sched, err := k.CreateVeleroSchedule(ctx, CreateVeleroScheduleRequest{Namespace: "velero", Name: "nightly", Schedule: "0 2 * * *", TTL: "72h"})
	if err != nil {
		t.Fatalf("CreateVeleroSchedule: %v", err)
	}
	if sched.Name != "nightly" || sched.Schedule != "0 2 * * *" {
		t.Fatalf("unexpected result: %+v", sched)
	}
	obj, err := k.dynamic.Resource(veleroSchedGVR).Namespace("velero").Get(ctx, "nightly", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("schedule not stored: %v", err)
	}
	if s, _, _ := unstructured.NestedString(obj.Object, "spec", "schedule"); s != "0 2 * * *" {
		t.Fatalf("cron expression not persisted: %+v", obj.Object["spec"])
	}
	if ttl, _, _ := unstructured.NestedString(obj.Object, "spec", "template", "ttl"); ttl != "72h" {
		t.Fatalf("ttl not persisted: %+v", obj.Object["spec"])
	}
	if inc, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "template", "includedNamespaces"); len(inc) != 1 || inc[0] != "*" {
		t.Fatalf("expected all namespaces included: %+v", obj.Object["spec"])
	}
	if obj.GetKind() != "Schedule" || obj.GetAPIVersion() != "velero.io/v1" {
		t.Fatalf("unexpected kind/apiVersion: %s %s", obj.GetKind(), obj.GetAPIVersion())
	}
	list, _ := k.ListVeleroSchedules(ctx, "velero")
	if len(list) != 1 || list[0].Name != "nightly" {
		t.Fatalf("expected schedule in ListVeleroSchedules, got %+v", list)
	}
}

func TestKubernetesCreateVeleroScheduleOmitsTTLWhenUnset(t *testing.T) {
	ctx := context.Background()
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{veleroSchedGVR: "ScheduleList"})
	if _, err := k.CreateVeleroSchedule(ctx, CreateVeleroScheduleRequest{Namespace: "velero", Name: "s", Schedule: "@daily"}); err != nil {
		t.Fatalf("CreateVeleroSchedule: %v", err)
	}
	obj, _ := k.dynamic.Resource(veleroSchedGVR).Namespace("velero").Get(ctx, "s", metav1.GetOptions{})
	if _, found, _ := unstructured.NestedString(obj.Object, "spec", "template", "ttl"); found {
		t.Fatalf("ttl must be omitted when not requested (Velero then applies its default): %+v", obj.Object["spec"])
	}
}

func TestKubernetesCreateVeleroScheduleRejectsMissingFieldsAndDuplicates(t *testing.T) {
	ctx := context.Background()
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{veleroSchedGVR: "ScheduleList"})
	for i, req := range []CreateVeleroScheduleRequest{
		{},
		{Namespace: "velero", Name: "n"},
		{Namespace: "velero", Schedule: "* * * * *"},
		{Name: "n", Schedule: "* * * * *"},
		{Namespace: "velero", Name: "n", Schedule: "   "},
	} {
		if _, err := k.CreateVeleroSchedule(ctx, req); err == nil {
			t.Fatalf("case %d: expected an error for %+v", i, req)
		}
	}
	if list, _ := k.dynamic.Resource(veleroSchedGVR).Namespace("velero").List(ctx, metav1.ListOptions{}); len(list.Items) != 0 {
		t.Fatalf("invalid requests must not create schedules, got %d", len(list.Items))
	}
	ok := CreateVeleroScheduleRequest{Namespace: "velero", Name: "dup", Schedule: "@daily"}
	if _, err := k.CreateVeleroSchedule(ctx, ok); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := k.CreateVeleroSchedule(ctx, ok)
	wantStatus(t, err, http.StatusConflict)
	if _, err := (&KubernetesProvider{}).CreateVeleroSchedule(ctx, ok); err == nil {
		t.Fatal("expected an error without a dynamic client")
	}
}

// --- GitOps sync ---

func TestKubernetesTriggerGitOpsSyncFluxAnnotatesForReconcile(t *testing.T) {
	ctx := context.Background()
	kust := fluxKustomizationGVRs[0]
	hr := fluxHelmReleaseGVRs[0]
	k, _, dyn := newFakeFullProvider(map[schema.GroupVersionResource]string{kust: "KustomizationList", hr: "HelmReleaseList"},
		[]runtime.Object{
			newUnstructured("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps", nil),
			newUnstructured("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "other", "apps", nil),
			newUnstructured("helm.toolkit.fluxcd.io/v2", "HelmRelease", "flux-system", "web", nil),
		})
	if err := k.TriggerGitOpsSync(ctx, "Kustomization", "flux-system", "apps"); err != nil {
		t.Fatalf("sync kustomization: %v", err)
	}
	if err := k.TriggerGitOpsSync(ctx, "HelmRelease", "flux-system", "web"); err != nil {
		t.Fatalf("sync helmrelease: %v", err)
	}
	for _, tc := range []struct {
		gvr        schema.GroupVersionResource
		ns, name   string
		wantSynced bool
	}{{kust, "flux-system", "apps", true}, {hr, "flux-system", "web", true}, {kust, "other", "apps", false}} {
		obj, err := getDyn(t, dyn, tc.gvr, tc.ns, tc.name)
		if err != nil {
			t.Fatalf("get %s/%s: %v", tc.ns, tc.name, err)
		}
		has := obj.GetAnnotations()["reconcile.fluxcd.io/requestedAt"] != ""
		if has != tc.wantSynced {
			t.Fatalf("%s/%s: reconcile annotation present=%v, want %v (%v)", tc.ns, tc.name, has, tc.wantSynced, obj.GetAnnotations())
		}
	}
}

func TestKubernetesTriggerGitOpsSyncFluxFallsBackToOlderAPIVersion(t *testing.T) {
	ctx := context.Background()
	old := fluxHelmReleaseGVRs[1] // v2beta2 — the only version this cluster serves the object under
	k, _, dyn := newFakeFullProvider(map[schema.GroupVersionResource]string{old: "HelmReleaseList"},
		[]runtime.Object{newUnstructured("helm.toolkit.fluxcd.io/v2beta2", "HelmRelease", "flux-system", "web", nil)})
	if err := k.TriggerGitOpsSync(ctx, "HelmRelease", "flux-system", "web"); err != nil {
		t.Fatalf("sync via older API version: %v", err)
	}
	obj, _ := getDyn(t, dyn, old, "flux-system", "web")
	if obj.GetAnnotations()["reconcile.fluxcd.io/requestedAt"] == "" {
		t.Fatalf("reconcile annotation missing: %v", obj.GetAnnotations())
	}
}

func TestKubernetesTriggerGitOpsSyncArgoSetsOperationSync(t *testing.T) {
	ctx := context.Background()
	app := argoAppGVRs[0]
	k, _, dyn := newFakeFullProvider(map[schema.GroupVersionResource]string{app: "ApplicationList"},
		[]runtime.Object{newUnstructured("argoproj.io/v1alpha1", "Application", "argocd", "guestbook", nil)})
	if err := k.TriggerGitOpsSync(ctx, "Application", "argocd", "guestbook"); err != nil {
		t.Fatalf("sync application: %v", err)
	}
	obj, _ := getDyn(t, dyn, app, "argocd", "guestbook")
	if _, found, _ := unstructured.NestedMap(obj.Object, "operation", "sync"); !found {
		t.Fatalf("expected operation.sync to be set (what `argocd app sync` does): %+v", obj.Object)
	}
}

func TestKubernetesTriggerGitOpsSyncErrors(t *testing.T) {
	ctx := context.Background()
	app := argoAppGVRs[0]
	k, _, _ := newFakeFullProvider(map[schema.GroupVersionResource]string{app: "ApplicationList"}, nil)
	if err := k.TriggerGitOpsSync(ctx, "Deployment", "ns", "x"); err == nil {
		t.Fatal("expected an unsupported kind to be rejected")
	}
	wantStatus(t, k.TriggerGitOpsSync(ctx, "Application", "argocd", "ghost"), http.StatusNotFound)
	if err := (&KubernetesProvider{}).TriggerGitOpsSync(ctx, "Application", "a", "b"); err == nil {
		t.Fatal("expected an error without a dynamic client")
	}
}

// --- Backups (VolumeSnapshot) ---

func TestKubernetesCreateBackupStoresVolumeSnapshot(t *testing.T) {
	ctx := context.Background()
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{volumeSnapshotGVR: "VolumeSnapshotList"})
	b, err := k.CreateBackup(ctx, CreateBackupRequest{Namespace: "a", Name: "snap", PVC: "data", VolumeSnapshotClass: "csi-hostpath"})
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if b.PVC != "data" || b.Status != "Pending" {
		t.Fatalf("unexpected result: %+v", b)
	}
	obj, err := k.dynamic.Resource(volumeSnapshotGVR).Namespace("a").Get(ctx, "snap", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("snapshot not stored: %v", err)
	}
	if pvc, _, _ := unstructured.NestedString(obj.Object, "spec", "source", "persistentVolumeClaimName"); pvc != "data" {
		t.Fatalf("source pvc not persisted: %+v", obj.Object["spec"])
	}
	if cls, _, _ := unstructured.NestedString(obj.Object, "spec", "volumeSnapshotClassName"); cls != "csi-hostpath" {
		t.Fatalf("snapshot class not persisted: %+v", obj.Object["spec"])
	}
	for i, req := range []CreateBackupRequest{{}, {Namespace: "a", Name: "n"}, {Namespace: "a", PVC: "p"}, {Name: "n", PVC: "p"}} {
		if _, err := k.CreateBackup(ctx, req); err == nil {
			t.Fatalf("case %d: expected missing fields to be rejected", i)
		}
	}
	_, err = k.CreateBackup(ctx, CreateBackupRequest{Namespace: "a", Name: "snap", PVC: "data"})
	wantStatus(t, err, http.StatusConflict)
}

func TestKubernetesRestoreBackupCreatesPVCFromSnapshot(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	vol, err := k.RestoreBackup(ctx, RestoreBackupRequest{Namespace: "a", SnapshotName: "snap", PVCName: "restored", StorageClass: "fast"})
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if vol.Name != "restored" || vol.Capacity != "10Gi" {
		t.Fatalf("unexpected result: %+v", vol)
	}
	pvc, err := cs.CoreV1().PersistentVolumeClaims("a").Get(ctx, "restored", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("pvc not stored: %v", err)
	}
	ds := pvc.Spec.DataSource
	if ds == nil || ds.Kind != "VolumeSnapshot" || ds.Name != "snap" || ds.APIGroup == nil || *ds.APIGroup != "snapshot.storage.k8s.io" {
		t.Fatalf("pvc must be provisioned from the VolumeSnapshot: %+v", ds)
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "fast" {
		t.Fatalf("storage class not persisted: %+v", pvc.Spec.StorageClassName)
	}
	if len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != "ReadWriteOnce" {
		t.Fatalf("expected default ReadWriteOnce: %+v", pvc.Spec.AccessModes)
	}
	if q := pvc.Spec.Resources.Requests["storage"]; q.String() != "10Gi" {
		t.Fatalf("expected default size 10Gi, got %s", q.String())
	}
	for i, req := range []RestoreBackupRequest{{}, {Namespace: "a", SnapshotName: "s"}, {Namespace: "a", PVCName: "p"}, {SnapshotName: "s", PVCName: "p"}} {
		if _, err := k.RestoreBackup(ctx, req); err == nil {
			t.Fatalf("case %d: expected missing fields to be rejected", i)
		}
	}
	_, err = k.RestoreBackup(ctx, RestoreBackupRequest{Namespace: "a", SnapshotName: "snap", PVCName: "restored"})
	wantStatus(t, err, http.StatusConflict)
}

func TestKubernetesRestoreBackupInvalidSizeIsRejected(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	if _, err := k.RestoreBackup(context.Background(), RestoreBackupRequest{Namespace: "a", SnapshotName: "s", PVCName: "p", Size: "lots"}); err == nil {
		t.Fatal("expected an invalid size to be rejected with an error")
	}
}

// --- AgentRuns ---

func agentRunProvider(objs ...runtime.Object) (*KubernetesProvider, *dynamicfake.FakeDynamicClient) {
	k, _, dyn := newFakeFullProvider(map[schema.GroupVersionResource]string{agentRunGVR: "AgentRunList"}, objs)
	return k, dyn
}

func TestKubernetesCreateAgentRunPersistsSpecWithSafeDefaults(t *testing.T) {
	ctx := context.Background()
	k, dyn := agentRunProvider()
	run, err := k.CreateAgentRun(ctx, CreateAgentRunRequest{Name: "fix-it", Goal: "  scale web to 3  "})
	if err != nil {
		t.Fatalf("CreateAgentRun: %v", err)
	}
	if run.Name != "fix-it" || run.Namespace != "default" {
		t.Fatalf("unexpected run: %+v", run)
	}
	obj, err := getDyn(t, dyn, agentRunGVR, "default", "fix-it")
	if err != nil {
		t.Fatalf("agentrun not stored: %v", err)
	}
	if goal, _, _ := unstructured.NestedString(obj.Object, "spec", "goal"); goal != "scale web to 3" {
		t.Fatalf("goal should be trimmed and stored: %+v", obj.Object["spec"])
	}
	if dry, _, _ := unstructured.NestedBool(obj.Object, "spec", "dryRun"); !dry {
		t.Fatalf("dryRun must default to true (safe default): %+v", obj.Object["spec"])
	}
	if auto, _, _ := unstructured.NestedString(obj.Object, "spec", "autonomy"); auto != "ProposeOnly" {
		t.Fatalf("autonomy must be ProposeOnly: %+v", obj.Object["spec"])
	}
}

func TestKubernetesCreateAgentRunHonoursExplicitDryRunAndTarget(t *testing.T) {
	ctx := context.Background()
	k, dyn := agentRunProvider()
	no := false
	_, err := k.CreateAgentRun(ctx, CreateAgentRunRequest{
		Namespace: "ops", Name: "r1", Goal: "restart", DryRun: &no,
		Target: &AgentObjectRef{Kind: "Deployment", Name: "web"},
	})
	if err != nil {
		t.Fatalf("CreateAgentRun: %v", err)
	}
	obj, _ := getDyn(t, dyn, agentRunGVR, "ops", "r1")
	if dry, found, _ := unstructured.NestedBool(obj.Object, "spec", "dryRun"); !found || dry {
		t.Fatalf("explicit dryRun=false must be honoured: %+v", obj.Object["spec"])
	}
	if ns, _, _ := unstructured.NestedString(obj.Object, "spec", "target", "namespace"); ns != "ops" {
		t.Fatalf("target namespace should default to the run's namespace: %+v", obj.Object["spec"])
	}
	if name, _, _ := unstructured.NestedString(obj.Object, "spec", "target", "name"); name != "web" {
		t.Fatalf("target not persisted: %+v", obj.Object["spec"])
	}
}

func TestKubernetesCreateAgentRunRequiresGoalAndRejectsDuplicates(t *testing.T) {
	ctx := context.Background()
	k, dyn := agentRunProvider()
	if _, err := k.CreateAgentRun(ctx, CreateAgentRunRequest{Name: "x", Goal: "   "}); err == nil {
		t.Fatal("expected a blank goal to be rejected")
	}
	if list, _ := dyn.Resource(agentRunGVR).Namespace("default").List(ctx, metav1.ListOptions{}); len(list.Items) != 0 {
		t.Fatalf("rejected request must not create anything, got %d", len(list.Items))
	}
	req := CreateAgentRunRequest{Name: "dup", Goal: "g"}
	if _, err := k.CreateAgentRun(ctx, req); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := k.CreateAgentRun(ctx, req)
	wantStatus(t, err, http.StatusConflict)
	if _, err := (&KubernetesProvider{}).CreateAgentRun(ctx, req); err == nil {
		t.Fatal("expected an error without a dynamic client")
	}
}

func TestKubernetesApproveAndDenyAgentRunAnnotate(t *testing.T) {
	ctx := context.Background()
	k, dyn := agentRunProvider(
		newUnstructured("platform.example.com/v1", "AgentRun", "ops", "r1", nil),
		newUnstructured("platform.example.com/v1", "AgentRun", "ops", "r2", nil),
	)
	if _, err := k.ApproveAgentRun(ctx, "ops", "r1"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := k.DenyAgentRun(ctx, "ops", "r2"); err != nil {
		t.Fatalf("Deny: %v", err)
	}
	r1, _ := getDyn(t, dyn, agentRunGVR, "ops", "r1")
	r2, _ := getDyn(t, dyn, agentRunGVR, "ops", "r2")
	if r1.GetAnnotations()[agentAnnotationApproved] != "true" || r1.GetAnnotations()[agentAnnotationDenied] != "" {
		t.Fatalf("r1 should be approved only: %v", r1.GetAnnotations())
	}
	if r2.GetAnnotations()[agentAnnotationDenied] != "true" || r2.GetAnnotations()[agentAnnotationApproved] != "" {
		t.Fatalf("r2 should be denied only: %v", r2.GetAnnotations())
	}
	_, err := k.ApproveAgentRun(ctx, "ops", "ghost")
	wantStatus(t, err, http.StatusNotFound)
	_, err = k.DenyAgentRun(ctx, "elsewhere", "r1")
	wantStatus(t, err, http.StatusNotFound)
}
