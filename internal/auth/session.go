package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	SessionCookie = "axiom_session"
	OIDCVerifier  = "axiom_oidc_verifier"
	OIDCState     = "axiom_oidc_state"
)

// Session is a signed operator session issued after OIDC (or local) login.
type Session struct {
	User   string `json:"u"`
	Email  string `json:"e,omitempty"`
	Issuer string `json:"iss,omitempty"`
	Role   string `json:"r,omitempty"`
	Exp    int64  `json:"exp"`
	// ID names this one session so it can be revoked individually (logout).
	// IatMs is when it was issued, in Unix milliseconds, so "sign out
	// everywhere" can revoke every session a user opened before a moment.
	// Tokens minted before these fields existed carry neither and are revoked
	// only by the per-user cut-off.
	ID    string `json:"jti,omitempty"`
	IatMs int64  `json:"iat,omitempty"`
}

// sessionTTL is how long a session stays valid. Override with SetSessionTTL
// (AXIOM_SESSION_TTL); never below a minute or above 30 days.
var sessionTTL = 12 * time.Hour

// SetSessionTTL changes the lifetime of sessions issued from now on.
func SetSessionTTL(d time.Duration) {
	switch {
	case d < time.Minute:
		d = time.Minute
	case d > 30*24*time.Hour:
		d = 30 * 24 * time.Hour
	}
	sessionTTL = d
}

// SessionTTL reports the current session lifetime.
func SessionTTL() time.Duration { return sessionTTL }

func (s Session) Valid() bool {
	return s.User != "" && s.Exp > time.Now().Unix()
}

// SignSession returns an HMAC-SHA256 signed token using secret.
func SignSession(secret string, s Session) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("session secret is empty")
	}
	now := time.Now()
	if s.Exp == 0 {
		s.Exp = now.Add(sessionTTL).Unix()
	}
	if s.ID == "" {
		s.ID = RandomURLToken(12)
	}
	if s.IatMs == 0 {
		s.IatMs = now.UnixMilli()
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sig, nil
}

func ParseSession(secret, token string) (Session, bool) {
	if secret == "" || token == "" {
		return Session{}, false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Session{}, false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), []byte(parts[1])) != 1 {
		return Session{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Session{}, false
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil || !s.Valid() {
		return Session{}, false
	}
	return s, true
}

func RandomURLToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func SessionFromRequest(secret string, r *http.Request) (Session, bool) {
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
		if s, ok := ParseSession(secret, c.Value); ok {
			return s, true
		}
	}
	if authz := r.Header.Get("Authorization"); strings.HasPrefix(authz, "Bearer axiom.") {
		if s, ok := ParseSession(secret, strings.TrimPrefix(authz, "Bearer axiom.")); ok {
			return s, true
		}
	}
	if t := strings.TrimSpace(r.URL.Query().Get("axiom_session")); t != "" {
		if s, ok := ParseSession(secret, t); ok {
			return s, true
		}
	}
	return Session{}, false
}

func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   -1,
	})
}
