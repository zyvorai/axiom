package auth

import (
	"encoding/base64"
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

func TestGuardRateLimitResetsOnSuccess(t *testing.T) {
	g := Guard{Username: "demo", Password: "demo"}
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))

	basicAuthRequest := func(user, pass string) *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/overview", nil)
		r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
		return r
	}

	// A couple of failures, within the grace window, followed by a success.
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

	// Failure count should have reset — another full grace window of
	// failures should be tolerated before lockout.
	for i := 0; i < rateLimitGraceFailures; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, basicAuthRequest("demo", "wrongpass"))
		if w.Code != 401 {
			t.Fatalf("post-reset attempt %d: expected 401 (not yet locked out), got %d", i+1, w.Code)
		}
	}
}
