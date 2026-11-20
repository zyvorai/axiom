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
//
// That is the in-memory form, where only the replica that minted a ticket can
// redeem it and only the replica that created a session can serve it. With a
// stable shared secret the stores are built shared instead, and tickets and
// session cookies become signed tokens any replica can check; see
// podproxy_shared.go.

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
	User string `json:"u"`
	// Role is a snapshot of the creating identity's role.
	Role string `json:"r"`
	// Namespaces is a snapshot of the creating identity's namespace
	// restriction (empty: unrestricted). Namespace must be one of them.
	Namespaces []string `json:"nss,omitempty"`
	Namespace  string   `json:"ns"`
	Pod        string   `json:"pod"`
	Port       int32    `json:"port"`
	// Path is the base path the link opens (ticket only; always starts "/").
	Path string `json:"path"`
	// Cluster is the registered name of the cluster the pod was resolved on
	// when the link was minted (platform.SwitchableProvider; empty on a
	// single-cluster provider). Every proxied request looks the pod up on
	// exactly this cluster, whatever any console tab or replica has selected
	// since, and is refused when it is no longer registered.
	Cluster string `json:"cl,omitempty"`
	// IssuedMs is when the console credential minted the ticket (Unix ms). A
	// per-user revocation cut-off at or after this moment kills the ticket and
	// every session made from it.
	IssuedMs int64 `json:"iat"`
	// SessionID, set only in per-session-host mode, is the session id the
	// ticket was minted for: the leftmost label of the host the link points
	// at. The ticket may only be redeemed on that host, and the session it
	// becomes gets exactly this id (PodProxySessionStore.CreateWithID).
	SessionID string `json:"sid,omitempty"`
}

// NamespaceAllowed reports whether the grant's namespace restriction (if any)
// covers its namespace.
func (g PodProxyGrant) NamespaceAllowed() bool {
	if len(g.Namespaces) == 0 {
		return true
	}
	for _, ns := range g.Namespaces {
		if ns == g.Namespace {
			return true
		}
	}
	return false
}

func (g PodProxyGrant) valid() bool {
	return g.User != "" && g.Namespace != "" && g.Pod != "" && g.Port > 0 && g.Port <= 65535 && ValidPodProxyBasePath(g.Path) &&
		(g.SessionID == "" || ValidPodProxySessionID(g.SessionID)) && g.NamespaceAllowed()
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
//
// A store built by NewPodProxyTicketStore keeps random tickets in memory, so
// only this replica can redeem them. A shared store (NewSharedPodProxyTicketStore,
// podproxy_shared.go) issues signed tickets any replica holding the same
// secret can redeem; its entries map then only counts this replica's
// outstanding tickets for the caps.
type PodProxyTicketStore struct {
	mu      sync.Mutex
	entries map[string]podProxyTicket // sha256(token) -> ticket
	now     func() time.Time
	ttl     time.Duration

	signer   *podProxySigner   // shared mode only
	redeemed *PodProxyRedeemed // shared mode only
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

// NewSharedPodProxyTicketStore returns a store whose tickets are signed with a
// key derived from secret, so every replica configured with the same secret
// can redeem them; redeemed (shared with the other replicas through its store)
// makes them single use. secret must not be empty.
func NewSharedPodProxyTicketStore(secret string, redeemed *PodProxyRedeemed) *PodProxyTicketStore {
	return NewSharedPodProxyTicketStoreWithClock(secret, redeemed, time.Now, PodProxyTicketTTL)
}

// NewSharedPodProxyTicketStoreWithClock is NewSharedPodProxyTicketStore with
// an injected clock and TTL.
func NewSharedPodProxyTicketStoreWithClock(secret string, redeemed *PodProxyRedeemed, now func() time.Time, ttl time.Duration) *PodProxyTicketStore {
	if secret == "" {
		panic("auth: shared pod proxy tickets need a secret")
	}
	if redeemed == nil {
		redeemed = NewPodProxyRedeemedWithClock(nil, now)
	}
	s := NewPodProxyTicketStoreWithClock(now, ttl)
	s.signer, s.redeemed = newPodProxySigner(secret), redeemed
	return s
}

// Shared reports whether tickets are signed (redeemable on any replica).
func (s *PodProxyTicketStore) Shared() bool { return s.signer != nil }

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
	exp := now.Add(s.ttl)
	token := RandomURLToken(32)
	if s.signer != nil {
		var err error
		token, err = s.signer.seal(podProxyKindTicket, podProxyTicketClaims{V: 1, ID: RandomURLToken(16), Exp: exp.UnixMilli(), G: g})
		if err != nil {
			return "", time.Time{}, err
		}
	}
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
	if s.signer != nil {
		return s.consumeShared(token, now)
	}
	if !ok || !now.Before(e.exp) {
		return PodProxyGrant{}, false
	}
	return e.grant, true
}

// consumeShared redeems a signed ticket: it must verify, be unexpired, carry a
// valid grant and not have been redeemed before (here or, as far as the shared
// set shows, on another replica). A verified ticket is marked redeemed
// whatever happens next.
func (s *PodProxyTicketStore) consumeShared(token string, now time.Time) (PodProxyGrant, bool) {
	var c podProxyTicketClaims
	if !s.signer.open(podProxyKindTicket, token, &c) || c.V != 1 || c.ID == "" {
		return PodProxyGrant{}, false
	}
	exp := time.UnixMilli(c.Exp)
	if !now.Before(exp) || !c.G.valid() {
		return PodProxyGrant{}, false
	}
	if !s.redeemed.Mark(c.ID, exp) {
		return PodProxyGrant{}, false
	}
	return c.G, true
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

	// signer is set in shared mode: the cookie secret is then a signed token
	// any replica with the same secret accepts, and entries only holds the
	// sessions this replica created (for the caps, Live and sliding).
	signer *podProxySigner
}

// NewSharedPodProxySessionStore returns a store whose session cookies are
// signed with a key derived from secret, so every replica configured with the
// same secret accepts them. secret must not be empty.
func NewSharedPodProxySessionStore(secret string) *PodProxySessionStore {
	return NewSharedPodProxySessionStoreWithClock(secret, time.Now, PodProxySessionIdle, PodProxySessionMax)
}

// NewSharedPodProxySessionStoreWithClock is NewSharedPodProxySessionStore with
// an injected clock and lifetimes.
func NewSharedPodProxySessionStoreWithClock(secret string, now func() time.Time, idle, max time.Duration) *PodProxySessionStore {
	if secret == "" {
		panic("auth: shared pod proxy sessions need a secret")
	}
	s := NewPodProxySessionStoreWithClock(now, idle, max)
	s.signer = newPodProxySigner(secret)
	return s
}

// Shared reports whether session cookies are signed (accepted on any replica).
func (s *PodProxySessionStore) Shared() bool { return s.signer != nil }

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
	if s.signer != nil {
		if secret, err = s.sealSession(id, now, now, g); err != nil {
			return "", "", err
		}
	}
	s.entries[ticketDigest(id)] = &podProxySession{secret: ticketDigest(secret), grant: g, created: now, lastSeen: now}
	return id, secret, nil
}

