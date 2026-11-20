package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// External Secrets Operator sync-health view. Only names, kinds, conditions
// and timestamps are exposed: never provider credentials or secret values.

// ExternalSecret health buckets (mutually exclusive).
const (
	ESHealthReady   = "ready"
	ESHealthFailing = "failing"
	ESHealthStale   = "stale"
)

// ExternalSecretHealth is one ExternalSecret with its sync state.
type ExternalSecretHealth struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	StoreKind    string `json:"storeKind,omitempty"` // SecretStore | ClusterSecretStore
	Store        string `json:"store"`
	Refresh      string `json:"refreshInterval,omitempty"`
	LastSync     string `json:"lastSync,omitempty"` // RFC3339
	Ready        string `json:"ready"`              // True | False | Unknown
	Reason       string `json:"reason,omitempty"`   // e.g. SecretSynced
	Message      string `json:"message,omitempty"`
	TargetName   string `json:"targetSecret,omitempty"`
	Health       string `json:"health"` // ready | failing | stale
	StaleSeconds int64  `json:"staleSeconds,omitempty"`
}

// ExternalSecretStoreHealth is a SecretStore or ClusterSecretStore.
type ExternalSecretStoreHealth struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Provider  string `json:"provider,omitempty"` // vault, aws, gcpsm, ...
	Ready     string `json:"ready"`
	Message   string `json:"message,omitempty"`
}

// ExternalSecretsRollup counts ExternalSecrets per health bucket.
type ExternalSecretsRollup struct {
	Total   int `json:"total"`
	Ready   int `json:"ready"`
	Failing int `json:"failing"`
	Stale   int `json:"stale"`
}

// ExternalSecretsHealthReport is GET /api/v1/external-secrets/health.
type ExternalSecretsHealthReport struct {
	Available       bool                        `json:"available"`
	Note            string                      `json:"note,omitempty"`
	Rollup          ExternalSecretsRollup       `json:"rollup"`
	ExternalSecrets []ExternalSecretHealth      `json:"externalSecrets"`
	Stores          []ExternalSecretStoreHealth `json:"stores"`
}

func emptyESHealth(note string) ExternalSecretsHealthReport {
	return ExternalSecretsHealthReport{Note: note, ExternalSecrets: []ExternalSecretHealth{}, Stores: []ExternalSecretStoreHealth{}}
}

// ListExternalSecretsHealth returns the sync-health report when the provider
// supports it.
func ListExternalSecretsHealth(ctx context.Context, p Provider, ns string) (ExternalSecretsHealthReport, error) {
	if l, ok := p.(interface {
		ListExternalSecretsHealth(context.Context, string) (ExternalSecretsHealthReport, error)
	}); ok {
		return l.ListExternalSecretsHealth(ctx, ns)
	}
	return emptyESHealth("External Secrets view not available on this provider"), nil
}

// ExternalSecretStale reports whether an ExternalSecret has not synced for
// more than twice its refresh interval. A zero/unparseable interval (ESO's
// "never refresh") or a missing sync time is never stale.
func ExternalSecretStale(lastSync time.Time, refresh string, now time.Time) (bool, time.Duration) {
	d, err := time.ParseDuration(strings.TrimSpace(refresh))
	if err != nil || d <= 0 || lastSync.IsZero() {
		return false, 0
	}
	age := now.Sub(lastSync)
	return age > 2*d, age
}

// ClassifyExternalSecret returns the health bucket: failing wins over stale.
func ClassifyExternalSecret(ready string, lastSync time.Time, refresh string, now time.Time) (string, time.Duration) {
	if ready != "True" {
		return ESHealthFailing, 0
	}
	if stale, age := ExternalSecretStale(lastSync, refresh, now); stale {
		return ESHealthStale, age
	}
	return ESHealthReady, 0
}

// RollupExternalSecrets counts the health buckets.
func RollupExternalSecrets(items []ExternalSecretHealth) ExternalSecretsRollup {
	r := ExternalSecretsRollup{Total: len(items)}
	for _, e := range items {
		switch e.Health {
		case ESHealthFailing:
			r.Failing++
		case ESHealthStale:
			r.Stale++
		default:
			r.Ready++
		}
	}
	return r
}

