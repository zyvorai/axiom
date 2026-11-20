//go:build kubernetes

package platform

import (
	"context"
	"net/http"
	"strings"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// These tests cover the write operations of the real Kubernetes provider that
// the HTTP API exposes, against client-go's fake clientset. Every test checks
// the object state actually stored in the fake, not just "no error".

// --- Namespaces ---

func TestKubernetesCreateNamespaceStoresObjectAndTrimsName(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()

	ns, err := k.CreateNamespace(ctx, "  team-a  ")
	if err != nil {
		t.Fatalf("CreateNamespace: %v", err)
	}
	if ns.Name != "team-a" || ns.Status != "Active" {
		t.Fatalf("unexpected result: %+v", ns)
	}
	stored, err := cs.CoreV1().Namespaces().Get(ctx, "team-a", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("namespace not stored under the trimmed name: %v", err)
	}
	if stored.Name != "team-a" {
		t.Fatalf("unexpected stored namespace: %+v", stored)
	}
	list, err := k.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	if len(list) != 1 || list[0].Name != "team-a" {
		t.Fatalf("expected the new namespace in ListNamespaces, got %+v", list)
	}
}

func TestKubernetesCreateNamespaceRejectsBlankName(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	for _, name := range []string{"", "   "} {
		if _, err := k.CreateNamespace(ctx, name); err == nil {
			t.Fatalf("expected CreateNamespace(%q) to fail", name)
		}
	}
	all, _ := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if len(all.Items) != 0 {
		t.Fatalf("blank-name request must not create anything, got %d namespaces", len(all.Items))
	}
}

func TestKubernetesCreateNamespaceDuplicateIsConflict(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	if _, err := k.CreateNamespace(ctx, "dup"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := k.CreateNamespace(ctx, "dup")
	if !apierrors.IsAlreadyExists(err) {
		t.Fatalf("expected AlreadyExists, got %v", err)
	}
	wantStatus(t, err, http.StatusConflict)
}

func TestKubernetesDeleteNamespaceRemovesOnlyThatNamespace(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gone"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kept"}},
	)
	if err := k.DeleteNamespace(ctx, "gone"); err != nil {
		t.Fatalf("DeleteNamespace: %v", err)
	}
	if _, err := cs.CoreV1().Namespaces().Get(ctx, "gone", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected namespace gone, got err=%v", err)
	}
	if _, err := cs.CoreV1().Namespaces().Get(ctx, "kept", metav1.GetOptions{}); err != nil {
		t.Fatalf("unrelated namespace must survive: %v", err)
	}
}

func TestKubernetesDeleteNamespaceMissingIsNotFound(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	wantStatus(t, k.DeleteNamespace(context.Background(), "nope"), http.StatusNotFound)
}

// --- Secrets ---

