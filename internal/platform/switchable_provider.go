package platform

import (
	"context"
	"fmt"
	"sync"
)

// ClusterInfo describes one cluster registered with a SwitchableProvider,
// as surfaced to the Settings → Clusters UI. State, LatencyMs and Error come
// from the fleet prober (see fleet.go); Healthy is kept for older clients and
// is false only for an unreachable cluster.
type ClusterInfo struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Current   bool   `json:"current"`
	Healthy   bool   `json:"healthy"`
	Server    string `json:"server,omitempty"`
	State     string `json:"state"`
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

type clusterEntry struct {
	name     string
	label    string
	provider Provider
	server   string
}

// SwitchableProvider implements the Provider interface (see
// switchable_provider_gen.go) by delegating every call to whichever
// registered cluster is currently active for the calling session. Calling
// SetActive changes which cluster that session's API calls talk to — no
// handler code needs to know clusters exist at all.
//
// "Active" is tracked per cluster-session key (see cluster_session.go),
// falling back to a single shared defaultActive when a request carries no
// session key at all (e.g. a scripted API client with no session header) —
// this preserves the original single-switch behavior for that case while
// giving real browser tabs/users independent selections.
type SwitchableProvider struct {
	mu            sync.RWMutex
	defaultActive string
	perSession    map[string]string
	entries       map[string]*clusterEntry
	order         []string

	fleetOnce sync.Once
	fleetSt   *fleetState
}

func NewSwitchableProvider() *SwitchableProvider {
	return &SwitchableProvider{entries: map[string]*clusterEntry{}, perSession: map[string]string{}}
}

// Register adds (or replaces) a named cluster. The first cluster registered
// becomes the default active cluster automatically.
func (sp *SwitchableProvider) Register(name, label string, p Provider, server string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if _, exists := sp.entries[name]; !exists {
		sp.order = append(sp.order, name)
	}
	sp.entries[name] = &clusterEntry{name: name, label: label, provider: p, server: server}
	if sp.defaultActive == "" {
		sp.defaultActive = name
	}
	// A cluster registered while the fleet prober is running (Settings →
	// Clusters) is probed straight away rather than waiting for the next tick.
	if st := sp.fleet(); st != nil {
		st.mu.Lock()
		running := st.running
		delete(st.health, name) // a re-registered name starts from a clean slate
		st.mu.Unlock()
		if running {
			go sp.probe(context.Background(), name, p)
		}
	}
}

// activeForLocked resolves the active cluster name for a session key. Must
// be called with sp.mu held.
func (sp *SwitchableProvider) activeForLocked(session string) string {
	if session != "" {
		if name, ok := sp.perSession[session]; ok {
			if _, exists := sp.entries[name]; exists {
				return name
			}
		}
	}
	return sp.defaultActive
}

func (sp *SwitchableProvider) current(ctx context.Context) Provider {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	e := sp.entries[sp.activeForLocked(clusterSessionFromContext(ctx))]
	if e == nil {
		return nil
	}
	return e.provider
}

// CurrentProvider exposes the active cluster's Provider for handlers that need
// typed access (e.g. WebSocket upgrades that are not on the Provider interface).
func (sp *SwitchableProvider) CurrentProvider(ctx context.Context) Provider {
	return sp.current(ctx)
}

// Clusters lists every registered cluster, flagged Current relative to the
// requesting session's own active selection.
func (sp *SwitchableProvider) Clusters(ctx context.Context) []ClusterInfo {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	active := sp.activeForLocked(clusterSessionFromContext(ctx))
	out := make([]ClusterInfo, 0, len(sp.order))
	for _, name := range sp.order {
		e := sp.entries[name]
		h := sp.healthOf(e.name)
		out = append(out, ClusterInfo{
			Name: e.name, Label: e.label, Current: e.name == active,
			Healthy: e.provider != nil && h.State != ClusterUnreachable, Server: e.server,
			State: h.State, LatencyMs: h.LatencyMs, Error: h.Error,
		})
	}
	return out
}

// SetActive switches which registered cluster subsequent calls from this
// session hit. A request with no session key changes the shared default
// instead (matching the pre-existing single-switch behavior for scripted
// API clients that send no session header).
func (sp *SwitchableProvider) SetActive(ctx context.Context, name string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if _, ok := sp.entries[name]; !ok {
		return fmt.Errorf("unknown cluster %q", name)
	}
	if session := clusterSessionFromContext(ctx); session != "" {
		sp.perSession[session] = name
	} else {
		sp.defaultActive = name
	}
	return nil
}

// ActiveName returns the active cluster's registered name for this session.
func (sp *SwitchableProvider) ActiveName(ctx context.Context) string {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return sp.activeForLocked(clusterSessionFromContext(ctx))
}

// ProviderFor returns the Provider registered under cluster name, whatever
// any session has selected, or false when no such cluster is registered
// (never was, or was unregistered since). Callers that must stay on one
// cluster for longer than a request (the pod proxy origin) resolve the
// cluster once with ActiveName and then go through ProviderFor.
func (sp *SwitchableProvider) ProviderFor(name string) (Provider, bool) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	e, ok := sp.entries[name]
	if !ok || e.provider == nil {
		return nil, false
	}
	return e.provider, true
}

// Count returns how many clusters are registered.
func (sp *SwitchableProvider) Count() int {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return len(sp.order)
}

// Unregister removes a runtime-registered cluster. Refuses to remove the
// last remaining cluster so the console always has somewhere to talk to.
func (sp *SwitchableProvider) Unregister(name string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if _, ok := sp.entries[name]; !ok {
		return fmt.Errorf("unknown cluster %q", name)
	}
	if len(sp.order) <= 1 {
		return fmt.Errorf("cannot unregister the last cluster")
	}
	delete(sp.entries, name)
	sp.forgetHealth(name)
	out := sp.order[:0]
	for _, n := range sp.order {
		if n != name {
			out = append(out, n)
		}
	}
	sp.order = out
	if sp.defaultActive == name {
		sp.defaultActive = sp.order[0]
	}
	for session, active := range sp.perSession {
		if active == name {
			sp.perSession[session] = sp.defaultActive
		}
	}
	return nil
}
