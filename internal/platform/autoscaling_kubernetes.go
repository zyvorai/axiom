//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"sort"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	vpaGVRs = []schema.GroupVersionResource{
		{Group: "autoscaling.k8s.io", Version: "v1", Resource: "verticalpodautoscalers"},
		{Group: "autoscaling.k8s.io", Version: "v1beta2", Resource: "verticalpodautoscalers"},
	}
	scaledObjectGVRs = []schema.GroupVersionResource{
		{Group: "keda.sh", Version: "v1alpha1", Resource: "scaledobjects"},
	}
)

// --- HPA (autoscaling/v2) ---

func (k *KubernetesProvider) ListHPAs(ctx context.Context, ns string) ([]HPASummary, error) {
	list, err := k.client.AutoscalingV2().HorizontalPodAutoscalers(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]HPASummary, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, hpaSummaryFromV2(&list.Items[i]))
	}
	sortHPASummaries(out)
	return out, nil
}

func hpaSummaryFromV2(h *autoscalingv2.HorizontalPodAutoscaler) HPASummary {
	minReplicas := int32(1)
	if h.Spec.MinReplicas != nil {
		minReplicas = *h.Spec.MinReplicas
	}
	s := HPASummary{
		Namespace: h.Namespace, Name: h.Name,
		TargetKind: h.Spec.ScaleTargetRef.Kind, TargetName: h.Spec.ScaleTargetRef.Name,
		MinReplicas: minReplicas, MaxReplicas: h.Spec.MaxReplicas,
		CurrentReplicas: h.Status.CurrentReplicas,
		DesiredReplicas: h.Status.DesiredReplicas,
		Age:             age(h.CreationTimestamp.Time),
		Metrics:         []HPAMetric{},
		Conditions:      []AutoscalerCondition{},
		Behavior:        h.Spec.Behavior != nil && (h.Spec.Behavior.ScaleUp != nil || h.Spec.Behavior.ScaleDown != nil),
	}
	if h.Status.LastScaleTime != nil {
		s.LastScaleTime = h.Status.LastScaleTime.Time.UTC().Format(time.RFC3339)
	}
	current := map[string]autoscalingv2.MetricValueStatus{}
	for _, m := range h.Status.CurrentMetrics {
		if key, v, ok := hpaStatusMetric(m); ok {
			current[key] = v
		}
	}
	for _, m := range h.Spec.Metrics {
		key, name, target, ok := hpaSpecMetric(m)
		if !ok {
			continue
		}
		dm := HPAMetric{Type: string(m.Type), Name: name, TargetType: string(target.Type), Target: renderMetricTarget(target)}
		if v, ok := current[key]; ok {
			dm.Current = renderMetricCurrent(target.Type, v)
		}
		s.Metrics = append(s.Metrics, dm)
		if m.Type == autoscalingv2.ResourceMetricSourceType && name == string(corev1.ResourceCPU) && target.AverageUtilization != nil {
			s.TargetCPUPercent = *target.AverageUtilization
			if v, ok := current[key]; ok && v.AverageUtilization != nil {
				s.CurrentCPUPercent = *v.AverageUtilization
			}
		}
	}
	for _, c := range h.Status.Conditions {
		s.Conditions = append(s.Conditions, AutoscalerCondition{Type: string(c.Type), Status: string(c.Status), Reason: c.Reason, Message: c.Message})
	}
	return s
}

