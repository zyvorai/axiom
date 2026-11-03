//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clienttesting "k8s.io/client-go/testing"
)

func autoscalingListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		vpaGVRs[0]:          "VerticalPodAutoscalerList",
		vpaGVRs[1]:          "VerticalPodAutoscalerList",
		scaledObjectGVRs[0]: "ScaledObjectList",
	}
}

func notFoundReactor(match func(schema.GroupVersionResource) bool) clienttesting.ReactionFunc {
	return func(a clienttesting.Action) (bool, runtime.Object, error) {
		gvr := a.GetResource()
		if !match(gvr) {
			return false, nil, nil
		}
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource}, "")
	}
}

func TestKubernetesListHPAsV2MultiMetric(t *testing.T) {
	scaled := metav1.NewTime(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	window := int32(300)
	h := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "web", APIVersion: "apps/v1"},
			MinReplicas:    i32(2), MaxReplicas: 10,
			Metrics: []autoscalingv2.MetricSpec{
				{Type: autoscalingv2.ResourceMetricSourceType, Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU, Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: i32(60)}}},
				{Type: autoscalingv2.ResourceMetricSourceType, Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceMemory, Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: resourcePtr("500Mi")}}},
				{Type: autoscalingv2.ExternalMetricSourceType, External: &autoscalingv2.ExternalMetricSource{
					Metric: autoscalingv2.MetricIdentifier{Name: "queue_depth"}, Target: autoscalingv2.MetricTarget{Type: autoscalingv2.ValueMetricType, Value: resourcePtr("100")}}},
				{Type: autoscalingv2.PodsMetricSourceType, Pods: &autoscalingv2.PodsMetricSource{
					Metric: autoscalingv2.MetricIdentifier{Name: "rps"}, Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: resourcePtr("1k")}}},
			},
			Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{ScaleDown: &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: &window}},
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 4, DesiredReplicas: 5, LastScaleTime: &scaled,
			CurrentMetrics: []autoscalingv2.MetricStatus{
				{Type: autoscalingv2.ResourceMetricSourceType, Resource: &autoscalingv2.ResourceMetricStatus{
					Name: corev1.ResourceCPU, Current: autoscalingv2.MetricValueStatus{AverageUtilization: i32(42), AverageValue: resourcePtr("120m")}}},
				{Type: autoscalingv2.ResourceMetricSourceType, Resource: &autoscalingv2.ResourceMetricStatus{
					Name: corev1.ResourceMemory, Current: autoscalingv2.MetricValueStatus{AverageValue: resourcePtr("320Mi")}}},
				{Type: autoscalingv2.ExternalMetricSourceType, External: &autoscalingv2.ExternalMetricStatus{
					Metric: autoscalingv2.MetricIdentifier{Name: "queue_depth"}, Current: autoscalingv2.MetricValueStatus{Value: resourcePtr("37")}}},
			},
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{Type: autoscalingv2.AbleToScale, Status: corev1.ConditionTrue, Reason: "ReadyForNewScale", Message: "ok"},
				{Type: autoscalingv2.ScalingActive, Status: corev1.ConditionTrue, Reason: "ValidMetricFound", Message: "m"},
				{Type: autoscalingv2.ScalingLimited, Status: corev1.ConditionFalse, Reason: "DesiredWithinRange", Message: "r"},
			},
		},
	}
	bare := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: "shop"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "x"}, MaxReplicas: 3,
			Metrics: []autoscalingv2.MetricSpec{{Type: autoscalingv2.ResourceMetricSourceType, Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory, Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: i32(70)}}}}},
	}
	k, _ := newFakeKubernetesProvider(h, bare)
	list, err := k.ListHPAs(context.Background(), "shop")
	if err != nil || len(list) != 2 {
		t.Fatalf("ListHPAs: %+v %v", list, err)
	}
	got, nb := list[1], list[0] // sorted by name: new, web
	want := []HPAMetric{
		{Type: "Resource", Name: "cpu", TargetType: "Utilization", Target: "60%", Current: "42%"},
		{Type: "Resource", Name: "memory", TargetType: "AverageValue", Target: "500Mi", Current: "320Mi"},
		{Type: "External", Name: "queue_depth", TargetType: "Value", Target: "100", Current: "37"},
		{Type: "Pods", Name: "rps", TargetType: "AverageValue", Target: "1k", Current: ""},
	}
	if len(got.Metrics) != len(want) {
		t.Fatalf("metrics: %+v", got.Metrics)
	}
	for i := range want {
		if got.Metrics[i] != want[i] {
			t.Errorf("metric %d = %+v, want %+v", i, got.Metrics[i], want[i])
		}
	}
	if got.TargetCPUPercent != 60 || got.CurrentCPUPercent != 42 || got.CurrentReplicas != 4 || got.DesiredReplicas != 5 ||
		got.LastScaleTime != "2026-10-01T12:00:00Z" || !got.Behavior || got.MinReplicas != 2 {
		t.Fatalf("summary: %+v", got)
	}
	if len(got.Conditions) != 3 || got.Conditions[0] != (AutoscalerCondition{Type: "AbleToScale", Status: "True", Reason: "ReadyForNewScale", Message: "ok"}) ||
		got.Conditions[2].Type != "ScalingLimited" || got.Conditions[2].Status != "False" {
		t.Fatalf("conditions: %+v", got.Conditions)
	}
	// No status yet: current is empty, legacy CPU fields stay zero, arrays are not nil.
	if nb.Name != "new" || len(nb.Metrics) != 1 || nb.Metrics[0].Current != "" || nb.Metrics[0].Target != "70%" ||
		nb.TargetCPUPercent != 0 || nb.Conditions == nil || nb.LastScaleTime != "" || nb.Behavior || nb.MinReplicas != 1 {
		t.Fatalf("bare HPA: %+v", nb)
	}
	b, _ := json.Marshal(nb)
	for _, k := range []string{`"metrics":[`, `"conditions":[]`, `"lastScaleTime":""`, `"behavior":false`, `"desiredReplicas":0`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("JSON missing %s: %s", k, b)
		}
	}
}

