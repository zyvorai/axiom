//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// autoscalerProvider builds a provider whose discovery serves exactly the
// given GVRs; objects go to the typed clientset (HPAs), dynObjects to the
// dynamic one.
func autoscalerProvider(served []schema.GroupVersionResource, dynObjects []runtime.Object, objects ...runtime.Object) (*KubernetesProvider, *dynamicfake.FakeDynamicClient) {
	kinds := autoscalingListKinds()
	kinds[triggerAuthenticationGVRs[0]] = "TriggerAuthenticationList"
	k, cs, dyn := newFakeFullProvider(kinds, dynObjects, objects...)
	byGV := map[string]*metav1.APIResourceList{}
	for _, g := range served {
		gv := g.GroupVersion().String()
		if byGV[gv] == nil {
			byGV[gv] = &metav1.APIResourceList{GroupVersion: gv}
			cs.Resources = append(cs.Resources, byGV[gv])
		}
		byGV[gv].APIResources = append(byGV[gv].APIResources, metav1.APIResource{Name: g.Resource, Namespaced: true})
	}
	return k, dyn
}

func dynObj(gvr schema.GroupVersionResource, kind, ns, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(gvr.GroupVersion().String())
	u.SetKind(kind)
	u.SetNamespace(ns)
	u.SetName(name)
	return u
}

func cpuHPA(ns, name, kind, target string) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: kind, Name: target, APIVersion: "apps/v1"},
			MaxReplicas:    5,
			Metrics: []autoscalingv2.MetricSpec{{Type: autoscalingv2.ResourceMetricSourceType, Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU, Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: i32(70)}}}},
		},
	}
}

func TestKubernetesCreateVPA(t *testing.T) {
	ctx := context.Background()
	k, dyn := autoscalerProvider([]schema.GroupVersionResource{vpaGVRs[0], vpaGVRs[1]}, nil, cpuHPA("commerce", "checkout-hpa", "Deployment", "checkout"))
	res, err := k.CreateVPA(ctx, baseVPA())
	if err != nil || !res.OK || len(res.Created) != 1 || res.Created[0].APIVersion != "autoscaling.k8s.io/v1" {
		t.Fatalf("create: %+v %v", res, err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "VPA and HPA on the same resource conflict") {
		t.Fatalf("HPA conflict warning: %v", res.Warnings)
	}
	got, err := dyn.Resource(vpaGVRs[0]).Namespace("commerce").Get(ctx, "checkout-vpa", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	mode, _, _ := unstructured.NestedString(got.Object, "spec", "updatePolicy", "updateMode")
	pol, _, _ := unstructured.NestedSlice(got.Object, "spec", "resourcePolicy", "containerPolicies")
	if mode != "Off" || len(pol) != 1 || got.GetLabels()[autoscalerManagedByLabel] != autoscalerManagedByValue {
		t.Fatalf("stored: %+v", got.Object)
	}

	// Second create: 409.
	if _, err := k.CreateVPA(ctx, baseVPA()); HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("duplicate: %v", err)
	}

	// Delete, then 404.
	if err := k.DeleteVPA(ctx, "commerce", "checkout-vpa"); err != nil {
		t.Fatal(err)
	}
	if err := k.DeleteVPA(ctx, "commerce", "checkout-vpa"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("delete missing: %v", err)
	}
	// Invalid requests never reach the cluster.
	bad := baseVPA()
	bad.UpdateMode = "Sometimes"
	if _, err := k.CreateVPA(ctx, bad); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("bad: %v", err)
	}
}

