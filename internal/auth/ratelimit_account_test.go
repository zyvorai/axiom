package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func acctGuard(clock *fakeClock) (http.Handler, *recordingAuditLogger) {
	audit := &recordingAuditLogger{}
	users := &fakeUserAuthenticator{users: map[string]struct {
		password   string
		role       string
		namespaces []string
	}{
		"attacker": {password: "mine", role: "viewer"},
		"victim":   {password: "s3cret", role: "admin"},
	}}
	g := Guard{
		Username: "demo", Password: "demo", Users: users, Audit: audit,
		Limiter:            NewRateLimiterWithClock(clock.now),
		AccountLimiter:     NewAccountRateLimiter(clock.now),
		TrustedProxyHeader: "X-Forwarded-For",
	}
	return g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })), audit
}

func reqFrom(ip, user, pass string) *http.Request {
	r := httptest.NewRequest("GET", "/api/v1/overview", nil)
	r.Header.Set("X-Forwarded-For", ip)
	basicHdr(r, user, pass)
	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// The documented bypass: a caller with any valid account interleaves a valid
// request so the per-IP counter never trips. Guesses at *another* account must
// still lock that account.
func TestAccountThrottleDefeatsInterleavedSuccess(t *testing.T) {
	clock := newFakeClock()
	h, _ := acctGuard(clock)
	locked := false
	for i := 0; i < 40 && !locked; i++ {
		if w := serve(h, reqFrom("203.0.113.9", "attacker", "mine")); w.Code != 204 {
			t.Fatalf("attacker's own login must keep working while probing, got %d", w.Code)
		}
		w := serve(h, reqFrom("203.0.113.9", "victim", fmt.Sprintf("guess-%d", i)))
		if w.Code == 429 {
			locked = true
		}
	}
	if !locked {
		t.Fatal("interleaving a valid login must not let the attacker guess the victim's password forever")
	}
	// Even the right password is refused during the lockout (fail closed).
	if w := serve(h, reqFrom("203.0.113.9", "victim", "s3cret")); w.Code != 429 {
		t.Fatalf("locked account must refuse even correct credentials, got %d", w.Code)
	}
}

// Rotating source IPs defeats the per-IP limiter by design; the per-account
// counter must still accumulate across them.
func TestAccountThrottleAcrossRotatingIPs(t *testing.T) {
	clock := newFakeClock()
	h, audit := acctGuard(clock)
	for i := 0; i <= accountGraceFailures; i++ { // grace failures are free; the next one locks
		ip := fmt.Sprintf("198.51.100.%d", i+1)
		if w := serve(h, reqFrom(ip, "victim", "wrong")); w.Code != 401 {
			t.Fatalf("attempt %d from %s: got %d, want 401", i, ip, w.Code)
		}
	}
	// A fresh IP that has never failed is still refused because of the account.
	if w := serve(h, reqFrom("192.0.2.200", "victim", "s3cret")); w.Code != 429 {
		t.Fatalf("expected the account lock to hold across IPs, got %d", w.Code)
	}
	found := false
	for _, e := range audit.events {
		if e.action == "auth.account_rate_limited" {
			found = true
		}
	}
	if !found {
		t.Fatal("account lockouts must be audited")
	}
	// The attacker's own, different account is unaffected.
	if w := serve(h, reqFrom("192.0.2.201", "attacker", "mine")); w.Code != 204 {
		t.Fatalf("other accounts must be unaffected, got %d", w.Code)
	}
}

func TestAccountThrottleRecoversAfterBackoffWithInjectedClock(t *testing.T) {
	clock := newFakeClock()
	h, _ := acctGuard(clock)
	for i := 0; i <= accountGraceFailures; i++ {
		serve(h, reqFrom(fmt.Sprintf("10.1.0.%d", i+1), "victim", "wrong"))
	}
	w := serve(h, reqFrom("10.2.0.1", "victim", "s3cret"))
	if w.Code != 429 || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("first lockout: code=%d retry-after=%q", w.Code, w.Header().Get("Retry-After"))
	}
	clock.advance(1100 * time.Millisecond)
	if w := serve(h, reqFrom("10.2.0.2", "victim", "s3cret")); w.Code != 204 {
		t.Fatalf("after the backoff elapses the right password must work, got %d", w.Code)
	}
	// Success clears the account's history.
	for i := 0; i < accountGraceFailures; i++ {
		if w := serve(h, reqFrom(fmt.Sprintf("10.3.0.%d", i+1), "victim", "wrong")); w.Code != 401 {
			t.Fatalf("post-success attempt %d: got %d, want 401", i, w.Code)
		}
	}
}

