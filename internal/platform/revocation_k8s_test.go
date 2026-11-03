//go:build kubernetes

package platform

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/zyvorai/axiom/internal/auth"
)

// installOptimisticConfigMaps makes the fake clientset behave like the API
// server for the revocations ConfigMap: every write bumps resourceVersion and
// an Update carrying a stale resourceVersion fails with 409 Conflict. The
// stock fake tracker accepts any Update, so without this the test could not
// tell a conflict-safe Merge from a last-writer-wins Save.
func installOptimisticConfigMaps(cs *k8sfake.Clientset) *int {
	var (
		mu        sync.Mutex
		stored    *corev1.ConfigMap
		rv        int
		conflicts int
	)
	gr := schema.GroupResource{Resource: "configmaps"}
	cs.PrependReactor("*", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		defer mu.Unlock()
		switch action.GetVerb() {
		case "get":
			a := action.(clienttesting.GetAction)
			if stored == nil || a.GetName() != revocationsConfigMapName {
				return true, nil, apierrors.NewNotFound(gr, a.GetName())
			}
			return true, stored.DeepCopy(), nil
		case "create":
			a := action.(clienttesting.CreateAction)
			if stored != nil {
				return true, nil, apierrors.NewAlreadyExists(gr, revocationsConfigMapName)
			}
			cm := a.GetObject().(*corev1.ConfigMap).DeepCopy()
			rv++
			cm.ResourceVersion = strconv.Itoa(rv)
			stored = cm
			return true, cm.DeepCopy(), nil
		case "update":
			a := action.(clienttesting.UpdateAction)
			cm := a.GetObject().(*corev1.ConfigMap).DeepCopy()
			if stored == nil {
				return true, nil, apierrors.NewNotFound(gr, cm.Name)
			}
			// An empty resourceVersion is an unconditional update (Save).
			if cm.ResourceVersion != "" && cm.ResourceVersion != stored.ResourceVersion {
				conflicts++
				return true, nil, apierrors.NewConflict(gr, cm.Name, fmt.Errorf("stale resourceVersion %s", cm.ResourceVersion))
			}
			rv++
			cm.ResourceVersion = strconv.Itoa(rv)
			stored = cm
			return true, cm.DeepCopy(), nil
		}
		return false, nil, nil
	})
	return &conflicts
}

func TestK8sRevocationStoreSharedAcrossReplicas(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	installOptimisticConfigMaps(cs)
	store := NewK8sRevocationStore(cs, "axiom-system")
	a, b := auth.NewRevocations(store), auth.NewRevocations(store)

	sess := auth.Session{User: "alice", ID: "jti-1", Exp: time.Now().Add(time.Hour).Unix(), IatMs: time.Now().UnixMilli() - 1}
	a.RevokeSession(sess.ID, sess.Exp)
	if b.Revoked(sess) {
		t.Fatal("B saw A's revocation without refreshing")
	}
	if err := b.Refresh(); err != nil {
		t.Fatal(err)
	}
	if !b.Revoked(sess) {
		t.Fatal("B did not see A's revocation after a refresh")
	}
	// B's own revocation is merged into the store, not written over A's.
	b.RevokeUser("bob")
	c := auth.NewRevocations(store)
	if !c.Revoked(sess) || !c.Revoked(auth.Session{User: "bob", IatMs: 1}) {
		t.Fatal("the store lost an entry")
	}
}

