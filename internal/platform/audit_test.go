package platform

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
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

func (f *fakeAuditStore) savedLen() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.saved)
}

func resetAuditState(t *testing.T) {
	t.Helper()
	auditMu.Lock()
	auditRing = nil
	auditStore = nil
	auditMu.Unlock()
}

// waitFor polls cond until it returns true or the timeout elapses.
// RecordAxiomAudit's persistence is asynchronous (see audit.go), so tests
// need to wait for the background persister to catch up rather than
// asserting immediately after RecordAxiomAudit returns.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

func TestRecordAxiomAuditPersistsToStore(t *testing.T) {
	resetAuditState(t)
	defer resetAuditState(t)

	store := &fakeAuditStore{}
	SetAuditStore(store)

	RecordAxiomAudit("workload.launch", "default/app", "nginx", "alice")
	RecordAxiomAudit("secret.delete", "default/creds", "", "bob")

	waitFor(t, time.Second, func() bool { return store.savedLen() == 2 })

	store.mu.Lock()
	saved := store.saved
	store.mu.Unlock()
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

// slowAuditStore delays every Save by a random-ish small amount to
// encourage real goroutine-scheduling interleaving between concurrent
// RecordAxiomAudit callers and the background persister.
type slowAuditStore struct {
	mu    sync.Mutex
	saved []AuditEntry
	calls int
}

func (f *slowAuditStore) Load(ctx context.Context) ([]AuditEntry, error) { return nil, nil }

func (f *slowAuditStore) Save(ctx context.Context, entries []AuditEntry) error {
	time.Sleep(time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.saved = append([]AuditEntry(nil), entries...)
	return nil
}

func (f *slowAuditStore) savedLen() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.saved)
}

// TestConcurrentRecordAxiomAuditNeverLosesEntries is a regression test for a
// real bug: K8sAuditStore.Save submits an unconditional Kubernetes update
// (no resourceVersion — see audit_k8s.go), and the old RecordAxiomAudit
// called store.Save directly from every caller with no serialization. Two
// concurrent calls' Save requests could then complete out of order, and the
// persisted store would end up with whichever snapshot's write reached the
// API server last — which could be the older, incomplete one, silently
// losing entries that briefly existed in the in-memory ring. The fix routes
// all persistence through a single background goroutine that always
// re-reads the ring's current state, so this asserts the persisted store
// eventually contains every entry with none lost, no matter how the
// concurrent callers interleave.
func TestConcurrentRecordAxiomAuditNeverLosesEntries(t *testing.T) {
	resetAuditState(t)
	defer resetAuditState(t)

	store := &slowAuditStore{}
	SetAuditStore(store)

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			RecordAxiomAudit("workload.restart", fmt.Sprintf("default/app-%02d", i), "", "concurrent-test")
		}(i)
	}
	wg.Wait()

	waitFor(t, 2*time.Second, func() bool { return store.savedLen() == n })

	store.mu.Lock()
	saved := append([]AuditEntry(nil), store.saved...)
	store.mu.Unlock()

	seen := map[string]bool{}
	for _, e := range saved {
		seen[e.Target] = true
	}
	if len(seen) != n {
		t.Fatalf("expected all %d entries to survive to the persisted store, got %d unique targets: %+v", n, len(seen), saved)
	}

	listed := ListAxiomAudit()
	if len(listed) != n {
		t.Fatalf("expected %d in-memory entries, got %d", n, len(listed))
	}
}
