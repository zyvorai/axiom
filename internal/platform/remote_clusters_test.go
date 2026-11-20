//go:build !kubernetes

package platform

import (
	"context"
	"strings"
	"testing"
)

// --- MemoryRemoteClusterStore (shared demo-mode store) ---

func TestMemoryRemoteClusterStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryRemoteClusterStore()

	list, err := store.List(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("expected empty store, got %v err=%v", list, err)
	}

	if err := store.Upsert(ctx, RemoteCluster{Name: "east", Label: "East", Kubeconfig: "kc-1"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	list, err = store.List(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "east" {
		t.Fatalf("expected 1 cluster named east, got %+v err=%v", list, err)
	}

	// Upsert with the same name replaces the entry rather than duplicating it.
	if err := store.Upsert(ctx, RemoteCluster{Name: "east", Label: "East Coast", Kubeconfig: "kc-2"}); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}
	list, _ = store.List(ctx)
	if len(list) != 1 || list[0].Label != "East Coast" || list[0].Kubeconfig != "kc-2" {
		t.Fatalf("expected upsert to replace the existing entry in place, got %+v", list)
	}

	if err := store.Delete(ctx, "east"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ = store.List(ctx)
	if len(list) != 0 {
		t.Fatalf("expected empty store after delete, got %+v", list)
	}
}

func TestMemoryRemoteClusterStoreUpsertRejectsEmptyName(t *testing.T) {
	store := NewMemoryRemoteClusterStore()
	if err := store.Upsert(context.Background(), RemoteCluster{Kubeconfig: "kc"}); err == nil {
		t.Fatal("expected Upsert with an empty name to fail")
	}
}

func TestMemoryRemoteClusterStoreDeleteMissingIsNoop(t *testing.T) {
	store := NewMemoryRemoteClusterStore()
	if err := store.Delete(context.Background(), "does-not-exist"); err != nil {
		t.Fatalf("expected Delete of a missing cluster to be a no-op, got %v", err)
	}
}

// --- SaveRemoteCluster / DeleteRemoteCluster (shared package-level store indirection) ---

func TestSaveAndDeleteRemoteClusterWithoutStoreIsNoop(t *testing.T) {
	SetRemoteClusterStore(nil)
	defer SetRemoteClusterStore(nil)

	if err := SaveRemoteCluster(context.Background(), "demo", nil, "x", "X", "kc"); err != nil {
		t.Fatalf("expected a nil store to make Save a no-op, got %v", err)
	}
	if err := DeleteRemoteCluster(context.Background(), "demo", nil, "x"); err != nil {
		t.Fatalf("expected a nil store to make Delete a no-op, got %v", err)
	}
}

func TestSaveRemoteClusterPersistsToConfiguredStore(t *testing.T) {
	store := NewMemoryRemoteClusterStore()
	SetRemoteClusterStore(store)
	defer SetRemoteClusterStore(nil)

	if err := SaveRemoteCluster(context.Background(), "demo", nil, "west", "West", "kc-west"); err != nil {
		t.Fatalf("SaveRemoteCluster: %v", err)
	}
	list, _ := store.List(context.Background())
	if len(list) != 1 || list[0].Name != "west" || list[0].Kubeconfig != "kc-west" {
		t.Fatalf("expected SaveRemoteCluster to upsert into the configured store, got %+v", list)
	}

	if err := DeleteRemoteCluster(context.Background(), "demo", nil, "west"); err != nil {
		t.Fatalf("DeleteRemoteCluster: %v", err)
	}
	list, _ = store.List(context.Background())
	if len(list) != 0 {
		t.Fatalf("expected DeleteRemoteCluster to remove the entry, got %+v", list)
	}
}

// --- LoadRemoteClustersInto ---

func TestLoadRemoteClustersIntoRegistersEachOntoSwitchableProvider(t *testing.T) {
	store := NewMemoryRemoteClusterStore()
	if err := store.Upsert(context.Background(), RemoteCluster{Name: "lab", Label: "Lab Cluster", Kubeconfig: "irrelevant-in-demo-build"}); err != nil {
		t.Fatalf("seed Upsert: %v", err)
	}
	SetRemoteClusterStore(store)
	defer SetRemoteClusterStore(nil)

	sp := NewSwitchableProvider()
	LoadRemoteClustersInto(context.Background(), sp)

	clusters := sp.Clusters(context.Background())
	var found *ClusterInfo
	for i := range clusters {
		if clusters[i].Name == "lab" {
			found = &clusters[i]
		}
	}
	if found == nil {
		t.Fatalf("expected LoadRemoteClustersInto to register cluster 'lab', got %+v", clusters)
	}
	if found.Label != "Lab Cluster" {
		t.Fatalf("expected label 'Lab Cluster', got %q", found.Label)
	}
}

func TestLoadRemoteClustersIntoNilProviderIsNoop(t *testing.T) {
	// Must not panic.
	LoadRemoteClustersInto(context.Background(), nil)
}

func TestLoadRemoteClustersIntoNoStoreConfiguredIsNoop(t *testing.T) {
	SetRemoteClusterStore(nil)
	sp := NewSwitchableProvider()
	LoadRemoteClustersInto(context.Background(), sp)
	if sp.Count() != 0 {
		t.Fatalf("expected no clusters registered when no store is configured, got %d", sp.Count())
	}
}

func TestLoadRemoteClustersIntoSkipsEntriesProviderConstructionRejects(t *testing.T) {
	// In the !kubernetes build, ProviderFromKubeconfigYAML requires a
	// non-empty name (see remote_clusters_stub.go), so an entry with no name
	// must be skipped by LoadRemoteClustersInto rather than registered or
	// causing a panic. MemoryRemoteClusterStore.Upsert itself would refuse
	// to store a name-less RemoteCluster, so a minimal fake store is used
	// here to hand LoadRemoteClustersInto that entry directly.
	badStore := &erroringListRemoteClusterStore{
		entries: []RemoteCluster{{Name: "", Label: "no-name", Kubeconfig: "kc"}},
	}
	SetRemoteClusterStore(badStore)
	defer SetRemoteClusterStore(nil)

	sp := NewSwitchableProvider()
	LoadRemoteClustersInto(context.Background(), sp)
	if sp.Count() != 0 {
		t.Fatalf("expected the name-less entry to be skipped rather than registered, got %d clusters", sp.Count())
	}
}

// erroringListRemoteClusterStore lets a test hand LoadRemoteClustersInto a
// List result that MemoryRemoteClusterStore's own Upsert validation would
// normally have refused to store in the first place (e.g. an empty name),
// so the ProviderFromKubeconfigYAML-failure skip path can be exercised.
type erroringListRemoteClusterStore struct {
	entries []RemoteCluster
}

func (s *erroringListRemoteClusterStore) List(ctx context.Context) ([]RemoteCluster, error) {
	return s.entries, nil
}
func (s *erroringListRemoteClusterStore) Upsert(ctx context.Context, c RemoteCluster) error {
	return nil
}
func (s *erroringListRemoteClusterStore) Delete(ctx context.Context, name string) error {
	return nil
}

// --- ProviderFromKubeconfigYAML / OpenRemoteClusterStore (!kubernetes stub) ---

func TestProviderFromKubeconfigYAMLStubRejectsEmptyKubeconfig(t *testing.T) {
	_, _, err := ProviderFromKubeconfigYAML("", "name")
	if err == nil || !strings.Contains(err.Error(), "kubeconfig is required") {
		t.Fatalf("expected 'kubeconfig is required' error, got %v", err)
	}
}

func TestProviderFromKubeconfigYAMLStubRejectsEmptyName(t *testing.T) {
	_, _, err := ProviderFromKubeconfigYAML("apiVersion: v1\nkind: Config", "")
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected 'name is required' error, got %v", err)
	}
}

func TestProviderFromKubeconfigYAMLStubReturnsUsableDemoProvider(t *testing.T) {
	p, server, err := ProviderFromKubeconfigYAML("apiVersion: v1\nkind: Config", "lab")
	if err != nil {
		t.Fatalf("ProviderFromKubeconfigYAML: %v", err)
	}
	if server != "demo://lab" {
		t.Fatalf("expected demo server URL, got %q", server)
	}
	if _, ok := p.(*DemoProvider); !ok {
		t.Fatalf("expected a *DemoProvider stand-in in the !kubernetes build, got %T", p)
	}
	// The returned provider must be independently usable.
	if _, err := p.ListWorkloads(context.Background(), "all"); err != nil {
		t.Fatalf("stand-in provider ListWorkloads: %v", err)
	}
}

func TestOpenRemoteClusterStoreStubAlwaysReturnsMemoryStore(t *testing.T) {
	store := OpenRemoteClusterStore("live", nil)
	if _, ok := store.(*MemoryRemoteClusterStore); !ok {
		t.Fatalf("expected MemoryRemoteClusterStore in the !kubernetes build, got %T", store)
	}
}
