package platform

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestArgoRolloutMapping(t *testing.T) {
	it := unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "web", "namespace": "shop"},
		"spec": map[string]any{
			"replicas": int64(5),
			"strategy": map[string]any{"canary": map[string]any{"steps": []any{
				map[string]any{"setWeight": int64(20)}, map[string]any{"pause": map[string]any{}},
				map[string]any{"setWeight": int64(50)}}}},
			"template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"image": "web:2"}}}},
		},
		"status": map[string]any{
			"phase": "Paused", "currentStepIndex": int64(2), "updatedReplicas": int64(5), "readyReplicas": int64(4),
			"availableReplicas": int64(4), "stableRS": "aaa", "currentPodHash": "bbb",
			"pauseConditions": []any{map[string]any{"reason": "CanaryPauseStep"}},
		},
	}}
	r := argoRollout(it, "web-bbb-1", "Successful", "1d")
	if r.Strategy != "canary" || r.Phase != "Paused" || r.Weight != 20 || r.TotalSteps != 3 || r.CurrentStep != 2 ||
		r.Desired != 5 || r.Ready != 4 || r.PauseReason != "CanaryPauseStep" || r.Images[0] != "web:2" ||
		r.StableRevision != "aaa" || r.AnalysisResult != "Successful" {
		t.Fatalf("rollout: %+v", r)
	}
	bg := argoRollout(unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "g"},
		"spec":     map[string]any{"paused": true, "strategy": map[string]any{"blueGreen": map[string]any{}}},
	}}, "", "", "")
	if bg.Strategy != "blueGreen" || bg.Desired != 1 || bg.Phase != "Progressing" || bg.PauseReason != "Paused" {
		t.Fatalf("bluegreen: %+v", bg)
	}
	if canaryStepWeight([]any{map[string]any{"setWeight": int64(10)}}, 1) != 100 {
		t.Fatal("finished steps should be 100")
	}
}

func TestFlaggerAndAnalysis(t *testing.T) {
	c := flaggerCanary(unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "c", "namespace": "n"},
		"spec":     map[string]any{"targetRef": map[string]any{"name": "app"}},
		"status": map[string]any{"phase": "Progressing", "canaryWeight": int64(30), "failedChecks": int64(1),
			"lastTransitionTime": "2026-01-01T00:00:00Z"},
	}})
	if c.Target != "app" || c.CanaryWeight != 30 || c.FailedChecks != 1 || c.Phase != "Progressing" {
		t.Fatalf("canary: %+v", c)
	}
	mk := func(name, phase string, at time.Time) unstructured.Unstructured {
		u := unstructured.Unstructured{Object: map[string]any{"status": map[string]any{"phase": phase}}}
		u.SetName(name)
		u.SetNamespace("n")
		u.SetCreationTimestamp(metav1.NewTime(at))
		u.SetOwnerReferences([]metav1.OwnerReference{{Kind: "Rollout", Name: "web"}})
		return u
	}
	now := time.Now()
	got := latestAnalysisRuns([]unstructured.Unstructured{mk("old", "Failed", now.Add(-time.Hour)), mk("new", "Successful", now)})
	if got["n/web"] != [2]string{"new", "Successful"} {
		t.Fatalf("analysis: %v", got)
	}
}
