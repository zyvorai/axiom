package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// twoReplicas is the ticket side of two replicas sharing a secret and a
// redeemed-ticket store.
func twoReplicas(c *fakeClock, secret string) (*PodProxyTicketStore, *PodProxyTicketStore, *MemoryMergingStore) {
	shared := &MemoryMergingStore{}
	a := NewSharedPodProxyTicketStoreWithClock(secret, NewPodProxyRedeemedWithClock(shared, c.now), c.now, PodProxyTicketTTL)
	b := NewSharedPodProxyTicketStoreWithClock(secret, NewPodProxyRedeemedWithClock(shared, c.now), c.now, PodProxyTicketTTL)
	return a, b, shared
}

func TestSharedPodProxyTicketAcrossReplicas(t *testing.T) {
	c := newFakeClock()
	a, b, shared := twoReplicas(c, "s3cret")
	if !a.Shared() || NewPodProxyTicketStore().Shared() {
		t.Fatal("Shared() mismatch")
	}
	g := ppGrant("alice")
	g.Path = "/dash?x=1"
	g.Namespaces = []string{"team-a"}
	g.SessionID = NewPodProxySessionID()
	tok, exp, err := a.Issue(g)
	if err != nil || !exp.Equal(c.t.Add(PodProxyTicketTTL)) {
		t.Fatalf("issue: %v %v", exp, err)
	}
	if strings.ContainsAny(tok, ".=+/") {
		t.Fatalf("ticket must be URL-token safe: %q", tok)
	}
	got, ok := b.Consume(tok)
	if !ok || got.User != "alice" || got.Path != "/dash?x=1" || got.SessionID != g.SessionID || got.IssuedMs != c.t.UnixMilli() ||
		len(got.Namespaces) != 1 || got.Namespaces[0] != "team-a" {
		t.Fatalf("redeemed on B: %+v %v", got, ok)
	}
	if _, ok := a.Consume(tok); ok {
		t.Fatal("ticket redeemed on B must be refused on A")
	}
	if _, ok := b.Consume(tok); ok {
		t.Fatal("ticket redeemed on B must be refused on B")
	}
	// The shared set holds the ticket id, never the ticket.
	raw, _ := shared.Load()
	if strings.Contains(string(raw), tok) || !strings.Contains(string(raw), `"ids"`) {
		t.Fatalf("shared set: %s", raw)
	}
}

func TestSharedPodProxyTicketRefusals(t *testing.T) {
	c := newFakeClock()
	a, b, _ := twoReplicas(c, "s3cret")
	tok, _, _ := a.Issue(ppGrant("alice"))

	// Tampering anywhere breaks the MAC, and does not burn the real ticket.
	raw, _ := base64.RawURLEncoding.DecodeString(tok)
	for _, i := range []int{0, len(raw) / 2, len(raw) - 1} {
		bad := append([]byte(nil), raw...)
		bad[i] ^= 1
		if _, ok := b.Consume(base64.RawURLEncoding.EncodeToString(bad)); ok {
			t.Fatalf("tampered byte %d accepted", i)
		}
	}
	for _, bad := range []string{"", "x", tok[:len(tok)-2], tok + "AA", strings.Repeat("A", podProxyTokenMax+1)} {
		if _, ok := b.Consume(bad); ok {
			t.Fatalf("malformed ticket %q accepted", bad)
		}
	}
	// Another secret does not verify it.
	other := NewSharedPodProxyTicketStoreWithClock("other", nil, c.now, PodProxyTicketTTL)
	if _, ok := other.Consume(tok); ok {
		t.Fatal("ticket accepted under another secret")
	}
	// A session cookie is not a ticket.
	ss := NewSharedPodProxySessionStoreWithClock("s3cret", c.now, PodProxySessionIdle, PodProxySessionMax)
	_, cookie, _ := ss.Create(ppGrant("alice"))
	if _, ok := b.Consume(cookie); ok {
		t.Fatal("session cookie accepted as a ticket")
	}
	if _, ok := b.Consume(tok); !ok {
		t.Fatal("the untampered ticket must still redeem")
	}

	// Expiry at exactly the TTL, on any replica.
	tok2, _, _ := a.Issue(ppGrant("alice"))
	c.advance(PodProxyTicketTTL)
	if _, ok := b.Consume(tok2); ok {
		t.Fatal("expired ticket accepted")
	}
	// A signed grant that fails validation (namespace outside its own
	// restriction) is refused even though the MAC is right.
	g := ppGrant("alice")
	g.Namespaces = []string{"team-b"}
	if _, _, err := a.Issue(g); err == nil {
		t.Fatal("grant outside its namespace restriction issued")
	}
	forged, _ := a.signer.seal(podProxyKindTicket, podProxyTicketClaims{V: 1, ID: "x", Exp: c.t.Add(time.Minute).UnixMilli(), G: g})
	if _, ok := b.Consume(forged); ok {
		t.Fatal("invalid signed grant accepted")
	}
}

