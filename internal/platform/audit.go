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
)

// AuditStore persists the audit ring across restarts. Optional — when unset
// (the default, and always the case in demo mode) the ring is purely
// in-memory, exactly as before.
type AuditStore interface {
	Load(ctx context.Context) ([]AuditEntry, error)
	Save(ctx context.Context, entries []AuditEntry) error
}

// SetAuditStore wires a persistent backing store and loads any entries it
// already holds into the ring. Call once at startup, before serving traffic.
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
		return
	}
	if len(entries) == 0 {
		return
	}
	auditMu.Lock()
	auditRing = entries
	if len(auditRing) > auditRingSize {
		auditRing = auditRing[len(auditRing)-auditRingSize:]
	}
	auditMu.Unlock()
}

// RecordAxiomAudit appends an entry to the in-memory ring (last 200) and,
// when a persistent AuditStore is configured, saves it so the trail survives
// a pod restart or redeploy.
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
	snapshot := append([]AuditEntry(nil), auditRing...)
	store := auditStore
	auditMu.Unlock()

	if store != nil {
		if err := store.Save(context.Background(), snapshot); err != nil {
			log.Printf("audit: persist entry: %v", err)
		}
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
