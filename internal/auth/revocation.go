package auth

import (
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
// Each replica holds its own copy: with several replicas a revocation reaches
// the others only through the shared store at their next restart, so pair
// multi-replica installs with a short AXIOM_SESSION_TTL.

// RevocationStore persists the opaque snapshot Revocations produces.
type RevocationStore interface {
	Load() ([]byte, error)
	Save([]byte) error
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
		} else if len(raw) > 0 {
			var snap revocationSnapshot
			if err := json.Unmarshal(raw, &snap); err != nil {
				log.Printf("auth: session revocations are unreadable, starting empty: %v", err)
			} else {
				if snap.IDs != nil {
					r.ids = snap.IDs
				}
				if snap.Users != nil {
					r.users = snap.Users
				}
				r.pruneLocked()
			}
		}
	}
	return r
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
	snap := r.snapshotLocked()
	r.mu.Unlock()
	r.persist(snap)
}

// RevokeUser ends every session the user has opened so far. A session issued
// after this call is unaffected, so the person can sign straight back in.
func (r *Revocations) RevokeUser(user string) {
	if r == nil || userKey(user) == "" {
		return
	}
	r.mu.Lock()
	r.users[userKey(user)] = r.now().UnixMilli()
	r.pruneLocked()
	snap := r.snapshotLocked()
	r.mu.Unlock()
	r.persist(snap)
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

func (r *Revocations) snapshotLocked() []byte {
	if r.store == nil {
		return nil
	}
	raw, err := json.Marshal(revocationSnapshot{IDs: r.ids, Users: r.users})
	if err != nil {
		return nil
	}
	return raw
}

func (r *Revocations) persist(raw []byte) {
	if r.store == nil || raw == nil {
		return
	}
	r.saveMu.Lock()
	defer r.saveMu.Unlock()
	if err := r.store.Save(raw); err != nil {
		log.Printf("auth: persisting session revocations: %v", err)
	}
}
