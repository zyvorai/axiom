package auth

import "testing"

// RandomURLToken backs the per-process session-secret fallback (see
// api.AuthOptions.SessionSecret) — it must not be a fixed or predictable
// value across calls, or that fallback would be a guessable secret in
// practice, defeating the point of it.
func TestRandomURLTokenIsUniquePerCall(t *testing.T) {
	a := RandomURLToken(32)
	b := RandomURLToken(32)
	if a == "" || b == "" {
		t.Fatal("expected non-empty tokens")
	}
	if a == b {
		t.Fatal("expected two calls to produce different tokens")
	}
}

func TestSignAndParseSessionRoundTrip(t *testing.T) {
	tok, err := SignSession("secret-a", Session{User: "alice", Role: "admin"})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sess, ok := ParseSession("secret-a", tok)
	if !ok || sess.User != "alice" || sess.Role != "admin" {
		t.Fatalf("expected round-trip to recover the session, got %+v ok=%v", sess, ok)
	}
}

func TestParseSessionRejectsWrongSecret(t *testing.T) {
	tok, err := SignSession("secret-a", Session{User: "alice", Role: "admin"})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, ok := ParseSession("secret-b", tok); ok {
		t.Fatal("expected a session signed with a different secret to be rejected")
	}
}