func TestKubernetesCreateSecretPersistsDataTypeAndLabels(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()

	detail, err := k.CreateSecret(ctx, CreateSecretRequest{
		Namespace: "wtest", Name: "db", Type: "kubernetes.io/basic-auth",
		Data: map[string]string{"username": "admin", "password": "s3cret"}, Labels: map[string]string{"app": "db"},
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if detail.Values != nil {
		t.Fatalf("CreateSecret must not echo secret values back, got %+v", detail.Values)
	}
	if len(detail.Keys) != 2 || detail.Keys[0].Key != "password" || detail.Keys[1].Key != "username" {
		t.Fatalf("expected sorted key infos, got %+v", detail.Keys)
	}

	stored, err := cs.CoreV1().Secrets("wtest").Get(ctx, "db", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("secret not stored: %v", err)
	}
	if stored.Type != corev1.SecretTypeBasicAuth {
		t.Fatalf("type not persisted: %q", stored.Type)
	}
	if string(stored.Data["password"]) != "s3cret" || string(stored.Data["username"]) != "admin" {
		t.Fatalf("data not persisted: %+v", stored.Data)
	}
	if stored.Labels["app"] != "db" {
		t.Fatalf("labels not persisted: %+v", stored.Labels)
	}
}

func TestKubernetesCreateSecretDefaultsToOpaqueAndMergesKeys(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	_, err := k.CreateSecret(ctx, CreateSecretRequest{
		Namespace: "wtest", Name: "merge",
		Data: map[string]string{"A": "from-data"},
		Keys: map[string]string{"A": "ignored", "B": "from-keys"},
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	stored, _ := cs.CoreV1().Secrets("wtest").Get(ctx, "merge", metav1.GetOptions{})
	if stored.Type != corev1.SecretTypeOpaque {
		t.Fatalf("expected default Opaque type, got %q", stored.Type)
	}
	if string(stored.Data["A"]) != "from-data" || string(stored.Data["B"]) != "from-keys" {
		t.Fatalf("Data must win over Keys, Keys must fill gaps: %+v", stored.Data)
	}
}

func TestKubernetesCreateSecretRejectsMissingFields(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	bad := []CreateSecretRequest{
		{},
		{Namespace: "wtest", Data: map[string]string{"a": "b"}},
		{Name: "x", Data: map[string]string{"a": "b"}},
		{Namespace: "wtest", Name: "no-data"},
		{Namespace: "  ", Name: "blank-ns", Data: map[string]string{"a": "b"}},
	}
	for i, req := range bad {
		if _, err := k.CreateSecret(ctx, req); err == nil {
			t.Fatalf("case %d: expected CreateSecret(%+v) to fail", i, req)
		}
	}
	all, _ := cs.CoreV1().Secrets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(all.Items) != 0 {
		t.Fatalf("invalid requests must not create secrets, got %d", len(all.Items))
	}
}

func TestKubernetesCreateSecretDuplicateIsConflictButOtherNamespaceIsFine(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	req := CreateSecretRequest{Namespace: "a", Name: "same", Data: map[string]string{"k": "v"}}
	if _, err := k.CreateSecret(ctx, req); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := k.CreateSecret(ctx, req)
	wantStatus(t, err, http.StatusConflict)

	req.Namespace = "b"
	if _, err := k.CreateSecret(ctx, req); err != nil {
		t.Fatalf("same name in another namespace must be allowed: %v", err)
	}
	for _, ns := range []string{"a", "b"} {
		if _, err := cs.CoreV1().Secrets(ns).Get(ctx, "same", metav1.GetOptions{}); err != nil {
			t.Fatalf("secret missing in %s: %v", ns, err)
		}
	}
}

func TestKubernetesDeleteSecretRemovesOnlyTargetInNamespace(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "a"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "b"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "a"}},
	)
	if err := k.DeleteSecret(ctx, "a", "s"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if _, err := cs.CoreV1().Secrets("a").Get(ctx, "s", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected a/s deleted, err=%v", err)
	}
	for _, key := range [][2]string{{"b", "s"}, {"a", "other"}} {
		if _, err := cs.CoreV1().Secrets(key[0]).Get(ctx, key[1], metav1.GetOptions{}); err != nil {
			t.Fatalf("%s/%s must survive (cross-namespace leak?): %v", key[0], key[1], err)
		}
	}
	wantStatus(t, k.DeleteSecret(ctx, "a", "s"), http.StatusNotFound)
}

// --- ConfigMaps ---

func TestKubernetesCreateConfigMapPersistsDataAndLabels(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	detail, err := k.CreateConfigMap(ctx, CreateConfigMapRequest{
		Namespace: "wtest", Name: "cfg", Data: map[string]string{"k": "v"}, Labels: map[string]string{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("CreateConfigMap: %v", err)
	}
	if detail.Data["k"] != "v" || detail.Labels["env"] != "prod" {
		t.Fatalf("unexpected result: %+v", detail)
	}
	stored, err := cs.CoreV1().ConfigMaps("wtest").Get(ctx, "cfg", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("configmap not stored: %v", err)
	}
	if stored.Data["k"] != "v" || stored.Labels["env"] != "prod" {
		t.Fatalf("unexpected stored object: %+v", stored)
	}
}

func TestKubernetesCreateConfigMapRejectsMissingFields(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	bad := []CreateConfigMapRequest{
		{},
		{Namespace: "wtest", Data: map[string]string{"a": "b"}},
		{Name: "x", Data: map[string]string{"a": "b"}},
		{Namespace: "wtest", Name: "no-data"},
	}
	for i, req := range bad {
		if _, err := k.CreateConfigMap(ctx, req); err == nil {
			t.Fatalf("case %d: expected CreateConfigMap(%+v) to fail", i, req)
		}
	}
	all, _ := cs.CoreV1().ConfigMaps(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(all.Items) != 0 {
		t.Fatalf("invalid requests must not create configmaps, got %d", len(all.Items))
	}
}

func TestKubernetesCreateConfigMapDuplicateIsConflict(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider()
	req := CreateConfigMapRequest{Namespace: "wtest", Name: "dup", Data: map[string]string{"a": "b"}}
	if _, err := k.CreateConfigMap(ctx, req); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := k.CreateConfigMap(ctx, req)
	wantStatus(t, err, http.StatusConflict)
}

func TestKubernetesDeleteConfigMapRemovesOnlyTargetInNamespace(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "a"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "b"}},
	)
	if err := k.DeleteConfigMap(ctx, "a", "c"); err != nil {
		t.Fatalf("DeleteConfigMap: %v", err)
	}
	if _, err := cs.CoreV1().ConfigMaps("a").Get(ctx, "c", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected a/c deleted, err=%v", err)
	}
	if _, err := cs.CoreV1().ConfigMaps("b").Get(ctx, "c", metav1.GetOptions{}); err != nil {
		t.Fatalf("b/c must survive (cross-namespace leak?): %v", err)
	}
	wantStatus(t, k.DeleteConfigMap(ctx, "a", "c"), http.StatusNotFound)
}

// --- HorizontalPodAutoscalers ---

func TestKubernetesCreateHPAPersistsSpecWithDefaults(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	sum, err := k.CreateHPA(ctx, CreateHPARequest{Namespace: "wtest", Name: "web", TargetKind: "Deployment", TargetName: "web"})
	if err != nil {
		t.Fatalf("CreateHPA: %v", err)
	}
	if sum.MinReplicas != 1 || sum.MaxReplicas != 1 || sum.TargetCPUPercent != 80 {
		t.Fatalf("unexpected defaults in summary: %+v", sum)
	}
	hpa, err := cs.AutoscalingV2().HorizontalPodAutoscalers("wtest").Get(ctx, "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("hpa not stored: %v", err)
	}
	ref := hpa.Spec.ScaleTargetRef
	if ref.Kind != "Deployment" || ref.Name != "web" || ref.APIVersion != "apps/v1" {
		t.Fatalf("unexpected scaleTargetRef: %+v", ref)
	}
	if hpa.Spec.MinReplicas == nil || *hpa.Spec.MinReplicas != 1 || hpa.Spec.MaxReplicas != 1 {
		t.Fatalf("unexpected replica bounds: min=%v max=%d", hpa.Spec.MinReplicas, hpa.Spec.MaxReplicas)
	}
	if len(hpa.Spec.Metrics) != 1 {
		t.Fatalf("expected one metric, got %+v", hpa.Spec.Metrics)
	}
	m := hpa.Spec.Metrics[0]
	if m.Type != autoscalingv2.ResourceMetricSourceType || m.Resource == nil || m.Resource.Name != corev1.ResourceCPU ||
		m.Resource.Target.AverageUtilization == nil || *m.Resource.Target.AverageUtilization != 80 {
		t.Fatalf("expected default 80%% CPU utilization target, got %+v", m)
	}

	list, _ := k.ListHPAs(ctx, "wtest")
	if len(list) != 1 || list[0].TargetName != "web" {
		t.Fatalf("expected HPA in ListHPAs, got %+v", list)
	}
}

func TestKubernetesCreateHPAUsesRequestedBoundsAndValidatesMax(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	if _, err := k.CreateHPA(ctx, CreateHPARequest{
		Namespace: "wtest", Name: "explicit", TargetKind: "StatefulSet", TargetName: "db",
		MinReplicas: 2, MaxReplicas: 9, TargetCPUPercent: 55,
	}); err != nil {
		t.Fatalf("CreateHPA: %v", err)
	}
	hpa, _ := cs.AutoscalingV2().HorizontalPodAutoscalers("wtest").Get(ctx, "explicit", metav1.GetOptions{})
	if *hpa.Spec.MinReplicas != 2 || hpa.Spec.MaxReplicas != 9 || *hpa.Spec.Metrics[0].Resource.Target.AverageUtilization != 55 {
		t.Fatalf("requested values not persisted: %+v", hpa.Spec)
	}
	if hpa.Spec.ScaleTargetRef.Kind != "StatefulSet" {
		t.Fatalf("target kind not persisted: %+v", hpa.Spec.ScaleTargetRef)
	}

	// max < min (with max explicitly set) is invalid input, not something to silently rewrite.
	if _, err := k.CreateHPA(ctx, CreateHPARequest{Namespace: "wtest", Name: "inverted", TargetKind: "Deployment", TargetName: "x", MinReplicas: 4, MaxReplicas: 2}); err == nil {
		t.Fatal("expected maxReplicas < minReplicas to be rejected")
	}
	if _, err := cs.AutoscalingV2().HorizontalPodAutoscalers("wtest").Get(ctx, "inverted", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("a rejected request must not create an HPA, err=%v", err)
	}

	// max unset (0) falls back to min.
	if _, err := k.CreateHPA(ctx, CreateHPARequest{Namespace: "wtest", Name: "nomax", TargetKind: "Deployment", TargetName: "x", MinReplicas: 4}); err != nil {
		t.Fatalf("CreateHPA nomax: %v", err)
	}
	hpa, _ = cs.AutoscalingV2().HorizontalPodAutoscalers("wtest").Get(ctx, "nomax", metav1.GetOptions{})
	if hpa.Spec.MaxReplicas != 4 {
		t.Fatalf("unset max should default to min (4), got %d", hpa.Spec.MaxReplicas)
	}
}

func TestKubernetesCreateHPARejectsMissingFieldsAndDuplicates(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	for i, req := range []CreateHPARequest{
		{},
		{Namespace: "wtest", Name: "n", TargetKind: "Deployment"},
		{Namespace: "wtest", Name: "n", TargetName: "x"},
		{Namespace: "wtest", TargetKind: "Deployment", TargetName: "x"},
		{Name: "n", TargetKind: "Deployment", TargetName: "x"},
	} {
		if _, err := k.CreateHPA(ctx, req); err == nil {
			t.Fatalf("case %d: expected error for %+v", i, req)
		}
	}
	all, _ := cs.AutoscalingV2().HorizontalPodAutoscalers(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(all.Items) != 0 {
		t.Fatalf("invalid requests must not create HPAs, got %d", len(all.Items))
	}

	ok := CreateHPARequest{Namespace: "wtest", Name: "dup", TargetKind: "Deployment", TargetName: "x"}
	if _, err := k.CreateHPA(ctx, ok); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := k.CreateHPA(ctx, ok)
	wantStatus(t, err, http.StatusConflict)
}

func TestKubernetesDeleteHPARemovesOnlyTarget(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "h", Namespace: "a"}},
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "h", Namespace: "b"}},
	)
	if err := k.DeleteHPA(ctx, "a", "h"); err != nil {
		t.Fatalf("DeleteHPA: %v", err)
	}
	if _, err := cs.AutoscalingV2().HorizontalPodAutoscalers("a").Get(ctx, "h", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected a/h deleted, err=%v", err)
	}
	if _, err := cs.AutoscalingV2().HorizontalPodAutoscalers("b").Get(ctx, "h", metav1.GetOptions{}); err != nil {
		t.Fatalf("b/h must survive: %v", err)
	}
	wantStatus(t, k.DeleteHPA(ctx, "a", "h"), http.StatusNotFound)
}

