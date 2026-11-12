package platform

import (
	"context"
	"log"
	"os"
	"sync"
	"time"
)

// NotifyLeader decides whether this replica should dispatch Slack/webhook
// alerts and fleet/budget notifications. Without a backend every replica
// leads (demo / single process). With a Lease-backed elector, only one
// replica delivers — see docs/GAPS.md multi-replica alert dispatch.
type NotifyLeader interface {
	IsLeader(ctx context.Context) bool
}

type alwaysLeader struct{}

func (alwaysLeader) IsLeader(context.Context) bool { return true }

var (
	notifyLeaderMu sync.RWMutex
	notifyLeader   NotifyLeader = alwaysLeader{}
)

// SetNotifyLeader wires multi-replica notify leadership. Nil resets to always-lead.
func SetNotifyLeader(l NotifyLeader) {
	notifyLeaderMu.Lock()
	defer notifyLeaderMu.Unlock()
	if l == nil {
		notifyLeader = alwaysLeader{}
		return
	}
	notifyLeader = l
}

func currentNotifyLeader() NotifyLeader {
	notifyLeaderMu.RLock()
	defer notifyLeaderMu.RUnlock()
	return notifyLeader
}

func isNotifyLeader(ctx context.Context) bool {
	return currentNotifyLeader().IsLeader(ctx)
}

// MemoryNotifyLeader is a test double: only the holder with Hold=true leads.
type MemoryNotifyLeader struct {
	Hold bool
}

func (m *MemoryNotifyLeader) IsLeader(context.Context) bool { return m.Hold }

// leaseNotifyLeader uses a coordination Lease when available.
type leaseNotifyLeader struct {
	holder string
	store  LeaseStore
	ttl    time.Duration

	mu     sync.Mutex
	until  time.Time
	isLead bool
}

// LeaseStore is the minimal coordination surface for notify leadership.
type LeaseStore interface {
	TryAcquire(ctx context.Context, name, holder string, ttl time.Duration) (bool, error)
}

// NewLeaseNotifyLeader returns a leader that renews via store. Holder defaults to HOSTNAME/pod name.
func NewLeaseNotifyLeader(store LeaseStore, holder string, ttl time.Duration) NotifyLeader {
	if store == nil {
		return alwaysLeader{}
	}
	if holder == "" {
		holder = os.Getenv("HOSTNAME")
	}
	if holder == "" {
		holder = "zyvor-axiom"
	}
	if ttl <= 0 {
		ttl = 15 * time.Second
	}
	return &leaseNotifyLeader{holder: holder, store: store, ttl: ttl}
}

func (l *leaseNotifyLeader) IsLeader(ctx context.Context) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	// Cache leadership for half the TTL to avoid a Lease get per notify.
	if now.Before(l.until) {
		return l.isLead
	}
	ok, err := l.store.TryAcquire(ctx, "zyvor-axiom-notify", l.holder, l.ttl)
	if err != nil {
		log.Printf("notify: lease acquire: %v (failing open as leader)", err)
		ok = true // fail-open so a Lease outage does not silence alerts entirely
	}
	l.isLead = ok
	l.until = now.Add(l.ttl / 2)
	return ok
}
