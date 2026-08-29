package platform

import (
	"context"
	"log"
	"sync"
	"time"
)

const auditRingSize = 200

var (
	auditMu    sync.RWMutex
	auditRing  []AuditEntry
	auditStore AuditStore
	// auditDirty signals the single background persister (started once by
	// SetAuditStore) that the ring changed. Buffered to 1: a burst of rapid
	// RecordAxiomAudit calls coalesces into one pending signal rather than
	// piling up:
	//
	// Kubernetes-backed stores persist as a single ConfigMap/Secret
	// (K8sAuditStore.Save submits an unconditional update with no
	// resourceVersion, so it never even gets a conflict error to react to).
	// Calling store.Save directly from RecordAxiomAudit — as this used to
	// do — lets two concurrent calls' Save requests complete out of order,
	// so the persisted store can end up with whichever snapshot's write
	// reached the API server last, which can be the older, incomplete one:
	// entries silently vanish from the durable trail even though they're
	// still (temporarily) in the in-memory ring. A single persister
	// goroutine also means a slow/degraded audit backend never stalls the
	// mutating request that triggered the audit call — it only delays when
	// that entry gets durably saved, not the caller's response.
	auditDirty       chan struct{}
	auditPersistOnce sync.Once
)

// AuditStore persists the audit ring across restarts. Optional — when unset
// (the default, and always the case in demo mode) the ring is purely
// in-memory, exactly as before.
type AuditStore interface {
	Load(ctx context.Context) ([]AuditEntry, error)
	Save(ctx context.Context, entries []AuditEntry) error
}

// SetAuditStore wires a persistent backing store, loads any entries it
// already holds into the ring, and starts the single background persister
// goroutine (idempotent — safe to call more than once, e.g. in tests).
// Call once at startup, before serving traffic.
func SetAuditStore(store AuditStore) {
	if store == nil {
		return
	}
	auditMu.Lock()
	auditStore = store
	auditMu.Unlock()

	entries, err := store.Load(context.Background())
	if err != nil {
		log.Printf("audit: load persisted entries: %v", err)
	} else if len(entries) > 0 {
		auditMu.Lock()
		auditRing = entries
		if len(auditRing) > auditRingSize {
			auditRing = auditRing[len(auditRing)-auditRingSize:]
		}
		auditMu.Unlock()
	}

	auditPersistOnce.Do(func() {
		auditDirty = make(chan struct{}, 1)
		go auditPersistLoop()
	})
}

// auditPersistLoop is the only goroutine that ever calls auditStore.Save.
// Each time it wakes, it persists the ring's *current* state rather than
// whatever it was when a particular RecordAxiomAudit call signaled — so a
// Save can never regress to an older, incomplete snapshot no matter how
// the signals or network round-trips interleave.
func auditPersistLoop() {
	for range auditDirty {
		auditMu.RLock()
		store := auditStore
		snapshot := append([]AuditEntry(nil), auditRing...)
		auditMu.RUnlock()
		if store == nil {
			continue
		}
		if err := store.Save(context.Background(), snapshot); err != nil {
			log.Printf("audit: persist entries: %v", err)
		}
	}
}

// RecordAxiomAudit appends an entry to the in-memory ring (last 200) and,
// when a persistent AuditStore is configured, signals the background
// persister to save it so the trail survives a pod restart or redeploy.
func RecordAxiomAudit(action, target, detail, user string) {
	entry := AuditEntry{
		Time:   time.Now().UTC(),
		Action: action,
		Target: target,
		Detail: detail,
		User:   user,
	}
	auditMu.Lock()
	auditRing = append(auditRing, entry)
	if len(auditRing) > auditRingSize {
		auditRing = auditRing[len(auditRing)-auditRingSize:]
	}
	hasStore := auditStore != nil
	auditMu.Unlock()

	if !hasStore {
		return
	}
	select {
	case auditDirty <- struct{}{}:
	default:
		// A persist is already queued or in flight; it will pick up this
		// entry too, since auditPersistLoop always re-reads the ring's
		// current state rather than an earlier snapshot.
	}
}

// ListAxiomAudit returns audit entries newest-first.
func ListAxiomAudit() []AuditEntry {
	auditMu.RLock()
	defer auditMu.RUnlock()
	if len(auditRing) == 0 {
		return []AuditEntry{}
	}
	out := make([]AuditEntry, len(auditRing))
	for i, e := range auditRing {
		out[len(auditRing)-1-i] = e
	}
	return out
}
