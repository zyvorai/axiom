package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"
)

// Shared (multi-replica) pod-proxy credentials.
//
// By default pod-proxy tickets and sessions are random tokens held in one
// replica's memory (podproxy.go), so with several replicas the console and the
// proxy hosts need session affinity. When the operator configures a stable
// AXIOM_SESSION_SECRET (the Helm chart requires one with replicaCount>1) the
// stores are built "shared" instead (NewSharedPodProxyTicketStore,
// NewSharedPodProxySessionStore):
//
//   - a ticket is an HMAC-signed token carrying its grant (user, role,
//     namespace restriction, pod, port, path, cluster name, issue time, the
//     per-session-host id) plus a random ticket id and its expiry, so any
//     replica can redeem it. Single use is enforced in the replica's memory
//     and, best effort, through PodProxyRedeemed's shared set of redeemed
//     ticket ids (a ConfigMap in kubernetes mode);
//   - a session cookie is an HMAC-signed token carrying the grant, the session
//     id, its creation time and its idle deadline, so any replica can check
//     it. The sliding idle expiry is kept by re-issuing the cookie with a
//     later signed deadline (at most once a minute), never past the absolute
//     maximum counted from the signed creation time.
//
// The signing key is derived from the session secret for this purpose only
// (HMAC-SHA256(secret, podProxyKeyLabel)), so a pod-proxy token can never
// verify as a console session or the other way round, and tickets and
// session cookies are signed under different kinds so neither can stand in
// for the other. Revocation (auth.Revocations, checked on every request) and
// the per-user caps keep working; the caps count only what this replica
// issued.

const (
	podProxyKeyLabel = "zyvor-axiom pod-proxy token v1"
	// podProxyTokenMax bounds a token before it is decoded.
	podProxyTokenMax = 6 << 10
	// podProxyRenewAfter is how old a session cookie's idle deadline must be
	// before a request re-issues the cookie.
	podProxyRenewAfter = time.Minute
	// podProxyRedeemedMax bounds the shared redeemed-ticket set.
	podProxyRedeemedMax = 8192

	podProxyKindTicket  byte = 't'
	podProxyKindSession byte = 's'
)

type podProxySigner struct{ key []byte }

func newPodProxySigner(secret string) *podProxySigner {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(podProxyKeyLabel))
	return &podProxySigner{key: m.Sum(nil)}
}

func (s *podProxySigner) mac(kind byte, payload []byte) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte{kind})
	m.Write(payload)
	return m.Sum(nil)
}

// seal returns base64url(json(v) || mac(kind || json(v))): URL- and
// cookie-safe, no separator characters.
func (s *podProxySigner) seal(kind byte, v any) (string, error) {
	payload, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(append(payload, s.mac(kind, payload)...)), nil
}

