//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

var secretGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}

// dryRunRecorder emulates the API server's dry-run server-side apply on the fake dynamic
// client: it returns the object as it WOULD be stored (existing merged with the patch) and
// never touches the tracker, and keeps every request so tests can assert on the options.
type dryRunRecorder struct {
	requests []clienttesting.PatchActionImpl
	failWith error
}

func installDryRunApply(dyn *dynamicfake.FakeDynamicClient) *dryRunRecorder {
	rec := &dryRunRecorder{}
	dyn.PrependReactor("patch", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		pa, ok := action.(clienttesting.PatchActionImpl)
		if !ok || pa.GetPatchType() != types.ApplyPatchType {
			return false, nil, nil
		}
		rec.requests = append(rec.requests, pa)
		if rec.failWith != nil {
			return true, nil, rec.failWith
		}
		if len(pa.PatchOptions.DryRun) == 0 {
			return true, nil, apierrors.NewBadRequest("test: diff must always dry-run")
		}
		var body map[string]any
		if err := json.Unmarshal(pa.GetPatch(), &body); err != nil {
			return true, nil, apierrors.NewBadRequest(err.Error())
		}
		merged := body
		if existing, err := dyn.Tracker().Get(pa.GetResource(), pa.GetNamespace(), pa.GetName()); err == nil {
			cur := existing.(*unstructured.Unstructured).DeepCopy()
			deepMerge(cur.Object, body)
			merged = cur.Object
		}
		return true, &unstructured.Unstructured{Object: merged}, nil
	})
	return rec
}

func newDiffProvider(t *testing.T, dynObjects ...runtime.Object) (*KubernetesProvider, *dynamicfake.FakeDynamicClient, *dryRunRecorder) {
	t.Helper()
	k, cs, dyn := newFakeFullProvider(map[schema.GroupVersionResource]string{
		networkPolicyGVR: "NetworkPolicyList", secretGVR: "SecretList",
	}, dynObjects)
	cs.Discovery().(*fakediscovery.FakeDiscovery).Resources = []*metav1.APIResourceList{
		{GroupVersion: "networking.k8s.io/v1", APIResources: []metav1.APIResource{{Name: "networkpolicies", Kind: "NetworkPolicy", Namespaced: true}}},
		{GroupVersion: "v1", APIResources: []metav1.APIResource{{Name: "secrets", Kind: "Secret", Namespaced: true}}},
	}
	return k, dyn, installDryRunApply(dyn)
}

func liveNetworkPolicy(managed []any) *unstructured.Unstructured {
	meta := map[string]any{
		"name": "deny-all", "namespace": "wtest", "labels": map[string]any{"app": "web"},
		"resourceVersion": "42", "uid": "abc", "generation": int64(3),
	}
	if managed != nil {
		meta["managedFields"] = managed
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": meta,
		"spec":   map[string]any{"podSelector": map[string]any{}, "policyTypes": []any{"Ingress"}},
		"status": map[string]any{"noise": "x"},
	}}
}

func TestKubernetesDiffManifestNewObjectIsDryRunAndCreatesNothing(t *testing.T) {
	ctx := context.Background()
	k, dyn, rec := newDiffProvider(t)

	res, err := k.DiffManifest(ctx, networkPolicyYAML)
	if err != nil || !res.Valid {
		t.Fatalf("diff: err=%v res=%+v", err, res)
	}
	o := res.Objects[0]
	if o.Exists || o.Live != "" || !o.Changed || o.Kind != "NetworkPolicy" || o.Name != "deny-all" || o.Namespace != "wtest" {
		t.Fatalf("unexpected object: %+v", o)
	}
	if !strings.Contains(o.Proposed, "kind: NetworkPolicy") || !strings.Contains(o.Proposed, "app: web") {
		t.Fatalf("proposed is not the YAML of the object:\n%s", o.Proposed)
	}
	if len(rec.requests) != 1 {
		t.Fatalf("want exactly one apply request, got %d", len(rec.requests))
	}
	opts := rec.requests[0].PatchOptions
	if len(opts.DryRun) != 1 || opts.DryRun[0] != metav1.DryRunAll || opts.FieldManager != "zyvor-axiom" || opts.Force == nil || !*opts.Force {
		t.Fatalf("diff must be dryRun=All with fieldManager=zyvor-axiom force=true: %+v", opts)
	}
	if _, err := getDyn(t, dyn, networkPolicyGVR, "wtest", "deny-all"); !apierrors.IsNotFound(err) {
		t.Fatalf("a diff must not create the object, got err=%v", err)
	}
}

