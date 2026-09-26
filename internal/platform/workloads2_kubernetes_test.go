//go:build kubernetes

package platform

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
)

// --- fixtures ---

func newDeployment(ns, name string, replicas int32) *appsv1.Deployment {
	labels := map[string]string{"app": name}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: i32(replicas),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: name + ":v1"}}},
			},
		},
	}
}

func newStatefulSet(ns, name string, replicas int32) *appsv1.StatefulSet {
	labels := map[string]string{"app": name}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec: appsv1.StatefulSetSpec{
			Replicas: i32(replicas),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: name + ":v1"}}},
			},
		},
	}
}

func newDaemonSet(ns, name string) *appsv1.DaemonSet {
	labels := map[string]string{"app": name}
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: name + ":v1"}}},
			},
		},
	}
}

func newPod(ns, name, node string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       corev1.PodSpec{NodeName: node, Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
	}
}

// --- Launch (Run wizard) ---

func TestKubernetesLaunchCreatesDeploymentFromWizardRequest(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	w, err := k.Launch(ctx, LaunchRequest{
		Namespace: "wtest", Name: "web", Image: "nginx:1.27", Replicas: 3, Port: 8080,
		CPU: "250m", Memory: "128Mi", Environment: map[string]string{"MODE": "prod"}, Intent: "serve the storefront",
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if w.Name != "web" || w.Kind != "Deployment" || w.Namespace != "wtest" || w.Desired != 3 || w.Image != "nginx:1.27" {
		t.Fatalf("unexpected workload: %+v", w)
	}

	dep, err := cs.AppsV1().Deployments("wtest").Get(ctx, "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("deployment not stored: %v", err)
	}
	if *dep.Spec.Replicas != 3 {
		t.Fatalf("replicas not persisted: %d", *dep.Spec.Replicas)
	}
	if dep.Annotations[IntentAnnotation] != "serve the storefront" {
		t.Fatalf("intent annotation missing: %+v", dep.Annotations)
	}
	if dep.Labels["app.kubernetes.io/managed-by"] != "zyvor-axiom" || dep.Labels["app.kubernetes.io/name"] != "web" {
		t.Fatalf("expected managed-by/name labels: %+v", dep.Labels)
	}
	// The selector must match the pod template labels, otherwise the API server rejects the Deployment.
	for key, val := range dep.Spec.Selector.MatchLabels {
		if dep.Spec.Template.Labels[key] != val {
			t.Fatalf("selector %+v does not match template labels %+v", dep.Spec.Selector.MatchLabels, dep.Spec.Template.Labels)
		}
	}
	c := dep.Spec.Template.Spec.Containers[0]
	if c.Image != "nginx:1.27" || len(c.Ports) != 1 || c.Ports[0].ContainerPort != 8080 {
		t.Fatalf("container image/port not persisted: %+v", c)
	}
	if len(c.Env) != 1 || c.Env[0].Name != "MODE" || c.Env[0].Value != "prod" {
		t.Fatalf("env not persisted: %+v", c.Env)
	}
	if c.Resources.Requests.Cpu().String() != "250m" || c.Resources.Limits.Cpu().String() != "250m" ||
		c.Resources.Requests.Memory().String() != "128Mi" || c.Resources.Limits.Memory().String() != "128Mi" {
		t.Fatalf("resources not persisted: %+v", c.Resources)
	}

	// Not exposed => no Service.
	svcs, _ := cs.CoreV1().Services("wtest").List(ctx, metav1.ListOptions{})
	if len(svcs.Items) != 0 {
		t.Fatalf("expose=false must not create a Service, got %d", len(svcs.Items))
	}
	list, _ := k.ListWorkloads(ctx, "wtest")
	if len(list) != 1 || list[0].Name != "web" {
		t.Fatalf("expected new workload in ListWorkloads, got %+v", list)
	}
}

func TestKubernetesLaunchDefaults(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	// No namespace -> "default"; replicas < 1 -> 1.
	if _, err := k.Launch(ctx, LaunchRequest{Name: "dflt", Image: "busybox", Replicas: -5}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	dep, err := cs.AppsV1().Deployments("default").Get(ctx, "dflt", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected the deployment in the default namespace: %v", err)
	}
	if *dep.Spec.Replicas != 1 {
		t.Fatalf("expected replicas defaulted to 1, got %d", *dep.Spec.Replicas)
	}
	if _, has := dep.Annotations[IntentAnnotation]; has {
		t.Fatalf("no intent given, annotation must be absent: %+v", dep.Annotations)
	}
}

func TestKubernetesLaunchExposeCreatesMatchingService(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	if _, err := k.Launch(ctx, LaunchRequest{Namespace: "wtest", Name: "api", Image: "api:1", Port: 9000, Expose: true}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	svc, err := cs.CoreV1().Services("wtest").Get(ctx, "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected a Service named api: %v", err)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 9000 || svc.Spec.Ports[0].TargetPort.IntValue() != 9000 {
		t.Fatalf("unexpected service ports: %+v", svc.Spec.Ports)
	}
	dep, _ := cs.AppsV1().Deployments("wtest").Get(ctx, "api", metav1.GetOptions{})
	for key, val := range svc.Spec.Selector {
		if dep.Spec.Template.Labels[key] != val {
			t.Fatalf("service selector %+v does not select the deployment's pods %+v", svc.Spec.Selector, dep.Spec.Template.Labels)
		}
	}
	if svc.Labels["app.kubernetes.io/managed-by"] != "zyvor-axiom" {
		t.Fatalf("service must carry the managed-by label so DeleteWorkload can clean it up: %+v", svc.Labels)
	}
}

func TestKubernetesLaunchExposeWithoutPortCreatesNoService(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	if _, err := k.Launch(ctx, LaunchRequest{Namespace: "wtest", Name: "noport", Image: "x", Expose: true}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	svcs, _ := cs.CoreV1().Services("wtest").List(ctx, metav1.ListOptions{})
	if len(svcs.Items) != 0 {
		t.Fatalf("a Service without a port is meaningless; expected none, got %d", len(svcs.Items))
	}
}

func TestKubernetesLaunchRejectsInvalidInputWithoutCreatingAnything(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	for name, req := range map[string]LaunchRequest{
		"no name":    {Namespace: "wtest", Image: "x"},
		"blank name": {Namespace: "wtest", Name: "  ", Image: "x"},
		"no image":   {Namespace: "wtest", Name: "x"},
		"blank img":  {Namespace: "wtest", Name: "x", Image: "  "},
		"bad cpu":    {Namespace: "wtest", Name: "x", Image: "x", CPU: "lots"},
		"bad memory": {Namespace: "wtest", Name: "x", Image: "x", Memory: "12 parsecs"},
	} {
		if _, err := k.Launch(ctx, req); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	deps, _ := cs.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(deps.Items) != 0 {
		t.Fatalf("invalid requests must not create deployments, got %d", len(deps.Items))
	}
}

func TestKubernetesLaunchDuplicateIsConflict(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	req := LaunchRequest{Namespace: "wtest", Name: "dup", Image: "x:1", Replicas: 2}
	if _, err := k.Launch(ctx, req); err != nil {
		t.Fatalf("first launch: %v", err)
	}
	req.Image = "x:2"
	_, err := k.Launch(ctx, req)
	wantStatus(t, err, http.StatusConflict)
	dep, _ := cs.AppsV1().Deployments("wtest").Get(ctx, "dup", metav1.GetOptions{})
	if dep.Spec.Template.Spec.Containers[0].Image != "x:1" {
		t.Fatalf("a rejected duplicate launch must not modify the existing deployment: %+v", dep.Spec.Template.Spec.Containers[0])
	}
}

func TestKubernetesLaunchReportsServiceFailureButKeepsDeployment(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	cs.PrependReactor("create", "services", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(corev1.Resource("services"), "api", errors.New("rbac"))
	})
	_, err := k.Launch(ctx, LaunchRequest{Namespace: "wtest", Name: "api", Image: "x", Port: 80, Expose: true})
	if err == nil || !strings.Contains(err.Error(), "deployment created but service failed") {
		t.Fatalf("expected a partial-failure error, got %v", err)
	}
	wantStatus(t, err, http.StatusForbidden)
	if _, gerr := cs.AppsV1().Deployments("wtest").Get(ctx, "api", metav1.GetOptions{}); gerr != nil {
		t.Fatalf("the deployment was created and must still exist: %v", gerr)
	}
}

// --- Scale ---

func TestKubernetesScaleDeploymentAndStatefulSet(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		newDeployment("a", "web", 2), newDeployment("b", "web", 2), newStatefulSet("a", "db", 1),
	)
	if err := k.Scale(ctx, "a", "Deployment", "web", 7); err != nil {
		t.Fatalf("Scale deployment: %v", err)
	}
	if err := k.Scale(ctx, "a", "statefulset", "db", 3); err != nil {
		t.Fatalf("Scale statefulset (kind is case-insensitive): %v", err)
	}
	a, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	b, _ := cs.AppsV1().Deployments("b").Get(ctx, "web", metav1.GetOptions{})
	db, _ := cs.AppsV1().StatefulSets("a").Get(ctx, "db", metav1.GetOptions{})
	if *a.Spec.Replicas != 7 {
		t.Fatalf("a/web replicas = %d, want 7", *a.Spec.Replicas)
	}
	if *b.Spec.Replicas != 2 {
		t.Fatalf("b/web replicas = %d, want 2 (cross-namespace leak)", *b.Spec.Replicas)
	}
	if *db.Spec.Replicas != 3 {
		t.Fatalf("a/db replicas = %d, want 3", *db.Spec.Replicas)
	}
	// Scaling to zero is legal.
	if err := k.Scale(ctx, "a", "Deployment", "web", 0); err != nil {
		t.Fatalf("Scale to 0: %v", err)
	}
	a, _ = cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if *a.Spec.Replicas != 0 {
		t.Fatalf("scale to 0 not applied: %d", *a.Spec.Replicas)
	}
}

func TestKubernetesScaleRejectsNegativeAndUnsupported(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 2), newDaemonSet("a", "agent"))
	if err := k.Scale(ctx, "a", "Deployment", "web", -1); err == nil {
		t.Fatal("expected negative replicas to be rejected")
	}
	dep, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if *dep.Spec.Replicas != 2 {
		t.Fatalf("rejected scale must not change replicas, got %d", *dep.Spec.Replicas)
	}
	if err := k.Scale(ctx, "a", "DaemonSet", "agent", 2); err == nil {
		t.Fatal("expected scaling a DaemonSet to be rejected")
	}
	if err := k.Scale(ctx, "a", "Pod", "x", 1); err == nil {
		t.Fatal("expected an unsupported kind to be rejected")
	}
}

func TestKubernetesScaleMissingIsNotFound(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	wantStatus(t, k.Scale(context.Background(), "a", "Deployment", "nope", 1), http.StatusNotFound)
	wantStatus(t, k.Scale(context.Background(), "a", "StatefulSet", "nope", 1), http.StatusNotFound)
}

// --- Stop / Start ---

func TestKubernetesStopRemembersReplicasAndStartRestoresThem(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 5), newStatefulSet("a", "db", 4))

	if err := k.Stop(ctx, "a", "Deployment", "web"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	dep, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if *dep.Spec.Replicas != 0 || dep.Annotations[lastReplicasAnnotation] != "5" {
		t.Fatalf("stop must scale to 0 and remember 5 replicas: replicas=%d annotations=%+v", *dep.Spec.Replicas, dep.Annotations)
	}

	// A second Stop of an already stopped workload must not overwrite the remembered count with 0.
	if err := k.Stop(ctx, "a", "Deployment", "web"); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	dep, _ = cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if dep.Annotations[lastReplicasAnnotation] != "5" {
		t.Fatalf("second stop clobbered the remembered replica count: %+v", dep.Annotations)
	}

	if err := k.Start(ctx, "a", "Deployment", "web"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	dep, _ = cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if *dep.Spec.Replicas != 5 {
		t.Fatalf("start must restore 5 replicas, got %d", *dep.Spec.Replicas)
	}

	if err := k.Stop(ctx, "a", "StatefulSet", "db"); err != nil {
		t.Fatalf("Stop sts: %v", err)
	}
	sts, _ := cs.AppsV1().StatefulSets("a").Get(ctx, "db", metav1.GetOptions{})
	if *sts.Spec.Replicas != 0 || sts.Annotations[lastReplicasAnnotation] != "4" {
		t.Fatalf("sts stop: replicas=%d annotations=%+v", *sts.Spec.Replicas, sts.Annotations)
	}
	if err := k.Start(ctx, "a", "StatefulSet", "db"); err != nil {
		t.Fatalf("Start sts: %v", err)
	}
	sts, _ = cs.AppsV1().StatefulSets("a").Get(ctx, "db", metav1.GetOptions{})
	if *sts.Spec.Replicas != 4 {
		t.Fatalf("sts start must restore 4 replicas, got %d", *sts.Spec.Replicas)
	}
}

func TestKubernetesStartWithoutRememberedReplicasFallsBackToThree(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 0))
	if err := k.Start(ctx, "a", "Deployment", "web"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	dep, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if *dep.Spec.Replicas != 3 {
		t.Fatalf("documented fallback is 3 replicas, got %d", *dep.Spec.Replicas)
	}
}

func TestKubernetesStartOnRunningWorkloadKeepsReplicas(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 5))
	if err := k.Start(ctx, "a", "Deployment", "web"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	dep, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if *dep.Spec.Replicas != 5 {
		t.Fatalf("Start on a running workload changed replicas from 5 to %d", *dep.Spec.Replicas)
	}
}

