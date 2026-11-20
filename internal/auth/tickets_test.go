package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newFakeClock() *fakeClock               { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }

const termPath = "/api/v1/pods/team-a/web-0/terminal"

func TestTicketSingleUse(t *testing.T) {
	c := newFakeClock()
	s := NewTicketStoreWithClock(c.now, TicketTTL)
	tok, _, err := s.Issue(Identity{User: "alice", Role: "operator"}, termPath)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := s.Consume(tok, termPath)
	if !ok || id.User != "alice" || id.Role != "operator" {
		t.Fatalf("first redemption must succeed with the issuing identity, got %+v ok=%v", id, ok)
	}
	if _, ok := s.Consume(tok, termPath); ok {
		t.Fatal("a ticket must not be redeemable twice")
	}
}

func TestTicketExpires(t *testing.T) {
	c := newFakeClock()
	s := NewTicketStoreWithClock(c.now, TicketTTL)
	tok, _, _ := s.Issue(Identity{User: "alice", Role: "admin"}, termPath)
	c.advance(TicketTTL) // expiry is exclusive: now == exp is already dead
	if _, ok := s.Consume(tok, termPath); ok {
		t.Fatal("an expired ticket must be rejected")
	}
	tok, _, _ = s.Issue(Identity{User: "alice", Role: "admin"}, termPath)
	c.advance(TicketTTL - time.Second)
	if _, ok := s.Consume(tok, termPath); !ok {
		t.Fatal("a ticket inside its TTL must be accepted")
	}
}

func TestTicketWrongPathBurnsTicket(t *testing.T) {
	s := NewTicketStore()
	tok, _, _ := s.Issue(Identity{User: "alice", Role: "admin"}, termPath)
	if _, ok := s.Consume(tok, "/api/v1/pods/team-b/db-0/terminal"); ok {
		t.Fatal("a ticket must only be valid for the path it was issued for")
	}
	if _, ok := s.Consume(tok, termPath); ok {
		t.Fatal("a mismatched presentation must destroy the ticket")
	}
}

func TestTicketRejectsGarbageAndIneligiblePaths(t *testing.T) {
	s := NewTicketStore()
	if _, ok := s.Consume("", termPath); ok {
		t.Fatal("empty ticket accepted")
	}
	if _, ok := s.Consume("not-a-real-ticket", termPath); ok {
		t.Fatal("unknown ticket accepted")
	}
	for _, p := range []string{
		"", "/", "/api/v1/secrets/ns/s", "/api/v1/pods/ns/p", "/api/v1/pods/ns/p/exec",
		"/api/v1/pods/ns/p/terminal/", "/api/v1/pods/ns/../p/terminal", "/api/v1/pods//p/terminal",
		"/api/v1/pods/ns/p/terminal?x=1", "/api/v1/apply", "/api/v1/users", "/api/v1/ws-ticket",
		"api/v1/pods/ns/p/terminal", "/api/v1/vms/ns/v/start",
	} {
		if _, _, err := s.Issue(Identity{User: "alice", Role: "admin"}, p); err == nil {
			t.Errorf("ticket minted for ineligible path %q", p)
		}
	}
	for _, p := range []string{"/api/v1/events/stream", termPath, "/api/v1/pods/ns/p/logs", "/api/v1/vms/ns/v/vnc", "/api/v1/vms/ns/v/console"} {
		if _, _, err := s.Issue(Identity{User: "alice", Role: "admin"}, p); err != nil {
			t.Errorf("ticket refused for streaming path %q: %v", p, err)
		}
	}
	if _, _, err := s.Issue(Identity{}, termPath); err == nil {
		t.Error("ticket minted without an identity")
	}
}

func TestTicketStoreIsBounded(t *testing.T) {
	c := newFakeClock()
	s := NewTicketStoreWithClock(c.now, TicketTTL)
	for i := 0; i < ticketMaxPerIdentity; i++ {
		if _, _, err := s.Issue(Identity{User: "alice", Role: "admin"}, termPath); err != nil {
			t.Fatalf("issue %d: %v", i, err)
		}
	}
	if _, _, err := s.Issue(Identity{User: "alice", Role: "admin"}, termPath); err == nil {
		t.Fatal("per-identity cap not enforced")
	}
	if _, _, err := s.Issue(Identity{User: "bob", Role: "admin"}, termPath); err != nil {
		t.Fatalf("another identity must be unaffected: %v", err)
	}
	c.advance(time.Minute)
	if _, _, err := s.Issue(Identity{User: "alice", Role: "admin"}, termPath); err != nil {
		t.Fatalf("expired tickets should free capacity: %v", err)
	}
}

func ticketGuard(t *testing.T) (http.Handler, *TicketStore, *fakeClock) {
	t.Helper()
	c := newFakeClock()
	store := NewTicketStoreWithClock(c.now, TicketTTL)
	users := &fakeUserAuthenticator{users: map[string]struct {
		password   string
		role       string
		namespaces []string
	}{
		"viewer":  {password: "pw", role: "viewer"},
		"oper":    {password: "pw", role: "operator"},
		"roper":   {password: "pw", role: "operator", namespaces: []string{"team-a"}},
		"rviewer": {password: "pw", role: "viewer", namespaces: []string{"team-a"}},
	}}
	g := Guard{Username: "demo", Password: "demo", Users: users, Tickets: store}
	return g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := IdentityFromContext(r.Context())
		w.Header().Set("X-Who", id.User)
		w.WriteHeader(204)
	})), store, c
}