// --- NetworkPolicy delete ---

func TestKubernetesDeleteNetworkPolicy(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "np", Namespace: "a"}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "np", Namespace: "b"}},
	)
	if err := k.DeleteNetworkPolicy(ctx, "a", "np"); err != nil {
		t.Fatalf("DeleteNetworkPolicy: %v", err)
	}
	if _, err := cs.NetworkingV1().NetworkPolicies("a").Get(ctx, "np", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected a/np deleted, err=%v", err)
	}
	if _, err := cs.NetworkingV1().NetworkPolicies("b").Get(ctx, "np", metav1.GetOptions{}); err != nil {
		t.Fatalf("b/np must survive: %v", err)
	}
	wantStatus(t, k.DeleteNetworkPolicy(ctx, "a", "np"), http.StatusNotFound)
}

// --- RBAC ---

func TestKubernetesCreateRolePersistsRules(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	sum, err := k.CreateRole(ctx, CreateRoleRequest{Namespace: "wtest", Name: "pod-reader", Rules: []RBACPolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods", "pods/log"}, Verbs: []string{"get", "list"}},
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"}},
	}})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if sum.Rules != 2 || sum.Name != "pod-reader" || sum.Namespace != "wtest" {
		t.Fatalf("unexpected summary: %+v", sum)
	}
	role, err := cs.RbacV1().Roles("wtest").Get(ctx, "pod-reader", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("role not stored: %v", err)
	}
	if len(role.Rules) != 2 {
		t.Fatalf("expected 2 stored rules, got %+v", role.Rules)
	}
	r0 := role.Rules[0]
	if strings.Join(r0.Resources, ",") != "pods,pods/log" || strings.Join(r0.Verbs, ",") != "get,list" || len(r0.APIGroups) != 1 || r0.APIGroups[0] != "" {
		t.Fatalf("rule 0 not persisted faithfully: %+v", r0)
	}
	if role.Rules[1].APIGroups[0] != "apps" || role.Rules[1].Resources[0] != "deployments" {
		t.Fatalf("rule 1 not persisted faithfully: %+v", role.Rules[1])
	}
}