func TestKubernetesStopStartUnsupportedAndMissing(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider(newDaemonSet("a", "agent"))
	if err := k.Stop(ctx, "a", "DaemonSet", "agent"); err == nil {
		t.Fatal("expected Stop of a DaemonSet to be rejected")
	}
	if err := k.Start(ctx, "a", "DaemonSet", "agent"); err == nil {
		t.Fatal("expected Start of a DaemonSet to be rejected")
	}
	wantStatus(t, k.Stop(ctx, "a", "Deployment", "nope"), http.StatusNotFound)
	wantStatus(t, k.Start(ctx, "a", "StatefulSet", "nope"), http.StatusNotFound)
}

// --- Restart ---

func TestKubernetesRestartStampsPodTemplateForEveryWorkloadKind(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		newDeployment("a", "web", 2), newDeployment("b", "web", 2), newStatefulSet("a", "db", 1), newDaemonSet("a", "agent"),
	)
	for _, tc := range []struct{ kind, name string }{{"Deployment", "web"}, {"StatefulSet", "db"}, {"DaemonSet", "agent"}} {
		if err := k.Restart(ctx, "a", tc.kind, tc.name); err != nil {
			t.Fatalf("Restart %s: %v", tc.kind, err)
		}
	}
	dep, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	sts, _ := cs.AppsV1().StatefulSets("a").Get(ctx, "db", metav1.GetOptions{})
	ds, _ := cs.AppsV1().DaemonSets("a").Get(ctx, "agent", metav1.GetOptions{})
	other, _ := cs.AppsV1().Deployments("b").Get(ctx, "web", metav1.GetOptions{})
	for kind, ann := range map[string]map[string]string{
		"Deployment": dep.Spec.Template.Annotations, "StatefulSet": sts.Spec.Template.Annotations, "DaemonSet": ds.Spec.Template.Annotations,
	} {
		if ann[restartedAtAnnotation] == "" {
			t.Fatalf("%s: pod template lacks the restartedAt annotation (would not roll pods): %+v", kind, ann)
		}
	}
	if _, has := other.Spec.Template.Annotations[restartedAtAnnotation]; has {
		t.Fatalf("b/web must not be restarted (cross-namespace leak)")
	}
	if dep.Spec.Template.Spec.Containers[0].Image != "web:v1" || *dep.Spec.Replicas != 2 {
		t.Fatalf("restart must not change the rest of the spec: %+v", dep.Spec)
	}
}