func TestKubernetesDiffManifestExistingObjectShowsChangeAndForeignManagers(t *testing.T) {
	ctx := context.Background()
	managed := []any{
		map[string]any{"manager": "argocd-application-controller", "operation": "Apply", "apiVersion": "networking.k8s.io/v1"},
		map[string]any{"manager": "kube-controller-manager", "operation": "Update", "apiVersion": "networking.k8s.io/v1"},
		map[string]any{"manager": "zyvor-axiom", "operation": "Apply", "apiVersion": "networking.k8s.io/v1"},
		map[string]any{"manager": "some-status-writer", "operation": "Update", "subresource": "status", "apiVersion": "networking.k8s.io/v1"},
	}
	k, dyn, _ := newDiffProvider(t, liveNetworkPolicy(managed))

	changed := strings.Replace(networkPolicyYAML, "{app: web}", "{app: web, tier: edge}", 1)
	res, err := k.DiffManifest(ctx, changed)
	if err != nil || !res.Valid {
		t.Fatalf("diff: err=%v res=%+v", err, res)
	}
	o := res.Objects[0]
	if !o.Exists || !o.Changed {
		t.Fatalf("existing + edited object should be changed: %+v", o)
	}
	if strings.Contains(o.Live, "tier") || !strings.Contains(o.Proposed, "tier: edge") {
		t.Fatalf("edit must appear only in proposed.\nlive:\n%s\nproposed:\n%s", o.Live, o.Proposed)
	}
	for _, noisy := range []string{"resourceVersion", "uid:", "generation", "managedFields", "status"} {
		if strings.Contains(o.Live, noisy) || strings.Contains(o.Proposed, noisy) {
			t.Errorf("server-managed field %q leaked into the diff text", noisy)
		}
	}
	if len(o.ForeignManagers) != 1 || o.ForeignManagers[0] != "argocd-application-controller" {
		t.Fatalf("foreign managers = %v (axiom, core controllers and status-subresource writers must be ignored)", o.ForeignManagers)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "argocd-application-controller") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
	// the live object is untouched
	cur, _ := getDyn(t, dyn, networkPolicyGVR, "wtest", "deny-all")
	if _, has := cur.GetLabels()["tier"]; has {
		t.Fatal("a diff must not modify the live object")
	}

	// identical manifest on an object only Axiom manages: unchanged, no warning
	k2, _, _ := newDiffProvider(t, liveNetworkPolicy([]any{map[string]any{"manager": "zyvor-axiom", "operation": "Apply"}}))
	res, _ = k2.DiffManifest(ctx, networkPolicyYAML)
	if o := res.Objects[0]; o.Changed || len(o.ForeignManagers) != 0 || len(res.Warnings) != 0 {
		t.Fatalf("expected unchanged and warning-free, got %+v warnings=%v\nlive:\n%s\nproposed:\n%s", o, res.Warnings, o.Live, o.Proposed)
	}
}

