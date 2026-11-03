//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/zyvorai/axiom/internal/auth"
)

// installOptimisticSecrets makes the fake clientset enforce resourceVersion on
// the external-MFA Secret like the API server: a stale Update gets 409 and a
// second Create gets AlreadyExists (the stock fake accepts any Update).
func installOptimisticSecrets(cs *k8sfake.Clientset) (conflicts *int, stored func() *corev1.Secret) {
	var (
		mu  sync.Mutex
		cur *corev1.Secret
		rv  int
		n   int
	)
	gr := schema.GroupResource{Resource: "secrets"}
	cs.PrependReactor("*", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		defer mu.Unlock()
		switch action.GetVerb() {
		case "get":
			a := action.(clienttesting.GetAction)
			if cur == nil || a.GetName() != externalMFASecretName {
				return true, nil, apierrors.NewNotFound(gr, a.GetName())
			}
			return true, cur.DeepCopy(), nil
		case "create":
			if cur != nil {
				n++
				return true, nil, apierrors.NewAlreadyExists(gr, externalMFASecretName)
			}
			s := action.(clienttesting.CreateAction).GetObject().(*corev1.Secret).DeepCopy()
			rv++
			s.ResourceVersion = strconv.Itoa(rv)
			cur = s
			return true, s.DeepCopy(), nil
		case "update":
			s := action.(clienttesting.UpdateAction).GetObject().(*corev1.Secret).DeepCopy()
			if cur == nil {
				return true, nil, apierrors.NewNotFound(gr, s.Name)
			}
			if s.ResourceVersion != cur.ResourceVersion {
				n++
				return true, nil, apierrors.NewConflict(gr, s.Name, fmt.Errorf("stale resourceVersion %s", s.ResourceVersion))
			}
			rv++
			s.ResourceVersion = strconv.Itoa(rv)
			cur = s
			return true, s.DeepCopy(), nil
		}
		return false, nil, nil
	})
	return &n, func() *corev1.Secret {
		mu.Lock()
		defer mu.Unlock()
		if cur == nil {
			return nil
		}
		return cur.DeepCopy()
	}
}

func recordNames(s *ExternalMFAStore) string {
	var names []string
	for _, v := range s.List() {
		names = append(names, v.Username)
	}
	return strings.Join(names, ",")
}

func TestK8sExternalMFAStoreLifecycle(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	_, stored := installOptimisticSecrets(cs)
	exerciseExternalMFAStore(t, NewExternalMFAStore(NewK8sExternalMFABackend(cs, "axiom-system")))
	sec := stored()
	if sec == nil || sec.Namespace != "axiom-system" || sec.Type != corev1.SecretTypeOpaque {
		t.Fatalf("stored secret: %+v", sec)
	}
	if _, ok := sec.Data[externalMFASecretKey]; !ok {
		t.Fatalf("no %s key: %v", externalMFASecretKey, sec.Data)
	}
	// It is a Secret: no ConfigMap was written.
	cms, _ := cs.CoreV1().ConfigMaps("axiom-system").List(context.Background(), metav1.ListOptions{})
	if len(cms.Items) != 0 {
		t.Fatalf("a ConfigMap was written: %v", cms.Items)
	}
}

func TestK8sExternalMFAStorePersistsAcrossRestart(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	_, stored := installOptimisticSecrets(cs)
	ctx := context.Background()
	now := time.Now()
	a := NewExternalMFAStore(NewK8sExternalMFABackend(cs, "axiom-system"))
	secret, _ := a.BeginMFA(ctx, "carol")
	code, _ := auth.TOTPCodeAt(secret, now)
	if _, err := a.EnableMFA(ctx, "carol", code, now); err != nil {
		t.Fatal(err)
	}
	b := NewExternalMFAStore(NewK8sExternalMFABackend(cs, "axiom-system"))
	if !b.MFAEnrolled("carol") {
		t.Fatal("a fresh store does not see the record")
	}
	var recs map[string]ExternalMFARecord
	if err := json.Unmarshal(stored().Data[externalMFASecretKey], &recs); err != nil {
		t.Fatal(err)
	}
	r := recs["carol"]
	if !r.Enabled || r.Secret != secret || len(r.RecoveryHashes) != 10 || r.LastStep == 0 || r.UpdatedAt.IsZero() || r.Username != "carol" {
		t.Fatalf("record: %+v", r)
	}
	// The code used at enrolment is refused by the other replica too.
	if b.VerifySecondFactor("carol", code, now) {
		t.Fatal("replay accepted by another replica")
	}
}