func resourcePtr(s string) *resource.Quantity { q := resource.MustParse(s); return &q }

func TestKubernetesCreateHPAMemoryCPUAndStabilization(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	sum, err := k.CreateHPA(ctx, CreateHPARequest{Namespace: "shop", Name: "web", TargetKind: "Deployment", TargetName: "web",
		MinReplicas: 2, MaxReplicas: 8, TargetCPUPercent: 65, TargetMemoryPercent: 75, ScaleDownStabilizationSeconds: 600})
	if err != nil {
		t.Fatal(err)
	}
	hpa, err := cs.AutoscalingV2().HorizontalPodAutoscalers("shop").Get(ctx, "web", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hpa.Spec.Metrics) != 2 {
		t.Fatalf("metrics: %+v", hpa.Spec.Metrics)
	}
	for i, want := range []struct {
		name corev1.ResourceName
		pct  int32
	}{{corev1.ResourceCPU, 65}, {corev1.ResourceMemory, 75}} {
		m := hpa.Spec.Metrics[i]
		if m.Type != autoscalingv2.ResourceMetricSourceType || m.Resource == nil || m.Resource.Name != want.name ||
			m.Resource.Target.Type != autoscalingv2.UtilizationMetricType || *m.Resource.Target.AverageUtilization != want.pct {
			t.Fatalf("metric %d: %+v", i, m)
		}
	}
	if hpa.Spec.Behavior == nil || hpa.Spec.Behavior.ScaleDown == nil || *hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds != 600 || hpa.Spec.Behavior.ScaleUp != nil {
		t.Fatalf("behavior: %+v", hpa.Spec.Behavior)
	}
	if sum.TargetCPUPercent != 65 || len(sum.Metrics) != 2 || sum.Metrics[1].Name != "memory" || !sum.Behavior {
		t.Fatalf("summary: %+v", sum)
	}

	// Memory only, no stabilization: one metric and no behavior block.
	if _, err := k.CreateHPA(ctx, CreateHPARequest{Namespace: "shop", Name: "mem", TargetKind: "StatefulSet", TargetName: "db", TargetMemoryPercent: 80}); err != nil {
		t.Fatal(err)
	}
	mem, _ := cs.AutoscalingV2().HorizontalPodAutoscalers("shop").Get(ctx, "mem", metav1.GetOptions{})
	if len(mem.Spec.Metrics) != 1 || mem.Spec.Metrics[0].Resource.Name != corev1.ResourceMemory || mem.Spec.Behavior != nil {
		t.Fatalf("memory-only spec: %+v", mem.Spec)
	}

	// Validation failures never reach the API server.
	for name, req := range map[string]CreateHPARequest{
		"kind":      {Namespace: "shop", Name: "a", TargetKind: "DaemonSet", TargetName: "x"},
		"memory":    {Namespace: "shop", Name: "b", TargetKind: "Deployment", TargetName: "x", TargetMemoryPercent: 101},
		"window":    {Namespace: "shop", Name: "c", TargetKind: "Deployment", TargetName: "x", ScaleDownStabilizationSeconds: 4000},
		"max":       {Namespace: "shop", Name: "d", TargetKind: "Deployment", TargetName: "x", MaxReplicas: 5000},
		"name":      {Namespace: "shop", Name: "Bad_Name", TargetKind: "Deployment", TargetName: "x"},
		"namespace": {Namespace: "Shop", Name: "e", TargetKind: "Deployment", TargetName: "x"},
	} {
		_, err := k.CreateHPA(ctx, req)
		if HTTPStatus(err) != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %v", name, err)
		}
	}
	all, _ := cs.AutoscalingV2().HorizontalPodAutoscalers(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(all.Items) != 2 {
		t.Fatalf("only the two valid HPAs may exist, got %d", len(all.Items))
	}
}

