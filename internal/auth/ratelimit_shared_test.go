package auth

import (
	"testing"
	"time"
)

func TestSharedLoginThrottleAcrossReplicas(t *testing.T) {
	store := &MemoryMergingStore{}
	backing := NewSharedRateBacking(store)
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}

	a := NewAccountRateLimiter(clock.now)
	b := NewAccountRateLimiter(clock.now)
	a.AttachSharedBacking(backing)
	b.AttachSharedBacking(backing)

	key := accountKey("victim")
	for i := 0; i < accountGraceFailures+1; i++ {
		a.RecordFailure(key)
	}
	if a.Allowed(key) {
		t.Fatal("replica A should lock after grace+1 failures")
	}
	if b.Allowed(key) {
		t.Fatal("replica B should see the shared lockout")
	}
	b.RecordSuccess(key)
	if !a.Allowed(key) {
		t.Fatal("replica A should clear after shared success")
	}
}