// hpaSpecMetric returns the match key, display name and target of a spec metric.
func hpaSpecMetric(m autoscalingv2.MetricSpec) (key, name string, target autoscalingv2.MetricTarget, ok bool) {
	switch m.Type {
	case autoscalingv2.ResourceMetricSourceType:
		if m.Resource != nil {
			return "Resource/" + string(m.Resource.Name), string(m.Resource.Name), m.Resource.Target, true
		}
	case autoscalingv2.ContainerResourceMetricSourceType:
		if m.ContainerResource != nil {
			return "ContainerResource/" + m.ContainerResource.Container + "/" + string(m.ContainerResource.Name), string(m.ContainerResource.Name), m.ContainerResource.Target, true
		}
	case autoscalingv2.PodsMetricSourceType:
		if m.Pods != nil {
			return "Pods/" + m.Pods.Metric.Name, m.Pods.Metric.Name, m.Pods.Target, true
		}
	case autoscalingv2.ObjectMetricSourceType:
		if m.Object != nil {
			return "Object/" + m.Object.DescribedObject.Kind + "/" + m.Object.DescribedObject.Name + "/" + m.Object.Metric.Name, m.Object.Metric.Name, m.Object.Target, true
		}
	case autoscalingv2.ExternalMetricSourceType:
		if m.External != nil {
			return "External/" + m.External.Metric.Name, m.External.Metric.Name, m.External.Target, true
		}
	}
	return "", "", autoscalingv2.MetricTarget{}, false
}

func hpaStatusMetric(m autoscalingv2.MetricStatus) (string, autoscalingv2.MetricValueStatus, bool) {
	switch m.Type {
	case autoscalingv2.ResourceMetricSourceType:
		if m.Resource != nil {
			return "Resource/" + string(m.Resource.Name), m.Resource.Current, true
		}
	case autoscalingv2.ContainerResourceMetricSourceType:
		if m.ContainerResource != nil {
			return "ContainerResource/" + m.ContainerResource.Container + "/" + string(m.ContainerResource.Name), m.ContainerResource.Current, true
		}
	case autoscalingv2.PodsMetricSourceType:
		if m.Pods != nil {
			return "Pods/" + m.Pods.Metric.Name, m.Pods.Current, true
		}
	case autoscalingv2.ObjectMetricSourceType:
		if m.Object != nil {
			return "Object/" + m.Object.DescribedObject.Kind + "/" + m.Object.DescribedObject.Name + "/" + m.Object.Metric.Name, m.Object.Current, true
		}
	case autoscalingv2.ExternalMetricSourceType:
		if m.External != nil {
			return "External/" + m.External.Metric.Name, m.External.Current, true
		}
	}
	return "", autoscalingv2.MetricValueStatus{}, false
}

func renderMetricTarget(t autoscalingv2.MetricTarget) string {
	switch t.Type {
	case autoscalingv2.UtilizationMetricType:
		if t.AverageUtilization != nil {
			return fmt.Sprintf("%d%%", *t.AverageUtilization)
		}
	case autoscalingv2.AverageValueMetricType:
		if t.AverageValue != nil {
			return t.AverageValue.String()
		}
	case autoscalingv2.ValueMetricType:
		if t.Value != nil {
			return t.Value.String()
		}
	}
	return ""
}

// renderMetricCurrent renders the status value in the same unit as the target
// (utilization as a percentage, otherwise a quantity).
func renderMetricCurrent(tt autoscalingv2.MetricTargetType, v autoscalingv2.MetricValueStatus) string {
	switch {
	case tt == autoscalingv2.UtilizationMetricType && v.AverageUtilization != nil:
		return fmt.Sprintf("%d%%", *v.AverageUtilization)
	case tt == autoscalingv2.ValueMetricType && v.Value != nil:
		return v.Value.String()
	case v.AverageValue != nil:
		return v.AverageValue.String()
	case v.Value != nil:
		return v.Value.String()
	case v.AverageUtilization != nil:
		return fmt.Sprintf("%d%%", *v.AverageUtilization)
	}
	return ""
}