// FinalizeExternalSecretsHealth classifies, sorts and rolls up a report.
func FinalizeExternalSecretsHealth(items []ExternalSecretHealth, stores []ExternalSecretStoreHealth, now time.Time) ExternalSecretsHealthReport {
	for i := range items {
		var ls time.Time
		if items[i].LastSync != "" {
			ls, _ = time.Parse(time.RFC3339, items[i].LastSync)
		}
		h, age := ClassifyExternalSecret(items[i].Ready, ls, items[i].Refresh, now)
		items[i].Health = h
		items[i].StaleSeconds = int64(age.Seconds())
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
	})
	sort.Slice(stores, func(i, j int) bool {
		if stores[i].Kind != stores[j].Kind {
			return stores[i].Kind < stores[j].Kind
		}
		if stores[i].Namespace != stores[j].Namespace {
			return stores[i].Namespace < stores[j].Namespace
		}
		return stores[i].Name < stores[j].Name
	})
	if items == nil {
		items = []ExternalSecretHealth{}
	}
	if stores == nil {
		stores = []ExternalSecretStoreHealth{}
	}
	return ExternalSecretsHealthReport{Available: true, Rollup: RollupExternalSecrets(items), ExternalSecrets: items, Stores: stores}
}

// ParseExternalSecretHealth builds a row from an unstructured ExternalSecret.
func ParseExternalSecretHealth(obj map[string]any) ExternalSecretHealth {
	meta, _ := obj["metadata"].(map[string]any)
	spec, _ := obj["spec"].(map[string]any)
	st, _ := obj["status"].(map[string]any)
	e := ExternalSecretHealth{Ready: "Unknown"}
	e.Namespace, _ = meta["namespace"].(string)
	e.Name, _ = meta["name"].(string)
	ref, _ := spec["secretStoreRef"].(map[string]any)
	e.Store, _ = ref["name"].(string)
	e.StoreKind, _ = ref["kind"].(string)
	if e.StoreKind == "" {
		e.StoreKind = "SecretStore"
	}
	e.Refresh, _ = spec["refreshInterval"].(string)
	tgt, _ := spec["target"].(map[string]any)
	e.TargetName, _ = tgt["name"].(string)
	if e.TargetName == "" {
		e.TargetName = e.Name
	}
	e.LastSync, _ = st["refreshTime"].(string)
	conds, _ := st["conditions"].([]any)
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok || fmt.Sprint(m["type"]) != "Ready" {
			continue
		}
		if s, ok := m["status"].(string); ok && s != "" {
			e.Ready = s
		}
		e.Reason, _ = m["reason"].(string)
		e.Message, _ = m["message"].(string)
	}
	if len(e.Message) > 300 {
		e.Message = e.Message[:300] + "…"
	}
	return e
}

// ParseExternalSecretStore builds a row from an unstructured (Cluster)SecretStore.
// The provider is the single key under spec.provider; its contents (which may
// reference credentials) are never read.
func ParseExternalSecretStore(obj map[string]any, kind string) ExternalSecretStoreHealth {
	meta, _ := obj["metadata"].(map[string]any)
	spec, _ := obj["spec"].(map[string]any)
	st, _ := obj["status"].(map[string]any)
	s := ExternalSecretStoreHealth{Kind: kind, Ready: "Unknown"}
	s.Namespace, _ = meta["namespace"].(string)
	s.Name, _ = meta["name"].(string)
	if prov, ok := spec["provider"].(map[string]any); ok {
		keys := make([]string, 0, len(prov))
		for k := range prov {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			s.Provider = keys[0]
		}
	}
	conds, _ := st["conditions"].([]any)
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok || fmt.Sprint(m["type"]) != "Ready" {
			continue
		}
		if v, ok := m["status"].(string); ok && v != "" {
			s.Ready = v
		}
		s.Message, _ = m["message"].(string)
	}
	return s
}