func TestKubernetesRestartUnsupportedAndMissing(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	if err := k.Restart(ctx, "a", "Pod", "x"); err == nil {
		t.Fatal("expected an unsupported kind to be rejected")
	}
	wantStatus(t, k.Restart(ctx, "a", "Deployment", "nope"), http.StatusNotFound)
	wantStatus(t, k.Restart(ctx, "a", "DaemonSet", "nope"), http.StatusNotFound)
}

// --- Rollout (pause / resume / restart / undo) ---

func TestKubernetesRolloutPauseResume(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 2))
	if err := k.RolloutAction(ctx, "a", "Deployment", "web", "pause"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	dep, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if !dep.Spec.Paused {
		t.Fatal("deployment should be paused")
	}
	if err := k.RolloutAction(ctx, "a", "Deployment", "web", "resume"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	dep, _ = cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if dep.Spec.Paused {
		t.Fatal("deployment should be resumed")
	}
}

func TestKubernetesRolloutRestart(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 2), newStatefulSet("a", "db", 1))
	if err := k.RolloutAction(ctx, "a", "Deployment", "web", "restart"); err != nil {
		t.Fatalf("deployment restart: %v", err)
	}
	if err := k.RolloutAction(ctx, "a", "StatefulSet", "db", "restart"); err != nil {
		t.Fatalf("statefulset restart: %v", err)
	}
	dep, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	sts, _ := cs.AppsV1().StatefulSets("a").Get(ctx, "db", metav1.GetOptions{})
	if dep.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatalf("deployment template lacks restartedAt: %+v", dep.Spec.Template.Annotations)
	}
	if sts.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatalf("statefulset template lacks restartedAt: %+v", sts.Spec.Template.Annotations)
	}
}

