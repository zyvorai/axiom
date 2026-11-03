//go:build kubernetes

package platform

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func resContainer(name, image string, req, lim map[corev1.ResourceName]string) corev1.Container {
	c := corev1.Container{Name: name, Image: image, Env: []corev1.EnvVar{{Name: "KEEP", Value: "me"}}}
	if len(req) > 0 {
		c.Resources.Requests = corev1.ResourceList{}
		for k, v := range req {
			c.Resources.Requests[k] = resource.MustParse(v)
		}
	}
	if len(lim) > 0 {
		c.Resources.Limits = corev1.ResourceList{}
		for k, v := range lim {
			c.Resources.Limits[k] = resource.MustParse(v)
		}
	}
	return c
}

type rl = map[corev1.ResourceName]string

const (
	rCPU = corev1.ResourceCPU
	rMem = corev1.ResourceMemory
)

func applyFakeProvider(t *testing.T) (*KubernetesProvider, *k8sfake.Clientset) {
	t.Helper()
	now := time.Now()
	webApp := resContainer("app", "web:1", rl{rCPU: "1", rMem: "1Gi"}, rl{rCPU: "2", rMem: "1Gi"})
	webSidecar := resContainer("sidecar", "envoy:1", rl{rCPU: "100m", rMem: "64Mi"}, nil)
	dbC := resContainer("db", "postgres:16", rl{rCPU: "500m", rMem: "512Mi"}, rl{rCPU: "500m", rMem: "512Mi"})
	agentC := resContainer("agent", "agent:1", nil, nil)
	k, cs := newFakeKubernetesProvider(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{webApp, webSidecar}}}}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "idle", Namespace: "shop"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{webApp}}}}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "shop"},
			Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{dbC}}}}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "kube-system"},
			Spec: appsv1.DaemonSetSpec{UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType},
				Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{agentC}}}}},
		rsPod("shop", "web-6d4cf56db6-aaaaa", "ReplicaSet", "web-6d4cf56db6", now, webApp, webSidecar),
		rsPod("shop", "db-0", "StatefulSet", "db", now, dbC),
		rsPod("kube-system", "agent-abcde", "DaemonSet", "agent", now, agentC),
	)
	stubPodMetrics(t, `{"items":[
 {"metadata":{"name":"web-6d4cf56db6-aaaaa","namespace":"shop"},"containers":[
   {"name":"app","usage":{"cpu":"150m","memory":"200Mi"}},
   {"name":"sidecar","usage":{"cpu":"70m","memory":"50Mi"}}]},
 {"metadata":{"name":"db-0","namespace":"shop"},"containers":[{"name":"db","usage":{"cpu":"480m","memory":"300Mi"}}]},
 {"metadata":{"name":"agent-abcde","namespace":"kube-system"},"containers":[{"name":"agent","usage":{"cpu":"20m","memory":"30Mi"}}]}
]}`, nil)
	return k, cs
}

func patchActions(cs *k8sfake.Clientset) []clienttesting.PatchActionImpl {
	var out []clienttesting.PatchActionImpl
	for _, a := range cs.Actions() {
		if p, ok := a.(clienttesting.PatchActionImpl); ok {
			out = append(out, p)
		}
	}
	return out
}

func quantities(l corev1.ResourceList) map[string]string {
	out := map[string]string{}
	for k, v := range l {
		out[string(k)] = v.String()
	}
	return out
}

