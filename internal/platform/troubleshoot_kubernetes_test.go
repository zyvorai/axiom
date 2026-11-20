//go:build kubernetes

package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func tsPod(ns, name string, labels map[string]string, phase corev1.PodPhase, statuses ...corev1.ContainerStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Labels: labels},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "img:1"}}},
		Status:     corev1.PodStatus{Phase: phase, ContainerStatuses: statuses},
	}
}

func healthByName(r HealthReport, analyzer, name string) []HealthFinding {
	var out []HealthFinding
	for _, f := range r.Findings {
		if f.Analyzer == analyzer && f.Name == name {
			out = append(out, f)
		}
	}
	return out
}

func TestK8sHealthPodAnalyzer(t *testing.T) {
	oom := corev1.ContainerStatus{Name: "app", RestartCount: 6,
		State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}}}
	crash := corev1.ContainerStatus{Name: "app", RestartCount: 3,
		State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}}}
	pull := corev1.ContainerStatus{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "pull access denied"}}}
	oomRunning := corev1.ContainerStatus{Name: "app", RestartCount: 2, Ready: true,
		State:                corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}}}
	healthy := corev1.ContainerStatus{Name: "app", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}
	unsched := tsPod("a", "stuck", nil, corev1.PodPending)
	unsched.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "0/3 nodes are available: insufficient memory"}}
	done := tsPod("a", "job-pod", nil, corev1.PodSucceeded, crash) // finished pods are ignored

	k, _, _ := newFakeFullProvider(nil, nil,
		tsPod("a", "oom", nil, corev1.PodRunning, oom), tsPod("a", "crash", nil, corev1.PodRunning, crash),
		tsPod("a", "pull", nil, corev1.PodPending, pull), tsPod("a", "oomrun", nil, corev1.PodRunning, oomRunning),
		tsPod("a", "fine", nil, corev1.PodRunning, healthy), unsched, done,
		tsPod("b", "other-ns-crash", nil, corev1.PodRunning, crash))

	r, err := k.ClusterHealth(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if f := healthByName(r, AnalyzerPodStatus, "oom"); len(f) != 1 || f[0].Severity != SeverityCritical || !strings.Contains(f[0].Title, "OOMKilled") || !strings.Contains(f[0].Suggestion, "memory") {
		t.Fatalf("oom: %+v", f)
	}
	if f := healthByName(r, AnalyzerPodStatus, "crash"); len(f) != 1 || f[0].Severity != SeverityCritical || !strings.Contains(f[0].Message, "exit code 1") {
		t.Fatalf("crash: %+v", f)
	}
	if f := healthByName(r, AnalyzerPodStatus, "pull"); len(f) != 1 || !strings.Contains(f[0].Title, "Image pull") || !strings.Contains(f[0].Message, "img:1") {
		t.Fatalf("pull: %+v", f)
	}
	if f := healthByName(r, AnalyzerPodStatus, "oomrun"); len(f) != 1 || f[0].Severity != SeverityWarning {
		t.Fatalf("oomrun: %+v", f)
	}
	if f := healthByName(r, AnalyzerPodStatus, "stuck"); len(f) != 1 || !strings.Contains(f[0].Message, "insufficient memory") {
		t.Fatalf("unschedulable: %+v", f)
	}
	for _, name := range []string{"fine", "job-pod", "other-ns-crash"} {
		if f := healthByName(r, AnalyzerPodStatus, name); len(f) != 0 {
			t.Fatalf("%s must not be reported: %+v", name, f)
		}
	}
	if r.Status != "critical" || r.Counts[SeverityCritical] != 3 {
		t.Fatalf("status=%s counts=%v", r.Status, r.Counts)
	}
	// A namespaced report never includes node findings.
	for _, f := range r.Findings {
		if f.Analyzer == AnalyzerNodePressure {
			t.Fatalf("node finding in namespaced report: %+v", f)
		}
	}
}