func TestKubernetesRolloutRestartDaemonSet(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDaemonSet("a", "agent"))
	if err := k.RolloutAction(ctx, "a", "DaemonSet", "agent", "restart"); err != nil {
		t.Fatalf("daemonset restart: %v", err)
	}
	ds, _ := cs.AppsV1().DaemonSets("a").Get(ctx, "agent", metav1.GetOptions{})
	// A restart is a pod-template change; the list menu's Restart() stamps Axiom's own annotation.
	if ds.Spec.Template.Annotations[restartedAtAnnotation] == "" {
		t.Fatalf("daemonset template was not restamped: %+v", ds.Spec.Template.Annotations)
	}
}

func TestKubernetesRolloutRejectsUnknownActionsAndKinds(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 2), newStatefulSet("a", "db", 1))
	if err := k.RolloutAction(ctx, "a", "Deployment", "web", "explode"); err == nil {
		t.Fatal("expected an unknown action to be rejected")
	}
	if err := k.RolloutAction(ctx, "a", "StatefulSet", "db", "pause"); err == nil {
		t.Fatal("expected pause on a StatefulSet to be rejected")
	}
	if err := k.RolloutAction(ctx, "a", "Job", "j", "restart"); err == nil {
		t.Fatal("expected an unsupported kind to be rejected")
	}
	dep, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if dep.Spec.Paused || len(dep.Spec.Template.Annotations) != 0 {
		t.Fatalf("rejected actions must not mutate the deployment: %+v", dep.Spec)
	}
	wantStatus(t, k.RolloutAction(ctx, "a", "Deployment", "nope", "pause"), http.StatusNotFound)
	wantStatus(t, k.RolloutAction(ctx, "a", "Deployment", "nope", "restart"), http.StatusNotFound)
}

func newReplicaSet(ns, name, owner string, revision string, image string, replicas int32) *appsv1.ReplicaSet {
	labels := map[string]string{"app": owner, "pod-template-hash": name}
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns,
			Annotations:     map[string]string{"deployment.kubernetes.io/revision": revision},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: owner}},
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: i32(replicas),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: owner, Image: image}}},
			},
		},
		Status: appsv1.ReplicaSetStatus{Replicas: replicas},
	}
}

func TestKubernetesRolloutUndoRestoresPreviousRevisionTemplate(t *testing.T) {
	ctx := context.Background()
	dep := newDeployment("a", "web", 2)
	dep.Spec.Template.Spec.Containers[0].Image = "web:v3"
	// Another deployment's replicasets in the same namespace must never be picked as the rollback target.
	k, cs := newFakeKubernetesProvider(dep,
		newReplicaSet("a", "web-r1", "web", "1", "web:v1", 0),
		newReplicaSet("a", "web-r2", "web", "2", "web:v2", 0),
		newReplicaSet("a", "web-r3", "web", "3", "web:v3", 2),
		newReplicaSet("a", "other-r2", "other", "2", "other:v9", 1),
	)
	if err := k.RolloutAction(ctx, "a", "Deployment", "web", "undo"); err != nil {
		t.Fatalf("undo: %v", err)
	}
	got, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if img := got.Spec.Template.Spec.Containers[0].Image; img != "web:v2" {
		t.Fatalf("undo must roll back to the previous revision's image web:v2, got %q", img)
	}
	if *got.Spec.Replicas != 2 {
		t.Fatalf("undo must not change replicas: %d", *got.Spec.Replicas)
	}
}

