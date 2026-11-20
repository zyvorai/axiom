package platform

// Types for the autoscaling views beyond CPU: multi-metric HPA detail, the
// read-only VerticalPodAutoscaler view and the read-only KEDA ScaledObject
// view. HPASummary / CreateHPARequest themselves stay in types.go.

// HPAMetric is one autoscaling/v2 metric of an HPA, rendered for display.
type HPAMetric struct {
	Type       string `json:"type"`       // Resource | ContainerResource | Pods | Object | External
	Name       string `json:"name"`       // cpu | memory | <metric name>
	TargetType string `json:"targetType"` // Utilization | AverageValue | Value
	Target     string `json:"target"`     // "60%", "500Mi", "100"
	Current    string `json:"current"`    // "" while the HPA has no status for it
}

// AutoscalerCondition is a status condition of an HPA, VPA or ScaledObject.
type AutoscalerCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// ResourceAmounts is a cpu/memory pair as Kubernetes quantity strings.
type ResourceAmounts struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// VPAContainerRecommendation is the recommender's output for one container.
type VPAContainerRecommendation struct {
	Name           string          `json:"name"`
	LowerBound     ResourceAmounts `json:"lowerBound"`
	Target         ResourceAmounts `json:"target"`
	UpperBound     ResourceAmounts `json:"upperBound"`
	UncappedTarget ResourceAmounts `json:"uncappedTarget"`
}

// VPASummary is one autoscaling.k8s.io VerticalPodAutoscaler.
type VPASummary struct {
	Namespace  string                       `json:"namespace"`
	Name       string                       `json:"name"`
	TargetKind string                       `json:"targetKind"`
	TargetName string                       `json:"targetName"`
	UpdateMode string                       `json:"updateMode"` // Off | Initial | Recreate | Auto
	Containers []VPAContainerRecommendation `json:"containers"`
	Conditions []AutoscalerCondition        `json:"conditions"`
	Age        string                       `json:"age"`
}

// VPAList is the /api/v1/vpas response. Installed is false (with no items)
// when the VPA CRD is not present in the cluster.
type VPAList struct {
	Installed bool         `json:"installed"`
	Items     []VPASummary `json:"items"`
}

// ScaledObjectTrigger is a KEDA trigger reduced to its type and a display
// name. Trigger metadata and authentication references are deliberately NOT
// carried: they can contain endpoints and credentials.
type ScaledObjectTrigger struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// ScaledObjectSummary is one keda.sh ScaledObject.
type ScaledObjectSummary struct {
	Namespace       string                `json:"namespace"`
	Name            string                `json:"name"`
	TargetKind      string                `json:"targetKind"`
	TargetName      string                `json:"targetName"`
	MinReplicas     int32                 `json:"minReplicas"`
	MaxReplicas     int32                 `json:"maxReplicas"`
	CurrentReplicas int32                 `json:"currentReplicas"`
	Triggers        []ScaledObjectTrigger `json:"triggers"`
	Ready           bool                  `json:"ready"`
	Active          bool                  `json:"active"`
	Fallback        bool                  `json:"fallback"`
	Conditions      []AutoscalerCondition `json:"conditions"`
	Age             string                `json:"age"`
}

// ScaledObjectList is the /api/v1/scaledobjects response. Installed is false
// (with no items) when the KEDA CRD is not present in the cluster.
type ScaledObjectList struct {
	Installed bool                  `json:"installed"`
	Items     []ScaledObjectSummary `json:"items"`
}