func TestSharedPodProxyTicketConcurrentRedeemOnce(t *testing.T) {
	c := newFakeClock()
	a, b, _ := twoReplicas(c, "s3cret")
	tok, _, _ := a.Issue(ppGrant("alice"))
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 16; i++ {
		s := a
		if i%2 == 1 {
			s = b
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := s.Consume(tok); ok {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("ticket redeemed %d times", wins)
	}
}

func TestSharedPodProxyTicketPerReplicaCap(t *testing.T) {
	c := newFakeClock()
	a, b, _ := twoReplicas(c, "s3cret")
	var toks []string
	for i := 0; i < podProxyTicketsPerUser; i++ {
		tok, _, err := a.Issue(ppGrant("alice"))
		if err != nil {
			t.Fatal(err)
		}
		toks = append(toks, tok)
	}
	if _, _, err := a.Issue(ppGrant("alice")); err != ErrPodProxyTableFull {
		t.Fatalf("cap on A: %v", err)
	}
	// The cap is per replica: B has its own count.
	if _, _, err := b.Issue(ppGrant("alice")); err != nil {
		t.Fatalf("B: %v", err)
	}
	// Redeeming on A frees a slot on A.
	if _, ok := a.Consume(toks[0]); !ok {
		t.Fatal("redeem")
	}
	if _, _, err := a.Issue(ppGrant("alice")); err != nil {
		t.Fatalf("after redeem: %v", err)
	}
}

type failingStore struct{}

func (failingStore) Load() ([]byte, error) { return nil, errors.New("api down") }
func (failingStore) Save([]byte) error     { return errors.New("api down") }
func (failingStore) Merge(func([]byte) ([]byte, error)) error {
	return errors.New("api down")
}

func TestPodProxyRedeemed(t *testing.T) {
	c := newFakeClock()
	// A failing shared store does not block redemption; this replica still
	// refuses a second use.
	r := NewPodProxyRedeemedWithClock(failingStore{}, c.now)
	if !r.Mark("id1", c.t.Add(time.Minute)) || r.Mark("id1", c.t.Add(time.Minute)) {
		t.Fatal("failing store: first use must pass, second must not")
	}
	// Entries are pruned once their ticket has expired, locally and in the
	// shared snapshot.
	shared := &MemoryMergingStore{}
	r1 := NewPodProxyRedeemedWithClock(shared, c.now)
	r2 := NewPodProxyRedeemedWithClock(shared, c.now)
	if !r1.Mark("a", c.t.Add(time.Minute)) || r2.Mark("a", c.t.Add(time.Minute)) {
		t.Fatal("shared set must refuse a second use on another replica")
	}
	c.advance(time.Minute)
	if !r2.Mark("b", c.t.Add(time.Minute)) {
		t.Fatal("mark b")
	}
	if raw, _ := shared.Load(); strings.Contains(string(raw), `"a"`) {
		t.Fatalf("expired id kept: %s", raw)
	}
	// A non-merging store falls back to load + save.
	plain := &plainStore{}
	p1, p2 := NewPodProxyRedeemedWithClock(plain, c.now), NewPodProxyRedeemedWithClock(plain, c.now)
	if !p1.Mark("x", c.t.Add(time.Minute)) || p2.Mark("x", c.t.Add(time.Minute)) {
		t.Fatal("plain store")
	}
	// Bounded.
	ids := map[string]int64{}
	for i := 0; i < podProxyRedeemedMax+10; i++ {
		ids[RandomURLToken(8)] = c.t.Add(time.Duration(i+1) * time.Millisecond).UnixMilli()
	}
	pruneRedeemed(ids, c.t.UnixMilli())
	if len(ids) != podProxyRedeemedMax {
		t.Fatalf("pruned to %d", len(ids))
	}
	var nilSet *PodProxyRedeemed
	if nilSet.Mark("x", c.t) {
		t.Fatal("nil set must refuse")
	}
}

type plainStore struct {
	mu   sync.Mutex
	data []byte
}

func (p *plainStore) Load() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.data, nil
}
func (p *plainStore) Save(b []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data = b
	return nil
}

func TestSharedPodProxySessionAcrossReplicas(t *testing.T) {
	c := newFakeClock()
	a := NewSharedPodProxySessionStoreWithClock("s3cret", c.now, PodProxySessionIdle, PodProxySessionMax)
	b := NewSharedPodProxySessionStoreWithClock("s3cret", c.now, PodProxySessionIdle, PodProxySessionMax)
	g := ppGrant("alice")
	g.Path = "/long/base/path"
	id, cookie, err := a.Create(g)
	if err != nil {
		t.Fatal(err)
	}
	if got, renewed, maxAge, ok := b.Authorize(id, cookie); !ok || got.User != "alice" || got.Pod != "web-0" || renewed != "" || maxAge != PodProxySessionMax {
		t.Fatalf("B: %+v %q %v %v", got, renewed, maxAge, ok)
	}
	if !b.Valid(id, cookie) || b.Live(id) {
		t.Fatal("B accepts the cookie but holds no local session")
	}
	// Bound to its session id, its secret and its kind.
	if _, ok := b.Lookup("other", cookie); ok {
		t.Fatal("cookie accepted for another session")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(cookie)
	raw[len(raw)/3] ^= 1
	if _, ok := b.Lookup(id, base64.RawURLEncoding.EncodeToString(raw)); ok {
		t.Fatal("tampered cookie accepted")
	}
	if _, ok := NewSharedPodProxySessionStoreWithClock("other", c.now, PodProxySessionIdle, PodProxySessionMax).Lookup(id, cookie); ok {
		t.Fatal("cookie accepted under another secret")
	}
	ts := NewSharedPodProxyTicketStoreWithClock("s3cret", nil, c.now, PodProxyTicketTTL)
	tok, _, _ := ts.Issue(ppGrant("alice"))
	if _, ok := b.Lookup(id, tok); ok {
		t.Fatal("ticket accepted as a session cookie")
	}

	// Sliding: B re-issues the cookie once it is a minute old; only the
	// renewed cookie keeps the session alive on a replica without local
	// state.
	c.advance(5 * time.Minute)
	_, renewed, maxAge, ok := b.Authorize(id, cookie)
	if !ok || renewed == "" || renewed == cookie || maxAge != PodProxySessionMax-5*time.Minute {
		t.Fatalf("renew: %q %v %v", renewed, maxAge, ok)
	}
	c.advance(6 * time.Minute) // 11 min: past the first cookie's idle deadline
	if _, ok := b.Lookup(id, cookie); ok {
		t.Fatal("idle-expired cookie accepted on a replica without the session")
	}
	// A created the session but it has been idle there for 11 minutes, so A
	// refuses the old cookie too; the renewed one works on A.
	if _, ok := a.Lookup(id, cookie); ok {
		t.Fatal("idle-expired cookie accepted on A")
	}
	if _, ok := a.Lookup(id, renewed); !ok {
		t.Fatal("renewed cookie refused on A")
	}

	// Absolute maximum, however the cookie is renewed.
	cur := renewed
	elapsed := 11 * time.Minute
	for elapsed+3*time.Minute < PodProxySessionMax {
		c.advance(3 * time.Minute)
		elapsed += 3 * time.Minute
		_, nx, _, ok := b.Authorize(id, cur)
		if !ok {
			t.Fatalf("active session refused at %v", elapsed)
		}
		if nx != "" {
			cur = nx
		}
	}
	c.advance(3 * time.Minute)
	if _, ok := b.Lookup(id, cur); ok {
		t.Fatal("session accepted past its absolute maximum")
	}
}

func TestSharedPodProxySessionLocalSlide(t *testing.T) {
	// On the replica that created it, the session slides in memory as today,
	// even when the browser keeps presenting the first cookie.
	c := newFakeClock()
	s := NewSharedPodProxySessionStoreWithClock("s3cret", c.now, PodProxySessionIdle, PodProxySessionMax)
	id, cookie, _ := s.Create(ppGrant("alice"))
	for i := 0; i < 6; i++ {
		c.advance(9 * time.Minute)
		if _, ok := s.Lookup(id, cookie); !ok {
			t.Fatalf("died while in use at step %d", i)
		}
	}
	c.advance(7 * time.Minute)
	if _, ok := s.Lookup(id, cookie); ok {
		t.Fatal("past the absolute maximum")
	}
	// Per-user cap, per replica.
	for i := 0; i < podProxySessionsPerUser; i++ {
		if _, _, err := s.Create(ppGrant("bob")); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.Create(ppGrant("bob")); err != ErrPodProxyTableFull {
		t.Fatalf("cap: %v", err)
	}
	// Host mode: CreateWithID returns a signed cookie for that id.
	sid := NewPodProxySessionID()
	g := ppGrant("carol")
	g.SessionID = sid
	sec, err := s.CreateWithID(sid, g)
	if err != nil || !s.Live(sid) || !s.Valid(sid, sec) {
		t.Fatalf("CreateWithID: %v", err)
	}
	if got, ok := s.Lookup(sid, sec); !ok || got.SessionID != sid {
		t.Fatalf("lookup: %+v", got)
	}
}