func TestKubernetesRolloutUndoWithoutPreviousRevisionFails(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 2), newReplicaSet("a", "web-r1", "web", "1", "web:v1", 2))
	if err := k.RolloutAction(ctx, "a", "Deployment", "web", "undo"); err == nil {
		t.Fatal("expected undo with a single revision to fail")
	}
	dep, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if dep.Spec.Template.Spec.Containers[0].Image != "web:v1" {
		t.Fatalf("failed undo must not change the template: %+v", dep.Spec.Template.Spec.Containers[0])
	}
}

func TestKubernetesRolloutUndoMissingDeploymentIsNotFound(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	wantStatus(t, k.RolloutAction(context.Background(), "a", "Deployment", "nope", "undo"), http.StatusNotFound)
}

// --- DeleteWorkload ---

func TestKubernetesDeleteWorkloadRemovesWorkloadAndManagedService(t *testing.T) {
	ctx := context.Background()
	managed := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "a", Labels: map[string]string{"app.kubernetes.io/managed-by": "zyvor-axiom"}}}
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 2), newDeployment("b", "web", 2), managed,
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "b", Labels: map[string]string{"app.kubernetes.io/managed-by": "zyvor-axiom"}}})

	if err := k.DeleteWorkload(ctx, "a", "Deployment", "web"); err != nil {
		t.Fatalf("DeleteWorkload: %v", err)
	}
	if _, err := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("a/web should be gone, err=%v", err)
	}
	if _, err := cs.CoreV1().Services("a").Get(ctx, "web", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("the Axiom-managed Service a/web should be cleaned up, err=%v", err)
	}
	if _, err := cs.AppsV1().Deployments("b").Get(ctx, "web", metav1.GetOptions{}); err != nil {
		t.Fatalf("b/web must survive (cross-namespace leak?): %v", err)
	}
	if _, err := cs.CoreV1().Services("b").Get(ctx, "web", metav1.GetOptions{}); err != nil {
		t.Fatalf("b/web Service must survive (cross-namespace leak?): %v", err)
	}
}

func TestKubernetesDeleteWorkloadKeepsUnmanagedServiceOfSameName(t *testing.T) {
	ctx := context.Background()
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "a"}} // hand-made, no managed-by label
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 1), svc)
	if err := k.DeleteWorkload(ctx, "a", "Deployment", "web"); err != nil {
		t.Fatalf("DeleteWorkload: %v", err)
	}
	if _, err := cs.CoreV1().Services("a").Get(ctx, "web", metav1.GetOptions{}); err != nil {
		t.Fatalf("a Service Axiom does not own must not be deleted: %v", err)
	}
}

func TestKubernetesDeleteWorkloadStatefulSetAndDaemonSet(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newStatefulSet("a", "db", 1), newDaemonSet("a", "agent"))
	if err := k.DeleteWorkload(ctx, "a", "StatefulSet", "db"); err != nil {
		t.Fatalf("delete sts: %v", err)
	}
	if err := k.DeleteWorkload(ctx, "a", "daemonset", "agent"); err != nil {
		t.Fatalf("delete ds: %v", err)
	}
	if _, err := cs.AppsV1().StatefulSets("a").Get(ctx, "db", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("sts should be gone, err=%v", err)
	}
	if _, err := cs.AppsV1().DaemonSets("a").Get(ctx, "agent", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("ds should be gone, err=%v", err)
	}
}

func TestKubernetesDeleteWorkloadUsesForegroundPropagation(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 1))
	if err := k.DeleteWorkload(ctx, "a", "Deployment", "web"); err != nil {
		t.Fatalf("DeleteWorkload: %v", err)
	}
	var seen bool
	for _, act := range cs.Actions() {
		if del, ok := act.(clienttesting.DeleteActionImpl); ok && del.GetResource().Resource == "deployments" {
			seen = true
			if p := del.DeleteOptions.PropagationPolicy; p == nil || *p != metav1.DeletePropagationForeground {
				t.Fatalf("expected foreground propagation so dependents are removed, got %v", p)
			}
		}
	}
	if !seen {
		t.Fatal("no deployment delete action recorded")
	}
}

func TestKubernetesDeleteWorkloadMissingAndUnsupported(t *testing.T) {
	ctx := context.Background()
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "ghost", Namespace: "a", Labels: map[string]string{"app.kubernetes.io/managed-by": "zyvor-axiom"}}}
	k, cs := newFakeKubernetesProvider(svc)
	wantStatus(t, k.DeleteWorkload(ctx, "a", "Deployment", "ghost"), http.StatusNotFound)
	if _, err := cs.CoreV1().Services("a").Get(ctx, "ghost", metav1.GetOptions{}); err != nil {
		t.Fatalf("a failed delete of a missing workload must not remove the service: %v", err)
	}
	if err := k.DeleteWorkload(ctx, "a", "Pod", "x"); err == nil {
		t.Fatal("expected an unsupported kind to be rejected")
	}
}

// --- Pods ---