func (k *KubernetesProvider) CreateHPA(ctx context.Context, req CreateHPARequest) (HPASummary, error) {
	req, err := normalizeCreateHPARequest(req)
	if err != nil {
		return HPASummary{}, err
	}
	minReplicas := req.MinReplicas
	metrics := []autoscalingv2.MetricSpec{}
	addUtil := func(name corev1.ResourceName, pct int32) {
		p := pct
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name:   name,
				Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: &p},
			},
		})
	}
	if req.TargetCPUPercent > 0 {
		addUtil(corev1.ResourceCPU, req.TargetCPUPercent)
	}
	if req.TargetMemoryPercent > 0 {
		addUtil(corev1.ResourceMemory, req.TargetMemoryPercent)
	}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: req.TargetKind, Name: req.TargetName, APIVersion: "apps/v1",
			},
			MinReplicas: &minReplicas,
			MaxReplicas: req.MaxReplicas,
			Metrics:     metrics,
		},
	}
	if req.ScaleDownStabilizationSeconds > 0 {
		w := req.ScaleDownStabilizationSeconds
		hpa.Spec.Behavior = &autoscalingv2.HorizontalPodAutoscalerBehavior{
			ScaleDown: &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: &w},
		}
	}
	created, err := k.client.AutoscalingV2().HorizontalPodAutoscalers(req.Namespace).Create(ctx, hpa, metav1.CreateOptions{})
	if err != nil {
		return HPASummary{}, err
	}
	return hpaSummaryFromV2(created), nil
}

// --- VPA (autoscaling.k8s.io, read-only) ---

func (k *KubernetesProvider) ListVPAs(ctx context.Context, ns string) (VPAList, error) {
	empty := VPAList{Installed: false, Items: []VPASummary{}}
	if k.dynamic == nil {
		return empty, nil
	}
	list, _, err := k.listFirstGVR(ctx, vpaGVRs, ns)
	if err != nil {
		if isMissingCRD(err) {
			return empty, nil
		}
		return VPAList{}, err
	}
	out := VPAList{Installed: true, Items: make([]VPASummary, 0, len(list.Items))}
	for _, item := range list.Items {
		out.Items = append(out.Items, vpaFromUnstructured(item))
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.Namespace == b.Namespace {
			return a.Name < b.Name
		}
		return a.Namespace < b.Namespace
	})
	return out, nil
}

func vpaFromUnstructured(item unstructured.Unstructured) VPASummary {
	o := item.Object
	kind, _, _ := unstructured.NestedString(o, "spec", "targetRef", "kind")
	name, _, _ := unstructured.NestedString(o, "spec", "targetRef", "name")
	mode, _, _ := unstructured.NestedString(o, "spec", "updatePolicy", "updateMode")
	if mode == "" {
		mode = "Auto" // the VPA API default
	}
	v := VPASummary{
		Namespace: item.GetNamespace(), Name: item.GetName(),
		TargetKind: kind, TargetName: name, UpdateMode: mode,
		Containers: []VPAContainerRecommendation{},
		Conditions: unstructuredConditions(o),
		Age:        age(item.GetCreationTimestamp().Time),
	}
	recs, _, _ := unstructured.NestedSlice(o, "status", "recommendation", "containerRecommendations")
	for _, r := range recs {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		cn, _ := m["containerName"].(string)
		v.Containers = append(v.Containers, VPAContainerRecommendation{
			Name:           cn,
			LowerBound:     resourceAmounts(m["lowerBound"]),
			Target:         resourceAmounts(m["target"]),
			UpperBound:     resourceAmounts(m["upperBound"]),
			UncappedTarget: resourceAmounts(m["uncappedTarget"]),
		})
	}
	return v
}

func resourceAmounts(v any) ResourceAmounts {
	m, _ := v.(map[string]any)
	return ResourceAmounts{
		CPU:    quantityString(m["cpu"], resource.DecimalSI),
		Memory: quantityString(m["memory"], resource.BinarySI),
	}
}

// quantityString renders an unstructured quantity (string or number) as a
// canonical Kubernetes quantity string; "" when absent or unparsable.
func quantityString(v any, numFormat resource.Format) string {
	switch t := v.(type) {
	case string:
		q, err := resource.ParseQuantity(t)
		if err != nil {
			return ""
		}
		return q.String()
	case int64:
		return resource.NewQuantity(t, numFormat).String()
	case int:
		return resource.NewQuantity(int64(t), numFormat).String()
	case float64:
		return resource.NewMilliQuantity(int64(t*1000), numFormat).String()
	}
	return ""
}

func unstructuredConditions(o map[string]any) []AutoscalerCondition {
	out := []AutoscalerCondition{}
	conds, _, _ := unstructured.NestedSlice(o, "status", "conditions")
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		s := func(k string) string { v, _ := m[k].(string); return v }
		out = append(out, AutoscalerCondition{Type: s("type"), Status: s("status"), Reason: s("reason"), Message: s("message")})
	}
	return out
}

