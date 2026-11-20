//go:build kubernetes

package platform

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func newFakeRemoteClusterStore() (*k8sRemoteClusterStore, *k8sfake.Clientset) {
	cs := k8sfake.NewSimpleClientset()
	return &k8sRemoteClusterStore{client: cs, namespace: "axiom-system"}, cs
}

// --- k8sRemoteClusterStore against a fake clientset ---

func TestK8sRemoteClusterStoreListEmptyWhenSecretMissing(t *testing.T) {
	store, _ := newFakeRemoteClusterStore()
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no clusters before any Upsert, got %+v", list)
	}
}

func TestK8sRemoteClusterStoreUpsertCreatesBackingSecretThenUpdatesInPlace(t *testing.T) {
	ctx := context.Background()
	store, cs := newFakeRemoteClusterStore()

	if err := store.Upsert(ctx, RemoteCluster{Name: "prod", Label: "Prod", Kubeconfig: "kc-1"}); err != nil {
		t.Fatalf("Upsert (create): %v", err)
	}
	sec, err := cs.CoreV1().Secrets("axiom-system").Get(ctx, remoteClustersSecret, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected the backing secret to exist after Upsert, got %v", err)
	}
	if len(sec.Data["clusters.json"]) == 0 {
		t.Fatal("expected clusters.json to be populated")
	}

	// Upsert with the same name updates the entry in place rather than
	// appending a duplicate.
	if err := store.Upsert(ctx, RemoteCluster{Name: "prod", Label: "Prod Updated", Kubeconfig: "kc-2"}); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Label != "Prod Updated" || list[0].Kubeconfig != "kc-2" {
		t.Fatalf("expected Upsert to update the existing entry in place, got %+v", list)
	}
}

func TestK8sRemoteClusterStoreDeleteRemovesOnlyMatchingEntry(t *testing.T) {
	ctx := context.Background()
	store, _ := newFakeRemoteClusterStore()
	if err := store.Upsert(ctx, RemoteCluster{Name: "a", Kubeconfig: "kc-a"}); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := store.Upsert(ctx, RemoteCluster{Name: "b", Kubeconfig: "kc-b"}); err != nil {
		t.Fatalf("seed b: %v", err)
	}

	if err := store.Delete(ctx, "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != "b" {
		t.Fatalf("expected only cluster 'b' to remain, got %+v", list)
	}
}

// --- OpenRemoteClusterStore / underlyingKubernetes / axiomNamespace ---

func TestOpenRemoteClusterStoreDemoModeReturnsMemoryStore(t *testing.T) {
	store := OpenRemoteClusterStore("demo", nil)
	if _, ok := store.(*MemoryRemoteClusterStore); !ok {
		t.Fatalf("expected MemoryRemoteClusterStore for demo mode, got %T", store)
	}
}

func TestOpenRemoteClusterStoreWithoutKubernetesProviderReturnsMemoryStore(t *testing.T) {
	store := OpenRemoteClusterStore("live", nil)
	if _, ok := store.(*MemoryRemoteClusterStore); !ok {
		t.Fatalf("expected MemoryRemoteClusterStore when no *KubernetesProvider is available, got %T", store)
	}
}

func TestOpenRemoteClusterStoreWithKubernetesProviderReturnsK8sStore(t *testing.T) {
	t.Setenv("AXIOM_NAMESPACE", "custom-ns")
	cs := k8sfake.NewSimpleClientset()
	kp := &KubernetesProvider{client: cs}
	store := OpenRemoteClusterStore("live", kp)
	ks, ok := store.(*k8sRemoteClusterStore)
	if !ok {
		t.Fatalf("expected *k8sRemoteClusterStore, got %T", store)
	}
	if ks.namespace != "custom-ns" {
		t.Fatalf("expected namespace from AXIOM_NAMESPACE, got %q", ks.namespace)
	}
}

func TestAxiomNamespaceDefaultsWhenUnset(t *testing.T) {
	t.Setenv("AXIOM_NAMESPACE", "")
	t.Setenv("POD_NAMESPACE", "")
	if got := axiomNamespace(); got != "axiom-system" {
		t.Fatalf("expected default namespace axiom-system, got %q", got)
	}
}

func TestAxiomNamespacePrefersPodNamespaceOverDefault(t *testing.T) {
	t.Setenv("AXIOM_NAMESPACE", "")
	t.Setenv("POD_NAMESPACE", "from-pod")
	if got := axiomNamespace(); got != "from-pod" {
		t.Fatalf("expected POD_NAMESPACE fallback, got %q", got)
	}
}