func TestKubernetesDiffManifestMasksSecretValues(t *testing.T) {
	ctx := context.Background()
	live := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]any{
			"name": "db", "namespace": "wtest",
			"annotations": map[string]any{"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"password":"b2xkLXNlY3JldA=="}}`},
		},
		"data": map[string]any{"password": "b2xkLXNlY3JldA==", "user": "YWRtaW4="},
	}}
	k, _, _ := newDiffProvider(t, live)
	res, err := k.DiffManifest(ctx, "apiVersion: v1\nkind: Secret\nmetadata:\n  name: db\n  namespace: wtest\nstringData:\n  password: brand-new-secret\ndata:\n  extra: ZXh0cmE=\n")
	if err != nil || !res.Valid {
		t.Fatalf("diff: err=%v res=%+v", err, res)
	}
	all, _ := json.Marshal(res)
	for _, leak := range []string{"b2xkLXNlY3JldA", "YWRtaW4", "brand-new-secret", "ZXh0cmE"} {
		if strings.Contains(string(all), leak) {
			t.Fatalf("secret value %q leaked: %s", leak, all)
		}
	}
	o := res.Objects[0]
	if !strings.Contains(o.Live, "password: <redacted>") || !strings.Contains(o.Proposed, "extra: <redacted>") {
		t.Fatalf("keys should remain visible with masked values.\nlive:\n%s\nproposed:\n%s", o.Live, o.Proposed)
	}
}

func TestKubernetesDiffManifestSurfacesValidationErrorsAndBadInput(t *testing.T) {
	ctx := context.Background()
	k, _, rec := newDiffProvider(t)

	rec.failWith = apierrors.NewInvalid(schema.GroupKind{Group: "networking.k8s.io", Kind: "NetworkPolicy"}, "deny-all", nil)
	res, err := k.DiffManifest(ctx, networkPolicyYAML)
	if err != nil || res.Valid || len(res.Errors) == 0 {
		t.Fatalf("API rejection must come back as invalid with errors: err=%v res=%+v", err, res)
	}
	rec.failWith = nil

	for name, in := range map[string]string{
		"empty":        "",
		"not yaml":     "{{{{ definitely: [not valid",
		"no kind":      "metadata:\n  name: x\n",
		"no name":      "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\n",
		"unknown kind": "apiVersion: example.com/v1\nkind: Nope\nmetadata:\n  name: x\n",
		"multi-doc":    networkPolicyYAML + "\n---\n" + networkPolicyYAML,
	} {
		before := len(rec.requests)
		res, err := k.DiffManifest(ctx, in)
		if err != nil || res.Valid || len(res.Errors) == 0 {
			t.Errorf("%s: want invalid with errors, got err=%v res=%+v", name, err, res)
		}
		if len(rec.requests) != before {
			t.Errorf("%s: invalid input must not reach the API server", name)
		}
	}
}

// --- ResourceGraph ---

func ptr[T any](v T) *T { return &v }