func TestK8sHealthServiceEndpoints(t *testing.T) {
	svc := func(name string, sel map[string]string, typ corev1.ServiceType) *corev1.Service {
		return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: name}, Spec: corev1.ServiceSpec{Selector: sel, Type: typ}}
	}
	ep := func(name string, addrs int) *corev1.Endpoints {
		e := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: name}}
		if addrs > 0 {
			sub := corev1.EndpointSubset{}
			for i := 0; i < addrs; i++ {
				sub.Addresses = append(sub.Addresses, corev1.EndpointAddress{IP: fmt.Sprintf("10.0.0.%d", i+1)})
			}
			e.Subsets = []corev1.EndpointSubset{sub}
		}
		return e
	}
	k, _, _ := newFakeFullProvider(nil, nil,
		svc("healthy", map[string]string{"app": "h"}, corev1.ServiceTypeClusterIP), ep("healthy", 2),
		svc("nopods", map[string]string{"app": "ghost"}, corev1.ServiceTypeClusterIP), ep("nopods", 0),
		svc("notready", map[string]string{"app": "nr"}, corev1.ServiceTypeClusterIP), ep("notready", 0),
		tsPod("a", "nr-1", map[string]string{"app": "nr"}, corev1.PodRunning),
		svc("headless-manual", nil, corev1.ServiceTypeClusterIP), ep("headless-manual", 0),
		svc("external", map[string]string{"app": "x"}, corev1.ServiceTypeExternalName),
	)
	r, err := k.ClusterHealth(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if f := healthByName(r, AnalyzerServiceEndpoint, "nopods"); len(f) != 1 || !strings.Contains(f[0].Message, "no pod matches") {
		t.Fatalf("nopods: %+v", f)
	}
	if f := healthByName(r, AnalyzerServiceEndpoint, "notready"); len(f) != 1 || !strings.Contains(f[0].Message, "none are Ready") {
		t.Fatalf("notready: %+v", f)
	}
	for _, name := range []string{"healthy", "headless-manual", "external"} {
		if f := healthByName(r, AnalyzerServiceEndpoint, name); len(f) != 0 {
			t.Fatalf("%s must not be flagged: %+v", name, f)
		}
	}
}

func TestK8sHealthPVCs(t *testing.T) {
	wffc := storagev1.VolumeBindingWaitForFirstConsumer
	imm := storagev1.VolumeBindingImmediate
	pvc := func(name, class string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: name},
			Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: &class}, Status: corev1.PersistentVolumeClaimStatus{Phase: phase}}
	}
	k, _, _ := newFakeFullProvider(nil, nil,
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "lazy"}, VolumeBindingMode: &wffc},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "eager"}, VolumeBindingMode: &imm},
		pvc("stuck", "eager", corev1.ClaimPending), pvc("lazy-pvc", "lazy", corev1.ClaimPending), pvc("bound", "eager", corev1.ClaimBound))
	r, err := k.ClusterHealth(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if f := healthByName(r, AnalyzerPVC, "stuck"); len(f) != 1 || f[0].Severity != SeverityWarning {
		t.Fatalf("stuck: %+v", f)
	}
	if f := healthByName(r, AnalyzerPVC, "lazy-pvc"); len(f) != 1 || f[0].Severity != SeverityInfo {
		t.Fatalf("lazy: %+v", f)
	}
	if f := healthByName(r, AnalyzerPVC, "bound"); len(f) != 0 {
		t.Fatalf("bound PVC reported: %+v", f)
	}
}

func TestK8sHealthIngressBackends(t *testing.T) {
	ing := func(name, svc string, port networkingv1.ServiceBackendPort) *networkingv1.Ingress {
		return &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: name},
			Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{Path: "/", Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: svc, Port: port}}}}}}}}}}
	}
	web := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "web"},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 80}}}}
	k, _, _ := newFakeFullProvider(nil, nil, web,
		ing("ok-num", "web", networkingv1.ServiceBackendPort{Number: 80}),
		ing("ok-name", "web", networkingv1.ServiceBackendPort{Name: "http"}),
		ing("no-svc", "ghost", networkingv1.ServiceBackendPort{Number: 80}),
		ing("bad-port", "web", networkingv1.ServiceBackendPort{Number: 9999}))
	r, err := k.ClusterHealth(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if f := healthByName(r, AnalyzerIngress, "no-svc"); len(f) != 1 || f[0].Severity != SeverityCritical || !strings.Contains(f[0].Message, `"ghost"`) {
		t.Fatalf("no-svc: %+v", f)
	}
	if f := healthByName(r, AnalyzerIngress, "bad-port"); len(f) != 1 || f[0].Severity != SeverityWarning {
		t.Fatalf("bad-port: %+v", f)
	}
	for _, n := range []string{"ok-num", "ok-name"} {
		if f := healthByName(r, AnalyzerIngress, n); len(f) != 0 {
			t.Fatalf("%s flagged: %+v", n, f)
		}
	}
}

