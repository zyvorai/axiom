package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter applies exponential backoff per key after repeated failed
// authentication attempts, so a login endpoint isn't wide open to
// unlimited-speed password guessing. The Guard runs two instances: one keyed
// by client IP (NewRateLimiter) and one keyed by the *attempted account name*
// (NewAccountRateLimiter), so an attacker who rotates source addresses — or who
// holds a valid account and interleaves successes from it to reset the per-IP
// counter — still cannot guess another account's password at speed. Account
// keys are recorded for unknown usernames too, so the 429 behaviour never
// reveals whether an account exists.
type RateLimiter struct {
	mu         sync.Mutex
	state      map[string]*attemptState
	now        func() time.Time // injectable for tests
	grace      int
	maxBackoff time.Duration
	shared     *SharedRateBacking // optional multi-replica backing
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

	// accountGraceFailures is more forgiving than the per-IP grace: the
	// account limiter is shared by every client that targets the same name, so
	// a handful of honest typos from several people must not lock the account.
	accountGraceFailures = 5
	accountKeyMaxLen     = 128 // longer attempted usernames are shortened into the key
)

func NewRateLimiter() *RateLimiter {
	return NewRateLimiterWithClock(time.Now)
}

// NewRateLimiterWithClock builds a per-IP limiter with an injected clock.
func NewRateLimiterWithClock(now func() time.Time) *RateLimiter {
	return &RateLimiter{state: map[string]*attemptState{}, now: now, grace: rateLimitGraceFailures, maxBackoff: rateLimitMaxBackoff}
}

// NewAccountRateLimiter builds the per-account limiter (see RateLimiter). The
// clock is injectable so tests can step through backoff windows.
func NewAccountRateLimiter(now func() time.Time) *RateLimiter {
	rl := NewRateLimiterWithClock(now)
	rl.grace = accountGraceFailures
	return rl
}

// accountKey normalises an attempted username into a limiter key. Empty means
// "no account involved" (API key, session, ticket), which is not throttled by
// name. The configured demo alias (demo@zyvor.dev == demo) is folded so
// alternating spellings cannot double the guess budget, and over-long names are
// shortened so an attacker cannot make each key arbitrarily large.
func accountKey(user string) string {
	u := strings.ToLower(strings.TrimSpace(user))
	if u == "" {
		return ""
	}
	if u == "demo@zyvor.dev" {
		u = "demo"
	}
	if len(u) > accountKeyMaxLen {
		sum := sha256.Sum256([]byte(u))
		u = u[:accountKeyMaxLen/2] + "#" + hex.EncodeToString(sum[:8])
	}
	return "acct:" + u
}

// Allowed reports whether the given key (client IP) may attempt
// authentication right now.
func (rl *RateLimiter) Allowed(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.pullSharedLocked()
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
	rl.pullSharedLocked()
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
	rl.pullSharedLocked()
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
	if s.failures > rl.grace {
		shift := s.failures - rl.grace - 1
		if shift > 8 {
			shift = 8
		}
		backoff := time.Duration(1<<uint(shift)) * time.Second
		if backoff > rl.maxBackoff {
			backoff = rl.maxBackoff
		}
		s.lockedUntil = rl.now().Add(backoff)
	}
	rl.pushSharedLocked(key)
}

// RecordSuccess clears any failure history for the key.
//
// For the per-IP limiter a success resets the counter (a speed bump against one
// client, not a defence on its own): a caller holding any valid account can
// interleave a valid request to keep its own IP clear. The per-account limiter
// closes that gap because the Guard only calls RecordSuccess on an account key
// when that account's own credentials were actually verified, so guesses at
// another account's name keep accumulating on that account's counter whatever
// the attacker's own account does. MFA remains open (docs/GAPS.md).
func (rl *RateLimiter) RecordSuccess(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.pullSharedLocked()
	delete(rl.state, key)
	rl.pushSharedLocked(key)
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
//
// Entries that are currently locked out are evicted only when every entry is
// locked, so an attacker cannot flush a victim's lockout by spamming unique
// throwaway keys (those never reach the lock threshold).
func (rl *RateLimiter) evictOldestLocked() {
	now := rl.now()
	var oldestKey string
	var oldest time.Time
	first := true
	pass := func(skipLocked bool) {
		for k, s := range rl.state {
			if skipLocked && s.lockedUntil.After(now) {
				continue
			}
			if first || s.lastSeen.Before(oldest) {
				oldestKey, oldest, first = k, s.lastSeen, false
			}
		}
	}
	pass(true)
	if first {
		pass(false)
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