func TestAccountThrottleIsCaseAndAliasInsensitive(t *testing.T) {
	if accountKey("Victim") != accountKey(" victim ") {
		t.Fatal("account keys must be case/space-insensitive")
	}
	if accountKey("demo@zyvor.dev") != accountKey("demo") {
		t.Fatal("the demo alias must share one bucket")
	}
	if accountKey("") != "" {
		t.Fatal("empty username must not map to a key")
	}
}

// A request that presents a valid session/API key alongside a Basic header for
// someone else's account must not wipe that account's failure history.
func TestAccountCounterNotResetByUnrelatedSuccess(t *testing.T) {
	clock := newFakeClock()
	h, _ := acctGuard(clock)
	for i := 0; i <= accountGraceFailures; i++ {
		serve(h, reqFrom(fmt.Sprintf("10.9.0.%d", i+1), "victim", "wrong"))
	}
	// attacker authenticates as themself but names the victim in the Basic
	// header of a *different* request path: only creds verified for "victim"
	// may clear victim's counter, so this must not.
	if w := serve(h, reqFrom("10.9.1.1", "attacker", "mine")); w.Code != 204 {
		t.Fatalf("got %d", w.Code)
	}
	if w := serve(h, reqFrom("10.9.1.2", "victim", "wrong")); w.Code != 429 {
		t.Fatalf("victim's counter must survive the attacker's own success, got %d", w.Code)
	}
}

func TestAccountLimiterBoundedAndLocksSurviveKeyFlood(t *testing.T) {
	clock := newFakeClock()
	rl := NewAccountRateLimiter(clock.now)
	// Lock the victim.
	for i := 0; i <= accountGraceFailures; i++ {
		rl.RecordFailure(accountKey("victim"))
	}
	if rl.Allowed(accountKey("victim")) {
		t.Fatal("victim should be locked")
	}
	// Flood with unique throwaway usernames to try to evict the lock. The clock
	// is frozen, so the victim stays locked throughout.
	for i := 0; i < rateLimitMaxKeys+2000; i++ {
		rl.RecordFailure(accountKey(fmt.Sprintf("noise-%d", i)))
	}
	if n := len(rl.state); n > rateLimitMaxKeys {
		t.Fatalf("limiter table unbounded: %d entries", n)
	}
	rl.mu.Lock()
	_, present := rl.state[accountKey("victim")]
	rl.mu.Unlock()
	if !present || rl.Allowed(accountKey("victim")) {
		t.Fatal("a locked account entry must not be evicted by a flood of one-off keys")
	}
}

func TestAccountKeyLengthIsBounded(t *testing.T) {
	long := make([]byte, 100000)
	for i := range long {
		long[i] = 'a'
	}
	if k := accountKey(string(long)); len(k) > accountKeyMaxLen+32 {
		t.Fatalf("account key not bounded: %d bytes", len(k))
	}
	a := make([]byte, 5000)
	b := make([]byte, 5000)
	for i := range a {
		a[i], b[i] = 'a', 'a'
	}
	b[4999] = 'b'
	if accountKey(string(a)) == accountKey(string(b)) {
		t.Fatal("distinct long names must not collide")
	}
}