func TestK8sHealthHPAs(t *testing.T) {
	hpa := func(name, target string, max int32, cur, desired int32, conds ...autoscalingv2.HorizontalPodAutoscalerCondition) *autoscalingv2.HorizontalPodAutoscaler {
		return &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: name},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: target}, MaxReplicas: max},
			Status:     autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: cur, DesiredReplicas: desired, Conditions: conds},
		}
	}
	k, _, _ := newFakeFullProvider(nil, nil, newDeployment("a", "web", 2),
		hpa("missing-target", "ghost", 5, 1, 1),
		hpa("cannot-scale", "web", 5, 1, 1, autoscalingv2.HorizontalPodAutoscalerCondition{Type: autoscalingv2.AbleToScale, Status: corev1.ConditionFalse, Reason: "FailedGetScale", Message: "no scale subresource"}),
		hpa("no-metrics", "web", 5, 1, 1, autoscalingv2.HorizontalPodAutoscalerCondition{Type: autoscalingv2.ScalingActive, Status: corev1.ConditionFalse, Reason: "FailedGetResourceMetric", Message: "metrics API unavailable"}),
		hpa("maxed", "web", 4, 4, 4),
		hpa("fine", "web", 10, 2, 2, autoscalingv2.HorizontalPodAutoscalerCondition{Type: autoscalingv2.AbleToScale, Status: corev1.ConditionTrue}))
	r, err := k.ClusterHealth(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if f := healthByName(r, AnalyzerHPA, "missing-target"); len(f) != 1 || !strings.Contains(f[0].Title, "target not found") {
		t.Fatalf("missing-target: %+v", f)
	}
	if f := healthByName(r, AnalyzerHPA, "cannot-scale"); len(f) != 1 || !strings.Contains(f[0].Message, "FailedGetScale") {
		t.Fatalf("cannot-scale: %+v", f)
	}
	if f := healthByName(r, AnalyzerHPA, "no-metrics"); len(f) != 1 || !strings.Contains(f[0].Title, "metrics") {
		t.Fatalf("no-metrics: %+v", f)
	}
	if f := healthByName(r, AnalyzerHPA, "maxed"); len(f) != 1 || !strings.Contains(f[0].Title, "maximum") {
		t.Fatalf("maxed: %+v", f)
	}
	if f := healthByName(r, AnalyzerHPA, "fine"); len(f) != 0 {
		t.Fatalf("fine flagged: %+v", f)
	}
}

func TestK8sHealthNodePressureOnlyClusterWide(t *testing.T) {
	node := func(name string, conds ...corev1.NodeCondition) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Conditions: conds}}
	}
	k, _, _ := newFakeFullProvider(nil, nil,
		node("down", corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse}),
		node("disk", corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue}, corev1.NodeCondition{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue}),
		node("ok", corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue}, corev1.NodeCondition{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse}))
	r, err := k.ClusterHealth(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if f := healthByName(r, AnalyzerNodePressure, "down"); len(f) != 1 || f[0].Severity != SeverityCritical {
		t.Fatalf("down: %+v", f)
	}
	if f := healthByName(r, AnalyzerNodePressure, "disk"); len(f) != 1 || !strings.Contains(f[0].Title, "DiskPressure") {
		t.Fatalf("disk: %+v", f)
	}
	if f := healthByName(r, AnalyzerNodePressure, "ok"); len(f) != 0 {
		t.Fatalf("ok node flagged: %+v", f)
	}
	scoped, _ := k.ClusterHealth(context.Background(), "team-a")
	if len(scoped.Findings) != 0 {
		t.Fatalf("namespaced check must not report cluster-scoped nodes: %+v", scoped.Findings)
	}
}

