package auth

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiterAllowsWithinGrace(t *testing.T) {
	rl := NewRateLimiter()
	for i := 0; i < rateLimitGraceFailures; i++ {
		if !rl.Allowed("1.2.3.4") {
			t.Fatalf("expected allowed within grace period, failure #%d", i+1)
		}
		rl.RecordFailure("1.2.3.4")
	}
	if !rl.Allowed("1.2.3.4") {
		t.Fatal("expected still allowed exactly at the grace boundary")
	}
}

func TestRateLimiterLocksOutAfterGrace(t *testing.T) {
	rl := NewRateLimiter()
	for i := 0; i < rateLimitGraceFailures+1; i++ {
		rl.RecordFailure("5.6.7.8")
	}
	if rl.Allowed("5.6.7.8") {
		t.Fatal("expected locked out after exceeding grace failures")
	}
}

func TestRateLimiterSuccessClearsFailures(t *testing.T) {
	rl := NewRateLimiter()
	for i := 0; i < rateLimitGraceFailures+1; i++ {
		rl.RecordFailure("9.9.9.9")
	}
	if rl.Allowed("9.9.9.9") {
		t.Fatal("expected locked out before success")
	}
	rl.RecordSuccess("9.9.9.9")
	if !rl.Allowed("9.9.9.9") {
		t.Fatal("expected allowed again after a recorded success")
	}
}

func TestRateLimiterTableIsBounded(t *testing.T) {
	rl := NewRateLimiter()
	for i := 0; i < rateLimitMaxKeys+500; i++ {
		rl.RecordFailure(fmt.Sprintf("k%d", i))
	}
	if n := len(rl.state); n > rateLimitMaxKeys {
		t.Fatalf("tracked clients must be capped at %d, have %d", rateLimitMaxKeys, n)
	}
}

func TestClientKeyIPv6UsesSlash64(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "[2001:db8:1:2:aaaa:bbbb:cccc:dddd]:5555"
	a := clientKey(r, "")
	r.RemoteAddr = "[2001:db8:1:2:1111:2222:3333:4444]:6666"
	if b := clientKey(r, ""); a != b {
		t.Fatalf("hosts in the same /64 must share a bucket: %q vs %q", a, b)
	}
}

func TestRateLimiterIsolatesKeys(t *testing.T) {
	rl := NewRateLimiter()
	for i := 0; i < rateLimitGraceFailures+3; i++ {
		rl.RecordFailure("10.0.0.1")
	}
	if !rl.Allowed("10.0.0.2") {
		t.Fatal("a different client IP must not be affected by another IP's failures")
	}
}

func TestRateLimiterBackoffGrows(t *testing.T) {
	rl := NewRateLimiter()
	key := "11.11.11.11"
	for i := 0; i < rateLimitGraceFailures+1; i++ {
		rl.RecordFailure(key)
	}
	rl.mu.Lock()
	firstLockout := rl.state[key].lockedUntil
	rl.mu.Unlock()

	rl.RecordFailure(key)
	rl.mu.Lock()
	secondLockout := rl.state[key].lockedUntil
	rl.mu.Unlock()

	if !secondLockout.After(firstLockout) {
		t.Fatalf("expected backoff to grow with repeated failures: first=%v second=%v", firstLockout, secondLockout)
	}
}

func TestClientKeyIgnoresProxyHeaderByDefault(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.5")
	if got := clientKey(r, ""); got != "192.0.2.1" {
		t.Fatalf("expected the raw connection address when no trusted header is configured, got %q", got)
	}
}

func TestClientKeyUsesTrustedProxyHeaderWhenConfigured(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.99, 203.0.113.5")
	if got := clientKey(r, "X-Forwarded-For"); got != "203.0.113.5" {
		t.Fatalf("expected the rightmost (proxy-appended) address, not the client-controlled leftmost, got %q", got)
	}
}

func TestClientKeyFallsBackWhenTrustedHeaderMissing(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	if got := clientKey(r, "X-Forwarded-For"); got != "192.0.2.1" {
		t.Fatalf("expected fallback to RemoteAddr when the configured header is absent, got %q", got)
	}
}

// Regression coverage for a real gap: without a trusted proxy header
// configured, every client behind a shared reverse proxy gets the same
// rate-limit bucket (same RemoteAddr), so one user's failed logins can
// lock out everyone else behind that proxy.
func TestTrustedProxyHeaderIsolatesClientsBehindSharedProxy(t *testing.T) {
	g := Guard{Username: "demo", Password: "demo", TrustedProxyHeader: "X-Forwarded-For"}
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))

	requestFrom := func(realClientIP, user, pass string) *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/overview", nil)
		r.RemoteAddr = "10.0.0.1:443" // the shared reverse proxy's own address
		r.Header.Set("X-Forwarded-For", realClientIP)
		r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
		return r
	}

	// Client A exhausts its grace failures and gets locked out.
	for i := 0; i < rateLimitGraceFailures+1; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, requestFrom("203.0.113.1", "demo", "wrongpass"))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, requestFrom("203.0.113.1", "demo", "demo"))
	if w.Code != 429 {
		t.Fatalf("expected client A to be locked out, got %d", w.Code)
	}

	// Client B, behind the same proxy (same RemoteAddr) but a different
	// forwarded IP, must be unaffected.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, requestFrom("203.0.113.2", "demo", "demo"))
	if w.Code != 204 {
		t.Fatalf("expected client B (different forwarded IP) to be unaffected, got %d", w.Code)
	}
}

