//go:build kubernetes

package platform

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	plantedSecretValue = "S3cr3t-Value-From-A-K8s-Secret-0001"
	plantedConfigValue = "ConfigMapValue-must-not-leak-0002"
	plantedEnvValue    = "env-literal-hunter2-0003"
	plantedEventToken  = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
)

func aiFakeCluster() []runtime.Object {
	now := metav1.Now()
	pod := func(ns, name string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, CreationTimestamp: now},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{{
					Name: "app", Image: "registry.example.com/app:1",
					Env: []corev1.EnvVar{
						{Name: "DB_PASSWORD", Value: plantedEnvValue},
						{Name: "FROM_SECRET", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "creds"}, Key: "pw"}}},
					},
				}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning, PodIP: "10.42.0.7",
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "app", RestartCount: 3,
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				}},
			},
		}
	}
	event := func(ns, name, msg string) *corev1.Event {
		return &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Namespace: ns, Name: name + "-ev"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: name, Namespace: ns},
			Type:           corev1.EventTypeWarning, Reason: "BackOff", Message: msg, Count: 4, LastTimestamp: now,
		}
	}
	return []runtime.Object{
		pod("team-a", "web-1"),
		pod("team-b", "other-1"),
		event("team-a", "web-1", "Back-off restarting failed container app; token="+plantedEventToken+" peer 10.42.0.9"),
		event("team-b", "other-1", "TEAM-B-ONLY-EVENT must never appear in team-a context"),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "team-a", Name: "creds"}, Data: map[string][]byte{"pw": []byte(plantedSecretValue)}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "team-a", Name: "cfg"}, Data: map[string]string{"k": plantedConfigValue}},
	}
}

func TestAIContextFromFakeClusterNeverContainsSecretsOrEnv(t *testing.T) {
	k, cs, _ := newFakeFullProvider(map[schema.GroupVersionResource]string{}, nil, aiFakeCluster()...)
	a := NewAIAssistant(k, Integrations{AIEnabled: true, AIModel: "m"}, true)
	res, err := a.Explain(context.Background(), AIExplainRequest{Namespace: "team-a", Kind: "pod", Name: "web-1", Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	p := res.Payload.Prompt
	for _, want := range []string{"Pod status", "Diagnosis", "Events", "restarts: 3", "CrashLoopBackOff", "Back-off restarting failed container app"} {
		if !strings.Contains(p, want) {
			t.Errorf("context missing %q:\n%s", want, p)
		}
	}
	for _, bad := range []string{plantedSecretValue, plantedConfigValue, plantedEnvValue, plantedEventToken, "10.42.0.9", "10.42.0.7", "TEAM-B-ONLY-EVENT", "other-1"} {
		if strings.Contains(p, bad) {
			t.Errorf("payload leaked %q:\n%s", bad, p)
		}
	}
	// The builder must not even read Secrets or ConfigMaps.
	for _, act := range cs.Actions() {
		if r := act.GetResource().Resource; r == "secrets" || r == "configmaps" {
			t.Errorf("context building touched %s (%s)", r, act.GetVerb())
		}
		if v := act.GetVerb(); v != "get" && v != "list" {
			t.Errorf("context building must be read-only, saw verb %q on %s", v, act.GetResource().Resource)
		}
	}
}

func TestAIContextOutboundBodyFromFakeClusterIsRedacted(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer srv.Close()
	k, _, _ := newFakeFullProvider(map[schema.GroupVersionResource]string{}, nil, aiFakeCluster()...)
	in := Integrations{AIEnabled: true, AIBaseURL: srv.URL, AIModel: "m", AIAPIKey: "sk-test-outbound-key-0000"}
	res, err := NewAIAssistant(k, in, false).Explain(context.Background(), AIExplainRequest{
		Namespace: "team-a", Kind: "event", Name: "Pod/web-1", Question: "why? password=" + plantedEnvValue,
	})
	if err != nil || res.Answer != "done" {
		t.Fatalf("explain: %v %+v", err, res)
	}
	for _, bad := range []string{plantedSecretValue, plantedConfigValue, plantedEnvValue, plantedEventToken, "10.42.0.9", "sk-test-outbound-key-0000"} {
		if strings.Contains(string(body), bad) {
			t.Errorf("outbound body leaked %q: %s", bad, body)
		}
	}
}

func TestAIEventForDeletedPodStillExplainsEvents(t *testing.T) {
	k, _, _ := newFakeFullProvider(map[schema.GroupVersionResource]string{}, nil, aiFakeCluster()...)
	a := NewAIAssistant(k, Integrations{AIEnabled: true, AIModel: "m"}, true)
	// Event kind: pod missing is tolerated.
	k2, _, _ := newFakeFullProvider(map[schema.GroupVersionResource]string{}, nil,
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "team-a", Name: "gone-ev"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "gone", Namespace: "team-a"},
			Type:           corev1.EventTypeWarning, Reason: "Killing", Message: "Stopping container app", Count: 1, LastTimestamp: metav1.Now(),
		})
	res, err := NewAIAssistant(k2, Integrations{AIEnabled: true, AIModel: "m"}, true).Explain(context.Background(),
		AIExplainRequest{Namespace: "team-a", Kind: "event", Name: "Pod/gone", Preview: true})
	if err != nil || !strings.Contains(res.Payload.Prompt, "Stopping container app") {
		t.Fatalf("event for deleted pod: %v %+v", err, res)
	}
	// Pod kind: a missing pod is a 404.
	if _, err := a.Explain(context.Background(), AIExplainRequest{Namespace: "team-a", Kind: "pod", Name: "gone", Preview: true}); err == nil || HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("missing pod should be 404, got %v", err)
	}
	// A pod that exists only in another namespace is not visible from team-a.
	if _, err := a.Explain(context.Background(), AIExplainRequest{Namespace: "team-a", Kind: "pod", Name: "other-1", Preview: true}); err == nil || HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("cross-namespace pod must be 404, got %v", err)
	}
}