func TestKubernetesCreateVPAFallsBackToV1beta2(t *testing.T) {
	ctx := context.Background()
	k, dyn := autoscalerProvider([]schema.GroupVersionResource{vpaGVRs[1]}, nil)
	res, err := k.CreateVPA(ctx, baseVPA())
	if err != nil || res.Created[0].APIVersion != "autoscaling.k8s.io/v1beta2" {
		t.Fatalf("create: %+v %v", res, err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("no HPA, Off mode: no warnings, got %v", res.Warnings)
	}
	got, err := dyn.Resource(vpaGVRs[1]).Namespace("commerce").Get(ctx, "checkout-vpa", metav1.GetOptions{})
	if err != nil || got.GetAPIVersion() != "autoscaling.k8s.io/v1beta2" {
		t.Fatalf("stored at v1beta2: %v %v", got, err)
	}
	if err := k.DeleteVPA(ctx, "commerce", "checkout-vpa"); err != nil {
		t.Fatal(err)
	}
}

func TestKubernetesAutoscalerCRDAbsent(t *testing.T) {
	ctx := context.Background()
	k, dyn := autoscalerProvider(nil, nil)
	if _, err := k.CreateVPA(ctx, baseVPA()); HTTPStatus(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("vpa create without CRD: %v", err)
	}
	if err := k.DeleteVPA(ctx, "commerce", "x"); HTTPStatus(err) != http.StatusUnprocessableEntity {
		t.Fatalf("vpa delete without CRD: %v", err)
	}
	if _, err := k.CreateScaledObject(ctx, baseSO()); HTTPStatus(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "KEDA is not installed") {
		t.Fatalf("so create without CRD: %v", err)
	}
	if err := k.DeleteScaledObject(ctx, "payments", "x"); HTTPStatus(err) != http.StatusUnprocessableEntity {
		t.Fatalf("so delete without CRD: %v", err)
	}
	for _, a := range dyn.Actions() {
		if a.GetVerb() == "create" || a.GetVerb() == "delete" {
			t.Fatalf("touched the cluster: %v", a)
		}
	}
	dyn.PrependReactor("list", "*", notFoundReactor(func(schema.GroupVersionResource) bool { return true }))
	tas, err := k.ListTriggerAuthentications(ctx, "payments")
	if err != nil || tas.Installed || tas.Items == nil {
		t.Fatalf("trigger auths without KEDA: %+v %v", tas, err)
	}
}

func TestKubernetesCreateScaledObject(t *testing.T) {
	ctx := context.Background()
	ta := triggerAuthenticationGVRs[0]
	k, dyn := autoscalerProvider([]schema.GroupVersionResource{scaledObjectGVRs[0], ta},
		[]runtime.Object{dynObj(ta, "TriggerAuthentication", "payments", "prom-auth"), dynObj(ta, "TriggerAuthentication", "other", "kafka-auth")},
		cpuHPA("payments", "payments-hpa", "Deployment", "payments-api"),
		cpuHPA("payments", "keda-hpa-legacy", "Deployment", "payments-api"))

	// Missing TriggerAuthentication: 400, nothing created.
	missing := baseSO()
	missing.Triggers[1].AuthenticationRef = "kafka-auth" // exists, but in another namespace
	if _, err := k.CreateScaledObject(ctx, missing); HTTPStatus(err) != http.StatusBadRequest ||
		err.Error() != "TriggerAuthentication kafka-auth not found in namespace payments" {
		t.Fatalf("missing auth: %v", err)
	}

	res, err := k.CreateScaledObject(ctx, baseSO())
	if err != nil || !res.OK || res.Created[0] != (AutoscalerObjectRef{APIVersion: "keda.sh/v1alpha1", Kind: "ScaledObject", Namespace: "payments", Name: "payments-api-scaler"}) {
		t.Fatalf("create: %+v %v", res, err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "KEDA creates its own HPA") || !strings.Contains(res.Warnings[0], "payments-hpa") {
		t.Fatalf("HPA warning (KEDA-owned HPA ignored): %v", res.Warnings)
	}
	got, err := dyn.Resource(scaledObjectGVRs[0]).Namespace("payments").Get(ctx, "payments-api-scaler", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	minR, _, _ := unstructured.NestedInt64(got.Object, "spec", "minReplicaCount")
	trig, _, _ := unstructured.NestedSlice(got.Object, "spec", "triggers")
	if minR != 0 || len(trig) != 2 {
		t.Fatalf("stored: %+v", got.Object)
	}
	// The read side lists it with type and name only.
	list, err := k.ListScaledObjects(ctx, "payments")
	if err != nil || len(list.Items) != 1 || list.Items[0].Triggers[0].Name != "business-hours" {
		t.Fatalf("list: %+v %v", list, err)
	}
	b, _ := json.Marshal(list)
	if strings.Contains(string(b), "prometheus.monitoring") || strings.Contains(string(b), "prom-auth") {
		t.Fatalf("list leaks settings: %s", b)
	}

	if _, err := k.CreateScaledObject(ctx, baseSO()); HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("duplicate: %v", err)
	}
	if err := k.DeleteScaledObject(ctx, "payments", "payments-api-scaler"); err != nil {
		t.Fatal(err)
	}
	if err := k.DeleteScaledObject(ctx, "payments", "payments-api-scaler"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestKubernetesListTriggerAuthenticationsNamesOnly(t *testing.T) {
	ta := triggerAuthenticationGVRs[0]
	withSpec := dynObj(ta, "TriggerAuthentication", "payments", "prom-auth")
	_ = unstructured.SetNestedSlice(withSpec.Object, []any{map[string]any{"parameter": "bearerToken", "name": "prom-secret", "key": "token"}}, "spec", "secretTargetRef")
	k, _ := autoscalerProvider(nil, []runtime.Object{withSpec, dynObj(ta, "TriggerAuthentication", "payments", "kafka-auth"), dynObj(ta, "TriggerAuthentication", "data", "redis-auth")})
	list, err := k.ListTriggerAuthentications(context.Background(), "payments")
	if err != nil || !list.Installed || len(list.Items) != 2 || list.Items[0].Name != "kafka-auth" || list.Items[1].Kind != "TriggerAuthentication" {
		t.Fatalf("list: %+v %v", list, err)
	}
	b, _ := json.Marshal(list)
	if strings.Contains(string(b), "prom-secret") || strings.Contains(string(b), "bearerToken") {
		t.Fatalf("leaks spec: %s", b)
	}
}
