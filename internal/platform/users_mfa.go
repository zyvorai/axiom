package platform

import (
	"context"
	"crypto/subtle"
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
	if u.MFAEnabled {
		return "", fmt.Errorf("two-factor sign-in is already enabled; disable it first to re-enrol")
	}
	u.MFAPending = auth.GenerateTOTPSecret()
	if err := h.put(ctx, u); err != nil {
		return "", err
	}
	return u.MFAPending, nil
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
	if u.MFAEnabled {
		h.mfaMu.Unlock()
		return nil, fmt.Errorf("two-factor sign-in is already enabled")
	}
	if u.MFAPending == "" {
		h.mfaMu.Unlock()
		return nil, fmt.Errorf("start enrolment first")
	}
	step, ok := auth.VerifyTOTP(u.MFAPending, code, now, 0)
	if !ok {
		h.mfaMu.Unlock()
		return nil, fmt.Errorf("that code is not valid; check the time on your device and try again")
	}
	codes := auth.GenerateRecoveryCodes()
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = auth.HashRecoveryCode(c)
	}
	u.MFAEnabled, u.MFASecret, u.MFAPending = true, u.MFAPending, ""
	u.MFARecovery, u.MFALastStep = hashes, step
	err := h.put(ctx, u)
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
	u.MFAEnabled, u.MFASecret, u.MFAPending, u.MFARecovery, u.MFALastStep = false, "", "", nil, 0
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
	if !ok || !u.MFAEnabled {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if auth.LooksLikeRecoveryCode(code) {
		want := auth.HashRecoveryCode(code)
		idx := -1
		for i, hash := range u.MFARecovery {
			if subtle.ConstantTimeCompare([]byte(hash), []byte(want)) == 1 {
				idx = i
			}
		}
		if idx < 0 {
			return false
		}
		rest := make([]string, 0, len(u.MFARecovery)-1)
		rest = append(rest, u.MFARecovery[:idx]...)
		rest = append(rest, u.MFARecovery[idx+1:]...)
		u.MFARecovery = rest
		return h.put(ctx, u) == nil
	}
	step, ok := auth.VerifyTOTP(u.MFASecret, code, now, u.MFALastStep)
	if !ok {
		return false
	}
	u.MFALastStep = step
	return h.put(ctx, u) == nil
}

var (
	_ auth.UserLookup  = (*UserHub)(nil)
	_ auth.MFAProvider = (*UserHub)(nil)
)
