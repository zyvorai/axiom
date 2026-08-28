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
}

func (s Session) Valid() bool {
	return s.User != "" && s.Exp > time.Now().Unix()
}

// SignSession returns an HMAC-SHA256 signed token using secret.
func SignSession(secret string, s Session) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("session secret is empty")
	}
	if s.Exp == 0 {
		s.Exp = time.Now().Add(12 * time.Hour).Unix()
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
		MaxAge:   int((12 * time.Hour).Seconds()),
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