func graphFixture() []runtime.Object {
	labels := map[string]string{"app": "web"}
	yes := true
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop", UID: "dep-uid"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(2)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "web",
						EnvFrom: []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "web-env"}}}},
						Env: []corev1.EnvVar{{Name: "DB", ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "db-creds"}, Key: "url"},
						}}},
					}},
					Volumes: []corev1.Volume{
						{Name: "cfg", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "web-config"}}}},
						{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "web-data"}}},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 2},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "web-abc", Namespace: "shop", UID: "rs-uid",
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "web", UID: "dep-uid", Controller: &yes}}},
		Spec:   appsv1.ReplicaSetSpec{Replicas: ptr(int32(2))},
		Status: appsv1.ReplicaSetStatus{Replicas: 2, ReadyReplicas: 2},
	}
	oldRS := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "web-old", Namespace: "shop", UID: "rs-old",
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "web", UID: "dep-uid", Controller: &yes}}},
		Spec: appsv1.ReplicaSetSpec{Replicas: ptr(int32(0))},
	}
	otherRS := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "other-rs", Namespace: "shop", UID: "rs-other",
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "other", UID: "other-uid", Controller: &yes}}},
		Spec: appsv1.ReplicaSetSpec{Replicas: ptr(int32(1))},
	}
	pod := func(n string, owner types.UID, phase corev1.PodPhase) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: "shop", Labels: labels,
				OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "web-abc", UID: owner, Controller: &yes}}},
			Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "web"}}},
			Status: corev1.PodStatus{Phase: phase},
		}
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop"},
		Spec:       corev1.ServiceSpec{Selector: labels, Type: corev1.ServiceTypeClusterIP},
	}
	headless := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "no-selector", Namespace: "shop"}}
	unrelated := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "shop"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "db"}},
	}
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop"},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
			Host: "shop.example.com",
			IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
				Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "web"}},
			}}}},
		}}},
	}
	otherIng := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "db-ing", Namespace: "shop"},
		Spec:       networkingv1.IngressSpec{DefaultBackend: &networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "db"}}},
	}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "web"},
			MinReplicas:    ptr(int32(2)), MaxReplicas: 6,
		},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "web-data", Namespace: "shop"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	// Real secret + configmap data that must never appear in a graph.
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: "shop"}, Data: map[string][]byte{"url": []byte("postgres://leaky-password@db")}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "web-config", Namespace: "shop"}, Data: map[string]string{"k": "leaky-config-value"}}
	return []runtime.Object{dep, rs, oldRS, otherRS, pod("web-abc-1", "rs-uid", corev1.PodRunning), pod("web-abc-2", "rs-uid", corev1.PodPending),
		svc, headless, unrelated, ing, otherIng, hpa, pvc, secret, cm}
}

func graphKinds(g ResourceGraph) map[string][]string {
	out := map[string][]string{}
	for _, n := range g.Nodes {
		out[n.Kind] = append(out[n.Kind], n.Name)
	}
	return out
}

func hasEdge(g ResourceGraph, from, to, rel string) bool {
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Relation == rel {
			return true
		}
	}
	return false
}

func TestKubernetesResourceGraphDeployment(t *testing.T) {
	k, _, _ := newFakeFullProvider(nil, nil, graphFixture()...)
	g, err := k.ResourceGraph(context.Background(), "shop", "Deployment", "web")
	if err != nil {
		t.Fatal(err)
	}
	if g.Root != "deployment/shop/web" {
		t.Fatalf("root = %q", g.Root)
	}
	kinds := graphKinds(g)
	if len(kinds["Pod"]) != 2 || len(kinds["Service"]) != 1 || kinds["Service"][0] != "web" ||
		len(kinds["Ingress"]) != 1 || len(kinds["HorizontalPodAutoscaler"]) != 1 ||
		len(kinds["ConfigMap"]) != 1 || len(kinds["PersistentVolumeClaim"]) != 1 {
		t.Fatalf("unexpected nodes: %+v", kinds)
	}
	if got := kinds["Secret"]; len(got) != 2 || got[0] != "web-env" && got[1] != "web-env" {
		t.Fatalf("secrets (envFrom + secretKeyRef) = %v", got)
	}
	if len(kinds["ReplicaSet"]) != 2 { // active + one scaled-down revision; the other Deployment's RS is excluded
		t.Fatalf("replicasets = %v", kinds["ReplicaSet"])
	}
	for _, n := range kinds["ReplicaSet"] {
		if n == "other-rs" {
			t.Fatal("a ReplicaSet of another Deployment leaked into the graph")
		}
	}
	for _, n := range kinds["Service"] {
		if n == "db" || n == "no-selector" {
			t.Fatalf("service %q does not select the pods", n)
		}
	}
	if !hasEdge(g, "deployment/shop/web", "replicaset/shop/web-abc", "owns") ||
		!hasEdge(g, "replicaset/shop/web-abc", "pod/shop/web-abc-1", "owns") ||
		!hasEdge(g, "service/shop/web", "deployment/shop/web", "selects") ||
		!hasEdge(g, "ingress/shop/web", "service/shop/web", "routes") ||
		!hasEdge(g, "horizontalpodautoscaler/shop/web", "deployment/shop/web", "scales") ||
		!hasEdge(g, "deployment/shop/web", "persistentvolumeclaim/shop/web-data", "mounts") ||
		!hasEdge(g, "deployment/shop/web", "secret/shop/db-creds", "mounts") {
		t.Fatalf("missing expected edges: %+v", g.Edges)
	}
	for _, n := range g.Nodes {
		if n.Kind == "PersistentVolumeClaim" && n.Status != "Bound" {
			t.Errorf("pvc status = %q", n.Status)
		}
	}
	// Never any Secret/ConfigMap contents.
	raw, _ := json.Marshal(g)
	for _, leak := range []string{"leaky-password", "leaky-config-value", "postgres://"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("graph leaked %q: %s", leak, raw)
		}
	}
}

