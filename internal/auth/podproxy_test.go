package auth

import (
	"encoding/base64"
	"testing"
	"time"
)

func ppGrant(user string) PodProxyGrant {
	return PodProxyGrant{User: user, Role: "operator", Namespace: "team-a", Pod: "web-0", Port: 8080, Path: "/"}
}

func TestPodProxyTicketSingleUseAndBinding(t *testing.T) {
	c := newFakeClock()
	s := NewPodProxyTicketStoreWithClock(c.now, PodProxyTicketTTL)
	g := ppGrant("alice")
	g.Path = "/dash?x=1"
	tok, exp, err := s.Issue(g)
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Equal(c.t.Add(PodProxyTicketTTL)) {
		t.Fatalf("exp = %v", exp)
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil || len(raw) != 32 {
		t.Fatalf("ticket must be 256 random bits, got %d bytes (%v)", len(raw), err)
	}
	got, ok := s.Consume(tok)
	if !ok || got.User != "alice" || got.Namespace != "team-a" || got.Pod != "web-0" || got.Port != 8080 || got.Path != "/dash?x=1" {
		t.Fatalf("redeem: %+v ok=%v", got, ok)
	}
	if got.IssuedMs != c.t.UnixMilli() {
		t.Fatalf("IssuedMs not stamped: %d", got.IssuedMs)
	}
	if _, ok := s.Consume(tok); ok {
		t.Fatal("a pod proxy ticket must be single use")
	}
	if _, ok := s.Consume(""); ok {
		t.Fatal("empty ticket accepted")
	}
}

func TestPodProxyTicketTTL(t *testing.T) {
	c := newFakeClock()
	s := NewPodProxyTicketStoreWithClock(c.now, PodProxyTicketTTL)
	tok, _, _ := s.Issue(ppGrant("alice"))
	c.advance(PodProxyTicketTTL - time.Millisecond)
	tok2, _, _ := s.Issue(ppGrant("alice"))
	c.advance(time.Millisecond)
	if _, ok := s.Consume(tok); ok {
		t.Fatal("ticket must be dead at exactly its TTL")
	}
	if _, ok := s.Consume(tok2); !ok {
		t.Fatal("younger ticket must still be live")
	}
	if PodProxyTicketTTL != 60*time.Second {
		t.Fatalf("TTL = %v, want 60s", PodProxyTicketTTL)
	}
}

func TestPodProxyTicketHashOnly(t *testing.T) {
	s := NewPodProxyTicketStore()
	tok, _, _ := s.Issue(ppGrant("alice"))
	if s.hasKey(tok) {
		t.Fatal("raw ticket must not be stored")
	}
	if !s.hasKey(ticketDigest(tok)) {
		t.Fatal("ticket digest must be the key")
	}
	ss := NewPodProxySessionStore()
	id, secret, _ := ss.Create(ppGrant("alice"))
	if ss.hasKey(id) || !ss.hasKey(ticketDigest(id)) {
		t.Fatal("session ids must be stored hashed")
	}
	for _, e := range ss.entries {
		if e.secret == secret || e.secret != ticketDigest(secret) {
			t.Fatal("session secrets must be stored hashed")
		}
	}
}

func TestPodProxyTicketPerUserCap(t *testing.T) {
	c := newFakeClock()
	s := NewPodProxyTicketStoreWithClock(c.now, PodProxyTicketTTL)
	for i := 0; i < podProxyTicketsPerUser; i++ {
		if _, _, err := s.Issue(ppGrant("alice")); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
	}
	if _, _, err := s.Issue(ppGrant("alice")); err != ErrPodProxyTableFull {
		t.Fatalf("over the per-user cap: %v", err)
	}
	if _, _, err := s.Issue(ppGrant("bob")); err != nil {
		t.Fatalf("another user must not be starved: %v", err)
	}
	c.advance(PodProxyTicketTTL)
	if _, _, err := s.Issue(ppGrant("alice")); err != nil {
		t.Fatalf("expired tickets must be swept: %v", err)
	}
}

func TestPodProxyTicketRejectsBadGrants(t *testing.T) {
	s := NewPodProxyTicketStore()
	bad := map[string]func(*PodProxyGrant){
		"no user":       func(g *PodProxyGrant) { g.User = "" },
		"no ns":         func(g *PodProxyGrant) { g.Namespace = "" },
		"no pod":        func(g *PodProxyGrant) { g.Pod = "" },
		"port 0":        func(g *PodProxyGrant) { g.Port = 0 },
		"port too big":  func(g *PodProxyGrant) { g.Port = 70000 },
		"relative path": func(g *PodProxyGrant) { g.Path = "x" },
		"scheme-rel":    func(g *PodProxyGrant) { g.Path = "//evil.example/" },
		"backslash":     func(g *PodProxyGrant) { g.Path = "/\\evil" },
		"crlf":          func(g *PodProxyGrant) { g.Path = "/a\r\nSet-Cookie: x" },
		"empty":         func(g *PodProxyGrant) { g.Path = "" },
		"fragment":      func(g *PodProxyGrant) { g.Path = "/#x" },
	}
	for name, mut := range bad {
		g := ppGrant("alice")
		mut(&g)
		if _, _, err := s.Issue(g); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestPodProxySessionLifetimes(t *testing.T) {
	c := newFakeClock()
	s := NewPodProxySessionStoreWithClock(c.now, PodProxySessionIdle, PodProxySessionMax)
	id, secret, err := s.Create(ppGrant("alice"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Lookup(id, "wrong"); ok {
		t.Fatal("wrong secret accepted")
	}
	if _, ok := s.Lookup("wrong", secret); ok {
		t.Fatal("wrong id accepted")
	}
	// Sliding: keep using it every 9 minutes, it survives past 10 minutes…
	for i := 0; i < 6; i++ {
		c.advance(9 * time.Minute)
		if _, ok := s.Lookup(id, secret); !ok {
			t.Fatalf("session died while in use at step %d", i)
		}
	}
	// …but never past the absolute one-hour cap.
	c.advance(7 * time.Minute) // 61 minutes since creation
	if _, ok := s.Lookup(id, secret); ok {
		t.Fatal("session must die at its absolute maximum")
	}
	id2, secret2, _ := s.Create(ppGrant("alice"))
	c.advance(PodProxySessionIdle)
	if _, ok := s.Lookup(id2, secret2); ok {
		t.Fatal("idle session must die after 10 minutes")
	}
	if s.Len() != 0 {
		t.Fatalf("expired sessions must be removed, %d left", s.Len())
	}
}

func TestPodProxySessionPerUserCapAndDelete(t *testing.T) {
	s := NewPodProxySessionStore()
	var first string
	for i := 0; i < podProxySessionsPerUser; i++ {
		id, _, err := s.Create(ppGrant("alice"))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = id
		}
	}
	if _, _, err := s.Create(ppGrant("alice")); err != ErrPodProxyTableFull {
		t.Fatalf("per-user session cap: %v", err)
	}
	s.Delete(first)
	if _, _, err := s.Create(ppGrant("alice")); err != nil {
		t.Fatalf("after delete: %v", err)
	}
}