// Lookup returns the grant of the live session id whose cookie secret is
// secret, and slides its idle expiry. An expired session is removed.
func (s *PodProxySessionStore) Lookup(id, secret string) (PodProxyGrant, bool) {
	g, _, _, ok := s.Authorize(id, secret)
	return g, ok
}

// Authorize is Lookup that also returns, in shared mode, a re-issued cookie
// secret carrying a later idle deadline (renewed, "" when the presented one is
// still fresh) and how long the cookie may live (until the session's absolute
// maximum). In memory mode renewed is always "".
func (s *PodProxySessionStore) Authorize(id, secret string) (g PodProxyGrant, renewed string, maxAge time.Duration, ok bool) {
	if id == "" || secret == "" {
		return PodProxyGrant{}, "", 0, false
	}
	if s.signer != nil {
		return s.checkShared(id, secret, true)
	}
	g, ok = s.lookupMemory(id, secret)
	return g, "", 0, ok
}

// Valid reports whether secret is a live cookie of session id, without
// sliding anything.
func (s *PodProxySessionStore) Valid(id, secret string) bool {
	if id == "" || secret == "" {
		return false
	}
	if s.signer != nil {
		_, _, _, ok := s.checkShared(id, secret, false)
		return ok
	}
	key := ticketDigest(id)
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	return ok && !s.expiredLocked(e, now) && subtle.ConstantTimeCompare([]byte(e.secret), []byte(ticketDigest(secret))) == 1
}

func (s *PodProxySessionStore) sealSession(id string, created, now time.Time, g PodProxyGrant) (string, error) {
	// The base path only matters when the ticket is redeemed; leaving it out
	// keeps the cookie small.
	g.Path = "/"
	return s.signer.seal(podProxyKindSession, podProxySessionClaims{V: 1, SID: id, Created: created.UnixMilli(), Idle: now.Add(s.idle).UnixMilli(), G: g})
}

// checkShared verifies a signed session cookie for session id. The cookie is
// live until its signed idle deadline, or for a session this replica created,
// while its local idle expiry has not passed (so a browser that has not stored
// the latest re-issued cookie keeps working against the replica it uses), and
// never past the signed creation time plus the absolute maximum.
func (s *PodProxySessionStore) checkShared(id, secret string, slide bool) (PodProxyGrant, string, time.Duration, bool) {
	var c podProxySessionClaims
	if !s.signer.open(podProxyKindSession, secret, &c) || c.V != 1 || c.SID != id || !c.G.valid() {
		return PodProxyGrant{}, "", 0, false
	}
	now := s.now()
	created := time.UnixMilli(c.Created)
	key := ticketDigest(id)
	s.mu.Lock()
	e, local := s.entries[key]
	if local && s.expiredLocked(e, now) {
		delete(s.entries, key)
		local = false
	}
	if local && slide {
		e.lastSeen = now
	}
	s.mu.Unlock()
	end := created.Add(s.max)
	if !now.Before(end) || created.After(now.Add(time.Minute)) {
		return PodProxyGrant{}, "", 0, false
	}
	idleEnd := time.UnixMilli(c.Idle)
	if !now.Before(idleEnd) && !local {
		return PodProxyGrant{}, "", 0, false
	}
	renewed := ""
	if slide && now.Sub(idleEnd.Add(-s.idle)) >= podProxyRenewAfter {
		if tok, err := s.sealSession(id, created, now, c.G); err == nil {
			renewed = tok
		}
	}
	return c.G, renewed, end.Sub(now), true
}

func (s *PodProxySessionStore) lookupMemory(id, secret string) (PodProxyGrant, bool) {
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