func TestKubernetesDeletePodRemovesOnlyTarget(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newPod("a", "p", "n1"), newPod("b", "p", "n1"), newPod("a", "q", "n1"))
	if err := k.DeletePod(ctx, "a", "p"); err != nil {
		t.Fatalf("DeletePod: %v", err)
	}
	if _, err := cs.CoreV1().Pods("a").Get(ctx, "p", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("a/p should be gone, err=%v", err)
	}
	for _, key := range [][2]string{{"b", "p"}, {"a", "q"}} {
		if _, err := cs.CoreV1().Pods(key[0]).Get(ctx, key[1], metav1.GetOptions{}); err != nil {
			t.Fatalf("%s/%s must survive: %v", key[0], key[1], err)
		}
	}
	wantStatus(t, k.DeletePod(ctx, "a", "p"), http.StatusNotFound)
}

func TestKubernetesEvictPodRemovesPodViaEvictionSubresource(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newPod("a", "p", "n1"), newPod("b", "p", "n1"))
	rec := installEvictionReactor(cs)
	if err := k.EvictPod(ctx, "a", "p"); err != nil {
		t.Fatalf("EvictPod: %v", err)
	}
	if len(rec.evicted) != 1 || rec.evicted[0] != "a/p" {
		t.Fatalf("expected an eviction request for a/p, got %v", rec.evicted)
	}
	if _, err := cs.CoreV1().Pods("a").Get(ctx, "p", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("a/p should be evicted, err=%v", err)
	}
	if _, err := cs.CoreV1().Pods("b").Get(ctx, "p", metav1.GetOptions{}); err != nil {
		t.Fatalf("b/p must survive: %v", err)
	}
}

func TestKubernetesEvictPodUsesEvictionNotPlainDelete(t *testing.T) {
	// Eviction (not DELETE) is what makes PodDisruptionBudgets apply.
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newPod("a", "p", "n1"))
	installEvictionReactor(cs)
	if err := k.EvictPod(ctx, "a", "p"); err != nil {
		t.Fatalf("EvictPod: %v", err)
	}
	var evictions, deletes int
	for _, act := range cs.Actions() {
		switch {
		case act.GetVerb() == "create" && act.GetSubresource() == "eviction":
			evictions++
		case act.GetVerb() == "delete" && act.GetResource().Resource == "pods":
			deletes++
		}
	}
	if evictions != 1 || deletes != 0 {
		t.Fatalf("expected exactly one pods/eviction create and no pod delete, got evictions=%d deletes=%d", evictions, deletes)
	}
}

func TestKubernetesEvictPodBlockedByDisruptionBudgetIsTooManyRequests(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newPod("a", "p", "n1"))
	installEvictionReactor(cs, "a/p")
	err := k.EvictPod(ctx, "a", "p")
	wantStatus(t, err, http.StatusTooManyRequests)
	if _, gerr := cs.CoreV1().Pods("a").Get(ctx, "p", metav1.GetOptions{}); gerr != nil {
		t.Fatalf("a PDB-blocked pod must remain: %v", gerr)
	}
}

func TestKubernetesEvictPodMissingIsNotFound(t *testing.T) {
	k, cs := newFakeKubernetesProvider()
	installEvictionReactor(cs)
	wantStatus(t, k.EvictPod(context.Background(), "a", "nope"), http.StatusNotFound)
}

// --- Nodes ---

func newNode(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{
		Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
	}}
}

func TestKubernetesCordonUncordon(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newNode("n1"), newNode("n2"))
	if err := k.Cordon(ctx, "n1", true); err != nil {
		t.Fatalf("Cordon: %v", err)
	}
	n1, _ := cs.CoreV1().Nodes().Get(ctx, "n1", metav1.GetOptions{})
	n2, _ := cs.CoreV1().Nodes().Get(ctx, "n2", metav1.GetOptions{})
	if !n1.Spec.Unschedulable || n2.Spec.Unschedulable {
		t.Fatalf("only n1 should be cordoned: n1=%v n2=%v", n1.Spec.Unschedulable, n2.Spec.Unschedulable)
	}
	nodes, _ := k.ListNodes(ctx)
	status := map[string]string{}
	for _, n := range nodes {
		status[n.Name] = n.Status
	}
	if status["n1"] != "SchedulingDisabled" || status["n2"] != "Ready" {
		t.Fatalf("ListNodes should reflect the cordon: %+v", status)
	}

	if err := k.Cordon(ctx, "n1", false); err != nil {
		t.Fatalf("Uncordon: %v", err)
	}
	n1, _ = cs.CoreV1().Nodes().Get(ctx, "n1", metav1.GetOptions{})
	if n1.Spec.Unschedulable {
		t.Fatal("n1 should be schedulable again")
	}
}

func TestKubernetesCordonMissingNodeIsNotFound(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	wantStatus(t, k.Cordon(context.Background(), "ghost", true), http.StatusNotFound)
}