// open verifies tok as a kind token and decodes it into v.
func (s *podProxySigner) open(kind byte, tok string, v any) bool {
	if tok == "" || len(tok) > podProxyTokenMax {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil || len(raw) <= sha256.Size {
		return false
	}
	payload, sum := raw[:len(raw)-sha256.Size], raw[len(raw)-sha256.Size:]
	if !hmac.Equal(sum, s.mac(kind, payload)) {
		return false
	}
	return json.Unmarshal(payload, v) == nil
}

// podProxyTicketClaims is the signed content of a shared pod-proxy ticket.
type podProxyTicketClaims struct {
	V   int           `json:"v"`
	ID  string        `json:"jti"`
	Exp int64         `json:"exp"` // Unix ms
	G   PodProxyGrant `json:"g"`
}

// podProxySessionClaims is the signed content of a shared session cookie.
type podProxySessionClaims struct {
	V       int           `json:"v"`
	SID     string        `json:"sid"`
	Created int64         `json:"c"` // Unix ms
	Idle    int64         `json:"e"` // idle deadline, Unix ms
	G       PodProxyGrant `json:"g"`
}

// PodProxyRedeemed is the set of shared pod-proxy ticket ids already redeemed:
// in this replica's memory and, when a store is given, in a snapshot shared
// with the other replicas (the RevocationStore pattern: an opaque blob, merged
// with a conflict-safe read-modify-write when the store supports it). Entries
// are dropped once the ticket they name has expired. Safe for concurrent use;
// a nil pointer remembers nothing.
//
// Only random ticket ids are stored, never a ticket: an id alone cannot be
// turned into a redeemable ticket without the signing key.
type PodProxyRedeemed struct {
	mu    sync.Mutex
	ids   map[string]int64 // ticket id -> ticket expiry (Unix ms)
	store RevocationStore
	now   func() time.Time
}

// NewPodProxyRedeemed returns a set backed by store (nil: this replica only).
func NewPodProxyRedeemed(store RevocationStore) *PodProxyRedeemed {
	return NewPodProxyRedeemedWithClock(store, time.Now)
}

// NewPodProxyRedeemedWithClock is NewPodProxyRedeemed with an injected clock.
func NewPodProxyRedeemedWithClock(store RevocationStore, now func() time.Time) *PodProxyRedeemed {
	return &PodProxyRedeemed{ids: map[string]int64{}, store: store, now: now}
}

type podProxyRedeemedSnapshot struct {
	IDs map[string]int64 `json:"ids,omitempty"`
}

var errPodProxyAlreadyRedeemed = errors.New("pod proxy ticket already redeemed")

// Mark records ticket id as redeemed until exp. It reports false when id was
// already redeemed on this replica or, as far as the shared store shows, on
// another one. A store that cannot be read or written does not block the
// redemption (the in-process set still applies); the failure is logged.
func (r *PodProxyRedeemed) Mark(id string, exp time.Time) bool {
	if r == nil || id == "" {
		return false
	}
	nowMs := r.now().UnixMilli()
	r.mu.Lock()
	pruneRedeemed(r.ids, nowMs)
	if _, used := r.ids[id]; used {
		r.mu.Unlock()
		return false
	}
	r.ids[id] = exp.UnixMilli()
	r.mu.Unlock()
	if r.store == nil {
		return true
	}
	add := func(current []byte) ([]byte, error) {
		var snap podProxyRedeemedSnapshot
		if len(current) > 0 {
			if err := json.Unmarshal(current, &snap); err != nil {
				log.Printf("auth: stored pod proxy redeemed tickets are unreadable, replacing them: %v", err)
				snap = podProxyRedeemedSnapshot{}
			}
		}
		if snap.IDs == nil {
			snap.IDs = map[string]int64{}
		}
		pruneRedeemed(snap.IDs, nowMs)
		if at, used := snap.IDs[id]; used && at > nowMs {
			return nil, errPodProxyAlreadyRedeemed
		}
		snap.IDs[id] = exp.UnixMilli()
		return json.Marshal(snap)
	}
	var err error
	if ms, ok := r.store.(MergingRevocationStore); ok {
		err = ms.Merge(add)
	} else {
		// Best effort read-modify-write for stores without a conflict-safe path.
		var current, raw []byte
		if current, err = r.store.Load(); err == nil {
			if raw, err = add(current); err == nil {
				err = r.store.Save(raw)
			}
		}
	}
	switch {
	case errors.Is(err, errPodProxyAlreadyRedeemed):
		return false
	case err != nil:
		log.Printf("auth: recording a redeemed pod proxy ticket in the shared store: %v (enforced on this replica only)", err)
	}
	return true
}

// pruneRedeemed drops expired ids and, past podProxyRedeemedMax, the ones
// that expire soonest.
func pruneRedeemed(ids map[string]int64, nowMs int64) {
	for id, exp := range ids {
		if exp <= nowMs {
			delete(ids, id)
		}
	}
	for len(ids) > podProxyRedeemedMax {
		var oldID string
		var oldExp int64 = 1<<63 - 1
		for id, exp := range ids {
			if exp < oldExp {
				oldID, oldExp = id, exp
			}
		}
		delete(ids, oldID)
	}
}

// MemoryMergingStore is an in-memory MergingRevocationStore: one blob that
// several Revocations / PodProxyRedeemed instances (standing in for replicas,
// e.g. in tests) can share. Safe for concurrent use.
type MemoryMergingStore struct {
	mu   sync.Mutex
	data []byte
}

// Load returns the stored blob.
func (m *MemoryMergingStore) Load() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte(nil), m.data...), nil
}

// Save replaces the stored blob.
func (m *MemoryMergingStore) Save(b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = append([]byte(nil), b...)
	return nil
}

// Merge applies fn to the stored blob atomically.
func (m *MemoryMergingStore) Merge(fn func(current []byte) ([]byte, error)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, err := fn(append([]byte(nil), m.data...))
	if err != nil {
		return err
	}
	m.data = append([]byte(nil), raw...)
	return nil
}

var _ MergingRevocationStore = (*MemoryMergingStore)(nil)
