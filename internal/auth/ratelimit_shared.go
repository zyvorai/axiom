package auth

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// SharedRateBacking dual-writes lockout state so multi-replica login
// throttling shares account (and IP) counters without sticky sessions.
type SharedRateBacking struct {
	store RevocationStore
	mu    sync.Mutex
}

// NewSharedRateBacking wraps a blob store for shared login throttling.
func NewSharedRateBacking(store RevocationStore) *SharedRateBacking {
	if store == nil {
		return nil
	}
	return &SharedRateBacking{store: store}
}

type rateSharedSnap struct {
	Entries map[string]rateSharedEntry `json:"entries"`
}

type rateSharedEntry struct {
	Failures    int   `json:"failures"`
	LockedUntil int64 `json:"lockedUntil,omitempty"`
	LastSeen    int64 `json:"lastSeen"`
}

// AttachShared enables ConfigMap-backed rate-limit state across replicas.
func (rl *RateLimiter) AttachShared(store RevocationStore) {
	if rl == nil || store == nil {
		return
	}
	rl.AttachSharedBacking(&SharedRateBacking{store: store})
	log.Printf("auth: shared login-throttle store enabled")
}

// AttachSharedBacking wires an existing shared backing (so IP + account
// limiters can share one ConfigMap mutex and not clobber each other).
func (rl *RateLimiter) AttachSharedBacking(b *SharedRateBacking) {
	if rl == nil || b == nil || b.store == nil {
		return
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.shared = b
}

func (rl *RateLimiter) pullSharedLocked() {
	if rl.shared == nil || rl.shared.store == nil {
		return
	}
	rl.shared.mu.Lock()
	defer rl.shared.mu.Unlock()
	raw, err := rl.shared.store.Load()
	if err != nil || len(raw) == 0 {
		return
	}
	var snap rateSharedSnap
	if json.Unmarshal(raw, &snap) != nil || snap.Entries == nil {
		return
	}
	now := rl.now()
	for k, e := range snap.Entries {
		if e.LastSeen > 0 && now.Sub(time.Unix(e.LastSeen, 0)) > rateLimitIdleEviction {
			continue
		}
		// Cleared lockout (RecordSuccess tombstone).
		if e.Failures == 0 && e.LockedUntil <= 0 {
			delete(rl.state, k)
			continue
		}
		s := rl.state[k]
		if s == nil {
			if len(rl.state) >= rateLimitMaxKeys {
				rl.evictOldestLocked()
			}
			s = &attemptState{}
			rl.state[k] = s
		}
		// Keep the stricter of local vs shared (more failures / later lockout).
		if e.Failures > s.failures {
			s.failures = e.Failures
		}
		if e.LockedUntil > s.lockedUntil.Unix() {
			s.lockedUntil = time.Unix(e.LockedUntil, 0)
		}
		if e.LastSeen > s.lastSeen.Unix() {
			s.lastSeen = time.Unix(e.LastSeen, 0)
		}
	}
}

func (rl *RateLimiter) pushSharedLocked(key string) {
	if rl.shared == nil || rl.shared.store == nil || key == "" {
		return
	}
	rl.shared.mu.Lock()
	defer rl.shared.mu.Unlock()
	raw, _ := rl.shared.store.Load()
	var snap rateSharedSnap
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &snap)
	}
	if snap.Entries == nil {
		snap.Entries = map[string]rateSharedEntry{}
	}
	now := rl.now().Unix()
	for k, e := range snap.Entries {
		if e.LastSeen > 0 && now-e.LastSeen > int64(rateLimitIdleEviction/time.Second) {
			delete(snap.Entries, k)
		}
	}
	if s, ok := rl.state[key]; ok && s != nil {
		snap.Entries[key] = rateSharedEntry{
			Failures:    s.failures,
			LockedUntil: s.lockedUntil.Unix(),
			LastSeen:    s.lastSeen.Unix(),
		}
	} else {
		// Tombstone so other replicas drop a cleared lockout (not just omit the key).
		snap.Entries[key] = rateSharedEntry{Failures: 0, LockedUntil: 0, LastSeen: now}
	}
	out, _ := json.Marshal(snap)
	_ = rl.shared.store.Save(out)
}
