package platform

import (
	"sync"
	"time"
)

const auditRingSize = 200

var (
	auditMu   sync.RWMutex
	auditRing []AuditEntry
)

// RecordAxiomAudit appends an entry to the in-memory ring (last 200).
func RecordAxiomAudit(action, target, detail, user string) {
	entry := AuditEntry{
		Time:   time.Now().UTC(),
		Action: action,
		Target: target,
		Detail: detail,
		User:   user,
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	auditRing = append(auditRing, entry)
	if len(auditRing) > auditRingSize {
		auditRing = auditRing[len(auditRing)-auditRingSize:]
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
