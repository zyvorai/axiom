//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

func autoscalerPatchCount(dyn *dynamicfake.FakeDynamicClient) int {
	n := 0
	for _, a := range dyn.Actions() {
		if _, ok := a.(clienttesting.PatchAction); ok {
			n++
		}
	}
	return n
}

// createdWithExtras creates through the provider, then adds the unmanaged
// fields an edit must preserve and a resourceVersion.
func createdWithExtras(t *testing.T, dyn *dynamicfake.FakeDynamicClient, gvr schema.GroupVersionResource, ns, name string, mut func(map[string]any)) {
	t.Helper()
	ctx := context.Background()
	u, err := dyn.Resource(gvr).Namespace(ns).Get(ctx, name, metav1GetOptions())
	if err != nil {
		t.Fatal(err)
	}
	withExtras(u, "100")
	if mut != nil {
		mut(u.Object)
	}
	if _, err := dyn.Resource(gvr).Namespace(ns).Update(ctx, u, metav1UpdateOptions()); err != nil {
		t.Fatal(err)
	}
}

func TestKubernetesEditVPA(t *testing.T) {
	ctx := context.Background()
	k, dyn := autoscalerProvider([]schema.GroupVersionResource{vpaGVRs[0]}, nil, cpuHPA("commerce", "checkout-hpa", "Deployment", "checkout"))
	if _, err := k.CreateVPA(ctx, baseVPA()); err != nil {
		t.Fatal(err)
	}
	createdWithExtras(t, dyn, vpaGVRs[0], "commerce", "checkout-vpa", nil)

	ed, err := k.GetVPAEditable(ctx, "commerce", "checkout-vpa")
	if err != nil || !ed.Editable || ed.ResourceVersion != "100" || ed.UpdateMode != "Off" {
		t.Fatalf("get: %v %+v", err, ed)
	}
	req := vpaEditReq(ed, func(r *VPAEditRequest) {
		r.UpdateMode = "Auto"
		r.Containers = append(r.Containers, VPAContainerPolicyRequest{Name: "sidecar", MaxMemory: "256Mi"})
	})
	p, err := k.PreviewVPAEdit(ctx, "commerce", "checkout-vpa", req)
	if err != nil || len(p.Changes) != 2 || autoscalerPatchCount(dyn) != 0 {
		t.Fatalf("preview: %v %+v", err, p)
	}
	res, err := k.UpdateVPA(ctx, "commerce", "checkout-vpa", req)
	if err != nil || !res.OK || strings.Join(res.Changed, ",") != "spec.updatePolicy.updateMode,spec.resourcePolicy.containerPolicies[sidecar]" {
		t.Fatalf("update: %v %+v", err, res)
	}
	// Auto warning + the HPA conflict warning, via the provider.
	if len(res.Warnings) != 2 || !strings.Contains(res.Warnings[1], "VPA and HPA on the same resource conflict") {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	if autoscalerPatchCount(dyn) != 1 {
		t.Fatalf("patches: %d", autoscalerPatchCount(dyn))
	}
	o := liveObj(t, dyn, vpaGVRs[0], "commerce", "checkout-vpa")
	assertExtrasKept(t, o)
	pols, _, _ := unstructured.NestedSlice(o, "spec", "resourcePolicy", "containerPolicies")
	if nestedString(o, "spec", "updatePolicy", "updateMode") != "Auto" || len(pols) != 2 {
		t.Fatalf("live after: %+v", o)
	}

	// Stale resourceVersion: 409, nothing patched.
	before := autoscalerPatchCount(dyn)
	_, err = k.UpdateVPA(ctx, "commerce", "checkout-vpa", vpaEditReq(ed, func(r *VPAEditRequest) { r.ResourceVersion = "99"; r.UpdateMode = "Initial" }))
	if HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "the object changed since you opened it") || autoscalerPatchCount(dyn) != before {
		t.Fatalf("stale: %v", err)
	}
	// Immutable identity: 400.
	if _, err := k.UpdateVPA(ctx, "commerce", "checkout-vpa", vpaEditReq(ed, func(r *VPAEditRequest) { r.TargetName = "other" })); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("immutable: %v", err)
	}
	if _, err := k.GetVPAEditable(ctx, "commerce", "missing"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("404: %v", err)
	}
}