func TestKubernetesCreateRoleRejectsInvalidAndDuplicates(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	rule := []RBACPolicyRule{{Resources: []string{"pods"}, Verbs: []string{"get"}}}
	for i, req := range []CreateRoleRequest{
		{},
		{Namespace: "wtest", Rules: rule},
		{Name: "r", Rules: rule},
		{Namespace: "wtest", Name: "no-rules"},
	} {
		if _, err := k.CreateRole(ctx, req); err == nil {
			t.Fatalf("case %d: expected error for %+v", i, req)
		}
	}
	all, _ := cs.RbacV1().Roles(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(all.Items) != 0 {
		t.Fatalf("invalid requests must not create roles, got %d", len(all.Items))
	}
	ok := CreateRoleRequest{Namespace: "wtest", Name: "dup", Rules: rule}
	if _, err := k.CreateRole(ctx, ok); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := k.CreateRole(ctx, ok)
	wantStatus(t, err, http.StatusConflict)
}

func TestKubernetesDeleteRoleRemovesOnlyTarget(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "a"}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "b"}},
	)
	if err := k.DeleteRole(ctx, "a", "r"); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
	if _, err := cs.RbacV1().Roles("a").Get(ctx, "r", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected a/r deleted, err=%v", err)
	}
	if _, err := cs.RbacV1().Roles("b").Get(ctx, "r", metav1.GetOptions{}); err != nil {
		t.Fatalf("b/r must survive: %v", err)
	}
	wantStatus(t, k.DeleteRole(ctx, "a", "r"), http.StatusNotFound)
}

