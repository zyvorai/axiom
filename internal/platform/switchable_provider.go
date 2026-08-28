package platform

import (
	"fmt"
	"sync"
)

// ClusterInfo describes one cluster registered with a SwitchableProvider,
// as surfaced to the Settings → Clusters UI.
type ClusterInfo struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Current bool   `json:"current"`
	Healthy bool   `json:"healthy"`
	Server  string `json:"server,omitempty"`
}

type clusterEntry struct {
	name     string
	label    string
	provider Provider
	server   string
}

// SwitchableProvider implements the Provider interface (see
// switchable_provider_gen.go) by delegating every call to whichever
// registered cluster is currently active. Calling SetActive changes which
// cluster the entire API surface talks to at runtime — no handler code
// needs to know clusters exist at all.
type SwitchableProvider struct {
	mu      sync.RWMutex
	active  string
	entries map[string]*clusterEntry
	order   []string
}

func NewSwitchableProvider() *SwitchableProvider {
	return &SwitchableProvider{entries: map[string]*clusterEntry{}}
}

// Register adds (or replaces) a named cluster. The first cluster registered
// becomes active automatically.
func (sp *SwitchableProvider) Register(name, label string, p Provider, server string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if _, exists := sp.entries[name]; !exists {
		sp.order = append(sp.order, name)
	}
	sp.entries[name] = &clusterEntry{name: name, label: label, provider: p, server: server}
	if sp.active == "" {
		sp.active = name
	}
}

func (sp *SwitchableProvider) current() Provider {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	e := sp.entries[sp.active]
	if e == nil {
		return nil
	}
	return e.provider
}

// Clusters lists every registered cluster, current-first-flagged.
func (sp *SwitchableProvider) Clusters() []ClusterInfo {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	out := make([]ClusterInfo, 0, len(sp.order))
	for _, name := range sp.order {
		e := sp.entries[name]
		out = append(out, ClusterInfo{
			Name: e.name, Label: e.label, Current: e.name == sp.active,
			Healthy: e.provider != nil, Server: e.server,
		})
	}
	return out
}

// SetActive switches which registered cluster subsequent calls hit.
func (sp *SwitchableProvider) SetActive(name string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if _, ok := sp.entries[name]; !ok {
		return fmt.Errorf("unknown cluster %q", name)
	}
	sp.active = name
	return nil
}

// ActiveName returns the currently active cluster's registered name.
func (sp *SwitchableProvider) ActiveName() string {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return sp.active
}

// Count returns how many clusters are registered.
func (sp *SwitchableProvider) Count() int {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return len(sp.order)
}
