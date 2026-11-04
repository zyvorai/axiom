package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPodProxySessionIDShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		id := NewPodProxySessionID()
		if !ValidPodProxySessionID(id) || len(id) != 32 || strings.ToLower(id) != id {
			t.Fatalf("bad id %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
	for _, bad := range []string{"", "abc", strings.Repeat("g", 32), strings.Repeat("A", 32), strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("a", 31) + "."} {
		if ValidPodProxySessionID(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestPodProxySessionCreateWithID(t *testing.T) {
	c := newFakeClock()
	s := NewPodProxySessionStoreWithClock(c.now, PodProxySessionIdle, PodProxySessionMax)
	id := NewPodProxySessionID()
	g := ppGrant("alice")

	// The grant must carry the same id.
	if _, err := s.CreateWithID(id, g); err == nil {
		t.Fatal("grant without SessionID accepted")
	}
	g.SessionID = NewPodProxySessionID()
	if _, err := s.CreateWithID(id, g); err == nil {
		t.Fatal("grant for another id accepted")
	}
	if _, err := s.CreateWithID("not-hex", g); err == nil {
		t.Fatal("malformed id accepted")
	}
	g.SessionID = id
	if s.Live(id) {
		t.Fatal("session live before creation")
	}
	secret, err := s.CreateWithID(id, g)
	if err != nil || secret == "" || secret == id {
		t.Fatalf("create: %q %v", secret, err)
	}
	if !s.Live(id) {
		t.Fatal("session not live")
	}
	if _, err := s.CreateWithID(id, g); err == nil {
		t.Fatal("a live session id must not be reused")
	}
	if got, ok := s.Lookup(id, secret); !ok || got.SessionID != id || got.User != "alice" {
		t.Fatalf("lookup: %+v %v", got, ok)
	}
	if _, ok := s.Lookup(id, "wrong"); ok {
		t.Fatal("wrong secret accepted")
	}
	// Only digests are stored.
	if s.hasKey(id) || !s.hasKey(ticketDigest(id)) {
		t.Fatal("session id stored in the clear")
	}
	// Live does not slide the idle expiry.
	c.advance(PodProxySessionIdle - time.Second)
	if !s.Live(id) {
		t.Fatal("expired early")
	}
	c.advance(2 * time.Second)
	if s.Live(id) || s.Len() != 0 {
		t.Fatal("Live must not slide the session and must drop it once expired")
	}
	// An expired id may be taken again.
	if _, err := s.CreateWithID(id, g); err != nil {
		t.Fatalf("recreate after expiry: %v", err)
	}
	// A ticket store refuses a malformed SessionID.
	ts := NewPodProxyTicketStoreWithClock(c.now, PodProxyTicketTTL)
	bad := ppGrant("alice")
	bad.SessionID = "zz"
	if _, _, err := ts.Issue(bad); err == nil {
		t.Fatal("ticket with malformed SessionID issued")
	}
}