func TestK8sHealthPartialAndTotalFailure(t *testing.T) {
	crash := corev1.ContainerStatus{Name: "app", RestartCount: 3,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}}
	k, cs, _ := newFakeFullProvider(nil, nil, tsPod("a", "crash", nil, corev1.PodRunning, crash))
	cs.PrependReactor("list", "horizontalpodautoscalers", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden: hpa list denied")
	})
	r, err := k.ClusterHealth(context.Background(), "a")
	if err != nil {
		t.Fatalf("one denied analyzer must not fail the check: %v", err)
	}
	if len(r.Errors) != 1 || !strings.HasPrefix(r.Errors[0], AnalyzerHPA) || healthByName(r, AnalyzerPodStatus, "crash") == nil {
		t.Fatalf("errors=%v findings=%+v", r.Errors, r.Findings)
	}

	k2, cs2, dyn2 := newFakeFullProvider(nil, nil)
	cs2.PrependReactor("list", "*", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("apiserver down")
	})
	// analyzeCertExpiry uses the dynamic client; deny those LISTs too so every
	// analyzer fails (CertificateList is registered on the fake by default).
	dyn2.PrependReactor("list", "*", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("apiserver down")
	})
	if _, err := k2.ClusterHealth(context.Background(), "a"); err == nil {
		t.Fatal("expected an error when every analyzer fails")
	}
}

// --- workload logs ---

func logReactor(cs *k8sfake.Clientset, fn func(opts *corev1.PodLogOptions) (string, error)) {
	cs.PrependReactor("get", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		opts := action.(clienttesting.GenericAction).GetValue().(*corev1.PodLogOptions)
		out, err := fn(opts)
		if err != nil {
			return true, nil, err
		}
		return true, &runtime.Unknown{Raw: []byte(out)}, nil
	})
}