func basicHdr(r *http.Request, user, pass string) {
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
}

func TestGuardTicketFlow(t *testing.T) {
	h, store, _ := ticketGuard(t)
	tok, _, _ := store.Issue(Identity{User: "oper", Role: "operator"}, termPath)

	// No Authorization header at all: the ticket alone authenticates, once.
	r := httptest.NewRequest("GET", termPath+"?ticket="+tok, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("X-Who") != "oper" {
		t.Fatalf("ticket redemption: code=%d who=%q", w.Code, w.Header().Get("X-Who"))
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", termPath+"?ticket="+tok, nil))
	if w.Code != 401 {
		t.Fatalf("reused ticket must be 401, got %d", w.Code)
	}
}

func TestGuardTicketExpiredAndWrongPath(t *testing.T) {
	h, store, clock := ticketGuard(t)
	tok, _, _ := store.Issue(Identity{User: "oper", Role: "operator"}, termPath)
	clock.advance(2 * TicketTTL)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", termPath+"?ticket="+tok, nil))
	if w.Code != 401 {
		t.Fatalf("expired ticket: got %d", w.Code)
	}
	tok, _, _ = store.Issue(Identity{User: "oper", Role: "operator"}, termPath)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/pods/team-a/other/terminal?ticket="+tok, nil))
	if w.Code != 401 {
		t.Fatalf("ticket for a different path: got %d", w.Code)
	}
}

func TestGuardTicketIsNotAFallbackCredential(t *testing.T) {
	h, _, _ := ticketGuard(t)
	// A bogus ticket must not be rescued by otherwise-valid Basic credentials.
	r := httptest.NewRequest("GET", termPath+"?ticket=bogus", nil)
	basicHdr(r, "demo", "demo")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("invalid ticket + valid creds: got %d, want 401 (fail closed)", w.Code)
	}
}

func TestGuardTicketOnlyForGET(t *testing.T) {
	h, store, _ := ticketGuard(t)
	tok, _, _ := store.Issue(Identity{User: "demo", Role: "admin"}, termPath)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", termPath+"?ticket="+tok, nil))
	if w.Code != 401 {
		t.Fatalf("ticket on POST: got %d, want 401", w.Code)
	}
}

func TestGuardTicketKeepsIdentityPolicy(t *testing.T) {
	h, store, _ := ticketGuard(t)
	cases := []struct {
		name string
		id   Identity
		path string
		want int
	}{
		{"viewer ticket for a terminal is still denied", Identity{User: "viewer", Role: "viewer"}, termPath, 403},
		{"viewer ticket for logs allowed", Identity{User: "viewer", Role: "viewer"}, "/api/v1/pods/team-a/web-0/logs", 204},
		{"viewer ticket for events stream allowed", Identity{User: "viewer", Role: "viewer"}, "/api/v1/events/stream", 204},
		{"restricted operator ticket outside namespaces denied", Identity{User: "roper", Role: "operator", Namespaces: []string{"team-a"}}, "/api/v1/pods/team-b/x/terminal", 403},
		{"restricted operator ticket inside namespaces allowed", Identity{User: "roper", Role: "operator", Namespaces: []string{"team-a"}}, termPath, 204},
		{"restricted identity cannot ride the global events stream", Identity{User: "rviewer", Role: "viewer", Namespaces: []string{"team-a"}}, "/api/v1/events/stream", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok, _, err := store.Issue(tc.id, tc.path)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", tc.path+"?ticket="+tok, nil))
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestGuardTicketIssueAllowedForViewerAndRestricted(t *testing.T) {
	h, _, _ := ticketGuard(t)
	for _, user := range []string{"viewer", "rviewer", "roper", "oper"} {
		r := httptest.NewRequest("POST", "/api/v1/ws-ticket", nil)
		basicHdr(r, user, "pw")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 204 {
			t.Errorf("%s must be able to reach POST /ws-ticket, got %d", user, w.Code)
		}
	}
	// ... but the exemption must not widen to other POSTs.
	r := httptest.NewRequest("POST", "/api/v1/ws-ticket/extra", nil)
	basicHdr(r, "viewer", "pw")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Errorf("viewer POST /ws-ticket/extra: got %d, want 403", w.Code)
	}
}

func TestGuardDeprecatesURLCredentials(t *testing.T) {
	h, _, _ := ticketGuard(t)
	tok := base64.StdEncoding.EncodeToString([]byte("demo:demo"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/overview?access_token="+tok, nil))
	if w.Code != 204 {
		t.Fatalf("legacy ?access_token= must keep working, got %d", w.Code)
	}
	if w.Header().Get("Deprecation") != "true" {
		t.Fatal("legacy URL credentials must be flagged with a Deprecation header")
	}
	r := httptest.NewRequest("GET", "/api/v1/overview", nil)
	basicHdr(r, "demo", "demo")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Deprecation") != "" {
		t.Fatal("header credentials must not be flagged")
	}
	// The pod HTTP proxy cannot use single-use tickets; its URL token is not flagged.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/pods/ns/p/window/8080/?access_token="+tok, nil))
	if w.Code != 204 || w.Header().Get("Deprecation") != "" {
		t.Fatalf("window proxy: code=%d deprecation=%q", w.Code, w.Header().Get("Deprecation"))
	}
}