// Another replica creates the Secret between our read (not found) and our
// Create: the AlreadyExists is retried against its copy and nothing is lost.
func TestK8sExternalMFABackendRetriesAlreadyExists(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	conflicts, _ := installOptimisticSecrets(cs)
	ctx := context.Background()
	other := NewExternalMFAStore(NewK8sExternalMFABackend(cs, "axiom-system"))
	backend := NewK8sExternalMFABackend(cs, "axiom-system")
	calls := 0
	err := backend.Mutate(ctx, func(recs map[string]ExternalMFARecord) (bool, error) {
		calls++
		if calls == 1 {
			if len(recs) != 0 {
				t.Fatalf("no Secret yet, got %v", recs)
			}
			if _, err := other.BeginMFA(ctx, "dan"); err != nil {
				t.Fatal(err)
			}
		}
		recs["erin"] = ExternalMFARecord{Username: "erin", Pending: "X"}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || *conflicts != 1 {
		t.Fatalf("calls=%d conflicts=%d: AlreadyExists was not retried", calls, *conflicts)
	}
	if got := recordNames(NewExternalMFAStore(backend)); got != "dan,erin" {
		t.Fatalf("a replica's write was lost: %s", got)
	}
}

// Another replica updates the Secret between our Get and Update: the stale
// Update gets 409 and is retried against the new copy.
func TestK8sExternalMFABackendRetriesStaleUpdate(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	conflicts, _ := installOptimisticSecrets(cs)
	ctx := context.Background()
	other := NewExternalMFAStore(NewK8sExternalMFABackend(cs, "axiom-system"))
	if _, err := other.BeginMFA(ctx, "fred"); err != nil {
		t.Fatal(err)
	}
	backend := NewK8sExternalMFABackend(cs, "axiom-system")
	seen := 0
	err := backend.Mutate(ctx, func(recs map[string]ExternalMFARecord) (bool, error) {
		seen++
		if seen == 1 {
			if _, err := other.BeginMFA(ctx, "gail"); err != nil {
				t.Fatal(err)
			}
		} else if _, ok := recs["gail"]; !ok {
			t.Fatalf("retry did not read the new copy: %v", recs)
		}
		recs["hank"] = ExternalMFARecord{Username: "hank", Pending: "X"}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if *conflicts != 1 || seen != 2 {
		t.Fatalf("conflicts=%d calls=%d: the stale update was not retried", *conflicts, seen)
	}
	if got := recordNames(NewExternalMFAStore(backend)); got != "fred,gail,hank" {
		t.Fatalf("lost an update: %s", got)
	}
}

// An unchanged mutation (e.g. a code for an account without a factor) writes
// nothing.
func TestK8sExternalMFABackendNoWriteWhenUnchanged(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	installOptimisticSecrets(cs)
	s := NewExternalMFAStore(NewK8sExternalMFABackend(cs, "axiom-system"))
	if s.VerifySecondFactor("nobody", "123456", time.Now()) {
		t.Fatal("accepted a code for nobody")
	}
	for _, a := range cs.Actions() {
		if v := a.GetVerb(); v == "create" || v == "update" {
			t.Fatalf("unexpected write: %v", a)
		}
	}
}

func TestK8sExternalMFAStoreConcurrentWritersLoseNothing(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	conflicts, _ := installOptimisticSecrets(cs)
	ctx := context.Background()
	stores := []*ExternalMFAStore{
		NewExternalMFAStore(NewK8sExternalMFABackend(cs, "axiom-system")),
		NewExternalMFAStore(NewK8sExternalMFABackend(cs, "axiom-system")),
	}
	const n = 10
	var wg sync.WaitGroup
	for i, s := range stores {
		wg.Add(1)
		go func(i int, s *ExternalMFAStore) {
			defer wg.Done()
			for j := 0; j < n; j++ {
				if _, err := s.BeginMFA(ctx, fmt.Sprintf("u%d-%d", i, j)); err != nil {
					t.Errorf("begin: %v", err)
				}
			}
		}(i, s)
	}
	wg.Wait()
	fresh := NewExternalMFAStore(NewK8sExternalMFABackend(cs, "axiom-system"))
	if got := len(fresh.List()); got != 2*n {
		t.Fatalf("records: %d want %d (conflicts %d)", got, 2*n, *conflicts)
	}
	t.Logf("conflicts retried: %d", *conflicts)
}