func TestK8sWorkloadLogsMergesPodsAndContainers(t *testing.T) {
	dep := newDeployment("a", "web", 2)
	p1 := tsPod("a", "web-1", map[string]string{"app": "web"}, corev1.PodRunning)
	p2 := tsPod("a", "web-2", map[string]string{"app": "web"}, corev1.PodRunning)
	p2.Spec.Containers = append(p2.Spec.Containers, corev1.Container{Name: "sidecar", Image: "s:1"})
	other := tsPod("a", "other-1", map[string]string{"app": "other"}, corev1.PodRunning)
	k, cs, _ := newFakeFullProvider(nil, nil, dep, p1, p2, other)
	logReactor(cs, func(o *corev1.PodLogOptions) (string, error) {
		if o.Previous {
			return "", errors.New("previous terminated container not found")
		}
		if !o.Timestamps || o.TailLines == nil || *o.TailLines != 50 {
			return "", fmt.Errorf("unexpected options %+v", o)
		}
		if o.Container == "sidecar" {
			return "2026-01-01T00:00:02.000000000Z from sidecar\n", nil
		}
		return "2026-01-01T00:00:01.000000000Z from app\n2026-01-01T00:00:03.000000000Z later app\n", nil
	})
	rc, err := k.WorkloadLogs(context.Background(), "a", "Deployment", "web", WorkloadLogOptions{Tail: 50})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	want := []string{
		"[web-1/app] 2026-01-01T00:00:01.000000000Z from app",
		"[web-2/app] 2026-01-01T00:00:01.000000000Z from app",
		"[web-2/sidecar] 2026-01-01T00:00:02.000000000Z from sidecar",
		"[web-1/app] 2026-01-01T00:00:03.000000000Z later app",
		"[web-2/app] 2026-01-01T00:00:03.000000000Z later app",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("merged logs:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if strings.Contains(string(b), "other-1") {
		t.Fatal("pods outside the workload selector leaked into the stream")
	}

	rc, err = k.WorkloadLogs(context.Background(), "a", "Deployment", "web", WorkloadLogOptions{Tail: 50, Container: "sidecar"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(rc)
	rc.Close()
	if strings.TrimSpace(string(b)) != "[web-2/sidecar] 2026-01-01T00:00:02.000000000Z from sidecar" {
		t.Fatalf("container filter: %q", b)
	}

	if _, err := k.WorkloadLogs(context.Background(), "a", "Deployment", "web", WorkloadLogOptions{Tail: 50, Previous: true}); err == nil || !strings.Contains(err.Error(), "previous") {
		t.Fatalf("when no container has a previous instance the underlying error must surface, got %v", err)
	}
}

func TestK8sWorkloadLogsKindsAndErrors(t *testing.T) {
	sts := newStatefulSet("a", "db", 1)
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "migrate"},
		Spec: batchv1.JobSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"batch.kubernetes.io/job-name": "migrate"}}}}
	empty := newDeployment("a", "empty", 0)
	noSel := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "nosel"}}
	k, cs, _ := newFakeFullProvider(nil, nil, sts, job, empty, noSel,
		tsPod("a", "db-0", map[string]string{"app": "db"}, corev1.PodRunning),
		tsPod("a", "migrate-x", map[string]string{"batch.kubernetes.io/job-name": "migrate"}, corev1.PodSucceeded))
	logReactor(cs, func(*corev1.PodLogOptions) (string, error) { return "2026-01-01T00:00:00Z hello\n", nil })
	ctx := context.Background()
	for _, tc := range []struct{ kind, name string }{{"StatefulSet", "db"}, {"Job", "migrate"}} {
		rc, err := k.WorkloadLogs(ctx, "a", tc.kind, tc.name, WorkloadLogOptions{})
		if err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		if !strings.Contains(string(b), "] 2026-01-01T00:00:00Z hello") {
			t.Fatalf("%s: %q", tc.kind, b)
		}
	}
	for _, tc := range []struct {
		kind, name string
		want       int
	}{
		{"Deployment", "empty", 404}, {"Deployment", "ghost", 404}, {"CronJob", "x", 400}, {"Deployment", "nosel", 422},
	} {
		if _, err := k.WorkloadLogs(ctx, "a", tc.kind, tc.name, WorkloadLogOptions{}); HTTPStatus(err) != tc.want {
			t.Fatalf("%s/%s: status %d (%v), want %d", tc.kind, tc.name, HTTPStatus(err), err, tc.want)
		}
	}
}