func TestKubernetesApplyRightsizingDeployment(t *testing.T) {
	k, cs := applyFakeProvider(t)
	ctx := context.Background()
	cpu, mem := int64(1000), int64(1024)
	req := RightsizingApplyRequest{Namespace: "shop", Kind: "Deployment", Workload: "web", Container: "app", Resources: []string{"cpu", "memory"},
		DryRun: true, Observed: &RightsizingObserved{CPURequestMilli: &cpu, MemoryRequestMiB: &mem}}
	before, _ := cs.AppsV1().Deployments("shop").Get(ctx, "web", metav1.GetOptions{})

	dry, err := k.ApplyRightsizing(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	wantChanges := []RightsizingChange{
		{Resource: "cpu", Field: "requests", From: "1", To: "180m"},
		{Resource: "memory", Field: "requests", From: "1Gi", To: "232Mi"},
	}
	if !dry.OK || !dry.DryRun || dry.Verdict != "over" || dry.Source != "metrics-snapshot" || dry.Confidence != "low" ||
		!strings.Contains(dry.Rollout, "rolling update") || !reflect.DeepEqual(dry.Changes, wantChanges) {
		t.Fatalf("dry run %+v", dry)
	}
	if n := len(patchActions(cs)); n != 0 {
		t.Fatalf("dry run patched %d times", n)
	}
	if now, _ := cs.AppsV1().Deployments("shop").Get(ctx, "web", metav1.GetOptions{}); !reflect.DeepEqual(now.Spec, before.Spec) {
		t.Fatal("dry run changed the Deployment")
	}

	req.DryRun = false
	res, err := k.ApplyRightsizing(ctx, req)
	if err != nil || res.DryRun || !reflect.DeepEqual(res.Changes, wantChanges) {
		t.Fatalf("apply %+v %v", res, err)
	}
	pa := patchActions(cs)
	if len(pa) != 1 || pa[0].PatchType != types.StrategicMergePatchType || pa[0].PatchOptions.FieldManager != "zyvor-axiom" ||
		pa[0].GetResource().Resource != "deployments" || pa[0].Name != "web" {
		t.Fatalf("patch actions %+v", pa)
	}
	after, _ := cs.AppsV1().Deployments("shop").Get(ctx, "web", metav1.GetOptions{})
	cs0 := after.Spec.Template.Spec.Containers
	if len(cs0) != 2 || cs0[0].Name != "app" || cs0[1].Name != "sidecar" {
		t.Fatalf("containers %+v", cs0)
	}
	app := cs0[0]
	if got := quantities(app.Resources.Requests); !reflect.DeepEqual(got, map[string]string{"cpu": "180m", "memory": "232Mi"}) {
		t.Fatalf("app requests %v", got)
	}
	if got := quantities(app.Resources.Limits); !reflect.DeepEqual(got, map[string]string{"cpu": "2", "memory": "1Gi"}) {
		t.Fatalf("app limits must stay (never lowered): %v", got)
	}
	if app.Image != "web:1" || len(app.Env) != 1 {
		t.Fatalf("other app fields changed: %+v", app)
	}
	if !reflect.DeepEqual(cs0[1], before.Spec.Template.Spec.Containers[1]) {
		t.Fatalf("sidecar touched: %+v", cs0[1])
	}

	// The view is stale now (the template changed; the running pod has not yet
	// rolled in the fake, so a second apply finds nothing left to change).
	if _, err := k.ApplyRightsizing(ctx, req); HTTPStatus(err) != 409 {
		t.Fatalf("second apply: %v", err)
	}
}

func TestKubernetesApplyRightsizingRefusals(t *testing.T) {
	k, cs := applyFakeProvider(t)
	ctx := context.Background()
	base := RightsizingApplyRequest{Namespace: "shop", Kind: "Deployment", Workload: "web", Container: "app", Resources: []string{"cpu", "memory"}}

	stale := int64(500)
	r := base
	r.Observed = &RightsizingObserved{CPURequestMilli: &stale}
	if _, err := k.ApplyRightsizing(ctx, r); HTTPStatus(err) != 409 || !strings.Contains(err.Error(), "the live requests changed since you looked") {
		t.Fatalf("stale observed: %v", err)
	}

	r = base
	r.Container = "sidecar"
	if _, err := k.ApplyRightsizing(ctx, r); HTTPStatus(err) != 409 || !strings.Contains(err.Error(), "already right-sized") {
		t.Fatalf("verdict ok: %v", err)
	}

	r = base
	r.Workload = "missing"
	if _, err := k.ApplyRightsizing(ctx, r); HTTPStatus(err) != 404 {
		t.Fatalf("unknown workload: %v", err)
	}
	r = base
	r.Container = "nope"
	if _, err := k.ApplyRightsizing(ctx, r); HTTPStatus(err) != 404 {
		t.Fatalf("unknown container: %v", err)
	}
	r = base
	r.Kind = "StatefulSet"
	if _, err := k.ApplyRightsizing(ctx, r); HTTPStatus(err) != 404 {
		t.Fatalf("wrong kind: %v", err)
	}
	r = base
	r.Workload = "idle" // exists, but no running pods to measure
	if _, err := k.ApplyRightsizing(ctx, r); HTTPStatus(err) != 409 || !strings.Contains(err.Error(), "no usage data") {
		t.Fatalf("no pods: %v", err)
	}
	r = base
	r.Window = "2d"
	if _, err := k.ApplyRightsizing(ctx, r); HTTPStatus(err) != 400 {
		t.Fatalf("bad window: %v", err)
	}
	if n := len(patchActions(cs)); n != 0 {
		t.Fatalf("refusals patched %d times", n)
	}
}

func TestKubernetesApplyRightsizingStatefulSetAndDaemonSet(t *testing.T) {
	k, cs := applyFakeProvider(t)
	ctx := context.Background()

	res, err := k.ApplyRightsizing(ctx, RightsizingApplyRequest{Namespace: "shop", Kind: "StatefulSet", Workload: "db", Container: "db", Resources: []string{"cpu", "memory"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []RightsizingChange{
		{Resource: "cpu", Field: "requests", From: "500m", To: "580m"},
		{Resource: "cpu", Field: "limits", From: "500m", To: "580m", Note: "limit raised to stay >= the request"},
		{Resource: "memory", Field: "requests", From: "512Mi", To: "352Mi"},
	}
	if res.Verdict != "under" || !reflect.DeepEqual(res.Changes, want) || !strings.Contains(res.Rollout, "StatefulSets") {
		t.Fatalf("statefulset %+v", res)
	}
	sts, _ := cs.AppsV1().StatefulSets("shop").Get(ctx, "db", metav1.GetOptions{})
	db := sts.Spec.Template.Spec.Containers[0]
	if got := quantities(db.Resources.Requests); !reflect.DeepEqual(got, map[string]string{"cpu": "580m", "memory": "352Mi"}) {
		t.Fatalf("db requests %v", got)
	}
	if got := quantities(db.Resources.Limits); !reflect.DeepEqual(got, map[string]string{"cpu": "580m", "memory": "512Mi"}) {
		t.Fatalf("db limits %v", got)
	}

	res, err = k.ApplyRightsizing(ctx, RightsizingApplyRequest{Namespace: "kube-system", Kind: "DaemonSet", Workload: "agent", Container: "agent", Resources: []string{"cpu", "memory"}, Window: "30d"})
	if err != nil {
		t.Fatal(err)
	}
	want = []RightsizingChange{
		{Resource: "cpu", Field: "requests", From: "none", To: "30m"},
		{Resource: "memory", Field: "requests", From: "none", To: "40Mi"},
	}
	if res.Verdict != "under" || !reflect.DeepEqual(res.Changes, want) || !strings.Contains(res.Rollout, "OnDelete") {
		t.Fatalf("daemonset %+v", res)
	}
	ds, _ := cs.AppsV1().DaemonSets("kube-system").Get(ctx, "agent", metav1.GetOptions{})
	ag := ds.Spec.Template.Spec.Containers[0]
	if got := quantities(ag.Resources.Requests); !reflect.DeepEqual(got, map[string]string{"cpu": "30m", "memory": "40Mi"}) {
		t.Fatalf("agent requests %v", got)
	}
	if len(ag.Resources.Limits) != 0 {
		t.Fatalf("a limit was added: %v", ag.Resources.Limits)
	}
	if ds.Spec.UpdateStrategy.Type != appsv1.OnDeleteDaemonSetStrategyType {
		t.Fatal("update strategy changed")
	}
}
