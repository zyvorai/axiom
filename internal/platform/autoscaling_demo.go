package platform

import (
	"context"
	"sort"
)

// Deterministic seed data for the demo VPA and KEDA views (the in-memory
// store in autoscaling_create_demo.go starts from it). The demo pods
// run with 500m CPU / 512Mi memory, so the Auto-mode checkout VPA below
// recommends far more than the workload currently asks for.

func demoVPAs() []VPASummary {
	return []VPASummary{
		{
			Namespace: "commerce", Name: "checkout-vpa", TargetKind: "Deployment", TargetName: "checkout", UpdateMode: "Auto",
			Containers: []VPAContainerRecommendation{{
				Name:           "checkout",
				LowerBound:     ResourceAmounts{CPU: "800m", Memory: "1Gi"},
				Target:         ResourceAmounts{CPU: "1200m", Memory: "1536Mi"},
				UpperBound:     ResourceAmounts{CPU: "2", Memory: "3Gi"},
				UncappedTarget: ResourceAmounts{CPU: "1200m", Memory: "1536Mi"},
			}},
			Conditions: []AutoscalerCondition{
				{Type: "RecommendationProvided", Status: "True", Reason: "", Message: ""},
			},
			Age: "9d",
		},
		{
			Namespace: "data", Name: "redis-vpa", TargetKind: "StatefulSet", TargetName: "redis", UpdateMode: "Off",
			Containers: []VPAContainerRecommendation{{
				Name:           "redis",
				LowerBound:     ResourceAmounts{CPU: "50m", Memory: "200Mi"},
				Target:         ResourceAmounts{CPU: "120m", Memory: "320Mi"},
				UpperBound:     ResourceAmounts{CPU: "400m", Memory: "640Mi"},
				UncappedTarget: ResourceAmounts{CPU: "120m", Memory: "320Mi"},
			}},
			Conditions: []AutoscalerCondition{
				{Type: "RecommendationProvided", Status: "True", Reason: "", Message: ""},
				{Type: "LowConfidence", Status: "True", Reason: "", Message: "Fewer than 8 days of history"},
			},
			Age: "4d",
		},
	}
}

func demoScaledObjects() []ScaledObjectSummary {
	return []ScaledObjectSummary{
		{
			Namespace: "data", Name: "redis-consumer", TargetKind: "StatefulSet", TargetName: "redis",
			MinReplicas: 3, MaxReplicas: 6, CurrentReplicas: 3,
			Triggers: []ScaledObjectTrigger{{Type: "redis", Name: "jobs"}, {Type: "cron", Name: ""}},
			Ready:    true, Active: false, Fallback: false,
			Conditions: []AutoscalerCondition{
				{Type: "Ready", Status: "True", Reason: "ScaledObjectReady", Message: "ScaledObject is defined correctly and is ready for scaling"},
				{Type: "Active", Status: "False", Reason: "ScalerNotActive", Message: "Scaling is not performed because triggers are not active"},
				{Type: "Fallback", Status: "False", Reason: "NoFallbackFound", Message: "No fallbacks are active on this scaled object"},
			},
			Age: "6d",
		},
		{
			Namespace: "payments", Name: "payments-api", TargetKind: "Deployment", TargetName: "payments-api",
			MinReplicas: 2, MaxReplicas: 20, CurrentReplicas: 12,
			Triggers: []ScaledObjectTrigger{{Type: "kafka", Name: "orders-lag"}, {Type: "prometheus", Name: "http_requests_per_second"}},
			Ready:    true, Active: true, Fallback: false,
			Conditions: []AutoscalerCondition{
				{Type: "Ready", Status: "True", Reason: "ScaledObjectReady", Message: "ScaledObject is defined correctly and is ready for scaling"},
				{Type: "Active", Status: "True", Reason: "ScalerActive", Message: "Scaling is performed because triggers are active"},
				{Type: "Fallback", Status: "False", Reason: "NoFallbackFound", Message: "No fallbacks are active on this scaled object"},
			},
			Age: "3d",
		},
	}
}

func demoNamespaceMatch(filter, ns string) bool {
	return filter == "" || filter == "all" || filter == ns
}

func (d *DemoProvider) ListVPAs(ctx context.Context, ns string) (VPAList, error) {
	demoAutoscalersMu.Lock()
	defer demoAutoscalersMu.Unlock()
	demoAutoscalersInitLocked()
	out := VPAList{Installed: true, Items: []VPASummary{}}
	for _, v := range demoVPAStore {
		if demoNamespaceMatch(ns, v.Namespace) {
			out.Items = append(out.Items, v)
		}
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

func (d *DemoProvider) ListScaledObjects(ctx context.Context, ns string) (ScaledObjectList, error) {
	demoAutoscalersMu.Lock()
	defer demoAutoscalersMu.Unlock()
	demoAutoscalersInitLocked()
	out := ScaledObjectList{Installed: true, Items: []ScaledObjectSummary{}}
	for _, s := range demoScaledObjectStore {
		if demoNamespaceMatch(ns, s.Namespace) {
			out.Items = append(out.Items, s)
		}
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