func TestKubernetesResourceGraphPodWalksOwnersUp(t *testing.T) {
	k, _, _ := newFakeFullProvider(nil, nil, graphFixture()...)
	g, err := k.ResourceGraph(context.Background(), "shop", "Pod", "web-abc-1")
	if err != nil {
		t.Fatal(err)
	}
	if g.Root != "pod/shop/web-abc-1" {
		t.Fatalf("root = %q", g.Root)
	}
	if !hasEdge(g, "replicaset/shop/web-abc", "pod/shop/web-abc-1", "owns") ||
		!hasEdge(g, "deployment/shop/web", "replicaset/shop/web-abc", "owns") {
		t.Fatalf("owner chain missing: %+v", g.Edges)
	}
	// the pod's Service links to the top owner
	if !hasEdge(g, "service/shop/web", "deployment/shop/web", "selects") {
		t.Fatalf("service edge missing: %+v", g.Edges)
	}
}

func TestKubernetesResourceGraphErrors(t *testing.T) {
	k, _, _ := newFakeFullProvider(nil, nil, graphFixture()...)
	if _, err := k.ResourceGraph(context.Background(), "shop", "Deployment", "missing"); !apierrors.IsNotFound(err) {
		t.Fatalf("want NotFound, got %v", err)
	}
	if _, err := k.ResourceGraph(context.Background(), "shop", "Widget", "x"); err == nil || HTTPStatus(err) != 400 {
		t.Fatalf("unsupported kind should be a 400, got %v (%d)", err, HTTPStatus(err))
	}
}

func TestKubernetesResourceGraphCapsPods(t *testing.T) {
	yes := true
	objs := []runtime.Object{&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "big", Namespace: "shop", UID: "sts"},
		Spec:       appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"a": "b"}}}},
	}}
	for i := 0; i < maxGraphPods+10; i++ {
		objs = append(objs, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "big-" + strings.Repeat("x", i%3) + string(rune('a'+i%26)) + string(rune('a'+i/26)), Namespace: "shop",
			OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: "big", UID: "sts", Controller: &yes}},
		}})
	}
	k, _, _ := newFakeFullProvider(nil, nil, objs...)
	g, err := k.ResourceGraph(context.Background(), "shop", "StatefulSet", "big")
	if err != nil {
		t.Fatal(err)
	}
	if !g.Truncated || len(graphKinds(g)["Pod"]) != maxGraphPods {
		t.Fatalf("want %d pods and Truncated, got %d truncated=%v", maxGraphPods, len(graphKinds(g)["Pod"]), g.Truncated)
	}
}

func TestPodSpecReferencesCoversProjectedAndInit(t *testing.T) {
	cm, sec, pvc := podSpecReferences(&corev1.PodSpec{
		InitContainers: []corev1.Container{{EnvFrom: []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "init-cm"}}}}}},
		Volumes: []corev1.Volume{{VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{
			{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "proj-cm"}}},
			{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "proj-sec"}}},
		}}}}},
	})
	if strings.Join(cm, ",") != "init-cm,proj-cm" || strings.Join(sec, ",") != "proj-sec" || len(pvc) != 0 {
		t.Fatalf("cm=%v sec=%v pvc=%v", cm, sec, pvc)
	}
}
