package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/auth"
)

// exerciseExternalMFAStore runs the whole enrolment lifecycle against s.
func exerciseExternalMFAStore(t *testing.T, s *ExternalMFAStore) {
	t.Helper()
	ctx := context.Background()
	var invalidated []string
	s.SetInvalidateHook(func(u string) { invalidated = append(invalidated, u) })
	now := time.Now()

	if s.MFAEnrolled("Alice") || s.VerifySecondFactor("alice", "123456", now) {
		t.Fatal("no record yet")
	}
	if _, err := s.EnableMFA(ctx, "alice", "123456", now); err == nil {
		t.Fatal("enable before setup must fail")
	}
	secret, err := s.BeginMFA(ctx, "Alice")
	if err != nil || secret == "" {
		t.Fatalf("begin: %q %v", secret, err)
	}
	if s.MFAEnrolled("alice") {
		t.Fatal("pending is not enrolled")
	}
	if list := s.List(); len(list) != 1 || list[0].Username != "alice" || list[0].Enabled {
		t.Fatalf("list while pending: %+v", list)
	}
	if _, err := s.EnableMFA(ctx, "alice", "000000", now); err == nil {
		t.Fatal("wrong code enabled MFA")
	}
	code, _ := auth.TOTPCodeAt(secret, now)
	recovery, err := s.EnableMFA(ctx, "ALICE", code, now)
	if err != nil || len(recovery) != 10 {
		t.Fatalf("enable: %v %v", recovery, err)
	}
	if len(invalidated) != 1 || invalidated[0] != "ALICE" {
		t.Fatalf("enable did not revoke sessions: %v", invalidated)
	}
	if !s.MFAEnrolled("alice") {
		t.Fatal("not enrolled after enable")
	}
	if _, err := s.BeginMFA(ctx, "alice"); err == nil {
		t.Fatal("re-enrol while enabled must fail")
	}
	// The enrolment code's step is used; the next one works once.
	if s.VerifySecondFactor("alice", code, now) {
		t.Fatal("enrolment code replayed")
	}
	next, _ := auth.TOTPCodeAt(secret, now.Add(30*time.Second))
	if !s.VerifySecondFactor("alice", next, now.Add(30*time.Second)) {
		t.Fatal("next code refused")
	}
	if s.VerifySecondFactor("alice", next, now.Add(30*time.Second)) {
		t.Fatal("code replayed")
	}
	if !s.VerifySecondFactor("alice", recovery[3], now) || s.VerifySecondFactor("alice", recovery[3], now) {
		t.Fatal("recovery code not single-use")
	}
	if en, left := s.MFAStatus("alice"); !en || left != 9 {
		t.Fatalf("status: %v %d", en, left)
	}

	// The admin view and its JSON carry no secret material.
	raw, _ := json.Marshal(s.List())
	for _, leak := range []string{secret, auth.HashRecoveryCode(recovery[0]), "secret", "pending", "recovery"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("list JSON leaks %q: %s", leak, raw)
		}
	}

	existed, err := s.Reset(ctx, "alice")
	if err != nil || !existed {
		t.Fatalf("reset: %v %v", existed, err)
	}
	if s.MFAEnrolled("alice") || len(s.List()) != 0 || len(invalidated) != 2 {
		t.Fatalf("after reset: enrolled=%v list=%v invalidated=%v", s.MFAEnrolled("alice"), s.List(), invalidated)
	}
	if existed, err := s.Reset(ctx, "alice"); err != nil || existed {
		t.Fatalf("second reset: %v %v", existed, err)
	}
	if len(invalidated) != 2 {
		t.Fatal("resetting nothing revoked sessions")
	}
}

func TestExternalMFAStoreMemory(t *testing.T) {
	exerciseExternalMFAStore(t, NewMemoryExternalMFAStore())
}

func TestOpenExternalMFAStoreDemoIsMemory(t *testing.T) {
	s := OpenExternalMFAStore("demo", NewDemoProvider())
	if s == nil {
		t.Fatal("nil store")
	}
	exerciseExternalMFAStore(t, s)
}

// failingBackend refuses writes, so a verified code cannot be recorded.
type failingBackend struct{ memoryExternalMFABackend }

func (f *failingBackend) Mutate(ctx context.Context, fn func(map[string]ExternalMFARecord) (bool, error)) error {
	return f.memoryExternalMFABackend.Mutate(ctx, func(m map[string]ExternalMFARecord) (bool, error) {
		changed, err := fn(m)
		if err == nil && changed && m["alice"].Enabled {
			return false, errors.New("write refused")
		}
		return changed, err
	})
}

func TestExternalMFAVerifyFailsWhenStateCannotBePersisted(t *testing.T) {
	b := &failingBackend{}
	s := NewExternalMFAStore(b)
	ctx := context.Background()
	now := time.Now()
	secret, _ := s.BeginMFA(ctx, "alice")
	code, _ := auth.TOTPCodeAt(secret, now)
	if _, err := s.EnableMFA(ctx, "alice", code, now); err == nil {
		t.Fatal("enable should fail when the write fails")
	}
	// Seed an enabled record directly and check a verify cannot be replayed.
	b.memoryExternalMFABackend.recs = map[string]ExternalMFARecord{"alice": {Username: "alice", Enabled: true, Secret: secret}}
	s = NewExternalMFAStore(b)
	next, _ := auth.TOTPCodeAt(secret, now.Add(30*time.Second))
	if s.VerifySecondFactor("alice", next, now.Add(30*time.Second)) {
		t.Fatal("a code was accepted although its use could not be recorded")
	}
}

// Two stores on one backend (two replicas): a code used on one is refused on
// the other at once, because verification always re-reads the backend.
func TestExternalMFASharedBackendNoReplayAcrossReplicas(t *testing.T) {
	b := &memoryExternalMFABackend{}
	a, c := NewExternalMFAStore(b), NewExternalMFAStore(b)
	ctx := context.Background()
	now := time.Now()
	secret, _ := a.BeginMFA(ctx, "bob")
	code, _ := auth.TOTPCodeAt(secret, now)
	if _, err := a.EnableMFA(ctx, "bob", code, now); err != nil {
		t.Fatal(err)
	}
	if !c.MFAEnrolled("bob") {
		t.Fatal("replica C does not see the enrolment")
	}
	next, _ := auth.TOTPCodeAt(secret, now.Add(30*time.Second))
	if !a.VerifySecondFactor("bob", next, now.Add(30*time.Second)) {
		t.Fatal("first use refused")
	}
	if c.VerifySecondFactor("bob", next, now.Add(30*time.Second)) {
		t.Fatal("code replayed on another replica")
	}
}

// The factor state machine shared with UserHub behaves the same for both.
func TestMFAFactorStateMachine(t *testing.T) {
	var f mfaFactor
	now := time.Now()
	if f.verify("123456", now) {
		t.Fatal("verify without a factor")
	}
	if _, err := f.enable("123456", now); err != errMFANotStarted {
		t.Fatalf("enable before begin: %v", err)
	}
	sec, _ := f.begin()
	code, _ := auth.TOTPCodeAt(sec, now)
	codes, err := f.enable(code, now)
	if err != nil || !f.Enabled || f.Pending != "" || f.Secret != sec || len(f.Recovery) != len(codes) {
		t.Fatalf("enable: %+v %v", f, err)
	}
	if _, err := f.begin(); err != errMFAAlreadyEnabledReenrol {
		t.Fatalf("begin while enabled: %v", err)
	}
	f.reset()
	if f.Enabled || f.Secret != "" || f.Recovery != nil || f.LastStep != 0 {
		t.Fatalf("reset: %+v", f)
	}
}
