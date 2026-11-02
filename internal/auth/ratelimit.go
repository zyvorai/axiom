package auth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter applies exponential backoff per client IP after repeated
// failed authentication attempts, so a login endpoint isn't wide open to
// unlimited-speed password guessing. It only ever blocks based on a
// request's own IP — it never inspects which username was tried, so it
// can't be used to enumerate valid accounts by timing.
type RateLimiter struct {
	mu    sync.Mutex
	state map[string]*attemptState
	now   func() time.Time // injectable for tests
}

type attemptState struct {
	failures    int
	lockedUntil time.Time
	lastSeen    time.Time
}

const (
	rateLimitGraceFailures = 3                // don't lock out the first few typos
	rateLimitMaxBackoff    = 5 * time.Minute  // cap on lockout duration
	rateLimitIdleEviction  = 30 * time.Minute // stale entries are forgotten
	rateLimitSweepEvery    = 500              // opportunistic cleanup cadence
	rateLimitMaxKeys       = 10000            // hard cap on tracked clients (memory bound)
)

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{state: map[string]*attemptState{}, now: time.Now}
}

// Allowed reports whether the given key (client IP) may attempt
// authentication right now.
func (rl *RateLimiter) Allowed(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	s := rl.state[key]
	if s == nil {
		return true
	}
	return rl.now().After(s.lockedUntil)
}

// RetryAfter returns how much longer the given key stays locked out, for
// callers reporting an HTTP Retry-After header. The actual lockout ranges
// from 1 second (first offense past the grace count) up to
// rateLimitMaxBackoff, so this must be computed per key rather than
// hardcoded — a client that respects a fixed value would either wait far
// longer than necessary on a short lockout, or retry too early against a
// long one.
func (rl *RateLimiter) RetryAfter(key string) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	s := rl.state[key]
	if s == nil {
		return 0
	}
	if d := s.lockedUntil.Sub(rl.now()); d > 0 {
		return d
	}
	return 0
}

// RecordFailure counts a failed authentication attempt and, once the grace
// count is exceeded, locks the key out with exponential backoff.
func (rl *RateLimiter) RecordFailure(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.sweepLocked()
	s := rl.state[key]
	if s == nil {
		if len(rl.state) >= rateLimitMaxKeys {
			rl.evictOldestLocked()
		}
		s = &attemptState{}
		rl.state[key] = s
	}
	s.failures++
	s.lastSeen = rl.now()
	if s.failures > rateLimitGraceFailures {
		shift := s.failures - rateLimitGraceFailures - 1
		if shift > 8 {
			shift = 8
		}
		backoff := time.Duration(1<<uint(shift)) * time.Second
		if backoff > rateLimitMaxBackoff {
			backoff = rateLimitMaxBackoff
		}
		s.lockedUntil = rl.now().Add(backoff)
	}
}

// RecordSuccess clears any failure history for the key.
//
// Known limitation: because success resets the counter, a caller who holds any
// valid account can interleave one valid request among password guesses and
// never trip the limiter. Closing that needs per-account throttling or MFA
// (see docs/GAPS.md); the per-IP limiter here is a speed bump, not a defence
// against a determined authenticated attacker.
func (rl *RateLimiter) RecordSuccess(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.state, key)
}

// sweepLocked evicts idle entries. Must be called with rl.mu held.
func (rl *RateLimiter) sweepLocked() {
	if len(rl.state) < rateLimitSweepEvery {
		return
	}
	cutoff := rl.now().Add(-rateLimitIdleEviction)
	for k, s := range rl.state {
		if s.lastSeen.Before(cutoff) {
			delete(rl.state, k)
		}
	}
}

// evictOldestLocked drops the least-recently-seen entry once the table is
// full, so a flood of spoofed/rotating keys cannot grow memory without bound.
func (rl *RateLimiter) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	first := true
	for k, s := range rl.state {
		if first || s.lastSeen.Before(oldest) {
			oldestKey, oldest, first = k, s.lastSeen, false
		}
	}
	if !first {
		delete(rl.state, oldestKey)
	}
}

// clientKey extracts the client IP from a request, ignoring any port.
//
// By default it does not trust X-Forwarded-For (spoofable by the client
// unless a trusted proxy strips/overwrites it). When trustedHeader is set
// (opt-in via AXIOM_TRUSTED_PROXY_HEADER — only safe behind a reverse proxy
// that sets the header), the RIGHTMOST address is used: that is the entry the
// trusted proxy itself appended. The leftmost entry is client-controlled when
// a proxy appends rather than overwrites, so trusting it would let an attacker
// rotate keys at will to evade the limiter and bloat its table. IPv6 clients
// are keyed by their /64 so a single host can't mint 2^64 keys.
func clientKey(r *http.Request, trustedHeader string) string {
	if trustedHeader != "" {
		if v := strings.TrimSpace(r.Header.Get(trustedHeader)); v != "" {
			parts := strings.Split(v, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return normalizeIPKey(ip)
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return normalizeIPKey(r.RemoteAddr)
	}
	return normalizeIPKey(host)
}

func normalizeIPKey(s string) string {
	ip := net.ParseIP(strings.Trim(s, "[]"))
	if ip == nil {
		return s
	}
	if ip.To4() == nil {
		return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
	}
	return ip.String()
}
