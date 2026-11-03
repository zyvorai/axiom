package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// sharedStore is a RevocationStore shared by several Revocations instances, as
// replicas share one ConfigMap. mergingStore adds the conflict-safe Merge.
type sharedStore struct {
	mu   sync.Mutex
	data []byte
}

func (s *sharedStore) Load() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.data...), nil
}
func (s *sharedStore) Save(b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = append([]byte(nil), b...)
	return nil
}

type mergingStore struct{ sharedStore }

func (s *mergingStore) Merge(fn func([]byte) ([]byte, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out, err := fn(append([]byte(nil), s.data...))
	if err != nil {
		return err
	}
	s.data = out
	return nil
}

func TestRevocationsShareThroughRefresh(t *testing.T) {
	for _, store := range []RevocationStore{&sharedStore{}, &mergingStore{}} {
		a, b := NewRevocations(store), NewRevocations(store)
		sess := Session{User: "alice", ID: "sess-1", Exp: time.Now().Add(time.Hour).Unix(), IatMs: time.Now().UnixMilli() - 1000}
		a.RevokeSession(sess.ID, sess.Exp)
		if !a.Revoked(sess) {
			t.Fatal("A does not see its own revocation")
		}
		if b.Revoked(sess) {
			t.Fatal("B saw the revocation before refreshing (test is not exercising refresh)")
		}
		if err := b.Refresh(); err != nil {
			t.Fatal(err)
		}
		if !b.Revoked(sess) {
			t.Fatalf("%T: B did not pick up A's revocation", store)
		}
		// B's own later revocation does not drop A's entry from the store.
		b.RevokeUser("bob")
		c := NewRevocations(store)
		if !c.Revoked(sess) || !c.Revoked(Session{User: "bob", IatMs: 1}) {
			t.Fatalf("%T: a fresh replica lost an entry", store)
		}
	}
}

func TestRevocationsRefreshNeverUnrevokes(t *testing.T) {
	store := &mergingStore{}
	a := NewRevocations(store)
	a.RevokeSession("local", time.Now().Add(time.Hour).Unix())
	// The store is wiped (or a replica wrote an older, smaller snapshot).
	_ = store.Save([]byte(`{}`))
	if err := a.Refresh(); err != nil {
		t.Fatal(err)
	}
	if !a.Revoked(Session{ID: "local"}) {
		t.Fatal("refresh un-revoked a local entry")
	}
	// Garbage in the store is reported and changes nothing.
	_ = store.Save([]byte(`not json`))
	if err := a.Refresh(); err == nil {
		t.Fatal("expected an error for an unreadable snapshot")
	}
	if !a.Revoked(Session{ID: "local"}) {
		t.Fatal("an unreadable snapshot un-revoked a local entry")
	}
	// The next persist replaces the garbage with the full local state.
	a.RevokeUser("zed")
	if b := NewRevocations(store); !b.Revoked(Session{ID: "local"}) {
		t.Fatal("persist after unreadable snapshot lost local entries")
	}
}

func TestRevocationsMergeTakesLaterCutoff(t *testing.T) {
	store := &mergingStore{}
	a, b := NewRevocations(store), NewRevocations(store)
	base := time.Now()
	a.now = func() time.Time { return base.Add(10 * time.Second) }
	b.now = func() time.Time { return base }
	a.RevokeUser("carol") // later cut-off
	b.RevokeUser("carol") // earlier cut-off, written afterwards
	between := Session{User: "carol", IatMs: base.Add(5 * time.Second).UnixMilli()}
	if !b.Revoked(between) {
		t.Fatal("B's persist did not merge A's later cut-off into B")
	}
	var snap revocationSnapshot
	raw, _ := store.Load()
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	if got := snap.Users["carol"]; got != base.Add(10*time.Second).UnixMilli() {
		t.Fatalf("stored cut-off %d is not the later one", got)
	}
	// Refresh also takes the later of the two.
	c := NewRevocations(&mergingStore{})
	c.now = func() time.Time { return base.Add(20 * time.Second) }
	c.RevokeUser("carol")
	c.store = store
	if err := c.Refresh(); err != nil {
		t.Fatal(err)
	}
	if !c.Revoked(Session{User: "carol", IatMs: base.Add(15 * time.Second).UnixMilli()}) {
		t.Fatal("refresh replaced a later local cut-off with an earlier stored one")
	}
}

func TestRevocationsRefreshPrunes(t *testing.T) {
	store := &mergingStore{}
	past := time.Now().Add(-time.Hour).Unix()
	old := time.Now().Add(-userCutoffRetention - time.Hour).UnixMilli()
	raw, _ := json.Marshal(revocationSnapshot{IDs: map[string]int64{"expired": past}, Users: map[string]int64{"ancient": old}})
	_ = store.Save(raw)
	a := NewRevocations(nil)
	a.store = store
	if err := a.Refresh(); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.ids) != 0 || len(a.users) != 0 {
		t.Fatalf("refresh kept expired entries: %v %v", a.ids, a.users)
	}
}

func TestRevocationsConcurrentReplicasLoseNothing(t *testing.T) {
	store := &mergingStore{}
	a, b := NewRevocations(store), NewRevocations(store)
	const n = 50
	exp := time.Now().Add(time.Hour).Unix()
	var wg sync.WaitGroup
	for _, rv := range []struct {
		r      *Revocations
		prefix string
	}{{a, "a"}, {b, "b"}} {
		wg.Add(1)
		go func(r *Revocations, prefix string) {
			defer wg.Done()
			for i := 0; i < n; i++ {
				r.RevokeSession(fmt.Sprintf("%s-%d", prefix, i), exp)
			}
		}(rv.r, rv.prefix)
	}
	wg.Wait()
	c := NewRevocations(store)
	for _, p := range []string{"a", "b"} {
		for i := 0; i < n; i++ {
			if !c.Revoked(Session{ID: fmt.Sprintf("%s-%d", p, i)}) {
				t.Fatalf("lost %s-%d", p, i)
			}
		}
	}
}

func TestRevocationsStartRefresh(t *testing.T) {
	store := &mergingStore{}
	a, b := NewRevocations(store), NewRevocations(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.StartRefresh(ctx, 10*time.Millisecond)
	a.RevokeSession("live", time.Now().Add(time.Hour).Unix())
	deadline := time.Now().Add(2 * time.Second)
	for !b.Revoked(Session{ID: "live"}) {
		if time.Now().After(deadline) {
			t.Fatal("B never picked up A's revocation through StartRefresh")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// No store, nil receiver, and a zero interval are all no-ops.
	NewRevocations(nil).StartRefresh(ctx, time.Millisecond)
	(*Revocations)(nil).StartRefresh(ctx, time.Millisecond)
	a.StartRefresh(ctx, 0)
}