func vpaObject(apiVersion, ns, name, mode string) runtime.Object {
	spec := map[string]any{"targetRef": map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "web"}}
	if mode != "" {
		spec["updatePolicy"] = map[string]any{"updateMode": mode}
	}
	return newUnstructured(apiVersion, "VerticalPodAutoscaler", ns, name, map[string]any{
		"spec": spec,
		"status": map[string]any{
			"recommendation": map[string]any{"containerRecommendations": []any{map[string]any{
				"containerName":  "web",
				"lowerBound":     map[string]any{"cpu": "100m", "memory": "262144k"},
				"target":         map[string]any{"cpu": "250m", "memory": "512Mi"},
				"upperBound":     map[string]any{"cpu": "1", "memory": int64(1073741824)},
				"uncappedTarget": map[string]any{"cpu": "250m", "memory": "512Mi"},
			}}},
			"conditions": []any{map[string]any{"type": "RecommendationProvided", "status": "True", "reason": "", "message": ""}},
		},
	})
}

func TestKubernetesListVPAsV1(t *testing.T) {
	k, _, _ := newFakeFullProvider(autoscalingListKinds(), []runtime.Object{
		vpaObject("autoscaling.k8s.io/v1", "shop", "web-vpa", "Auto"),
		vpaObject("autoscaling.k8s.io/v1", "other", "x-vpa", ""),
	})
	v, err := k.ListVPAs(context.Background(), "shop")
	if err != nil || !v.Installed || len(v.Items) != 1 {
		t.Fatalf("ListVPAs: %+v %v", v, err)
	}
	it := v.Items[0]
	if it.TargetKind != "Deployment" || it.TargetName != "web" || it.UpdateMode != "Auto" || len(it.Containers) != 1 || len(it.Conditions) != 1 {
		t.Fatalf("vpa: %+v", it)
	}
	c := it.Containers[0]
	if c.Name != "web" || c.Target != (ResourceAmounts{CPU: "250m", Memory: "512Mi"}) || c.LowerBound.CPU != "100m" ||
		c.UpperBound != (ResourceAmounts{CPU: "1", Memory: "1Gi"}) || c.UncappedTarget.CPU != "250m" || c.LowerBound.Memory == "" {
		t.Fatalf("recommendation: %+v", c)
	}
	all, _ := k.ListVPAs(context.Background(), "all")
	if len(all.Items) != 2 || all.Items[0].Namespace != "other" || all.Items[0].UpdateMode != "Auto" {
		t.Fatalf("all namespaces (sorted; default mode Auto): %+v", all.Items)
	}
}

func TestKubernetesListVPAsFallsBackToV1beta2(t *testing.T) {
	k, _, dyn := newFakeFullProvider(autoscalingListKinds(), []runtime.Object{
		vpaObject("autoscaling.k8s.io/v1beta2", "shop", "old-vpa", "Off"),
	})
	dyn.PrependReactor("list", "verticalpodautoscalers", notFoundReactor(func(g schema.GroupVersionResource) bool { return g.Version == "v1" }))
	v, err := k.ListVPAs(context.Background(), "shop")
	if err != nil || !v.Installed || len(v.Items) != 1 || v.Items[0].Name != "old-vpa" || v.Items[0].UpdateMode != "Off" {
		t.Fatalf("v1beta2 fallback: %+v %v", v, err)
	}
}

