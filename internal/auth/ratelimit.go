package auth

import (
	"net"
	"net/http"
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
)

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{state: map[string]*attemptState{}}
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
	return time.Now().After(s.lockedUntil)
}

// RecordFailure counts a failed authentication attempt and, once the grace
// count is exceeded, locks the key out with exponential backoff.
func (rl *RateLimiter) RecordFailure(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.sweepLocked()
	s := rl.state[key]
	if s == nil {
		s = &attemptState{}
		rl.state[key] = s
	}
	s.failures++
	s.lastSeen = time.Now()
	if s.failures > rateLimitGraceFailures {
		shift := s.failures - rateLimitGraceFailures - 1
		if shift > 8 {
			shift = 8
		}
		backoff := time.Duration(1<<uint(shift)) * time.Second
		if backoff > rateLimitMaxBackoff {
			backoff = rateLimitMaxBackoff
		}
		s.lockedUntil = time.Now().Add(backoff)
	}
}

// RecordSuccess clears any failure history for the key.
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
	cutoff := time.Now().Add(-rateLimitIdleEviction)
	for k, s := range rl.state {
		if s.lastSeen.Before(cutoff) {
			delete(rl.state, k)
		}
	}
}

// clientKey extracts the client IP from a request, ignoring any port.
// Deliberately does not trust X-Forwarded-For (spoofable by the client
// unless a trusted proxy strips/overwrites it, which isn't guaranteed in
// this deployment model).
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
