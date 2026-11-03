package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
)

// ExternalMFARecord is the second factor of one directory (LDAP) account,
// keyed by its lower-cased directory username. The account itself lives in the
// directory; Axiom holds only this record. Secret, Pending and RecoveryHashes
// are secret material: they are persisted (in a Kubernetes Secret) but never
// leave the server in an API response.
type ExternalMFARecord struct {
	Username       string    `json:"username"`
	Enabled        bool      `json:"enabled"`
	Secret         string    `json:"secret,omitempty"`
	Pending        string    `json:"pending,omitempty"`
	RecoveryHashes []string  `json:"recoveryHashes,omitempty"`
	LastStep       int64     `json:"lastStep,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func (r ExternalMFARecord) factor() mfaFactor {
	return mfaFactor{Enabled: r.Enabled, Secret: r.Secret, Pending: r.Pending, Recovery: r.RecoveryHashes, LastStep: r.LastStep}
}

func (r *ExternalMFARecord) setFactor(f mfaFactor) {
	r.Enabled, r.Secret, r.Pending, r.RecoveryHashes, r.LastStep = f.Enabled, f.Secret, f.Pending, f.Recovery, f.LastStep
}

// ExternalMFAView is what an administrator sees about a directory account's
// second factor (GET /api/v1/users/external-mfa): never any secret material.
type ExternalMFAView struct {
	Username  string    `json:"username"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ExternalMFABackend persists the record set. Mutate is a read-modify-write
// that must be safe against concurrent writers (other replicas): fn gets the
// current records and returns whether it changed them; an error from fn aborts
// without writing and is returned as is.
type ExternalMFABackend interface {
	Load(ctx context.Context) (map[string]ExternalMFARecord, error)
	Mutate(ctx context.Context, fn func(recs map[string]ExternalMFARecord) (changed bool, err error)) error
}

// externalMFACacheTTL bounds how stale MFAEnrolled may be on a replica that
// did not make the change. Every state-changing operation (enrol, verify,
// reset) re-reads the backend, so replay protection never relies on the cache.
const externalMFACacheTTL = 5 * time.Second

// ExternalMFAStore holds second factors for directory (LDAP) accounts. It
// implements auth.MFAProvider keyed by directory username and the same
// self-service operations as UserHub (BeginMFA, EnableMFA, DisableMFA), using
// the same state machine (mfaFactor).
type ExternalMFAStore struct {
	backend ExternalMFABackend
	now     func() time.Time

	// mu serialises operations within this process; the backend's Mutate makes
	// them safe across replicas.
	mu sync.Mutex

	cacheMu  sync.RWMutex
	cache    map[string]ExternalMFARecord
	cachedAt time.Time

	invalidateMu sync.RWMutex
	onInvalidate func(username string)
}

// NewExternalMFAStore wraps backend (nil means in memory).
func NewExternalMFAStore(backend ExternalMFABackend) *ExternalMFAStore {
	if backend == nil {
		backend = &memoryExternalMFABackend{}
	}
	return &ExternalMFAStore{backend: backend, now: time.Now}
}

// NewMemoryExternalMFAStore keeps records in memory (demo builds and tests).
func NewMemoryExternalMFAStore() *ExternalMFAStore { return NewExternalMFAStore(nil) }

// SetInvalidateHook registers the function that revokes a user's sessions; it
// is called after enabling or removing a second factor.
func (s *ExternalMFAStore) SetInvalidateHook(fn func(username string)) {
	s.invalidateMu.Lock()
	s.onInvalidate = fn
	s.invalidateMu.Unlock()
}

func (s *ExternalMFAStore) invalidate(username string) {
	s.invalidateMu.RLock()
	fn := s.onInvalidate
	s.invalidateMu.RUnlock()
	if fn != nil {
		fn(username)
	}
}

func externalMFAKey(username string) string { return strings.ToLower(strings.TrimSpace(username)) }

// records returns the (possibly cached) record set. A backend error yields
// what was last seen, or nothing.
func (s *ExternalMFAStore) records() map[string]ExternalMFARecord {
	s.cacheMu.RLock()
	if s.cache != nil && s.now().Sub(s.cachedAt) < externalMFACacheTTL {
		c := s.cache
		s.cacheMu.RUnlock()
		return c
	}
	s.cacheMu.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	recs, err := s.backend.Load(ctx)
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if err != nil {
		if s.cache == nil {
			return map[string]ExternalMFARecord{}
		}
		return s.cache
	}
	s.cache, s.cachedAt = recs, s.now()
	return recs
}

func (s *ExternalMFAStore) record(username string) (ExternalMFARecord, bool) {
	r, ok := s.records()[externalMFAKey(username)]
	return r, ok
}

// mutate runs fn on one user's record (zero value and exists=false when there
// is none) under the backend's conflict-safe update. fn returns the record to
// store, or nil to delete it, and whether anything changed. The cache is
// refreshed from what was written.
func (s *ExternalMFAStore) mutate(ctx context.Context, username string, fn func(rec ExternalMFARecord, exists bool) (*ExternalMFARecord, bool, error)) error {
	key := externalMFAKey(username)
	if key == "" {
		return fmt.Errorf("username is required")
	}
	var written map[string]ExternalMFARecord
	err := s.backend.Mutate(ctx, func(recs map[string]ExternalMFARecord) (bool, error) {
		written = nil // fn may run again after a write conflict
		rec, exists := recs[key]
		next, changed, err := fn(rec, exists)
		if err != nil || !changed {
			return false, err
		}
		if next == nil {
			delete(recs, key)
		} else {
			next.Username, next.UpdatedAt = key, s.now().UTC()
			recs[key] = *next
		}
		written = recs
		return true, nil
	})
	if err == nil && written != nil {
		s.cacheMu.Lock()
		s.cache, s.cachedAt = copyExternalMFARecords(written), s.now()
		s.cacheMu.Unlock()
	}
	return err
}

// MFAEnrolled implements auth.MFAProvider.
func (s *ExternalMFAStore) MFAEnrolled(username string) bool {
	r, ok := s.record(username)
	return ok && r.Enabled
}

// MFAStatus describes a directory account's second-factor state for its owner.
func (s *ExternalMFAStore) MFAStatus(username string) (enabled bool, recoveryRemaining int) {
	r, ok := s.record(username)
	if !ok {
		return false, 0
	}
	return r.Enabled, len(r.RecoveryHashes)
}

// BeginMFA issues a pending secret for the directory account (creating its
// record on first use). The caller has already verified the directory password.
func (s *ExternalMFAStore) BeginMFA(ctx context.Context, username string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var secret string
	err := s.mutate(ctx, username, func(rec ExternalMFARecord, _ bool) (*ExternalMFARecord, bool, error) {
		f := rec.factor()
		sec, err := f.begin()
		if err != nil {
			return nil, false, err
		}
		secret = sec
		rec.setFactor(f)
		return &rec, true, nil
	})
	if err != nil {
		return "", err
	}
	return secret, nil
}

// EnableMFA confirms the pending secret, turns two-factor sign-in on, returns
// the one-time recovery codes and revokes the account's sessions.
func (s *ExternalMFAStore) EnableMFA(ctx context.Context, username, code string, now time.Time) ([]string, error) {
	s.mu.Lock()
	var codes []string
	err := s.mutate(ctx, username, func(rec ExternalMFARecord, exists bool) (*ExternalMFARecord, bool, error) {
		if !exists {
			return nil, false, errMFANotStarted
		}
		f := rec.factor()
		c, err := f.enable(code, now)
		if err != nil {
			return nil, false, err
		}
		codes = c
		rec.setFactor(f)
		return &rec, true, nil
	})
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	s.invalidate(username)
	return codes, nil
}

// DisableMFA removes the directory account's second factor (its record is
// deleted) and revokes its sessions. Removing a factor that does not exist is
// not an error.
func (s *ExternalMFAStore) DisableMFA(ctx context.Context, username string) error {
	_, err := s.Reset(ctx, username)
	return err
}

// Reset is DisableMFA that also reports whether there was a record to remove
// (the admin reset answers 404 when neither a local account nor a directory
// record has the name). Sessions are revoked when a record existed.
func (s *ExternalMFAStore) Reset(ctx context.Context, username string) (existed bool, err error) {
	s.mu.Lock()
	err = s.mutate(ctx, username, func(_ ExternalMFARecord, exists bool) (*ExternalMFARecord, bool, error) {
		existed = exists
		return nil, exists, nil
	})
	s.mu.Unlock()
	if err != nil {
		return false, err
	}
	if existed {
		s.invalidate(username)
	}
	return existed, nil
}

// VerifySecondFactor implements auth.MFAProvider: a TOTP is accepted once per
// time step, a recovery code once. It always re-reads the backend, and fails if
// the updated state cannot be written (a replayable code is worse than a
// refused login).
func (s *ExternalMFAStore) VerifySecondFactor(username, code string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	accepted := false
	err := s.mutate(ctx, username, func(rec ExternalMFARecord, exists bool) (*ExternalMFARecord, bool, error) {
		accepted = false
		if !exists {
			return nil, false, nil
		}
		f := rec.factor()
		if !f.verify(code, now) {
			return nil, false, nil
		}
		accepted = true
		rec.setFactor(f)
		return &rec, true, nil
	})
	return err == nil && accepted
}

// List returns every directory account with a record (enabled, or with an
// enrolment in progress), without secret material, sorted by username.
func (s *ExternalMFAStore) List() []ExternalMFAView {
	recs := s.records()
	out := make([]ExternalMFAView, 0, len(recs))
	for _, r := range recs {
		out = append(out, ExternalMFAView{Username: r.Username, Enabled: r.Enabled, UpdatedAt: r.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out
}

func copyExternalMFARecords(in map[string]ExternalMFARecord) map[string]ExternalMFARecord {
	out := make(map[string]ExternalMFARecord, len(in))
	for k, v := range in {
		v.RecoveryHashes = append([]string(nil), v.RecoveryHashes...)
		out[k] = v
	}
	return out
}

// memoryExternalMFABackend keeps the records in process memory.
type memoryExternalMFABackend struct {
	mu   sync.Mutex
	recs map[string]ExternalMFARecord
}

func (m *memoryExternalMFABackend) Load(context.Context) (map[string]ExternalMFARecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return copyExternalMFARecords(m.recs), nil
}

func (m *memoryExternalMFABackend) Mutate(_ context.Context, fn func(map[string]ExternalMFARecord) (bool, error)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs := copyExternalMFARecords(m.recs)
	changed, err := fn(recs)
	if err != nil || !changed {
		return err
	}
	m.recs = recs
	return nil
}

var _ auth.MFAProvider = (*ExternalMFAStore)(nil)