func TestK8sWorkloadLogsCapsStreams(t *testing.T) {
	objs := []runtime.Object{newDeployment("a", "big", 60)}
	for i := 0; i < 60; i++ {
		objs = append(objs, tsPod("a", fmt.Sprintf("big-%02d", i), map[string]string{"app": "big"}, corev1.PodRunning))
	}
	k, cs, _ := newFakeFullProvider(nil, nil, objs...)
	logReactor(cs, func(*corev1.PodLogOptions) (string, error) { return "2026-01-01T00:00:00Z x\n", nil })
	rc, err := k.WorkloadLogs(context.Background(), "a", "Deployment", "big", WorkloadLogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if n := len(strings.Split(strings.TrimSpace(string(b)), "\n")); n != MaxWorkloadLogStreams {
		t.Fatalf("got %d streams, want the cap %d", n, MaxWorkloadLogStreams)
	}
}

// --- ephemeral debug containers ---

func TestK8sCreateDebugContainer(t *testing.T) {
	debugContainerWait = 0
	t.Setenv("AXIOM_DEBUG_IMAGE", "")
	k, cs, _ := newFakeFullProvider(nil, nil, tsPod("a", "web-1", nil, corev1.PodRunning))
	v, err := k.CreateDebugContainer(context.Background(), "a", "web-1", DebugRequest{Target: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Image != "busybox" || v.Target != "app" || !strings.HasPrefix(v.Container, "debugger-") || v.Ready {
		t.Fatalf("result %+v", v)
	}
	p, _ := cs.CoreV1().Pods("a").Get(context.Background(), "web-1", metav1.GetOptions{})
	if len(p.Spec.EphemeralContainers) != 1 {
		t.Fatalf("pod has %d ephemeral containers", len(p.Spec.EphemeralContainers))
	}
	ec := p.Spec.EphemeralContainers[0]
	sc := ec.SecurityContext
	if ec.Name != v.Container || ec.Image != "busybox" || ec.TargetContainerName != "app" || !ec.Stdin || !ec.TTY {
		t.Fatalf("ephemeral container %+v", ec)
	}
	if sc == nil || sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation || sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot ||
		sc.Capabilities == nil || len(sc.Capabilities.Drop) != 1 || sc.Capabilities.Drop[0] != "ALL" || sc.SeccompProfile == nil || sc.Privileged != nil {
		t.Fatalf("debug container must satisfy the restricted Pod Security profile, got %+v", sc)
	}
	var sawSubresource bool
	for _, a := range cs.Actions() {
		if a.GetVerb() == "update" && a.GetSubresource() == "ephemeralcontainers" {
			sawSubresource = true
		}
	}
	if !sawSubresource {
		t.Fatal("the container must be added through the pods/ephemeralcontainers subresource")
	}

	// A second debug container is appended, not replaced; AXIOM_DEBUG_IMAGE is honoured.
	t.Setenv("AXIOM_DEBUG_IMAGE", "nicolaka/netshoot:v0.13")
	v2, err := k.CreateDebugContainer(context.Background(), "a", "web-1", DebugRequest{})
	if err != nil || v2.Image != "nicolaka/netshoot:v0.13" || v2.Container == v.Container {
		t.Fatalf("second container: %+v err=%v", v2, err)
	}
	p, _ = cs.CoreV1().Pods("a").Get(context.Background(), "web-1", metav1.GetOptions{})
	if len(p.Spec.EphemeralContainers) != 2 {
		t.Fatalf("expected 2 ephemeral containers, got %d", len(p.Spec.EphemeralContainers))
	}
}

func TestK8sCreateDebugContainerValidation(t *testing.T) {
	debugContainerWait = 0
	done := tsPod("a", "done", nil, corev1.PodSucceeded)
	deleting := tsPod("a", "going", nil, corev1.PodRunning)
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	k, _, _ := newFakeFullProvider(nil, nil, tsPod("a", "web-1", nil, corev1.PodRunning), done, deleting)
	ctx := context.Background()
	if _, err := k.CreateDebugContainer(ctx, "a", "ghost", DebugRequest{}); HTTPStatus(err) != 404 {
		t.Fatalf("missing pod: %v", err)
	}
	if _, err := k.CreateDebugContainer(ctx, "a", "web-1", DebugRequest{Target: "nope"}); HTTPStatus(err) != 404 {
		t.Fatalf("unknown target: %v", err)
	}
	for _, name := range []string{"done", "going"} {
		if _, err := k.CreateDebugContainer(ctx, "a", name, DebugRequest{}); HTTPStatus(err) != 409 {
			t.Fatalf("%s: want 409, got %v", name, err)
		}
	}
}

func TestK8sCreateDebugContainerWaitsForRunning(t *testing.T) {
	debugContainerWait = 3 * time.Second
	t.Cleanup(func() { debugContainerWait = 0 })
	k, cs, _ := newFakeFullProvider(nil, nil, tsPod("a", "web-1", nil, corev1.PodRunning))
	cs.PrependReactor("update", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "ephemeralcontainers" {
			return false, nil, nil
		}
		pod := action.(clienttesting.UpdateAction).GetObject().(*corev1.Pod)
		for _, ec := range pod.Spec.EphemeralContainers {
			pod.Status.EphemeralContainerStatuses = append(pod.Status.EphemeralContainerStatuses,
				corev1.ContainerStatus{Name: ec.Name, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}})
		}
		return false, nil, nil
	})
	v, err := k.CreateDebugContainer(context.Background(), "a", "web-1", DebugRequest{})
	if err != nil || !v.Ready {
		t.Fatalf("expected a ready container, got %+v err=%v", v, err)
	}

	// An image that cannot be pulled is reported, not waited on forever.
	k2, cs2, _ := newFakeFullProvider(nil, nil, tsPod("a", "web-1", nil, corev1.PodRunning))
	cs2.PrependReactor("update", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "ephemeralcontainers" {
			return false, nil, nil
		}
		pod := action.(clienttesting.UpdateAction).GetObject().(*corev1.Pod)
		for _, ec := range pod.Spec.EphemeralContainers {
			pod.Status.EphemeralContainerStatuses = append(pod.Status.EphemeralContainerStatuses,
				corev1.ContainerStatus{Name: ec.Name, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}})
		}
		return false, nil, nil
	})
	v, err = k2.CreateDebugContainer(context.Background(), "a", "web-1", DebugRequest{})
	if err != nil || v.Ready || !strings.Contains(v.Message, "ImagePullBackOff") || !strings.Contains(v.Message, "AXIOM_DEBUG_IMAGE") {
		t.Fatalf("pull failure: %+v err=%v", v, err)
	}
}