func TestKubernetesCreateRoleBindingPersistsRoleRefAndSubjects(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	sum, err := k.CreateRoleBinding(ctx, CreateRoleBindingRequest{
		Namespace: "wtest", Name: "rb", RoleName: "pod-reader",
		Subjects: []RBACSubject{
			{Kind: "ServiceAccount", Name: "ci"},
			{Kind: "ServiceAccount", Name: "other-ns-sa", Namespace: "elsewhere"},
		},
	})
	if err != nil {
		t.Fatalf("CreateRoleBinding: %v", err)
	}
	if sum.RoleRef != "Role/pod-reader" || sum.Subjects != 2 {
		t.Fatalf("unexpected summary: %+v", sum)
	}
	rb, err := cs.RbacV1().RoleBindings("wtest").Get(ctx, "rb", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("rolebinding not stored: %v", err)
	}
	if rb.RoleRef.Kind != "Role" || rb.RoleRef.Name != "pod-reader" || rb.RoleRef.APIGroup != "rbac.authorization.k8s.io" {
		t.Fatalf("unexpected roleRef: %+v", rb.RoleRef)
	}
	if len(rb.Subjects) != 2 {
		t.Fatalf("expected 2 subjects, got %+v", rb.Subjects)
	}
	if rb.Subjects[0].Namespace != "wtest" {
		t.Fatalf("a ServiceAccount subject without namespace must default to the binding's namespace, got %+v", rb.Subjects[0])
	}
	if rb.Subjects[1].Namespace != "elsewhere" {
		t.Fatalf("explicit subject namespace must be preserved, got %+v", rb.Subjects[1])
	}
}

func TestKubernetesCreateRoleBindingRejectsInvalidAndDuplicates(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	subj := []RBACSubject{{Kind: "User", Name: "alice"}}
	for i, req := range []CreateRoleBindingRequest{
		{},
		{Namespace: "wtest", Name: "rb", Subjects: subj},
		{Namespace: "wtest", RoleName: "r", Subjects: subj},
		{Name: "rb", RoleName: "r", Subjects: subj},
		{Namespace: "wtest", Name: "rb", RoleName: "r"},
	} {
		if _, err := k.CreateRoleBinding(ctx, req); err == nil {
			t.Fatalf("case %d: expected error for %+v", i, req)
		}
	}
	all, _ := cs.RbacV1().RoleBindings(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(all.Items) != 0 {
		t.Fatalf("invalid requests must not create rolebindings, got %d", len(all.Items))
	}
	ok := CreateRoleBindingRequest{Namespace: "wtest", Name: "dup", RoleName: "r", Subjects: subj}
	if _, err := k.CreateRoleBinding(ctx, ok); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := k.CreateRoleBinding(ctx, ok)
	wantStatus(t, err, http.StatusConflict)
}

func TestKubernetesDeleteRoleBindingRemovesOnlyTarget(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "a"}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "b"}},
	)
	if err := k.DeleteRoleBinding(ctx, "a", "rb"); err != nil {
		t.Fatalf("DeleteRoleBinding: %v", err)
	}
	if _, err := cs.RbacV1().RoleBindings("a").Get(ctx, "rb", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected a/rb deleted, err=%v", err)
	}
	if _, err := cs.RbacV1().RoleBindings("b").Get(ctx, "rb", metav1.GetOptions{}); err != nil {
		t.Fatalf("b/rb must survive: %v", err)
	}
	wantStatus(t, k.DeleteRoleBinding(ctx, "a", "rb"), http.StatusNotFound)
}