// --- KEDA ScaledObjects (keda.sh, read-only) ---

func (k *KubernetesProvider) ListScaledObjects(ctx context.Context, ns string) (ScaledObjectList, error) {
	empty := ScaledObjectList{Installed: false, Items: []ScaledObjectSummary{}}
	if k.dynamic == nil {
		return empty, nil
	}
	list, _, err := k.listFirstGVR(ctx, scaledObjectGVRs, ns)
	if err != nil {
		if isMissingCRD(err) {
			return empty, nil
		}
		return ScaledObjectList{}, err
	}
	// KEDA drives scaling through an HPA it owns; its status carries the live
	// replica count. A failure here only leaves currentReplicas at 0.
	hpaReplicas := map[string]int32{}
	if len(list.Items) > 0 {
		if hpas, herr := k.client.AutoscalingV2().HorizontalPodAutoscalers(nsSelector(ns)).List(ctx, metav1.ListOptions{}); herr == nil {
			for _, h := range hpas.Items {
				hpaReplicas[h.Namespace+"/"+h.Name] = h.Status.CurrentReplicas
			}
		}
	}
	out := ScaledObjectList{Installed: true, Items: make([]ScaledObjectSummary, 0, len(list.Items))}
	for _, item := range list.Items {
		so := scaledObjectFromUnstructured(item)
		hpaName, _, _ := unstructured.NestedString(item.Object, "status", "hpaName")
		if hpaName == "" {
			hpaName = "keda-hpa-" + item.GetName()
		}
		so.CurrentReplicas = hpaReplicas[item.GetNamespace()+"/"+hpaName]
		out.Items = append(out.Items, so)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.Namespace == b.Namespace {
			return a.Name < b.Name
		}
		return a.Namespace < b.Namespace
	})
	return out, nil
}

// scaledObjectFromUnstructured reads only the fields the view shows. Trigger
// metadata values and authenticationRef are never copied out (they can carry
// endpoints and credentials); kedaTriggerName reads identifier keys only.
func scaledObjectFromUnstructured(item unstructured.Unstructured) ScaledObjectSummary {
	o := item.Object
	kind, _, _ := unstructured.NestedString(o, "spec", "scaleTargetRef", "kind")
	if kind == "" {
		kind = "Deployment"
	}
	name, _, _ := unstructured.NestedString(o, "spec", "scaleTargetRef", "name")
	so := ScaledObjectSummary{
		Namespace: item.GetNamespace(), Name: item.GetName(),
		TargetKind: kind, TargetName: name,
		MinReplicas: unstructuredInt32(o, 0, "spec", "minReplicaCount"),
		MaxReplicas: unstructuredInt32(o, 100, "spec", "maxReplicaCount"),
		Triggers:    []ScaledObjectTrigger{},
		Conditions:  unstructuredConditions(o),
		Age:         age(item.GetCreationTimestamp().Time),
	}
	triggers, _, _ := unstructured.NestedSlice(o, "spec", "triggers")
	for _, t := range triggers {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		tn, _ := m["name"].(string)
		md, _ := m["metadata"].(map[string]any)
		so.Triggers = append(so.Triggers, ScaledObjectTrigger{Type: typ, Name: kedaTriggerName(tn, md)})
	}
	for _, c := range so.Conditions {
		isTrue := c.Status == "True"
		switch c.Type {
		case "Ready":
			so.Ready = isTrue
		case "Active":
			so.Active = isTrue
		case "Fallback":
			so.Fallback = isTrue
		}
	}
	return so
}

func unstructuredInt32(o map[string]any, def int32, fields ...string) int32 {
	v, found, err := unstructured.NestedFieldNoCopy(o, fields...)
	if !found || err != nil {
		return def
	}
	switch t := v.(type) {
	case int64:
		return int32(t)
	case int:
		return int32(t)
	case int32:
		return t
	case float64:
		return int32(t)
	}
	return def
}