func TestKubernetesAddAndRemoveTaint(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newNode("n1"), newNode("n2"))
	if err := k.AddTaint(ctx, "n1", "dedicated", "gpu", "NoSchedule"); err != nil {
		t.Fatalf("AddTaint: %v", err)
	}
	// Adding the same key+effect again is idempotent.
	if err := k.AddTaint(ctx, "n1", "dedicated", "other-value", "NoSchedule"); err != nil {
		t.Fatalf("AddTaint again: %v", err)
	}
	if err := k.AddTaint(ctx, "n1", "dedicated", "gpu", "NoExecute"); err != nil {
		t.Fatalf("AddTaint NoExecute: %v", err)
	}
	n1, _ := cs.CoreV1().Nodes().Get(ctx, "n1", metav1.GetOptions{})
	n2, _ := cs.CoreV1().Nodes().Get(ctx, "n2", metav1.GetOptions{})
	if len(n1.Spec.Taints) != 2 || n1.Spec.Taints[0].Value != "gpu" {
		t.Fatalf("expected two distinct taints (NoSchedule keeps its original value): %+v", n1.Spec.Taints)
	}
	if len(n2.Spec.Taints) != 0 {
		t.Fatalf("n2 must have no taints: %+v", n2.Spec.Taints)
	}

	if err := k.RemoveTaint(ctx, "n1", "dedicated", "NoSchedule"); err != nil {
		t.Fatalf("RemoveTaint: %v", err)
	}
	n1, _ = cs.CoreV1().Nodes().Get(ctx, "n1", metav1.GetOptions{})
	if len(n1.Spec.Taints) != 1 || n1.Spec.Taints[0].Effect != corev1.TaintEffectNoExecute {
		t.Fatalf("only the NoSchedule taint should be removed: %+v", n1.Spec.Taints)
	}
	wantStatus(t, k.AddTaint(ctx, "ghost", "k", "v", "NoSchedule"), http.StatusNotFound)
	wantStatus(t, k.RemoveTaint(ctx, "ghost", "k", "NoSchedule"), http.StatusNotFound)
}

func TestKubernetesDrainCordonsAndEvictsButSkipsDaemonSetMirrorAndKubeSystemPods(t *testing.T) {
	ctx := context.Background()
	isController := true
	dsPod := newPod("a", "agent-x", "n1")
	dsPod.OwnerReferences = []metav1.OwnerReference{{Kind: "DaemonSet", Name: "agent", Controller: &isController}}
	nonControllerDS := newPod("a", "weird-ds-ref", "n1") // DaemonSet ref without controller=true is NOT a daemonset pod
	nonControllerDS.OwnerReferences = []metav1.OwnerReference{{Kind: "DaemonSet", Name: "agent"}}
	mirror := newPod("a", "static", "n1")
	mirror.Annotations = map[string]string{corev1.MirrorPodAnnotationKey: "hash"}

	k, cs := newFakeKubernetesProvider(
		newNode("n1"), newNode("n2"),
		newPod("a", "app-1", "n1"), newPod("b", "app-2", "n1"), // evicted
		dsPod, mirror, newPod("kube-system", "coredns", "n1"), // skipped
		nonControllerDS,                // evicted
		newPod("a", "elsewhere", "n2"), // other node, untouched
	)
	installNodeNameFieldSelector(cs)
	rec := installEvictionReactor(cs)

	if err := k.Drain(ctx, "n1"); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	n1, _ := cs.CoreV1().Nodes().Get(ctx, "n1", metav1.GetOptions{})
	if !n1.Spec.Unschedulable {
		t.Fatal("drain must cordon the node first")
	}
	sort.Strings(rec.evicted)
	if got, want := strings.Join(rec.evicted, ","), "a/app-1,a/weird-ds-ref,b/app-2"; got != want {
		t.Fatalf("evicted pods = %q, want %q", got, want)
	}
	for _, key := range [][2]string{{"a", "agent-x"}, {"a", "static"}, {"kube-system", "coredns"}, {"a", "elsewhere"}} {
		if _, err := cs.CoreV1().Pods(key[0]).Get(ctx, key[1], metav1.GetOptions{}); err != nil {
			t.Fatalf("%s/%s must not be evicted by a drain of n1: %v", key[0], key[1], err)
		}
	}
	n2, _ := cs.CoreV1().Nodes().Get(ctx, "n2", metav1.GetOptions{})
	if n2.Spec.Unschedulable {
		t.Fatal("draining n1 must not cordon n2")
	}
}

func TestKubernetesDrainUsesEvictionNeverPlainDelete(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newNode("n1"), newPod("a", "app", "n1"))
	installNodeNameFieldSelector(cs)
	installEvictionReactor(cs)
	if err := k.Drain(ctx, "n1"); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	for _, act := range cs.Actions() {
		if act.GetVerb() == "delete" && act.GetResource().Resource == "pods" {
			t.Fatalf("drain must respect PodDisruptionBudgets by evicting, but issued a plain pod delete: %v", act)
		}
	}
}

func TestKubernetesDrainReportsBlockedPodsButStillEvictsOthers(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newNode("n1"), newPod("a", "guarded", "n1"), newPod("a", "free", "n1"))
	installNodeNameFieldSelector(cs)
	rec := installEvictionReactor(cs, "a/guarded")

	err := k.Drain(ctx, "n1")
	if err == nil || !strings.Contains(err.Error(), "a/guarded") {
		t.Fatalf("expected an error naming the PDB-blocked pod, got %v", err)
	}
	if strings.Join(rec.evicted, ",") != "a/free" {
		t.Fatalf("the unblocked pod should still be evicted, got %v", rec.evicted)
	}
	if _, gerr := cs.CoreV1().Pods("a").Get(ctx, "guarded", metav1.GetOptions{}); gerr != nil {
		t.Fatalf("guarded pod must remain: %v", gerr)
	}
	n1, _ := cs.CoreV1().Nodes().Get(ctx, "n1", metav1.GetOptions{})
	if !n1.Spec.Unschedulable {
		t.Fatal("the node stays cordoned after a partially failed drain")
	}
}