// A JSON Patch test op that fails on the server (the object changed between
// read and patch) is a 409 with the same text.
func TestKubernetesEditPatchTestFailure(t *testing.T) {
	ctx := context.Background()
	k, dyn := autoscalerProvider([]schema.GroupVersionResource{vpaGVRs[0]}, nil)
	if _, err := k.CreateVPA(ctx, baseVPA()); err != nil {
		t.Fatal(err)
	}
	createdWithExtras(t, dyn, vpaGVRs[0], "commerce", "checkout-vpa", nil)
	ed, _ := k.GetVPAEditable(ctx, "commerce", "checkout-vpa")
	dyn.PrependReactor("patch", "verticalpodautoscalers", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errString("testing value /metadata/resourceVersion failed: test failed")
	})
	_, err := k.UpdateVPA(ctx, "commerce", "checkout-vpa", vpaEditReq(ed, func(r *VPAEditRequest) { r.UpdateMode = "Initial" }))
	if HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "the object changed since you opened it") {
		t.Fatalf("test op failure: %v", err)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestKubernetesEditScaledObject(t *testing.T) {
	ctx := context.Background()
	ta := triggerAuthenticationGVRs[0]
	k, dyn := autoscalerProvider([]schema.GroupVersionResource{scaledObjectGVRs[0], ta},
		[]runtime.Object{dynObj(ta, "TriggerAuthentication", "payments", "prom-auth")})
	if _, err := k.CreateScaledObject(ctx, baseSO()); err != nil {
		t.Fatal(err)
	}
	createdWithExtras(t, dyn, scaledObjectGVRs[0], "payments", "payments-api-scaler", nil)

	ed, err := k.GetScaledObjectEditable(ctx, "payments", "payments-api-scaler")
	if err != nil || !ed.Editable || ed.MaxReplicas != 20 || len(ed.Triggers) != 2 {
		t.Fatalf("get: %v %+v", err, ed)
	}
	req := soEditReq(ed, func(r *ScaledObjectEditRequest) {
		r.MaxReplicas = 30
		r.Triggers = r.Triggers[1:] // drop the cron trigger
	})
	p, err := k.PreviewScaledObjectEdit(ctx, "payments", "payments-api-scaler", req)
	if err != nil || p.Changes[0] != (GitOpsEditChange{Object: "ScaledObject/payments/payments-api-scaler", Field: "spec.maxReplicaCount", From: "20", To: "30"}) {
		t.Fatalf("preview: %v %+v", err, p)
	}
	res, err := k.UpdateScaledObject(ctx, "payments", "payments-api-scaler", req)
	if err != nil || !res.OK {
		t.Fatalf("update: %v %+v", err, res)
	}
	o := liveObj(t, dyn, scaledObjectGVRs[0], "payments", "payments-api-scaler")
	assertExtrasKept(t, o)
	trig, _, _ := unstructured.NestedSlice(o, "spec", "triggers")
	maxR, _, _ := unstructured.NestedFieldNoCopy(o, "spec", "maxReplicaCount")
	if len(trig) != 1 || jsonNormalize(maxR) != float64(30) {
		t.Fatalf("live after: %+v", o)
	}
	// Unknown TriggerAuthentication: 400, nothing patched.
	before := autoscalerPatchCount(dyn)
	ed, _ = k.GetScaledObjectEditable(ctx, "payments", "payments-api-scaler")
	if _, err := k.UpdateScaledObject(ctx, "payments", "payments-api-scaler", soEditReq(ed, func(r *ScaledObjectEditRequest) { r.Triggers[0].AuthenticationRef = "nope-auth" })); HTTPStatus(err) != http.StatusBadRequest || autoscalerPatchCount(dyn) != before {
		t.Fatalf("unknown auth: %v", err)
	}

	// advanced present → non-editable, PUT 400, credentials never returned.
	const secret = "s3cr3t-hunter2"
	createdWithExtras(t, dyn, scaledObjectGVRs[0], "payments", "payments-api-scaler", func(o map[string]any) {
		o["spec"].(map[string]any)["advanced"] = map[string]any{"restoreToOriginalReplicaCount": true}
		soTrigger(o, 0)["metadata"].(map[string]any)["serverAddress"] = "http://admin:" + secret + "@prom:9090"
	})
	ed, err = k.GetScaledObjectEditable(ctx, "payments", "payments-api-scaler")
	b, _ := json.Marshal(ed)
	if err != nil || ed.Editable || len(ed.Reasons) < 2 || strings.Contains(string(b), secret) {
		t.Fatalf("non-editable: %v %s", err, b)
	}
	if _, err := k.UpdateScaledObject(ctx, "payments", "payments-api-scaler", soEditReq(ed, nil)); HTTPStatus(err) != http.StatusBadRequest ||
		strings.Contains(err.Error(), secret) || autoscalerPatchCount(dyn) != before {
		t.Fatalf("non-editable put: %v", err)
	}
	// Without KEDA: 422.
	k2, _ := autoscalerProvider(nil, nil)
	if _, err := k2.GetScaledObjectEditable(ctx, "payments", "x"); HTTPStatus(err) != http.StatusUnprocessableEntity {
		t.Fatalf("no CRD: %v", err)
	}
}

func metav1GetOptions() metav1.GetOptions       { return metav1.GetOptions{} }
func metav1UpdateOptions() metav1.UpdateOptions { return metav1.UpdateOptions{} }
