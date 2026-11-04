package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

// The pod HTTP proxy can be served from a separate origin (AXIOM_POD_PROXY_ADDR
// / AXIOM_POD_PROXY_ORIGIN, see internal/api/podproxy_origin.go). That origin
// must never accept the console's credentials — Basic, Bearer, API keys or the
// axiom_session cookie, which browsers also send to other ports of the same
// host — so it has its own two-step credential:
//
//  1. a pod-proxy ticket, minted by the authenticated console
//     (POST /api/v1/pods/{ns}/{name}/window-link) and put in the link the
//     browser opens: random 256-bit, single use, 60 s, bound to the user, the
//     pod, the port and the base path;
//  2. a pod-proxy session, created when the ticket is redeemed on the proxy
//     origin: a public random id that goes in the URL path (/s/<id>/) plus a
//     separate random secret that goes in an HttpOnly cookie on the proxy
//     origin, bound to the user, the pod and the port; 10 minutes sliding, one
//     hour at most.
//
// In per-session-host mode the session id is chosen when the ticket is minted
// (NewPodProxySessionID), carried in the ticket (PodProxyGrant.SessionID) and
// put in the link's hostname instead of its path; redeeming creates the session
// under exactly that id (CreateWithID).
//
// Only SHA-256 digests of tickets, session ids and session secrets are kept.
// These stores are separate from the stream TicketStore on purpose: stream
// tickets keep their 30 s TTL and strict path binding.

const (
	// PodProxyTicketTTL is how long an unused pod-proxy ticket stays redeemable.
	PodProxyTicketTTL = 60 * time.Second
	// PodProxySessionIdle is the sliding lifetime of a pod-proxy session.
	PodProxySessionIdle = 10 * time.Minute
	// PodProxySessionMax is the absolute lifetime of a pod-proxy session.
	PodProxySessionMax = time.Hour

	podProxyTicketsMax        = 4096
	podProxyTicketsPerUser    = 16
	podProxySessionsMax       = 4096
	podProxySessionsPerUser   = 32
	podProxyMaxBasePathLength = 2048
)

// ErrPodProxyTableFull is returned when the caller (or the server) holds the
// maximum number of live pod-proxy tickets or sessions.
var ErrPodProxyTableFull = errors.New("too many outstanding pod proxy links; retry shortly")

// PodProxyGrant is what a pod-proxy ticket and the session it becomes are
// bound to.
type PodProxyGrant struct {
	User string
	// Role is a snapshot of the creating identity's role.
	Role      string
	Namespace string
	Pod       string
	Port      int32
	// Path is the base path the link opens (ticket only; always starts "/").
	Path string
	// ClusterKey is the console's cluster-session key at link time, so the
	// proxied pod is looked up on the cluster the operator was looking at.
	ClusterKey string
	// IssuedMs is when the console credential minted the ticket (Unix ms). A
	// per-user revocation cut-off at or after this moment kills the ticket and
	// every session made from it.
	IssuedMs int64
	// SessionID, set only in per-session-host mode, is the session id the
	// ticket was minted for: the leftmost label of the host the link points
	// at. The ticket may only be redeemed on that host, and the session it
	// becomes gets exactly this id (PodProxySessionStore.CreateWithID).
	SessionID string
}

func (g PodProxyGrant) valid() bool {
	return g.User != "" && g.Namespace != "" && g.Pod != "" && g.Port > 0 && g.Port <= 65535 && ValidPodProxyBasePath(g.Path) &&
		(g.SessionID == "" || ValidPodProxySessionID(g.SessionID))
}

// podProxySessionIDLen is the length of a per-session-host session id: 128
// random bits in lowercase hex, a valid DNS label.
const podProxySessionIDLen = 32

