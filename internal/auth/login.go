package auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// LoginPath is the sign-in endpoint that can collect a second factor and
	// returns a signed session. It is handled by the Guard itself so password
	// and code guesses are throttled exactly like any other credential check.
	LoginPath = "/api/v1/auth/login"
	// MFAHeader is set (to "required") on a 401 whose cause is a missing second
	// factor rather than a wrong password.
	MFAHeader = "X-Axiom-MFA"
)

// isSelfServiceAuth lists the account-security actions every signed-in user
// may take on their own account whatever their role — including read-only
// viewers, whom the guard otherwise blocks from every non-GET request. The
// handlers act only on the caller's own identity.
func isSelfServiceAuth(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/v1/auth/logout-all", "/api/v1/auth/mfa/setup", "/api/v1/auth/mfa/enable", "/api/v1/auth/mfa/disable":
		return true
	}
	return false
}

// EnvTOTP is the second factor for the single env-configured account
// (AXIOM_TOTP_SECRET). It remembers the last accepted time step so a code
// cannot be replayed within its validity window.
type EnvTOTP struct {
	mu     sync.Mutex
	secret string
	last   int64
}

// NewEnvTOTP returns nil for an empty secret (no second factor) and an error
// message via ok=false for a malformed one.
func NewEnvTOTP(secret string) (*EnvTOTP, bool) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, true
	}
	if !ValidTOTPSecret(secret) {
		return nil, false
	}
	return &EnvTOTP{secret: NormalizeTOTPSecret(secret)}, true
}

// Verify checks a code and, on success, records its step as used.
func (e *EnvTOTP) Verify(code string, now time.Time) bool {
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	step, ok := VerifyTOTP(e.secret, code, now, e.last)
	if ok {
		e.last = step
	}
	return ok
}

func (g Guard) verifySecondFactor(user, code string) bool {
	now := time.Now()
	if userMatches(g.Username, user) && g.EnvTOTP != nil {
		return g.EnvTOTP.Verify(code, now)
	}
	if g.MFA != nil {
		return g.MFA.VerifySecondFactor(user, code, now)
	}
	return false
}

// VerifyPassword reports whether password is the current password of user. The
// account-security endpoints use it to confirm the caller really is the
// account owner (a stolen session alone must not be able to change MFA).
func (g Guard) VerifyPassword(user, password string) bool {
	id, ok := g.authenticatePassword(user, password)
	return ok && id.viaCredentials
}

// RequestSecure reports whether the request arrived over TLS, directly or via
// a proxy that says so.
func RequestSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

func writeLoginJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// serveLogin handles POST /api/v1/auth/login: it verifies the password, then
// the second factor if the account has one, and issues a signed session
// (cookie plus a bearer token for scripts). It is the only way an account with
// two-factor sign-in can authenticate.
func (g Guard) serveLogin(w http.ResponseWriter, r *http.Request, limiter, acctLimiter *RateLimiter) {
	key := clientKey(r, g.TrustedProxyHeader)
	var req loginRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeLoginJSON(w, http.StatusBadRequest, map[string]any{"error": "expected JSON {username, password, code?}"})
		return
	}
	user := strings.TrimSpace(req.Username)
	acct := accountKey(user)
	audit := func(action string) {
		if g.Audit != nil {
			g.Audit.Record(action, key, LoginPath, user)
		}
	}
	if !limiter.Allowed(key) {
		audit("auth.rate_limited")
		writeRateLimited(w, limiter.RetryAfter(key))
		return
	}
	if acct != "" && !acctLimiter.Allowed(acct) {
		audit("auth.account_rate_limited")
		writeRateLimited(w, acctLimiter.RetryAfter(acct))
		return
	}
	failed := func(action string, body map[string]any) {
		limiter.RecordFailure(key)
		if acct != "" {
			acctLimiter.RecordFailure(acct)
		}
		audit(action)
		writeLoginJSON(w, http.StatusUnauthorized, body)
	}
	if user == "" || req.Password == "" {
		failed("auth.login_failed", map[string]any{"error": "invalid credentials"})
		return
	}
	id, ok := g.authenticatePassword(user, req.Password)
	if !ok {
		failed("auth.login_failed", map[string]any{"error": "invalid credentials"})
		return
	}
	if id.mfaPending {
		code := strings.TrimSpace(req.Code)
		if code == "" {
			// Not a failed guess: the password was right. Ask for the code.
			w.Header().Set(MFAHeader, "required")
			writeLoginJSON(w, http.StatusUnauthorized, map[string]any{"error": "two-factor code required", "mfa": true})
			return
		}
		if !g.verifySecondFactor(id.User, code) {
			failed("auth.mfa_failed", map[string]any{"error": "invalid two-factor code", "mfa": true})
			return
		}
	}
	limiter.RecordSuccess(key)
	if acct != "" {
		acctLimiter.RecordSuccess(acct)
	}
	token, err := SignSession(g.sessionSecret(), Session{User: id.User, Role: id.Role, Issuer: LocalSessionIssuer})
	if err != nil {
		writeLoginJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not create a session"})
		return
	}
	SetSessionCookie(w, token, RequestSecure(r))
	detail := "password"
	if id.mfaPending {
		detail = "password+mfa"
	}
	if g.Audit != nil {
		g.Audit.Record("auth.login", key, detail, id.User)
	}
	writeLoginJSON(w, http.StatusOK, map[string]any{
		"ok": true, "user": id.User, "role": id.Role, "mfa": id.mfaPending,
		"token": "axiom." + token, "expiresIn": int(SessionTTL().Seconds()),
	})
}
