package platform

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
)

// Lookup returns a local account's current role and namespace restriction. It
// implements auth.UserLookup, which lets sessions track live account changes.
func (h *UserHub) Lookup(username string) (string, []string, bool) {
	h.mu.RLock()
	u, ok := h.cache[strings.ToLower(strings.TrimSpace(username))]
	h.mu.RUnlock()
	if !ok {
		return "", nil, false
	}
	return u.Role, u.Namespaces, true
}

func (h *UserHub) user(username string) (User, bool) {
	h.mu.RLock()
	u, ok := h.cache[strings.ToLower(strings.TrimSpace(username))]
	h.mu.RUnlock()
	return u, ok
}

// put persists u and, only if that succeeded, updates the cache.
func (h *UserHub) put(ctx context.Context, u User) error {
	if err := h.store.Update(ctx, u); err != nil {
		return err
	}
	h.mu.Lock()
	h.cache[strings.ToLower(u.Username)] = u
	h.mu.Unlock()
	return nil
}

// factor and setFactor move a User's second-factor fields to and from the
// shared mfaFactor state machine (mfa_factor.go).
func (u User) factor() mfaFactor {
	return mfaFactor{Enabled: u.MFAEnabled, Secret: u.MFASecret, Pending: u.MFAPending, Recovery: u.MFARecovery, LastStep: u.MFALastStep}
}

func (u *User) setFactor(f mfaFactor) {
	u.MFAEnabled, u.MFASecret, u.MFAPending, u.MFARecovery, u.MFALastStep = f.Enabled, f.Secret, f.Pending, f.Recovery, f.LastStep
}

// MFAEnrolled reports whether the account has two-factor sign-in enabled.
func (h *UserHub) MFAEnrolled(username string) bool {
	u, ok := h.user(username)
	return ok && u.MFAEnabled
}

// MFAStatus describes an account's second-factor state for its owner.
func (h *UserHub) MFAStatus(username string) (enabled bool, recoveryRemaining int, exists bool) {
	u, ok := h.user(username)
	if !ok {
		return false, 0, false
	}
	return u.MFAEnabled, len(u.MFARecovery), true
}

// BeginMFA issues a new shared secret for the account and remembers it as
// pending. Nothing changes about how the account signs in until EnableMFA
// confirms the person's authenticator app produces valid codes from it.
func (h *UserHub) BeginMFA(ctx context.Context, username string) (string, error) {
	h.mfaMu.Lock()
	defer h.mfaMu.Unlock()
	u, ok := h.user(username)
	if !ok {
		return "", fmt.Errorf("user %q not found", username)
	}
	f := u.factor()
	secret, err := f.begin()
	if err != nil {
		return "", err
	}
	u.setFactor(f)
	if err := h.put(ctx, u); err != nil {
		return "", err
	}
	return secret, nil
}

// EnableMFA confirms the pending secret with a code from the authenticator,
// turns two-factor sign-in on and returns the one-time recovery codes (shown to
// the person exactly once; only their hashes are kept). The account's existing
// sessions are revoked, so anything signed in without a second factor must
// sign in again.
func (h *UserHub) EnableMFA(ctx context.Context, username, code string, now time.Time) ([]string, error) {
	h.mfaMu.Lock()
	u, ok := h.user(username)
	if !ok {
		h.mfaMu.Unlock()
		return nil, fmt.Errorf("user %q not found", username)
	}
	f := u.factor()
	codes, err := f.enable(code, now)
	if err != nil {
		h.mfaMu.Unlock()
		return nil, err
	}
	u.setFactor(f)
	err = h.put(ctx, u)
	h.mfaMu.Unlock()
	if err != nil {
		return nil, err
	}
	h.invalidate(u.Username)
	return codes, nil
}

// DisableMFA removes the account's second factor and any pending enrolment, and
// revokes its sessions. Callers decide who may do this (the owner after proving
// the password and a code, or an administrator resetting a locked-out person).
func (h *UserHub) DisableMFA(ctx context.Context, username string) error {
	h.mfaMu.Lock()
	u, ok := h.user(username)
	if !ok {
		h.mfaMu.Unlock()
		return fmt.Errorf("user %q not found", username)
	}
	f := u.factor()
	f.reset()
	u.setFactor(f)
	err := h.put(ctx, u)
	h.mfaMu.Unlock()
	if err != nil {
		return err
	}
	h.invalidate(u.Username)
	return nil
}

// VerifySecondFactor implements auth.MFAProvider. A six-digit TOTP is accepted
// once per time step; a recovery code is accepted once and then burned. If the
// updated state cannot be persisted the check fails: a replayable code is worse
// than a refused login.
func (h *UserHub) VerifySecondFactor(username, code string, now time.Time) bool {
	h.mfaMu.Lock()
	defer h.mfaMu.Unlock()
	u, ok := h.user(username)
	if !ok {
		return false
	}
	f := u.factor()
	if !f.verify(code, now) {
		return false
	}
	u.setFactor(f)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.put(ctx, u) == nil
}

var (
	_ auth.UserLookup  = (*UserHub)(nil)
	_ auth.MFAProvider = (*UserHub)(nil)
)
