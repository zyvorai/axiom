package platform

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
)

// Shared (build-tag free) autoscaling helpers: CreateHPARequest validation and
// the KEDA trigger display-name rule, used by both the demo and the
// Kubernetes provider so they behave identically.

const (
	hpaMaxReplicasLimit    = 1000
	hpaMaxStabilizationSec = 3600
	hpaDefaultCPUPercent   = 80
)

var (
	hpaDNSLabelRE     = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	hpaDNSSubdomainRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	hpaTargetKinds    = map[string]bool{"Deployment": true, "StatefulSet": true, "ReplicaSet": true}
)

func hpaBadRequest(format string, a ...any) error {
	return NewStatusError(http.StatusBadRequest, fmt.Errorf(format, a...))
}

// normalizeCreateHPARequest validates req and fills the defaults the HPA
// dialog has always relied on: minReplicas 0 → 1, maxReplicas 0 → min, and a
// CPU target of 80% when neither a CPU nor a memory target is given.
func normalizeCreateHPARequest(req CreateHPARequest) (CreateHPARequest, error) {
	if req.Namespace == "" || req.Name == "" || req.TargetKind == "" || req.TargetName == "" {
		return req, hpaBadRequest("namespace, name, targetKind, and targetName are required")
	}
	if !hpaDNSLabelRE.MatchString(req.Namespace) {
		return req, hpaBadRequest("invalid namespace %q: must be a DNS-1123 label", req.Namespace)
	}
	if len(req.Name) > 253 || !hpaDNSSubdomainRE.MatchString(req.Name) {
		return req, hpaBadRequest("invalid name %q: must be a DNS-1123 subdomain", req.Name)
	}
	if len(req.TargetName) > 253 || !hpaDNSSubdomainRE.MatchString(req.TargetName) {
		return req, hpaBadRequest("invalid targetName %q: must be a DNS-1123 subdomain", req.TargetName)
	}
	if !hpaTargetKinds[req.TargetKind] {
		return req, hpaBadRequest("invalid targetKind %q: must be Deployment, StatefulSet or ReplicaSet", req.TargetKind)
	}
	if req.MinReplicas < 0 {
		return req, hpaBadRequest("minReplicas must be at least 1")
	}
	if req.MinReplicas == 0 {
		req.MinReplicas = 1
	}
	if req.MaxReplicas < 0 {
		return req, hpaBadRequest("maxReplicas must be at least minReplicas")
	}
	if req.MaxReplicas > 0 && req.MaxReplicas < req.MinReplicas {
		return req, hpaBadRequest("maxReplicas (%d) must be at least minReplicas (%d)", req.MaxReplicas, req.MinReplicas)
	}
	if req.MaxReplicas == 0 {
		req.MaxReplicas = req.MinReplicas
	}
	if req.MaxReplicas > hpaMaxReplicasLimit {
		return req, hpaBadRequest("maxReplicas must be at most %d", hpaMaxReplicasLimit)
	}
	if req.TargetCPUPercent < 0 || req.TargetCPUPercent > 100 {
		return req, hpaBadRequest("targetCpuPercent must be between 1 and 100")
	}
	if req.TargetMemoryPercent < 0 || req.TargetMemoryPercent > 100 {
		return req, hpaBadRequest("targetMemoryPercent must be between 1 and 100")
	}
	if req.TargetCPUPercent == 0 && req.TargetMemoryPercent == 0 {
		req.TargetCPUPercent = hpaDefaultCPUPercent
	}
	if req.ScaleDownStabilizationSeconds < 0 || req.ScaleDownStabilizationSeconds > hpaMaxStabilizationSec {
		return req, hpaBadRequest("scaleDownStabilizationSeconds must be between 0 and %d", hpaMaxStabilizationSec)
	}
	return req, nil
}

// hpaRequestMetrics is the display form of the metrics a (normalized) create
// request produces, in the order they are written to the spec.
func hpaRequestMetrics(req CreateHPARequest) []HPAMetric {
	out := []HPAMetric{}
	if req.TargetCPUPercent > 0 {
		out = append(out, HPAMetric{Type: "Resource", Name: "cpu", TargetType: "Utilization", Target: fmt.Sprintf("%d%%", req.TargetCPUPercent)})
	}
	if req.TargetMemoryPercent > 0 {
		out = append(out, HPAMetric{Type: "Resource", Name: "memory", TargetType: "Utilization", Target: fmt.Sprintf("%d%%", req.TargetMemoryPercent)})
	}
	return out
}

// kedaTriggerNameKeys are trigger-metadata keys that hold an identifier (not
// an endpoint or a credential), tried in order when a trigger has no name.
var kedaTriggerNameKeys = []string{"metricName", "queueName", "topic", "listName", "streamName", "subscriptionName"}

// kedaTriggerName is the display name for a KEDA trigger: its own `name`,
// else the first identifier-like metadata value, else "". No other metadata
// value is ever read out.
func kedaTriggerName(name string, metadata map[string]any) string {
	if name != "" {
		return name
	}
	for _, k := range kedaTriggerNameKeys {
		if v, ok := metadata[k].(string); ok && v != "" {
			if len(v) > 253 {
				v = v[:253]
			}
			return v
		}
	}
	return ""
}

func sortHPASummaries(out []HPASummary) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
}
