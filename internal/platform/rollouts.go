package platform

import (
	"context"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ArgoRollout is one argoproj.io Rollout (read-only).
type ArgoRollout struct {
	Namespace      string   `json:"namespace"`
	Name           string   `json:"name"`
	Strategy       string   `json:"strategy"` // canary | blueGreen | unknown
	Phase          string   `json:"phase"`    // Healthy | Progressing | Paused | Degraded
	Message        string   `json:"message,omitempty"`
	PauseReason    string   `json:"pauseReason,omitempty"`
	CurrentStep    int      `json:"currentStep"`
	TotalSteps     int      `json:"totalSteps"`
	Weight         int      `json:"weight"`
	Desired        int64    `json:"desired"`
	Updated        int64    `json:"updated"`
	Ready          int64    `json:"ready"`
	Available      int64    `json:"available"`
	StableRevision string   `json:"stableRevision,omitempty"`
	CanaryRevision string   `json:"canaryRevision,omitempty"`
	Images         []string `json:"images"` // desired (canary) pod template images
	AnalysisRun    string   `json:"analysisRun,omitempty"`
	AnalysisResult string   `json:"analysisResult,omitempty"` // Successful | Failed | Running | Error | Inconclusive
	Age            string   `json:"age,omitempty"`
}

// FlaggerCanary is one flagger.app Canary (read-only).
type FlaggerCanary struct {
	Namespace          string `json:"namespace"`
	Name               string `json:"name"`
	Target             string `json:"target,omitempty"`
	Phase              string `json:"phase"`
	CanaryWeight       int64  `json:"canaryWeight"`
	FailedChecks       int64  `json:"failedChecks"`
	Iterations         int64  `json:"iterations"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

// RolloutsInventory is GET /api/v1/rollouts.
type RolloutsInventory struct {
	Available bool            `json:"available"`
	Note      string          `json:"note,omitempty"`
	Rollouts  []ArgoRollout   `json:"rollouts"`
	Canaries  []FlaggerCanary `json:"canaries"`
}

// ListRollouts returns the read-only progressive-delivery inventory.
func ListRollouts(ctx context.Context, p Provider, namespace string) (RolloutsInventory, error) {
	if l, ok := p.(interface {
		ListRollouts(context.Context, string) (RolloutsInventory, error)
	}); ok {
		return l.ListRollouts(ctx, namespace)
	}
	return emptyRollouts("Progressive delivery view not available on this provider"), nil
}

func emptyRollouts(note string) RolloutsInventory {
	return RolloutsInventory{Note: note, Rollouts: []ArgoRollout{}, Canaries: []FlaggerCanary{}}
}

// argoRollout maps a Rollout object; the latest AnalysisRun and age are supplied by the caller.
func argoRollout(it unstructured.Unstructured, analysisRun, analysisResult, age string) ArgoRollout {
	o := it.Object
	str := func(path ...string) string { v, _, _ := unstructured.NestedString(o, path...); return v }
	num := func(path ...string) int64 { v, _, _ := unstructured.NestedFieldNoCopy(o, path...); return toInt64(v) }
	r := ArgoRollout{
		Namespace: it.GetNamespace(), Name: it.GetName(), Strategy: "unknown",
		Phase: str("status", "phase"), Message: str("status", "message"),
		Desired: 1, Updated: num("status", "updatedReplicas"), Ready: num("status", "readyReplicas"),
		Available: num("status", "availableReplicas"), Images: []string{},
		StableRevision: str("status", "stableRS"), CanaryRevision: str("status", "currentPodHash"),
		AnalysisRun: analysisRun, AnalysisResult: analysisResult, Age: age,
	}
	if v, found, _ := unstructured.NestedFieldNoCopy(o, "spec", "replicas"); found {
		r.Desired = toInt64(v)
	}
	if _, found, _ := unstructured.NestedMap(o, "spec", "strategy", "blueGreen"); found {
		r.Strategy = "blueGreen"
	}
	if _, found, _ := unstructured.NestedMap(o, "spec", "strategy", "canary"); found {
		r.Strategy = "canary"
		steps, _, _ := unstructured.NestedSlice(o, "spec", "strategy", "canary", "steps")
		r.TotalSteps = len(steps)
		r.CurrentStep = int(num("status", "currentStepIndex"))
		if v, found, _ := unstructured.NestedFieldNoCopy(o, "status", "canary", "weights", "canary", "weight"); found {
			r.Weight = int(toInt64(v))
		} else {
			r.Weight = canaryStepWeight(steps, r.CurrentStep)
		}
	}
	if r.Phase == "" {
		r.Phase = "Progressing"
	}
	if pcs, _, _ := unstructured.NestedSlice(o, "status", "pauseConditions"); len(pcs) > 0 {
		if m, ok := pcs[0].(map[string]any); ok {
			r.PauseReason, _ = m["reason"].(string)
		}
	}
	if b, _, _ := unstructured.NestedBool(o, "spec", "paused"); b && r.PauseReason == "" {
		r.PauseReason = "Paused"
	}
	cs, _, _ := unstructured.NestedSlice(o, "spec", "template", "spec", "containers")
	for _, c := range cs {
		if m, ok := c.(map[string]any); ok {
			if img, _ := m["image"].(string); img != "" {
				r.Images = append(r.Images, img)
			}
		}
	}
	return r
}

// canaryStepWeight is the last setWeight among the steps already completed (before currentStepIndex);
// past the last step the canary is at 100.
func canaryStepWeight(steps []any, current int) int {
	if len(steps) > 0 && current >= len(steps) {
		return 100
	}
	w := 0
	for i := 0; i < current && i < len(steps); i++ {
		if m, ok := steps[i].(map[string]any); ok {
			if v, found := m["setWeight"]; found {
				w = int(toInt64(v))
			}
		}
	}
	return w
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

// flaggerCanary maps a Flagger Canary object.
func flaggerCanary(it unstructured.Unstructured) FlaggerCanary {
	str := func(path ...string) string { v, _, _ := unstructured.NestedString(it.Object, path...); return v }
	num := func(path ...string) int64 {
		v, _, _ := unstructured.NestedFieldNoCopy(it.Object, path...)
		return toInt64(v)
	}
	c := FlaggerCanary{
		Namespace: it.GetNamespace(), Name: it.GetName(),
		Target:       str("spec", "targetRef", "name"),
		Phase:        str("status", "phase"),
		CanaryWeight: num("status", "canaryWeight"), FailedChecks: num("status", "failedChecks"),
		Iterations:         num("status", "iterations"),
		LastTransitionTime: str("status", "lastTransitionTime"),
	}
	if c.Phase == "" {
		c.Phase = "Unknown"
	}
	return c
}

// latestAnalysisRuns maps "ns/rollout" to the newest owned AnalysisRun's name and phase.
func latestAnalysisRuns(items []unstructured.Unstructured) map[string][2]string {
	out := map[string][2]string{}
	newest := map[string]string{}
	for _, it := range items {
		for _, ref := range it.GetOwnerReferences() {
			if ref.Kind != "Rollout" {
				continue
			}
			key := it.GetNamespace() + "/" + ref.Name
			ts := it.GetCreationTimestamp().Time.UTC().Format("20060102150405.000000000")
			if ts >= newest[key] {
				newest[key] = ts
				phase, _, _ := unstructured.NestedString(it.Object, "status", "phase")
				out[key] = [2]string{it.GetName(), phase}
			}
		}
	}
	return out
}

func sortRollouts(inv *RolloutsInventory) {
	sort.Slice(inv.Rollouts, func(i, j int) bool {
		a, b := inv.Rollouts[i], inv.Rollouts[j]
		if a.Namespace != b.Namespace {
			return strings.Compare(a.Namespace, b.Namespace) < 0
		}
		return a.Name < b.Name
	})
	sort.Slice(inv.Canaries, func(i, j int) bool {
		a, b := inv.Canaries[i], inv.Canaries[j]
		if a.Namespace != b.Namespace {
			return strings.Compare(a.Namespace, b.Namespace) < 0
		}
		return a.Name < b.Name
	})
}

func (d *DemoProvider) ListRollouts(ctx context.Context, namespace string) (RolloutsInventory, error) {
	return RolloutsInventory{
		Available: true,
		Note:      "Demo progressive delivery (simulated)",
		Rollouts: []ArgoRollout{
			{Namespace: "shop", Name: "checkout", Strategy: "canary", Phase: "Paused", PauseReason: "CanaryPauseStep",
				CurrentStep: 2, TotalSteps: 6, Weight: 20, Desired: 5, Updated: 5, Ready: 5, Available: 5,
				StableRevision: "6d4cf56db6", CanaryRevision: "7f9b8c4d5", Images: []string{"shop/checkout:2.4.0"},
				AnalysisRun: "checkout-7f9b8c4d5-2", AnalysisResult: "Successful", Age: "12d"},
			{Namespace: "shop", Name: "frontend", Strategy: "blueGreen", Phase: "Healthy",
				Desired: 3, Updated: 3, Ready: 3, Available: 3, StableRevision: "5c8d7f9b66", CanaryRevision: "5c8d7f9b66",
				Images: []string{"shop/frontend:1.9.2"}, Age: "30d"},
			{Namespace: "payments", Name: "ledger", Strategy: "canary", Phase: "Degraded", Message: "RolloutAborted: analysis failed",
				CurrentStep: 1, TotalSteps: 4, Weight: 0, Desired: 4, Updated: 1, Ready: 4, Available: 4,
				StableRevision: "84c6d5b7f", CanaryRevision: "9a1e2f3c4", Images: []string{"payments/ledger:3.1.0"},
				AnalysisRun: "ledger-9a1e2f3c4-1", AnalysisResult: "Failed", Age: "55d"},
		},
		Canaries: []FlaggerCanary{
			{Namespace: "shop", Name: "search", Target: "search", Phase: "Progressing", CanaryWeight: 30, Iterations: 3, LastTransitionTime: "2026-11-13T09:12:00Z"},
			{Namespace: "shop", Name: "cart", Target: "cart", Phase: "Succeeded", LastTransitionTime: "2026-11-12T17:40:00Z"},
		},
	}, nil
}