func TestAxiomNamespacePrefersAxiomNamespaceOverPodNamespace(t *testing.T) {
	t.Setenv("AXIOM_NAMESPACE", "from-axiom")
	t.Setenv("POD_NAMESPACE", "from-pod")
	if got := axiomNamespace(); got != "from-axiom" {
		t.Fatalf("expected AXIOM_NAMESPACE to take priority, got %q", got)
	}
}

func TestUnderlyingKubernetesFindsDirectProvider(t *testing.T) {
	kp := &KubernetesProvider{}
	got, ok := underlyingKubernetes(kp)
	if !ok || got != kp {
		t.Fatalf("expected direct *KubernetesProvider to be found, got %v ok=%v", got, ok)
	}
}

func TestUnderlyingKubernetesSearchesSwitchableProviderEntries(t *testing.T) {
	sp := NewSwitchableProvider()
	kp := &KubernetesProvider{clusterKey: "east"}
	sp.Register("east", "East", kp, "https://east.example")
	got, ok := underlyingKubernetes(sp)
	if !ok || got != kp {
		t.Fatalf("expected to find the registered *KubernetesProvider, got %v ok=%v", got, ok)
	}
}

func TestUnderlyingKubernetesFalseForNonKubernetesProvider(t *testing.T) {
	sp := NewSwitchableProvider()
	sp.Register("demo", "Demo", NewDemoProvider(), "demo://x")
	if _, ok := underlyingKubernetes(sp); ok {
		t.Fatal("expected no *KubernetesProvider to be found among demo-only entries")
	}
}

// --- ProviderFromKubeconfigYAML (real kubernetes build) ---

// ProviderFromKubeconfigYAML only parses the kubeconfig document and
// constructs clientset/dynamic-client objects locally; it never dials the
// target API server (kubernetes.NewForConfig/dynamic.NewForConfig just wire
// up a REST client, they don't connect). So the "happy path" is fully
// testable without a live cluster — nothing here needs to be skipped for
// requiring one.
const validTestKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://cluster.example.invalid:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`

func TestProviderFromKubeconfigYAMLKubernetesBuildRejectsEmpty(t *testing.T) {
	_, _, err := ProviderFromKubeconfigYAML("", "name")
	if err == nil || !strings.Contains(err.Error(), "kubeconfig is required") {
		t.Fatalf("expected 'kubeconfig is required' error, got %v", err)
	}
}

func TestProviderFromKubeconfigYAMLKubernetesBuildRejectsMalformed(t *testing.T) {
	_, _, err := ProviderFromKubeconfigYAML("this is not: [valid kubeconfig", "name")
	if err == nil || !strings.Contains(err.Error(), "invalid kubeconfig") {
		t.Fatalf("expected 'invalid kubeconfig' error, got %v", err)
	}
}

func TestProviderFromKubeconfigYAMLKubernetesBuildBuildsLiveProvider(t *testing.T) {
	p, server, err := ProviderFromKubeconfigYAML(validTestKubeconfig, "east")
	if err != nil {
		t.Fatalf("ProviderFromKubeconfigYAML: %v", err)
	}
	if server != "https://cluster.example.invalid:6443" {
		t.Fatalf("expected server host from kubeconfig, got %q", server)
	}
	kp, ok := p.(*KubernetesProvider)
	if !ok {
		t.Fatalf("expected *KubernetesProvider, got %T", p)
	}
	if kp.clusterKey != "east" {
		t.Fatalf("expected clusterKey %q, got %q", "east", kp.clusterKey)
	}
	if kp.client == nil || kp.dynamic == nil || kp.config == nil {
		t.Fatal("expected client, dynamic, and config to all be populated")
	}
}

func TestProviderFromKubeconfigYAMLKubernetesBuildDefaultsKeyToHost(t *testing.T) {
	p, _, err := ProviderFromKubeconfigYAML(validTestKubeconfig, "")
	if err != nil {
		t.Fatalf("ProviderFromKubeconfigYAML: %v", err)
	}
	kp := p.(*KubernetesProvider)
	if kp.clusterKey != "https://cluster.example.invalid:6443" {
		t.Fatalf("expected clusterKey to default to cfg.Host, got %q", kp.clusterKey)
	}
}