// --- Jobs / CronJobs ---

func TestKubernetesDeleteJob(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j", Namespace: "a"}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j", Namespace: "b"}},
	)
	if err := k.DeleteJob(ctx, "a", "j"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	if _, err := cs.BatchV1().Jobs("a").Get(ctx, "j", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected a/j deleted, err=%v", err)
	}
	if _, err := cs.BatchV1().Jobs("b").Get(ctx, "j", metav1.GetOptions{}); err != nil {
		t.Fatalf("b/j must survive: %v", err)
	}
	wantStatus(t, k.DeleteJob(ctx, "a", "j"), http.StatusNotFound)
}

func newCronJob(ns, name, schedule string, suspend bool) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID("uid-" + ns + "-" + name)},
		Spec: batchv1.CronJobSpec{
			Schedule: schedule, Suspend: &suspend,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"tier": "batch"}},
				Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers:    []corev1.Container{{Name: "c", Image: "busybox:1.36", Command: []string{"true"}}},
				}}},
			},
		},
	}
}

func TestKubernetesTriggerCronJobCreatesOwnedJobFromTemplate(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newCronJob("a", "nightly", "0 3 * * *", false), newCronJob("b", "nightly", "0 4 * * *", false))
	installJobNameGenerator(cs)

	sum, err := k.TriggerCronJob(ctx, "a", "nightly")
	if err != nil {
		t.Fatalf("TriggerCronJob: %v", err)
	}
	if sum.Namespace != "a" || sum.OwnerCronJob != "nightly" {
		t.Fatalf("unexpected summary: %+v", sum)
	}

	jobsA, _ := cs.BatchV1().Jobs("a").List(ctx, metav1.ListOptions{})
	if len(jobsA.Items) != 1 {
		t.Fatalf("expected exactly one job in namespace a, got %d", len(jobsA.Items))
	}
	job := jobsA.Items[0]
	if !strings.HasPrefix(job.Name, "nightly-manual-") {
		t.Fatalf("expected generated name nightly-manual-*, got %q", job.Name)
	}
	if job.Annotations["cronjob.kubernetes.io/instantiate"] != "manual" {
		t.Fatalf("missing manual-instantiate annotation: %+v", job.Annotations)
	}
	if job.Labels["tier"] != "batch" {
		t.Fatalf("job template labels not copied: %+v", job.Labels)
	}
	if len(job.OwnerReferences) != 1 || job.OwnerReferences[0].Kind != "CronJob" || job.OwnerReferences[0].Name != "nightly" ||
		job.OwnerReferences[0].UID != "uid-a-nightly" {
		t.Fatalf("expected an ownerReference to a/nightly, got %+v", job.OwnerReferences)
	}
	if len(job.Spec.Template.Spec.Containers) != 1 || job.Spec.Template.Spec.Containers[0].Image != "busybox:1.36" {
		t.Fatalf("job spec not copied from the cronjob template: %+v", job.Spec.Template.Spec)
	}

	jobsB, _ := cs.BatchV1().Jobs("b").List(ctx, metav1.ListOptions{})
	if len(jobsB.Items) != 0 {
		t.Fatalf("triggering a/nightly must not create a job in b, got %d", len(jobsB.Items))
	}
}

func TestKubernetesTriggerCronJobMissingIsNotFound(t *testing.T) {
	k, cs := newFakeKubernetesProvider()
	_, err := k.TriggerCronJob(context.Background(), "a", "nope")
	wantStatus(t, err, http.StatusNotFound)
	jobs, _ := cs.BatchV1().Jobs(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Fatalf("no job may be created for a missing cronjob, got %d", len(jobs.Items))
	}
}

