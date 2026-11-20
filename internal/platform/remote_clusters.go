package platform

import (
	"context"
	"fmt"
	"sync"
)

// RemoteCluster is a kubeconfig-backed cluster registered at runtime
// (Settings → Clusters → Add), persisted so it survives restarts.
type RemoteCluster struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Kubeconfig string `json:"kubeconfig"`
}

// RemoteClusterStore persists runtime-registered clusters.
type RemoteClusterStore interface {
	List(ctx context.Context) ([]RemoteCluster, error)
	Upsert(ctx context.Context, c RemoteCluster) error
	Delete(ctx context.Context, name string) error
}

var (
	remoteStoreMu sync.RWMutex
	remoteStore   RemoteClusterStore
)

func SetRemoteClusterStore(store RemoteClusterStore) {
	remoteStoreMu.Lock()
	remoteStore = store
	remoteStoreMu.Unlock()
}

func SaveRemoteCluster(ctx context.Context, mode string, provider Provider, name, label, kubeconfig string) error {
	remoteStoreMu.RLock()
	store := remoteStore
	remoteStoreMu.RUnlock()
	if store == nil {
		return nil
	}
	return store.Upsert(ctx, RemoteCluster{Name: name, Label: label, Kubeconfig: kubeconfig})
}

func DeleteRemoteCluster(ctx context.Context, mode string, provider Provider, name string) error {
	remoteStoreMu.RLock()
	store := remoteStore
	remoteStoreMu.RUnlock()
	if store == nil {
		return nil
	}
	return store.Delete(ctx, name)
}

// LoadRemoteClustersInto registers persisted remote clusters onto sp.
func LoadRemoteClustersInto(ctx context.Context, sp *SwitchableProvider) {
	if sp == nil {
		return
	}
	remoteStoreMu.RLock()
	store := remoteStore
	remoteStoreMu.RUnlock()
	if store == nil {
		return
	}
	list, err := store.List(ctx)
	if err != nil || len(list) == 0 {
		return
	}
	for _, c := range list {
		p, server, err := ProviderFromKubeconfigYAML(c.Kubeconfig, c.Name)
		if err != nil {
			continue
		}
		label := c.Label
		if label == "" {
			label = c.Name
		}
		sp.Register(c.Name, label, p, server)
	}
}

// MemoryRemoteClusterStore is used in demo mode.
type MemoryRemoteClusterStore struct {
	mu   sync.Mutex
	data map[string]RemoteCluster
}

func NewMemoryRemoteClusterStore() *MemoryRemoteClusterStore {
	return &MemoryRemoteClusterStore{data: map[string]RemoteCluster{}}
}

func (m *MemoryRemoteClusterStore) List(ctx context.Context) ([]RemoteCluster, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RemoteCluster, 0, len(m.data))
	for _, c := range m.data {
		out = append(out, c)
	}
	return out, nil
}

func (m *MemoryRemoteClusterStore) Upsert(ctx context.Context, c RemoteCluster) error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[c.Name] = c
	return nil
}

func (m *MemoryRemoteClusterStore) Delete(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, name)
	return nil
}
