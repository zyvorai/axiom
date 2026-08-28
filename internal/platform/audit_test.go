package platform

import (
	"context"
	"sync"
	"testing"
)

type fakeAuditStore struct {
	mu      sync.Mutex
	saved   []AuditEntry
	loadRet []AuditEntry
}

func (f *fakeAuditStore) Load(ctx context.Context) ([]AuditEntry, error) {
	return f.loadRet, nil
}

func (f *fakeAuditStore) Save(ctx context.Context, entries []AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved = append([]AuditEntry(nil), entries...)
	return nil
}

func resetAuditState(t *testing.T) {
	t.Helper()
	auditMu.Lock()
	auditRing = nil
	auditStore = nil
	auditMu.Unlock()
}

func TestRecordAxiomAuditPersistsToStore(t *testing.T) {
	resetAuditState(t)
	defer resetAuditState(t)

	store := &fakeAuditStore{}
	SetAuditStore(store)

	RecordAxiomAudit("workload.launch", "default/app", "nginx", "alice")
	RecordAxiomAudit("secret.delete", "default/creds", "", "bob")

	store.mu.Lock()
	saved := store.saved
	store.mu.Unlock()
	if len(saved) != 2 {
		t.Fatalf("expected 2 persisted entries, got %d", len(saved))
	}
	if saved[1].Action != "secret.delete" || saved[1].User != "bob" {
		t.Fatalf("unexpected last persisted entry: %+v", saved[1])
	}

	listed := ListAxiomAudit()
	if len(listed) != 2 {
		t.Fatalf("expected 2 listed entries, got %d", len(listed))
	}
	if listed[0].Action != "secret.delete" {
		t.Fatalf("expected newest-first order, got %+v", listed[0])
	}
}

func TestSetAuditStoreLoadsExistingEntries(t *testing.T) {
	resetAuditState(t)
	defer resetAuditState(t)

	store := &fakeAuditStore{loadRet: []AuditEntry{
		{Action: "user.create", Target: "carol", User: "admin"},
	}}
	SetAuditStore(store)

	listed := ListAxiomAudit()
	if len(listed) != 1 || listed[0].Action != "user.create" {
		t.Fatalf("expected preloaded entry to appear, got %+v", listed)
	}
}

func TestRecordAxiomAuditWithoutStoreStaysInMemory(t *testing.T) {
	resetAuditState(t)
	defer resetAuditState(t)

	RecordAxiomAudit("cluster.activate", "eu-west-1", "", "admin")
	listed := ListAxiomAudit()
	if len(listed) != 1 {
		t.Fatalf("expected 1 in-memory entry with no store configured, got %d", len(listed))
	}
}