// NewPodProxySessionID returns a fresh per-session-host session id (128-bit
// random, 32 lowercase hex characters), used as the leftmost host label.
func NewPodProxySessionID() string {
	b := make([]byte, podProxySessionIDLen/2)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// ValidPodProxySessionID reports whether id has the shape NewPodProxySessionID
// produces: exactly 32 lowercase hex characters.
func ValidPodProxySessionID(id string) bool {
	if len(id) != podProxySessionIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// ValidPodProxyBasePath reports whether p may be used as the base path of a
// pod-proxy link: an absolute path (optionally with a query), never a
// scheme-relative "//host", no backslashes and no control characters.
func ValidPodProxyBasePath(p string) bool {
	if p == "" || p[0] != '/' || strings.HasPrefix(p, "//") || len(p) > podProxyMaxBasePathLength {
		return false
	}
	if strings.ContainsAny(p, "\\#") {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] < 0x20 || p[i] == 0x7f {
			return false
		}
	}
	return true
}

type podProxyTicket struct {
	grant PodProxyGrant
	exp   time.Time
}

// PodProxyTicketStore issues and redeems single-use pod-proxy tickets. Safe
// for concurrent use.
type PodProxyTicketStore struct {
	mu      sync.Mutex
	entries map[string]podProxyTicket // sha256(token) -> ticket
	now     func() time.Time
	ttl     time.Duration
}

// NewPodProxyTicketStore returns a store using the wall clock and PodProxyTicketTTL.
func NewPodProxyTicketStore() *PodProxyTicketStore {
	return NewPodProxyTicketStoreWithClock(time.Now, PodProxyTicketTTL)
}

// NewPodProxyTicketStoreWithClock is NewPodProxyTicketStore with an injected
// clock and TTL.
func NewPodProxyTicketStoreWithClock(now func() time.Time, ttl time.Duration) *PodProxyTicketStore {
	return &PodProxyTicketStore{entries: map[string]podProxyTicket{}, now: now, ttl: ttl}
}

// Issue mints a ticket for g. A zero IssuedMs is set to now.
func (s *PodProxyTicketStore) Issue(g PodProxyGrant) (string, time.Time, error) {
	if !g.valid() {
		return "", time.Time{}, errors.New("invalid pod proxy grant")
	}
	now := s.now()
	if g.IssuedMs == 0 {
		g.IssuedMs = now.UnixMilli()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) >= podProxyTicketsMax/2 {
		s.sweepLocked(now)
	}
	if len(s.entries) >= podProxyTicketsMax {
		return "", time.Time{}, ErrPodProxyTableFull
	}
	if s.countLocked(g.User) >= podProxyTicketsPerUser {
		s.sweepLocked(now)
		if s.countLocked(g.User) >= podProxyTicketsPerUser {
			return "", time.Time{}, ErrPodProxyTableFull
		}
	}
	token := RandomURLToken(32)
	exp := now.Add(s.ttl)
	s.entries[ticketDigest(token)] = podProxyTicket{grant: g, exp: exp}
	return token, exp, nil
}

// Consume redeems a ticket once. The ticket is destroyed whatever the outcome.
func (s *PodProxyTicketStore) Consume(token string) (PodProxyGrant, bool) {
	if token == "" {
		return PodProxyGrant{}, false
	}
	key := ticketDigest(token)
	now := s.now()
	s.mu.Lock()
	e, ok := s.entries[key]
	if ok {
		delete(s.entries, key)
	}
	s.mu.Unlock()
	if !ok || !now.Before(e.exp) {
		return PodProxyGrant{}, false
	}
	return e.grant, true
}

// Len reports the number of stored tickets; tests only.
func (s *PodProxyTicketStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *PodProxyTicketStore) countLocked(user string) int {
	n := 0
	for _, e := range s.entries {
		if e.grant.User == user {
			n++
		}
	}
	return n
}

func (s *PodProxyTicketStore) sweepLocked(now time.Time) {
	for k, e := range s.entries {
		if !now.Before(e.exp) {
			delete(s.entries, k)
		}
	}
}

// hasKey reports whether the store holds k as a map key; tests use it to
// prove only digests (never raw tokens) are stored.
func (s *PodProxyTicketStore) hasKey(k string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.entries[k]
	return ok
}

type podProxySession struct {
	secret   string // sha256(cookie secret)
	grant    PodProxyGrant
	created  time.Time
	lastSeen time.Time
}

// PodProxySessionStore holds live pod-proxy sessions. Safe for concurrent use.
type PodProxySessionStore struct {
	mu      sync.Mutex
	entries map[string]*podProxySession // sha256(session id) -> session
	now     func() time.Time
	idle    time.Duration
	max     time.Duration
}

// NewPodProxySessionStore returns a store using the wall clock,
// PodProxySessionIdle and PodProxySessionMax.
func NewPodProxySessionStore() *PodProxySessionStore {
	return NewPodProxySessionStoreWithClock(time.Now, PodProxySessionIdle, PodProxySessionMax)
}

// NewPodProxySessionStoreWithClock is NewPodProxySessionStore with an injected
// clock and lifetimes.
func NewPodProxySessionStoreWithClock(now func() time.Time, idle, max time.Duration) *PodProxySessionStore {
	return &PodProxySessionStore{entries: map[string]*podProxySession{}, now: now, idle: idle, max: max}
}

// Create starts a session for g and returns its public id (for the URL path)
// and its secret (for the cookie).
func (s *PodProxySessionStore) Create(g PodProxyGrant) (id, secret string, err error) {
	return s.create("", g)
}

// CreateWithID starts a session for g under a caller-chosen public id (the
// per-session-host mode, where the id is put in the link's host before the
// ticket is redeemed) and returns its cookie secret. id must be a
// ValidPodProxySessionID equal to g.SessionID and must not name a live
// session.
func (s *PodProxySessionStore) CreateWithID(id string, g PodProxyGrant) (secret string, err error) {
	if !ValidPodProxySessionID(id) || g.SessionID != id {
		return "", errors.New("invalid pod proxy session id")
	}
	_, secret, err = s.create(id, g)
	return secret, err
}

func (s *PodProxySessionStore) create(fixedID string, g PodProxyGrant) (id, secret string, err error) {
	if !g.valid() {
		return "", "", errors.New("invalid pod proxy grant")
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if fixedID != "" {
		if e, ok := s.entries[ticketDigest(fixedID)]; ok {
			if !s.expiredLocked(e, now) {
				return "", "", errors.New("pod proxy session id already in use")
			}
			delete(s.entries, ticketDigest(fixedID))
		}
	}
	if len(s.entries) >= podProxySessionsMax/2 {
		s.sweepLocked(now)
	}
	if len(s.entries) >= podProxySessionsMax {
		return "", "", ErrPodProxyTableFull
	}
	if s.countLocked(g.User) >= podProxySessionsPerUser {
		s.sweepLocked(now)
		if s.countLocked(g.User) >= podProxySessionsPerUser {
			return "", "", ErrPodProxyTableFull
		}
	}
	id, secret = fixedID, RandomURLToken(32)
	if id == "" {
		id = RandomURLToken(32)
	}
	s.entries[ticketDigest(id)] = &podProxySession{secret: ticketDigest(secret), grant: g, created: now, lastSeen: now}
	return id, secret, nil
}

// Lookup returns the grant of the live session id whose cookie secret is
// secret, and slides its idle expiry. An expired session is removed.
func (s *PodProxySessionStore) Lookup(id, secret string) (PodProxyGrant, bool) {
	if id == "" || secret == "" {
		return PodProxyGrant{}, false
	}
	key := ticketDigest(id)
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok {
		return PodProxyGrant{}, false
	}
	if s.expiredLocked(e, now) {
		delete(s.entries, key)
		return PodProxyGrant{}, false
	}
	if subtle.ConstantTimeCompare([]byte(e.secret), []byte(ticketDigest(secret))) != 1 {
		return PodProxyGrant{}, false
	}
	e.lastSeen = now
	return e.grant, true
}

// Live reports whether id names a session that has not expired, without
// sliding it or checking any secret. An expired session is removed.
func (s *PodProxySessionStore) Live(id string) bool {
	if id == "" {
		return false
	}
	key := ticketDigest(id)
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok {
		return false
	}
	if s.expiredLocked(e, now) {
		delete(s.entries, key)
		return false
	}
	return true
}

// Delete ends session id.
func (s *PodProxySessionStore) Delete(id string) {
	s.mu.Lock()
	delete(s.entries, ticketDigest(id))
	s.mu.Unlock()
}

// Len reports the number of stored sessions; tests only.
func (s *PodProxySessionStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *PodProxySessionStore) expiredLocked(e *podProxySession, now time.Time) bool {
	return !now.Before(e.lastSeen.Add(s.idle)) || !now.Before(e.created.Add(s.max))
}

func (s *PodProxySessionStore) countLocked(user string) int {
	n := 0
	for _, e := range s.entries {
		if e.grant.User == user {
			n++
		}
	}
	return n
}

func (s *PodProxySessionStore) sweepLocked(now time.Time) {
	for k, e := range s.entries {
		if s.expiredLocked(e, now) {
			delete(s.entries, k)
		}
	}
}

func (s *PodProxySessionStore) hasKey(k string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.entries[k]
	return ok
}