func TestK8sRevocationStoreConcurrentRevokesLoseNothing(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	conflicts := installOptimisticConfigMaps(cs)
	store := NewK8sRevocationStore(cs, "axiom-system")
	a, b := auth.NewRevocations(store), auth.NewRevocations(store)
	const n = 25
	exp := time.Now().Add(time.Hour).Unix()
	var wg sync.WaitGroup
	for _, rep := range []struct {
		r      *auth.Revocations
		prefix string
	}{{a, "a"}, {b, "b"}} {
		wg.Add(1)
		go func(r *auth.Revocations, prefix string) {
			defer wg.Done()
			for i := 0; i < n; i++ {
				r.RevokeSession(fmt.Sprintf("%s-%d", prefix, i), exp)
				r.RevokeUser(fmt.Sprintf("%s-user-%d", prefix, i))
			}
		}(rep.r, rep.prefix)
	}
	wg.Wait()
	check := auth.NewRevocations(store)
	for _, p := range []string{"a", "b"} {
		for i := 0; i < n; i++ {
			if !check.Revoked(auth.Session{ID: fmt.Sprintf("%s-%d", p, i)}) {
				t.Fatalf("lost session %s-%d (conflicts retried: %d)", p, i, *conflicts)
			}
			if !check.Revoked(auth.Session{User: fmt.Sprintf("%s-user-%d", p, i), IatMs: 1}) {
				t.Fatalf("lost user cut-off %s-user-%d", p, i)
			}
		}
	}
	t.Logf("write conflicts retried: %d", *conflicts)
}

func TestK8sRevocationStoreCutoffMergeTakesLater(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	installOptimisticConfigMaps(cs)
	store := NewK8sRevocationStore(cs, "axiom-system")
	later := time.Now().UnixMilli()
	earlier := later - 60_000
	raw, _ := json.Marshal(map[string]any{"users": map[string]int64{"carol": later}})
	if err := store.Save(raw); err != nil {
		t.Fatal(err)
	}
	// Another replica writes an earlier cut-off through Merge.
	err := store.Merge(func(current []byte) ([]byte, error) {
		var snap struct {
			Users map[string]int64 `json:"users"`
		}
		_ = json.Unmarshal(current, &snap)
		if snap.Users["carol"] != later {
			t.Fatalf("Merge did not hand over the current snapshot: %s", current)
		}
		return current, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r := auth.NewRevocations(store)
	r.RevokeUser("dan")
	// A session issued between the two cut-offs is revoked: the later one won.
	if !r.Revoked(auth.Session{User: "carol", IatMs: earlier + 1}) {
		t.Fatal("the later cut-off was lost")
	}
	got, _ := store.Load()
	var snap struct {
		Users map[string]int64 `json:"users"`
	}
	if err := json.Unmarshal(got, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Users["carol"] != later || snap.Users["dan"] == 0 {
		t.Fatalf("stored cut-offs: %v", snap.Users)
	}
}

func TestK8sRevocationStoreMergeCreatesAndRetries(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	conflicts := installOptimisticConfigMaps(cs)
	store := NewK8sRevocationStore(cs, "axiom-system")
	calls := 0
	err := store.Merge(func(current []byte) ([]byte, error) {
		calls++
		if calls == 1 {
			if len(current) != 0 {
				t.Fatalf("no ConfigMap yet, got %q", current)
			}
			// Someone else writes between our read and our write.
			if err := store.Save([]byte(`{"ids":{"theirs":9999999999}}`)); err != nil {
				t.Fatal(err)
			}
			return []byte(`{"ids":{"ours":9999999999}}`), nil
		}
		return []byte(`{"ids":{"ours":9999999999,"theirs":9999999999}}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("Merge should have retried once after AlreadyExists, calls=%d conflicts=%d", calls, *conflicts)
	}
	got, _ := store.Load()
	if string(got) != `{"ids":{"ours":9999999999,"theirs":9999999999}}` {
		t.Fatalf("stored %s", got)
	}
}

func TestK8sRevocationStoreMergeRetriesStaleUpdate(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	conflicts := installOptimisticConfigMaps(cs)
	store := NewK8sRevocationStore(cs, "axiom-system")
	if err := store.Save([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	var seen []string
	err := store.Merge(func(current []byte) ([]byte, error) {
		seen = append(seen, string(current))
		if len(seen) == 1 {
			// Another replica updates the ConfigMap between our Get and Update.
			if err := store.Save([]byte(`{"ids":{"theirs":9999999999}}`)); err != nil {
				t.Fatal(err)
			}
		}
		return []byte(`{"ids":{"ours":9999999999}}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if *conflicts != 1 || len(seen) != 2 || seen[1] != `{"ids":{"theirs":9999999999}}` {
		t.Fatalf("conflicts=%d seen=%q: the stale update was not retried against the new copy", *conflicts, seen)
	}
}
