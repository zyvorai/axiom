package platform

import (
	"crypto/subtle"
	"errors"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
)

// mfaFactor is the pure second-factor state shared by local accounts (UserHub)
// and directory accounts (ExternalMFAStore): the TOTP enrolment state machine,
// recovery codes and replay protection, with no storage of its own. Callers
// load it from their record, call one method under their own lock, and persist
// the result.
type mfaFactor struct {
	Enabled bool
	// Secret is the active shared secret (only meaningful while Enabled).
	Secret string
	// Pending is a secret issued by begin but not yet confirmed by enable.
	Pending string
	// Recovery holds SHA-256 hashes of the unused one-time recovery codes.
	Recovery []string
	// LastStep is the last accepted TOTP time step (a code is single-use).
	LastStep int64
}

var (
	errMFAAlreadyEnabledReenrol = errors.New("two-factor sign-in is already enabled; disable it first to re-enrol")
	errMFAAlreadyEnabled        = errors.New("two-factor sign-in is already enabled")
	errMFANotStarted            = errors.New("start enrolment first")
	errMFABadCode               = errors.New("that code is not valid; check the time on your device and try again")
)

// begin issues a new pending secret. Nothing changes about how the account
// signs in until enable confirms it.
func (f *mfaFactor) begin() (string, error) {
	if f.Enabled {
		return "", errMFAAlreadyEnabledReenrol
	}
	f.Pending = auth.GenerateTOTPSecret()
	return f.Pending, nil
}

// enable confirms the pending secret with a code, turns the factor on and
// returns fresh recovery codes (only their hashes are kept).
func (f *mfaFactor) enable(code string, now time.Time) ([]string, error) {
	if f.Enabled {
		return nil, errMFAAlreadyEnabled
	}
	if f.Pending == "" {
		return nil, errMFANotStarted
	}
	step, ok := auth.VerifyTOTP(f.Pending, code, now, 0)
	if !ok {
		return nil, errMFABadCode
	}
	codes := auth.GenerateRecoveryCodes()
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = auth.HashRecoveryCode(c)
	}
	f.Enabled, f.Secret, f.Pending = true, f.Pending, ""
	f.Recovery, f.LastStep = hashes, step
	return codes, nil
}

// verify checks a TOTP or recovery code. On success the state is updated so the
// code cannot be used again (the TOTP step is remembered, the recovery code is
// burned); the caller must persist it and treat a failed write as a refusal.
func (f *mfaFactor) verify(code string, now time.Time) bool {
	if !f.Enabled {
		return false
	}
	if auth.LooksLikeRecoveryCode(code) {
		want := auth.HashRecoveryCode(code)
		idx := -1
		for i, hash := range f.Recovery {
			if subtle.ConstantTimeCompare([]byte(hash), []byte(want)) == 1 {
				idx = i
			}
		}
		if idx < 0 {
			return false
		}
		rest := make([]string, 0, len(f.Recovery)-1)
		rest = append(rest, f.Recovery[:idx]...)
		rest = append(rest, f.Recovery[idx+1:]...)
		f.Recovery = rest
		return true
	}
	step, ok := auth.VerifyTOTP(f.Secret, code, now, f.LastStep)
	if !ok {
		return false
	}
	f.LastStep = step
	return true
}

// reset removes the factor and any pending enrolment.
func (f *mfaFactor) reset() { *f = mfaFactor{} }
