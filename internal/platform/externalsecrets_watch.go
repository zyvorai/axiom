package platform

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	// DefaultExternalSecretsCheckInterval is how often sync health is checked.
	DefaultExternalSecretsCheckInterval = 5 * time.Minute
	externalSecretsWatcherUser          = "external-secrets-watcher"
	externalSecretsEvalTimeout          = 30 * time.Second
)

// ExternalSecretsWatcher notifies (debounced, through the existing hub
// webhook/Slack settings) when an ExternalSecret flips to failing, and
// audits each flip.
type ExternalSecretsWatcher struct {
	provider Provider
	notifier Notifier
	now      func() time.Time

	mu     sync.Mutex
	primed bool
	failed map[string]bool
	gate   *notifyGate
}

// NewExternalSecretsWatcher builds a watcher; a nil notifier only audits.
func NewExternalSecretsWatcher(p Provider, n Notifier) *ExternalSecretsWatcher {
	if n == nil {
		n = NopNotifier{}
	}
	return &ExternalSecretsWatcher{provider: p, notifier: n, now: time.Now, failed: map[string]bool{}, gate: newNotifyGate()}
}

// StartExternalSecretsWatcher evaluates now and then every interval until ctx
// is cancelled. An interval <= 0 or nil provider disables it.
func StartExternalSecretsWatcher(ctx context.Context, interval time.Duration, p Provider, n Notifier) *ExternalSecretsWatcher {
	w := NewExternalSecretsWatcher(p, n)
	if interval <= 0 || p == nil {
		return w
	}
	go func() {
		w.Evaluate(ctx)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				w.Evaluate(ctx)
			}
		}
	}()
	return w
}

// Evaluate runs one check. The first successful check only records state.
func (w *ExternalSecretsWatcher) Evaluate(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("external-secrets watcher: recovered from panic: %v", r)
		}
	}()
	ectx, cancel := context.WithTimeout(ctx, externalSecretsEvalTimeout)
	defer cancel()
	rep, err := ListExternalSecretsHealth(ectx, w.provider, "")
	if err != nil {
		log.Printf("external-secrets watcher: evaluate: %v", err)
		return
	}
	if !rep.Available {
		return
	}
	now := w.now()
	var out []Notification
	w.mu.Lock()
	seen := map[string]bool{}
	for _, e := range rep.ExternalSecrets {
		key := e.Namespace + "/" + e.Name
		seen[key] = true
		failing := e.Health == ESHealthFailing
		was := w.failed[key]
		w.failed[key] = failing
		if !w.primed || failing == was {
			continue
		}
		if failing {
			detail := e.Reason
			if e.Message != "" {
				detail += ": " + e.Message
			}
			n := Notification{Kind: "externalsecret.failing", Severity: SeverityWarning,
				Title: "Axiom: ExternalSecret " + key + " is failing to sync", Detail: detail, Path: "/secrets", Time: now}
			RecordAxiomAudit(n.Kind, key, detail, externalSecretsWatcherUser)
			if w.gate.allowBad(key, "failing", now) {
				out = append(out, n)
			}
		} else {
			n := Notification{Kind: "externalsecret.recovered", Severity: SeverityInfo,
				Title: "Axiom: ExternalSecret " + key + " is syncing again", Path: "/secrets", Time: now}
			RecordAxiomAudit(n.Kind, key, "", externalSecretsWatcherUser)
			if w.gate.allowGood(key) {
				out = append(out, n)
			}
		}
	}
	for k := range w.failed {
		if !seen[k] {
			delete(w.failed, k)
			w.gate.forget(k)
		}
	}
	w.primed = true
	w.mu.Unlock()
	for _, n := range out {
		w.notifier.Notify(ctx, n)
	}
}