func TestKubernetesDrainBlockedByPDBKeepsTooManyRequestsClass(t *testing.T) {
	k, cs := newFakeKubernetesProvider(newNode("n1"), newPod("a", "guarded", "n1"))
	installNodeNameFieldSelector(cs)
	installEvictionReactor(cs, "a/guarded")
	wantStatus(t, k.Drain(context.Background(), "n1"), http.StatusTooManyRequests)
}

func TestKubernetesDrainMissingNodeIsNotFound(t *testing.T) {
	k, cs := newFakeKubernetesProvider(newPod("a", "app", "ghost"))
	installNodeNameFieldSelector(cs)
	rec := installEvictionReactor(cs)
	wantStatus(t, k.Drain(context.Background(), "ghost"), http.StatusNotFound)
	if len(rec.evicted) != 0 {
		t.Fatalf("no pods may be evicted for a node that does not exist, got %v", rec.evicted)
	}
}

// --- Seal / Unseal (NetworkPolicy + annotation) ---

func TestKubernetesSealWorkloadCreatesDenyAllPolicyAndAnnotates(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDeployment("a", "web", 1), newDeployment("b", "web", 1))

	if err := k.SealWorkload(ctx, "a", "Deployment", "web"); err != nil {
		t.Fatalf("SealWorkload: %v", err)
	}
	np, err := cs.NetworkingV1().NetworkPolicies("a").Get(ctx, sealPolicyName("web"), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("seal policy not created: %v", err)
	}
	if np.Spec.PodSelector.MatchLabels["app"] != "web" {
		t.Fatalf("seal policy must select the workload's pods: %+v", np.Spec.PodSelector)
	}
	if len(np.Spec.Ingress) != 0 || len(np.Spec.Egress) != 0 {
		t.Fatalf("a sealed policy must allow nothing: %+v", np.Spec)
	}
	types := map[networkingv1.PolicyType]bool{}
	for _, pt := range np.Spec.PolicyTypes {
		types[pt] = true
	}
	if !types[networkingv1.PolicyTypeIngress] || !types[networkingv1.PolicyTypeEgress] {
		t.Fatalf("expected both Ingress and Egress policy types: %+v", np.Spec.PolicyTypes)
	}
	dep, _ := cs.AppsV1().Deployments("a").Get(ctx, "web", metav1.GetOptions{})
	if !IsSealedAnnotation(dep.Annotations) {
		t.Fatalf("workload must be annotated as sealed: %+v", dep.Annotations)
	}
	if other, _ := cs.AppsV1().Deployments("b").Get(ctx, "web", metav1.GetOptions{}); IsSealedAnnotation(other.Annotations) {
		t.Fatal("b/web must not be sealed")
	}
	if nps, _ := cs.NetworkingV1().NetworkPolicies("b").List(ctx, metav1.ListOptions{}); len(nps.Items) != 0 {
		t.Fatal("no policy may be created in namespace b")
	}
	sealed, _ := k.SealedWorkloads(ctx, "all")
	if !sealed["a/web"] || sealed["b/web"] {
		t.Fatalf("SealedWorkloads should report only a/web: %+v", sealed)
	}

	// Sealing twice is idempotent.
	if err := k.SealWorkload(ctx, "a", "Deployment", "web"); err != nil {
		t.Fatalf("second SealWorkload: %v", err)
	}
}

func TestKubernetesUnsealWorkloadRemovesPolicyAndAnnotation(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newStatefulSet("a", "db", 1))
	if err := k.SealWorkload(ctx, "a", "StatefulSet", "db"); err != nil {
		t.Fatalf("SealWorkload: %v", err)
	}
	if err := k.UnsealWorkload(ctx, "a", "StatefulSet", "db"); err != nil {
		t.Fatalf("UnsealWorkload: %v", err)
	}
	if _, err := cs.NetworkingV1().NetworkPolicies("a").Get(ctx, sealPolicyName("db"), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("seal policy should be gone, err=%v", err)
	}
	sts, _ := cs.AppsV1().StatefulSets("a").Get(ctx, "db", metav1.GetOptions{})
	if IsSealedAnnotation(sts.Annotations) {
		t.Fatalf("workload must no longer be sealed: %+v", sts.Annotations)
	}
	// Unsealing something that is not sealed is harmless.
	if err := k.UnsealWorkload(ctx, "a", "StatefulSet", "db"); err != nil {
		t.Fatalf("second UnsealWorkload: %v", err)
	}
}

func TestKubernetesSealMissingAndUnsupported(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newDaemonSet("a", "agent"))
	wantStatus(t, k.SealWorkload(ctx, "a", "Deployment", "ghost"), http.StatusNotFound)
	if err := k.SealWorkload(ctx, "a", "DaemonSet", "agent"); err == nil {
		t.Fatal("expected sealing a DaemonSet to be rejected")
	}
	if nps, _ := cs.NetworkingV1().NetworkPolicies(metav1.NamespaceAll).List(ctx, metav1.ListOptions{}); len(nps.Items) != 0 {
		t.Fatalf("failed seals must not leave policies behind, got %d", len(nps.Items))
	}
}
