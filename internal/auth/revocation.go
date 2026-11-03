package auth

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"
)

// Sessions are stateless signed tokens, so on their own nothing can end one
// before it expires. Revocations is the server-side memory that can: a set of
// individually revoked session IDs (logout) and a per-user cut-off (sign out
// everywhere, or an admin changing someone's role/password or deleting them),
// which revokes every session of that user issued at or before the cut-off.
//
// The data is tiny and is optionally persisted through a RevocationStore so a
// revocation survives a restart; without a store (demo mode) it is in memory.
// Each replica holds its own copy and shares it through the store: every
// change is merged into what the store holds (never overwriting another
// replica's entries), and StartRefresh re-reads the store periodically, so a
// revocation on one replica reaches the others within the refresh interval.
// Merging is a union: nothing is ever un-revoked by a merge.

// RevocationStore persists the opaque snapshot Revocations produces.
type RevocationStore interface {
	Load() ([]byte, error)
	Save([]byte) error
}

// MergingRevocationStore is an optional, conflict-safe extension of
// RevocationStore: Merge reads the current snapshot, passes it to fn and
// writes back fn's result only if nobody changed the snapshot in between,
// calling fn again on a conflict. Stores without it fall back to Load + Save,
// which is last-writer-wins between replicas.
type MergingRevocationStore interface {
	RevocationStore
	Merge(fn func(current []byte) ([]byte, error)) error
}

const (
	maxRevokedSessionIDs = 20000
	// A user cut-off only has to outlive the longest possible session.
	userCutoffRetention = 31 * 24 * time.Hour
)

type revocationSnapshot struct {
	IDs   map[string]int64 `json:"ids,omitempty"`   // session id -> its expiry (Unix s)
	Users map[string]int64 `json:"users,omitempty"` // lower-cased user -> cut-off (Unix ms)
}

// Revocations is safe for concurrent use. The zero value and a nil pointer are
// both valid and revoke nothing.
type Revocations struct {
	mu    sync.Mutex
	ids   map[string]int64
	users map[string]int64
	store RevocationStore
	now   func() time.Time
	// saveMu keeps persisted snapshots in the order they were taken.
	saveMu sync.Mutex
}

// NewRevocations loads any persisted state from store (which may be nil).
func NewRevocations(store RevocationStore) *Revocations {
	r := &Revocations{ids: map[string]int64{}, users: map[string]int64{}, store: store, now: time.Now}
	if store != nil {
		raw, err := store.Load()
		if err != nil {
			log.Printf("auth: loading session revocations: %v", err)
		} else if err := r.mergeRaw(raw); err != nil {
			log.Printf("auth: session revocations are unreadable, starting empty: %v", err)
		}
	}
	return r
}

// mergeRaw folds a persisted snapshot into the in-memory state.
func (r *Revocations) mergeRaw(raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var snap revocationSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return err
	}
	r.mu.Lock()
	r.mergeLocked(snap)
	r.pruneLocked()
	r.mu.Unlock()
	return nil
}

// mergeLocked takes the union of revoked session IDs (keeping the later
// expiry) and, per user, the later cut-off. It never removes an entry.
func (r *Revocations) mergeLocked(snap revocationSnapshot) {
	for id, exp := range snap.IDs {
		if cur, ok := r.ids[id]; !ok || exp > cur {
			r.ids[id] = exp
		}
	}
	for u, at := range snap.Users {
		if cur, ok := r.users[u]; !ok || at > cur {
			r.users[u] = at
		}
	}
}

// Refresh re-reads the store and merges what other replicas have revoked.
func (r *Revocations) Refresh() error {
	if r == nil || r.store == nil {
		return nil
	}
	raw, err := r.store.Load()
	if err != nil {
		return err
	}
	return r.mergeRaw(raw)
}

// StartRefresh re-reads the store every interval until ctx is cancelled, so
// revocations made by other replicas apply here too. It does nothing without a
// store or for a non-positive interval.
func (r *Revocations) StartRefresh(ctx context.Context, interval time.Duration) {
	if r == nil || r.store == nil || interval <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := r.Refresh(); err != nil {
					log.Printf("auth: refreshing session revocations: %v", err)
				}
			}
		}
	}()
}

func userKey(u string) string { return strings.ToLower(strings.TrimSpace(u)) }

// RevokeSession ends one session (identified by its jti) until it would have
// expired anyway.
func (r *Revocations) RevokeSession(id string, exp int64) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	if exp == 0 {
		exp = r.now().Add(sessionTTL).Unix()
	}
	r.ids[id] = exp
	r.pruneLocked()
	r.mu.Unlock()
	r.persist()
}

// RevokeUser ends every session the user has opened so far. A session issued
// after this call is unaffected, so the person can sign straight back in.
func (r *Revocations) RevokeUser(user string) {
	if r == nil || userKey(user) == "" {
		return
	}
	r.mu.Lock()
	if now := r.now().UnixMilli(); now > r.users[userKey(user)] {
		r.users[userKey(user)] = now
	}
	r.pruneLocked()
	r.mu.Unlock()
	r.persist()
}

// Revoked reports whether s has been revoked.
func (r *Revocations) Revoked(s Session) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID != "" {
		if _, ok := r.ids[s.ID]; ok {
			return true
		}
	}
	if cut, ok := r.users[userKey(s.User)]; ok {
		// A token without an issue time predates this feature; treat it as
		// issued before any cut-off.
		if s.IatMs == 0 || s.IatMs <= cut {
			return true
		}
	}
	return false
}

func (r *Revocations) pruneLocked() {
	now := r.now()
	for id, exp := range r.ids {
		if exp <= now.Unix() {
			delete(r.ids, id)
		}
	}
	// Bound memory against a flood of logouts: drop the soonest-to-expire first.
	for len(r.ids) > maxRevokedSessionIDs {
		var oldID string
		var oldExp int64 = 1<<63 - 1
		for id, exp := range r.ids {
			if exp < oldExp {
				oldID, oldExp = id, exp
			}
		}
		delete(r.ids, oldID)
	}
	cutoff := now.Add(-userCutoffRetention).UnixMilli()
	for u, at := range r.users {
		if at < cutoff {
			delete(r.users, u)
		}
	}
}

// mergeAndSnapshot folds current (the store's snapshot) into the in-memory
// state and returns the merged snapshot to write back.
func (r *Revocations) mergeAndSnapshot(current []byte) ([]byte, error) {
	if err := r.mergeRaw(current); err != nil {
		// An unreadable stored snapshot is replaced rather than blocking every
		// future revocation.
		log.Printf("auth: stored session revocations are unreadable, replacing them: %v", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return json.Marshal(revocationSnapshot{IDs: r.ids, Users: r.users})
}

// persist writes the state to the store merged with whatever the store holds
// now, so concurrent revocations on two replicas do not overwrite each other.
func (r *Revocations) persist() {
	if r.store == nil {
		return
	}
	r.saveMu.Lock()
	defer r.saveMu.Unlock()
	var err error
	if ms, ok := r.store.(MergingRevocationStore); ok {
		err = ms.Merge(r.mergeAndSnapshot)
	} else {
		// Best effort read-modify-write for stores without a conflict-safe path.
		current, lerr := r.store.Load()
		if lerr != nil {
			current = nil
		}
		var raw []byte
		if raw, err = r.mergeAndSnapshot(current); err == nil {
			err = r.store.Save(raw)
		}
	}
	if err != nil {
		log.Printf("auth: persisting session revocations: %v", err)
	}
}