// --- node shell ---

func nodeShellFixture(t *testing.T) (*KubernetesProvider, *k8sfake.Clientset) {
	t.Helper()
	t.Setenv("AXIOM_ENABLE_NODE_SHELL", "true")
	t.Setenv("AXIOM_NODE_SHELL_NAMESPACE", "")
	t.Setenv("AXIOM_NODE_SHELL_IMAGE", "")
	k, cs, _ := newFakeFullProvider(nil, nil, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}})
	// The fake has no kubelet: mark node-shell pods Running when created.
	cs.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		pod := action.(clienttesting.CreateAction).GetObject().(*corev1.Pod)
		pod.Status.Phase = corev1.PodRunning
		return false, nil, nil
	})
	return k, cs
}

func TestK8sNodeShellDisabledByDefaultCreatesNothing(t *testing.T) {
	t.Setenv("AXIOM_ENABLE_NODE_SHELL", "")
	k, cs, _ := newFakeFullProvider(nil, nil, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}})
	_, cleanup, err := k.prepareNodeShell(context.Background(), "worker-1", "admin", time.Minute)
	cleanup()
	if err == nil || HTTPStatus(err) != 403 || !strings.Contains(err.Error(), "AXIOM_ENABLE_NODE_SHELL") {
		t.Fatalf("expected a clear 403, got %v", err)
	}
	for _, a := range cs.Actions() {
		if a.GetVerb() == "create" {
			t.Fatalf("a disabled node shell must not create anything, saw %v", a)
		}
	}
}

func TestK8sNodeShellPrepareAndCleanup(t *testing.T) {
	k, cs := nodeShellFixture(t)
	ctx := context.Background()
	pod, cleanup, err := k.prepareNodeShell(ctx, "worker-1", "alice", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if pod.Namespace != DefaultNodeShellNamespace || !strings.HasPrefix(pod.Name, "axiom-node-shell-") {
		t.Fatalf("pod %s/%s", pod.Namespace, pod.Name)
	}
	spec := pod.Spec
	c := spec.Containers[0]
	if spec.NodeName != "worker-1" || !spec.HostPID || !spec.HostNetwork || c.SecurityContext == nil || c.SecurityContext.Privileged == nil || !*c.SecurityContext.Privileged {
		t.Fatalf("pod must be a privileged, hostPID pod pinned to the node: %+v", spec)
	}
	if spec.ActiveDeadlineSeconds == nil || *spec.ActiveDeadlineSeconds != int64((10*time.Minute+nodeShellGraceExtra).Seconds()) {
		t.Fatalf("activeDeadlineSeconds = %v", spec.ActiveDeadlineSeconds)
	}
	if spec.RestartPolicy != corev1.RestartPolicyNever || spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		t.Fatalf("restart/automount: %+v", spec)
	}
	if pod.Annotations["axiom.zyvor.dev/user"] != "alice" || pod.Labels[nodeShellLabel] != "true" {
		t.Fatalf("metadata %+v %+v", pod.Annotations, pod.Labels)
	}
	ns, err := cs.CoreV1().Namespaces().Get(ctx, DefaultNodeShellNamespace, metav1.GetOptions{})
	if err != nil || ns.Labels["pod-security.kubernetes.io/enforce"] != "privileged" {
		t.Fatalf("dedicated namespace must be created with the privileged PSA label: %v %+v", err, ns)
	}
	cleanup()
	if _, err := cs.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{}); err == nil {
		t.Fatal("cleanup must delete the privileged pod")
	}
	// The namespace is reused (no AlreadyExists failure) by the next session.
	if _, cleanup2, err := k.prepareNodeShell(ctx, "worker-1", "alice", time.Minute); err != nil {
		t.Fatal(err)
	} else {
		cleanup2()
	}
}