func TestKubernetesAutoscalingCRDsAbsent(t *testing.T) {
	k, _, dyn := newFakeFullProvider(autoscalingListKinds(), nil)
	dyn.PrependReactor("list", "*", notFoundReactor(func(schema.GroupVersionResource) bool { return true }))
	v, err := k.ListVPAs(context.Background(), "")
	if err != nil || v.Installed || v.Items == nil || len(v.Items) != 0 {
		t.Fatalf("VPA CRD absent: %+v %v", v, err)
	}
	s, err := k.ListScaledObjects(context.Background(), "")
	if err != nil || s.Installed || s.Items == nil || len(s.Items) != 0 {
		t.Fatalf("KEDA CRD absent: %+v %v", s, err)
	}
	if b, _ := json.Marshal(s); string(b) != `{"installed":false,"items":[]}` {
		t.Fatalf("JSON: %s", b)
	}
	// A real failure (not a missing CRD) is still an error.
	k2, _, dyn2 := newFakeFullProvider(autoscalingListKinds(), nil)
	dyn2.PrependReactor("list", "*", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "keda.sh", Resource: "scaledobjects"}, "", nil)
	})
	if _, err := k2.ListScaledObjects(context.Background(), ""); err == nil {
		t.Fatal("Forbidden must surface as an error")
	}
}

func TestKubernetesListScaledObjectsExcludesCredentials(t *testing.T) {
	so := newUnstructured("keda.sh/v1alpha1", "ScaledObject", "shop", "orders", map[string]any{
		"spec": map[string]any{
			"scaleTargetRef":  map[string]any{"name": "orders-worker"},
			"minReplicaCount": int64(1),
			"maxReplicaCount": int64(30),
			"triggers": []any{
				map[string]any{
					"type":              "rabbitmq",
					"metadata":          map[string]any{"queueName": "orders", "host": "amqp://admin:s3cr3t-host@rabbit:5672", "password": "hunter2-password"},
					"authenticationRef": map[string]any{"name": "rabbit-trigger-auth-ref"},
				},
				map[string]any{"type": "prometheus", "name": "rps-trigger", "metadata": map[string]any{"serverAddress": "http://prom.internal:9090", "query": "sum(rate(x[1m]))"}},
				map[string]any{"type": "cpu", "metadata": map[string]any{"value": "60"}},
			},
		},
		"status": map[string]any{
			"hpaName": "keda-hpa-orders",
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True", "reason": "ScaledObjectReady", "message": "ready"},
				map[string]any{"type": "Active", "status": "True", "reason": "ScalerActive", "message": "active"},
				map[string]any{"type": "Fallback", "status": "False", "reason": "NoFallbackFound", "message": ""},
			},
		},
	})
	idle := newUnstructured("keda.sh/v1alpha1", "ScaledObject", "shop", "idle", map[string]any{
		"spec": map[string]any{"scaleTargetRef": map[string]any{"kind": "StatefulSet", "name": "db"}, "triggers": []any{map[string]any{"type": "cron"}}},
	})
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "keda-hpa-orders", Namespace: "shop"},
		Status:     autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: 7},
	}
	k, _, _ := newFakeFullProvider(autoscalingListKinds(), []runtime.Object{so, idle}, hpa)
	list, err := k.ListScaledObjects(context.Background(), "shop")
	if err != nil || !list.Installed || len(list.Items) != 2 {
		t.Fatalf("ListScaledObjects: %+v %v", list, err)
	}
	idleSO, o := list.Items[0], list.Items[1]
	if o.Name != "orders" || o.TargetKind != "Deployment" || o.TargetName != "orders-worker" || o.MinReplicas != 1 || o.MaxReplicas != 30 ||
		o.CurrentReplicas != 7 || !o.Ready || !o.Active || o.Fallback || len(o.Conditions) != 3 {
		t.Fatalf("orders: %+v", o)
	}
	wantTriggers := []ScaledObjectTrigger{{Type: "rabbitmq", Name: "orders"}, {Type: "prometheus", Name: "rps-trigger"}, {Type: "cpu", Name: ""}}
	if len(o.Triggers) != 3 {
		t.Fatalf("triggers: %+v", o.Triggers)
	}
	for i := range wantTriggers {
		if o.Triggers[i] != wantTriggers[i] {
			t.Errorf("trigger %d = %+v, want %+v", i, o.Triggers[i], wantTriggers[i])
		}
	}
	if idleSO.TargetKind != "StatefulSet" || idleSO.MinReplicas != 0 || idleSO.MaxReplicas != 100 || idleSO.Ready || idleSO.Active || idleSO.CurrentReplicas != 0 {
		t.Fatalf("defaults: %+v", idleSO)
	}
	b, _ := json.Marshal(list)
	for _, secret := range []string{"hunter2", "s3cr3t", "amqp://", "rabbit-trigger-auth-ref", "authenticationRef", "prom.internal", "sum(rate", "password", "metadata", "serverAddress"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("ScaledObject JSON leaks %q: %s", secret, b)
		}
	}
}
