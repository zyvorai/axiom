package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

type Guard struct {
	Username string
	Password string
	APIKey   string
	// SessionSecret signs OIDC session cookies. Defaults to Password when empty.
	SessionSecret string
}

func (g Guard) sessionSecret() string {
	if g.SessionSecret != "" {
		return g.SessionSecret
	}
	return g.Password
}

func (g Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		if g.authorize(r) {
			next.ServeHTTP(w, r)
			return
		}
		// Do not set WWW-Authenticate — that triggers the browser's native
		// Basic-auth popup on top of Axiom's custom login form.
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func isPublicPath(r *http.Request) bool {
	path := r.URL.Path
	if r.Method == http.MethodGet && path == "/api/v1/health" {
		return true
	}
	if r.Method == http.MethodGet && path == "/api/v1/auth/providers" {
		return true
	}
	if r.Method == http.MethodGet && (path == "/api/v1/auth/oidc/login" || path == "/api/v1/auth/oidc/callback" || path == "/api/v1/auth/saml/login") {
		return true
	}
	if r.Method == http.MethodPost && path == "/api/v1/auth/logout" {
		return true
	}
	return false
}

func (g Guard) authorize(r *http.Request) bool {
	if _, ok := SessionFromRequest(g.sessionSecret(), r); ok {
		return true
	}
	if g.APIKey != "" {
		if bearer := bearerToken(r.Header.Get("Authorization")); bearer != "" {
			if subtle.ConstantTimeCompare([]byte(bearer), []byte(g.APIKey)) == 1 {
				return true
			}
		}
		if key := strings.TrimSpace(r.URL.Query().Get("api_key")); key != "" {
			if subtle.ConstantTimeCompare([]byte(key), []byte(g.APIKey)) == 1 {
				return true
			}
		}
	}
	u, p, ok := credentialsFromRequest(r)
	if !ok {
		return false
	}
	if g.APIKey != "" && (u == "apikey" || u == "__apikey__") {
		return subtle.ConstantTimeCompare([]byte(p), []byte(g.APIKey)) == 1
	}
	return userMatches(g.Username, u) &&
		subtle.ConstantTimeCompare([]byte(p), []byte(g.Password)) == 1
}

// userMatches accepts the configured username, and when that is "demo"
// also accepts demo@zyvor.dev (preferred login identity).
func userMatches(configured, provided string) bool {
	configured = strings.TrimSpace(configured)
	provided = strings.TrimSpace(provided)
	if subtle.ConstantTimeCompare([]byte(provided), []byte(configured)) == 1 {
		return true
	}
	if strings.EqualFold(configured, "demo") && strings.EqualFold(provided, "demo@zyvor.dev") {
		return true
	}
	if strings.EqualFold(configured, "demo@zyvor.dev") && strings.EqualFold(provided, "demo") {
		return true
	}
	return false
}

func bearerToken(v string) string {
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(v, "Bearer "))
}

func parseBasic(v string) (string, string, bool) {
	if !strings.HasPrefix(v, "Basic ") {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(v, "Basic ")))
	if err != nil {
		return "", "", false
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	return user, pass, ok
}

// credentialsFromRequest reads Basic auth from the header or access_token query
// (base64 user:pass) so browser EventSource/WebSocket clients can authenticate.
func credentialsFromRequest(r *http.Request) (string, string, bool) {
	if u, p, ok := parseBasic(r.Header.Get("Authorization")); ok {
		return u, p, true
	}
	token := strings.TrimSpace(r.URL.Query().Get("access_token"))
	if token == "" {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", "", false
	}
	return strings.Cut(string(raw), ":")
}

// Basic keeps the legacy constructor used by older call sites / tests.
type Basic struct{ Username, Password string }

func (b Basic) Middleware(next http.Handler) http.Handler {
	return Guard{Username: b.Username, Password: b.Password}.Middleware(next)
}
