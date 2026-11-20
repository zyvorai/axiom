package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Browser WebSocket and EventSource constructors cannot set an Authorization
// header, so historically Axiom accepted credentials in the URL
// (?access_token=, ?axiom_session=, ?api_key=) — which reverse-proxy access
// logs capture. A ticket replaces that: the already-authenticated SPA calls
// POST /api/v1/ws-ticket for the exact stream path it is about to open and gets
// back a random, single-use, short-lived token to put in `?ticket=` instead.
//
// A ticket is deliberately weak currency: it is bound to one GET path, it
// carries the caller's resolved Identity (so every role / namespace policy check
// still runs when it is redeemed), it expires in 30 seconds and it is destroyed
// the first time anyone presents it — even with the wrong path. Only the SHA-256
// of the token is kept in memory.

const (
	// TicketParam is the query parameter that carries a ticket.
	TicketParam = "ticket"
	// TicketTTL is how long an unused ticket stays redeemable.
	TicketTTL = 30 * time.Second

	ticketMaxOutstanding = 4096 // hard cap on live tickets (memory bound)
	ticketMaxPerIdentity = 64   // one noisy caller cannot exhaust the table
)

// ErrTicketTableFull is returned by Issue when the caller (or the whole server)
// already holds the maximum number of live tickets.
var ErrTicketTableFull = errors.New("too many outstanding tickets; retry shortly")

type ticketEntry struct {
	ident Identity
	path  string
	exp   time.Time
}

// TicketStore issues and redeems single-use stream tickets. It is safe for
// concurrent use. The clock is injectable for tests.
type TicketStore struct {
	mu      sync.Mutex
	entries map[string]ticketEntry // sha256(token) -> entry
	now     func() time.Time
	ttl     time.Duration
	shared  *SharedTicketBacking // optional multi-replica backing
}

// NewTicketStore returns a store using the wall clock and TicketTTL.
func NewTicketStore() *TicketStore { return NewTicketStoreWithClock(time.Now, TicketTTL) }

// NewTicketStoreWithClock is NewTicketStore with an injected clock and TTL.
func NewTicketStoreWithClock(now func() time.Time, ttl time.Duration) *TicketStore {
	return &TicketStore{entries: map[string]ticketEntry{}, now: now, ttl: ttl}
}

func ticketDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ticketablePath reports whether a ticket may be minted for path: only the
// streaming GET routes browsers cannot authenticate with headers. The check is
// shape-based and strict (cleaned, no query/fragment/traversal, no empty
// segments) so a ticket can never be bound to an unintended route. Workstreams
// that add further WebSocket/SSE routes extend this switch.
func ticketablePath(path string) bool {
	if !strings.HasPrefix(path, "/api/v1/") || strings.ContainsAny(path, "?#\\") ||
		strings.Contains(path, "//") || strings.Contains(path, "..") || strings.HasSuffix(path, "/") {
		return false
	}
	if path == "/api/v1/events/stream" {
		return true
	}
	segs := strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/")
	for _, s := range segs {
		if s == "" || s == "." {
			return false
		}
	}
	switch {
	case len(segs) == 4 && segs[0] == "pods":
		return segs[3] == "terminal" || segs[3] == "logs"
	case len(segs) == 4 && segs[0] == "vms":
		return segs[3] == "vnc" || segs[3] == "console"
	}
	return false
}

// TicketablePath is the exported form of ticketablePath for the API handler.
func TicketablePath(path string) bool { return ticketablePath(path) }

// Issue mints a ticket for id, redeemable once, for GET path, until now+TTL.
func (s *TicketStore) Issue(id Identity, path string) (string, time.Time, error) {
	if !ticketablePath(path) {
		return "", time.Time{}, errors.New("path is not eligible for a stream ticket")
	}
	if id.User == "" {
		return "", time.Time{}, errors.New("no authenticated identity")
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) >= ticketMaxOutstanding/2 {
		s.sweepLocked(now)
	}
	if len(s.entries) >= ticketMaxOutstanding {
		return "", time.Time{}, ErrTicketTableFull
	}
	count := func() int {
		n := 0
		for _, e := range s.entries {
			if e.ident.User == id.User {
				n++
			}
		}
		return n
	}
	if count() >= ticketMaxPerIdentity {
		s.sweepLocked(now)
		if count() >= ticketMaxPerIdentity {
			return "", time.Time{}, ErrTicketTableFull
		}
	}
	token := RandomURLToken(32)
	exp := now.Add(s.ttl)
	dig := ticketDigest(token)
	entry := ticketEntry{ident: id, path: path, exp: exp}
	s.entries[dig] = entry
	if s.shared != nil {
		s.shared.put(dig, entry)
	}
	return token, exp, nil
}

// Consume redeems a ticket for a request to path. The ticket is destroyed
// whether or not it matches, so a leaked-but-misused ticket cannot be retried
// against the right path. It returns the Identity the ticket was issued to.
func (s *TicketStore) Consume(token, path string) (Identity, bool) {
	if token == "" {
		return Identity{}, false
	}
	key := ticketDigest(token)
	now := s.now()
	s.mu.Lock()
	e, ok := s.entries[key]
	if ok {
		delete(s.entries, key)
	}
	shared := s.shared
	s.mu.Unlock()
	if !ok && shared != nil {
		e, ok = shared.take(key)
	}
	if !ok || !now.Before(e.exp) || e.path != path {
		return Identity{}, false
	}
	return e.ident, true
}

// Len reports the number of live (not necessarily unexpired) tickets; tests only.
func (s *TicketStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *TicketStore) sweepLocked(now time.Time) {
	for k, e := range s.entries {
		if !now.Before(e.exp) {
			delete(s.entries, k)
		}
	}
}

// isTicketIssueRequest is POST /api/v1/ws-ticket. It is a "write" by method but
// harmless by itself (it only mints a credential for the caller's own identity),
// so read-only viewers and namespace-restricted accounts — who also use
// terminals/streams within their rights — must be able to call it. Whatever a
// ticket is later used for is still checked against the issuing identity.
func isTicketIssueRequest(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == "/api/v1/ws-ticket"
}

// usesURLCredentials reports whether the request carried credentials in the
// query string, the deprecated path tickets replace.
func usesURLCredentials(r *http.Request) bool {
	// The pod HTTP proxy is navigated to directly and loads many sub-resources,
	// so a single-use ticket cannot cover it; its URL token is still the
	// sanctioned form (documented in docs/GAPS.md) and is not flagged.
	if strings.HasPrefix(r.URL.Path, "/api/v1/pods/") && strings.Contains(r.URL.Path, "/window") {
		return false
	}
	q := r.URL.Query()
	return q.Get("access_token") != "" || q.Get("api_key") != "" || q.Get("axiom_session") != ""
}

var warnURLCredsOnce sync.Once

func noteDeprecatedURLCredentials(w http.ResponseWriter) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("X-Axiom-Deprecated", "credentials in the URL are deprecated; request a single-use ticket via POST /api/v1/ws-ticket")
	warnURLCredsOnce.Do(func() {
		log.Printf("auth: a client authenticated with credentials in the URL (?access_token=/?api_key=/?axiom_session=); these end up in proxy access logs — use POST /api/v1/ws-ticket and ?ticket= instead")
	})
}