func TestKubernetesSetCronJobSuspendTogglesOnlyTarget(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newCronJob("a", "cj", "* * * * *", false), newCronJob("b", "cj", "* * * * *", false))

	if err := k.SetCronJobSuspend(ctx, "a", "cj", true); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	a, _ := cs.BatchV1().CronJobs("a").Get(ctx, "cj", metav1.GetOptions{})
	b, _ := cs.BatchV1().CronJobs("b").Get(ctx, "cj", metav1.GetOptions{})
	if a.Spec.Suspend == nil || !*a.Spec.Suspend {
		t.Fatalf("a/cj should be suspended: %+v", a.Spec.Suspend)
	}
	if b.Spec.Suspend == nil || *b.Spec.Suspend {
		t.Fatalf("b/cj must remain unsuspended (cross-namespace leak?): %+v", b.Spec.Suspend)
	}
	if a.Spec.Schedule != "* * * * *" || len(a.Spec.JobTemplate.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("suspend must not clobber the rest of the spec: %+v", a.Spec)
	}
	list, _ := k.ListCronJobs(ctx, "a")
	if len(list) != 1 || !list[0].Suspend {
		t.Fatalf("ListCronJobs should report suspended, got %+v", list)
	}

	if err := k.SetCronJobSuspend(ctx, "a", "cj", false); err != nil {
		t.Fatalf("resume: %v", err)
	}
	a, _ = cs.BatchV1().CronJobs("a").Get(ctx, "cj", metav1.GetOptions{})
	if a.Spec.Suspend == nil || *a.Spec.Suspend {
		t.Fatalf("a/cj should be resumed: %+v", a.Spec.Suspend)
	}
}

func TestKubernetesSetCronJobSuspendMissingIsNotFound(t *testing.T) {
	k, _ := newFakeKubernetesProvider()
	wantStatus(t, k.SetCronJobSuspend(context.Background(), "a", "nope", true), http.StatusNotFound)
}

func TestKubernetesDeleteCronJob(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider(newCronJob("a", "cj", "* * * * *", false), newCronJob("b", "cj", "* * * * *", false))
	if err := k.DeleteCronJob(ctx, "a", "cj"); err != nil {
		t.Fatalf("DeleteCronJob: %v", err)
	}
	if _, err := cs.BatchV1().CronJobs("a").Get(ctx, "cj", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected a/cj deleted, err=%v", err)
	}
	if _, err := cs.BatchV1().CronJobs("b").Get(ctx, "cj", metav1.GetOptions{}); err != nil {
		t.Fatalf("b/cj must survive: %v", err)
	}
	wantStatus(t, k.DeleteCronJob(ctx, "a", "cj"), http.StatusNotFound)
}

// --- Helm (Job-based) ---

func TestKubernetesInstallHelmChartCreatesJobWithLiteralArgs(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	res, err := k.InstallHelmChart(ctx, InstallHelmRequest{
		Namespace: "apps", ReleaseName: "web", RepoURL: "https://charts.example/repo", ChartName: "nginx", ChartVersion: "1.2.3",
	})
	if err != nil {
		t.Fatalf("InstallHelmChart: %v", err)
	}
	if !res.OK || res.Action != "install" || !strings.HasPrefix(res.JobName, "axiom-system/axiom-helm-install-web-") {
		t.Fatalf("unexpected result: %+v", res)
	}
	jobs, _ := cs.BatchV1().Jobs("axiom-system").List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 1 {
		t.Fatalf("expected one helm job, got %d", len(jobs.Items))
	}
	job := jobs.Items[0]
	if job.Labels["axiom.zyvor.dev/helm-action"] != "install" || job.Labels["axiom.zyvor.dev/release"] != "web" {
		t.Fatalf("unexpected job labels: %+v", job.Labels)
	}
	c := job.Spec.Template.Spec.Containers[0]
	want := "helm install web nginx --repo https://charts.example/repo -n apps --create-namespace --version 1.2.3"
	if got := strings.Join(c.Command, " "); got != want {
		t.Fatalf("unexpected helm command:\n got %q\nwant %q", got, want)
	}
	if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever || job.Spec.Template.Spec.ServiceAccountName != "zyvor-axiom" {
		t.Fatalf("unexpected pod spec: %+v", job.Spec.Template.Spec)
	}
	if len(job.Spec.Template.Spec.Volumes) != 0 {
		t.Fatalf("no values were given, so no values volume/configmap expected: %+v", job.Spec.Template.Spec.Volumes)
	}
	cms, _ := cs.CoreV1().ConfigMaps("axiom-system").List(ctx, metav1.ListOptions{})
	if len(cms.Items) != 0 {
		t.Fatalf("no values were given, so no configmap expected, got %d", len(cms.Items))
	}
}