func TestK8sNodeShellUnknownNodeAndStartFailures(t *testing.T) {
	k, cs := nodeShellFixture(t)
	ctx := context.Background()
	if _, cleanup, err := k.prepareNodeShell(ctx, "ghost", "alice", time.Minute); HTTPStatus(err) != 404 {
		t.Fatalf("unknown node: %v", err)
	} else {
		cleanup()
	}
	if pods, _ := cs.CoreV1().Pods(NodeShellNamespace()).List(ctx, metav1.ListOptions{}); pods != nil && len(pods.Items) != 0 {
		t.Fatal("no pod may be created for an unknown node")
	}

	// A pod that never starts: 504 and the pod is removed.
	nodeShellStartTimeout = 100 * time.Millisecond
	t.Cleanup(func() { nodeShellStartTimeout = nodeShellStartWait })
	k2, cs2, _ := newFakeFullProvider(nil, nil, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}})
	if _, cleanup, err := k2.prepareNodeShell(ctx, "worker-1", "alice", time.Minute); HTTPStatus(err) != 504 {
		t.Fatalf("never-running pod: %v", err)
	} else {
		cleanup()
	}
	if pods, _ := cs2.CoreV1().Pods(NodeShellNamespace()).List(ctx, metav1.ListOptions{}); len(pods.Items) != 0 {
		t.Fatalf("timed-out node-shell pod was not deleted: %d left", len(pods.Items))
	}

	// ImagePullBackOff fails fast with a pointer to the image setting.
	k3, cs3, _ := newFakeFullProvider(nil, nil, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}})
	cs3.PrependReactor("create", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		pod := action.(clienttesting.CreateAction).GetObject().(*corev1.Pod)
		pod.Status.Phase = corev1.PodPending
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "shell", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}}
		return false, nil, nil
	})
	nodeShellStartTimeout = 5 * time.Second
	if _, cleanup, err := k3.prepareNodeShell(ctx, "worker-1", "alice", time.Minute); err == nil || !strings.Contains(err.Error(), "AXIOM_NODE_SHELL_IMAGE") {
		t.Fatalf("image pull failure: %v", err)
	} else {
		cleanup()
	}
	if pods, _ := cs3.CoreV1().Pods(NodeShellNamespace()).List(ctx, metav1.ListOptions{}); len(pods.Items) != 0 {
		t.Fatal("pod with an unpullable image was not deleted")
	}
}

func TestK8sNodeShellSweepsStalePods(t *testing.T) {
	t.Setenv("AXIOM_NODE_SHELL_NAMESPACE", "")
	labels := map[string]string{nodeShellLabel: "true"}
	old := metav1.NewTime(time.Now().Add(-time.Hour))
	mk := func(name string, created metav1.Time, phase corev1.PodPhase, l map[string]string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: DefaultNodeShellNamespace, Name: name, Labels: l, CreationTimestamp: created}, Status: corev1.PodStatus{Phase: phase}}
	}
	k, cs, _ := newFakeFullProvider(nil, nil,
		mk("overdue", old, corev1.PodRunning, labels), mk("failed", metav1.Now(), corev1.PodFailed, labels),
		mk("fresh", metav1.Now(), corev1.PodRunning, labels), mk("unrelated", old, corev1.PodRunning, map[string]string{"x": "y"}))
	if n := k.sweepNodeShellPods(context.Background(), 15*time.Minute); n != 2 {
		t.Fatalf("swept %d pods, want 2", n)
	}
	left, _ := cs.CoreV1().Pods(DefaultNodeShellNamespace).List(context.Background(), metav1.ListOptions{})
	names := map[string]bool{}
	for _, p := range left.Items {
		names[p.Name] = true
	}
	if !names["fresh"] || !names["unrelated"] || names["overdue"] || names["failed"] {
		t.Fatalf("remaining pods: %v", names)
	}
}