func TestGuardRateLimitsRepeatedFailedLogins(t *testing.T) {
	g := Guard{Username: "demo", Password: "demo"}
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))

	basicAuthRequest := func(user, pass string) *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/overview", nil)
		r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
		return r
	}

	// Use up the grace failures.
	for i := 0; i < rateLimitGraceFailures+1; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, basicAuthRequest("demo", "wrongpass"))
		if w.Code != 401 {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}

	// Now even the *correct* password is rejected with 429 — the client IP
	// itself is locked out, before credentials are even checked.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, basicAuthRequest("demo", "demo"))
	if w.Code != 429 {
		t.Fatalf("expected 429 after repeated failures even with correct credentials, got %d", w.Code)
	}
}

// The Retry-After header used to be hardcoded to 60 regardless of the
// actual computed backoff, which ranges from 1 second (first offense past
// the grace count) up to 5 minutes. A client that honestly waits the
// stated 60 seconds on a 1-second lockout is misled into an unnecessarily
// long wait.
func TestGuardRateLimitRetryAfterReflectsActualBackoff(t *testing.T) {
	g := Guard{Username: "demo", Password: "demo"}
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))

	basicAuthRequest := func(user, pass string) *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/overview", nil)
		r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
		return r
	}

	// One failure past the grace count triggers the shortest possible
	// backoff: 1<<0 = 1 second.
	for i := 0; i < rateLimitGraceFailures+1; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, basicAuthRequest("demo", "wrongpass"))
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, basicAuthRequest("demo", "demo"))
	if w.Code != 429 {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("expected Retry-After: 1 for the first lockout, got %q", got)
	}
}

func TestGuardRateLimitResetsOnSuccess(t *testing.T) {
	g := Guard{Username: "demo", Password: "demo"}
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	basicAuthRequest := func(user, pass string) *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/overview", nil)
		r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
		return r
	}
	for i := 0; i < rateLimitGraceFailures-1; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, basicAuthRequest("demo", "wrongpass"))
		if w.Code != 401 {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, basicAuthRequest("demo", "demo"))
	if w.Code != 204 {
		t.Fatalf("expected successful login within grace window, got %d", w.Code)
	}
	for i := 0; i < rateLimitGraceFailures; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, basicAuthRequest("demo", "wrongpass"))
		if w.Code != 401 {
			t.Fatalf("post-reset attempt %d: expected 401 (not yet locked out), got %d", i+1, w.Code)
		}
	}
}

type recordingAuditLogger struct {
	events []auditEvent
}

type auditEvent struct {
	action, target, detail, user string
}

func (r *recordingAuditLogger) Record(action, target, detail, user string) {
	r.events = append(r.events, auditEvent{action, target, detail, user})
}

func TestGuardAuditsFailedLoginsAndLockouts(t *testing.T) {
	audit := &recordingAuditLogger{}
	g := Guard{Username: "demo", Password: "demo", Audit: audit}
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))

	basicAuthRequest := func(user, pass string) *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/overview", nil)
		r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
		return r
	}

	for i := 0; i < rateLimitGraceFailures+1; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, basicAuthRequest("demo", "wrongpass"))
	}
	// One more request should now be rate-limited rather than re-checking credentials.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, basicAuthRequest("demo", "demo"))
	if w.Code != 429 {
		t.Fatalf("expected 429, got %d", w.Code)
	}

	failedCount, lockedCount := 0, 0
	for _, e := range audit.events {
		switch e.action {
		case "auth.login_failed":
			failedCount++
			if e.user != "demo" {
				t.Errorf("expected audited username 'demo', got %q", e.user)
			}
		case "auth.rate_limited":
			lockedCount++
		}
	}
	if failedCount != rateLimitGraceFailures+1 {
		t.Fatalf("expected %d login_failed audit events, got %d", rateLimitGraceFailures+1, failedCount)
	}
	if lockedCount != 1 {
		t.Fatalf("expected 1 rate_limited audit event, got %d", lockedCount)
	}

	// A successful login must never be audited as a failure/lockout.
	audit.events = nil
	w = httptest.NewRecorder()
	h2 := Guard{Username: "demo", Password: "demo", Audit: audit}.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	h2.ServeHTTP(w, basicAuthRequest("demo", "demo"))
	if len(audit.events) != 0 {
		t.Fatalf("expected no audit events for a successful login, got %+v", audit.events)
	}
}