func TestKubernetesInstallHelmChartValuesGoToMountedSecret(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	values := "replicaCount: 3\nimage:\n  tag: v9\n"
	if _, err := k.InstallHelmChart(ctx, InstallHelmRequest{
		Namespace: "apps", ReleaseName: "web", RepoURL: "https://charts.example/repo", ChartName: "nginx", Values: values,
	}); err != nil {
		t.Fatalf("InstallHelmChart: %v", err)
	}
	jobs, _ := cs.BatchV1().Jobs("axiom-system").List(ctx, metav1.ListOptions{})
	secs, _ := cs.CoreV1().Secrets("axiom-system").List(ctx, metav1.ListOptions{})
	cms, _ := cs.CoreV1().ConfigMaps("axiom-system").List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 1 || len(secs.Items) != 1 {
		t.Fatalf("expected 1 job + 1 secret, got %d/%d", len(jobs.Items), len(secs.Items))
	}
	if len(cms.Items) != 0 {
		t.Fatalf("chart values (often passwords) must not be stored in a plaintext ConfigMap, got %d", len(cms.Items))
	}
	sec := secs.Items[0]
	if string(sec.Data["values.yaml"]) != values && sec.StringData["values.yaml"] != values {
		t.Fatalf("values not stored verbatim in the secret: %+v", sec)
	}
	pod := jobs.Items[0].Spec.Template.Spec
	if len(pod.Volumes) != 1 || pod.Volumes[0].Secret == nil || pod.Volumes[0].Secret.SecretName != sec.Name {
		t.Fatalf("job must mount the values secret %q: %+v", sec.Name, pod.Volumes)
	}
	cmd := strings.Join(pod.Containers[0].Command, " ")
	if !strings.HasSuffix(cmd, "-f /values/values.yaml") {
		t.Fatalf("helm must be pointed at the mounted values file: %q", cmd)
	}
}

func TestKubernetesInstallHelmChartDoesNotInterpolateUserInputIntoAShell(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	evil := "x; rm -rf / #"
	if _, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "apps", ReleaseName: evil, RepoURL: "https://r.example", ChartName: "c"}); err != nil {
		t.Fatalf("InstallHelmChart: %v", err)
	}
	jobs, _ := cs.BatchV1().Jobs("axiom-system").List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs.Items))
	}
	if jobs.Items[0].Name != strings.ToLower(jobs.Items[0].Name) || strings.ContainsAny(jobs.Items[0].Name, " ;#/") {
		t.Fatalf("job name must be DNS-safe, got %q", jobs.Items[0].Name)
	}
	cmd := jobs.Items[0].Spec.Template.Spec.Containers[0].Command
	if cmd[0] != "helm" || cmd[2] != evil {
		t.Fatalf("the release name must remain a single literal argv element, got %q", cmd)
	}
}

func TestKubernetesInstallHelmChartRequiresAllFields(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()
	full := InstallHelmRequest{Namespace: "apps", ReleaseName: "web", RepoURL: "https://r.example", ChartName: "c"}
	for name, mutate := range map[string]func(*InstallHelmRequest){
		"namespace": func(r *InstallHelmRequest) { r.Namespace = "" },
		"release":   func(r *InstallHelmRequest) { r.ReleaseName = " " },
		"repo":      func(r *InstallHelmRequest) { r.RepoURL = "" },
		"chart":     func(r *InstallHelmRequest) { r.ChartName = "" },
	} {
		req := full
		mutate(&req)
		if _, err := k.InstallHelmChart(ctx, req); err == nil {
			t.Fatalf("missing %s: expected an error", name)
		}
	}
	jobs, _ := cs.BatchV1().Jobs(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Fatalf("invalid requests must not create jobs, got %d", len(jobs.Items))
	}
}

func TestKubernetesHelmUninstallAndRollbackCreateJobs(t *testing.T) {
	ctx := context.Background()
	k, cs := newFakeKubernetesProvider()

	if res, err := k.HelmUninstall(ctx, "apps", "web"); err != nil || !res.OK || res.Action != "uninstall" {
		t.Fatalf("HelmUninstall: res=%+v err=%v", res, err)
	}
	if res, err := k.HelmRollback(ctx, "apps", "web", 4); err != nil || !res.OK || res.Action != "rollback" {
		t.Fatalf("HelmRollback: res=%+v err=%v", res, err)
	}
	jobs, _ := cs.BatchV1().Jobs("axiom-system").List(ctx, metav1.ListOptions{})
	cmds := map[string]string{}
	for _, j := range jobs.Items {
		cmds[j.Labels["axiom.zyvor.dev/helm-action"]] = strings.Join(j.Spec.Template.Spec.Containers[0].Command, " ")
	}
	if cmds["uninstall"] != "helm uninstall web -n apps --wait" {
		t.Fatalf("unexpected uninstall command: %q", cmds["uninstall"])
	}
	if cmds["rollback"] != "helm rollback web 4 -n apps --wait" {
		t.Fatalf("unexpected rollback command: %q", cmds["rollback"])
	}
}
