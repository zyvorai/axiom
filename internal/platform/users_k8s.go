//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const usersSecretName = "zyvor-axiom-users"

// K8sUserStore persists local accounts as a single JSON blob in a Secret,
// mirroring K8sIntegrationStore's pattern.
//
// writeMu serializes Create/Update/Delete in-process. Verified empirically
// against the real cluster: even a generous retry-on-conflict budget
// (userStoreRetryBackoff) doesn't *guarantee* success under enough
// concurrent writers — at 16 concurrent account creations, one still
// exhausted its retries and failed outright. User management is
// low-frequency, so serializing writes from this process has no
// meaningful cost, and it turns "very unlikely to fail" into "cannot fail
// from this process's own concurrency" — retry-on-conflict stays in place
// underneath as defense-in-depth against an external actor (another
// replica, a direct kubectl edit) touching the same Secret.
type K8sUserStore struct {
	client  kubernetes.Interface
	ns      string
	writeMu sync.Mutex
}

func NewK8sUserStore(client kubernetes.Interface, ns string) *K8sUserStore {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return &K8sUserStore{client: client, ns: ns}
}

func OpenUserStore(mode string, provider Provider) UserStore {
	if strings.ToLower(mode) != "kubernetes" {
		return NewMemoryUserStore()
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return NewMemoryUserStore()
	}
	return NewK8sUserStore(k.client, "")
}

func (s *K8sUserStore) load(ctx context.Context) (map[string]User, *corev1.Secret, error) {
	sec, err := s.client.CoreV1().Secrets(s.ns).Get(ctx, usersSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return map[string]User{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	out := map[string]User{}
	if raw, ok := sec.Data["users.json"]; ok {
		_ = json.Unmarshal(raw, &out)
	}
	return out, sec, nil
}

func (s *K8sUserStore) save(ctx context.Context, users map[string]User, existing *corev1.Secret) error {
	raw, err := json.Marshal(users)
	if err != nil {
		return err
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: usersSecretName, Namespace: s.ns, Labels: map[string]string{"app": "zyvor-axiom"}},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"users.json": raw},
	}
	if existing != nil {
		// Carry the fetched object's ResourceVersion so the API server can
		// actually detect a conflicting concurrent write instead of always
		// accepting an unconditional overwrite.
		sec.ResourceVersion = existing.ResourceVersion
		_, err = s.client.CoreV1().Secrets(s.ns).Update(ctx, sec, metav1.UpdateOptions{})
	} else {
		_, err = s.client.CoreV1().Secrets(s.ns).Create(ctx, sec, metav1.CreateOptions{})
	}
	return err
}

// userStoreRetryBackoff is more generous than retry.DefaultRetry (5 steps,
// flat 10ms) — verified empirically against the real cluster that the
// default budget isn't always enough: 8 concurrent account creations still
// produced one real, unretried "the object has been modified" error under
// it. User management is a low-frequency admin action, so trading a bit
// more latency under real contention for not failing outright is the right
// call here.
var userStoreRetryBackoff = wait.Backoff{Steps: 10, Duration: 20 * time.Millisecond, Factor: 1.5, Jitter: 0.2}

// retryOnConflictOrExists retries fn against the latest state on either a
// resourceVersion conflict (a concurrent Update raced ahead) or an
// AlreadyExists error (two concurrent calls both found the Secret missing
// and both tried to Create it — only one can win; the loser should retry
// and fall through to Update once it re-loads and sees the Secret now
// exists). retry.RetryOnConflict alone only covers the first case.
func retryOnConflictOrExists(fn func() error) error {
	return retry.OnError(userStoreRetryBackoff, func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, fn)
}

func (s *K8sUserStore) List(ctx context.Context) ([]User, error) {
	m, _, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(m))
	for _, u := range m {
		out = append(out, u)
	}
	return out, nil
}

// Create/Update/Delete all follow the same load-modify-save cycle against a
// single shared Secret. Two concurrent calls (e.g. two admins creating
// different accounts at the same time) can both load the same starting
// state before either saves — the second write then either finds the
// Secret already created (if neither existed yet) or races an Update
// against a now-stale resourceVersion — even though there's no actual
// business-logic conflict (different usernames). retryOnConflictOrExists
// re-runs the whole cycle against the latest state in either case, instead
// of letting one of two logically-independent requests fail outright.
func (s *K8sUserStore) Create(ctx context.Context, u User) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	key := strings.ToLower(u.Username)
	return retryOnConflictOrExists(func() error {
		m, sec, err := s.load(ctx)
		if err != nil {
			return err
		}
		if _, exists := m[key]; exists {
			return fmt.Errorf("user %q already exists", u.Username)
		}
		m[key] = u
		return s.save(ctx, m, sec)
	})
}

func (s *K8sUserStore) Update(ctx context.Context, u User) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	key := strings.ToLower(u.Username)
	return retryOnConflictOrExists(func() error {
		m, sec, err := s.load(ctx)
		if err != nil {
			return err
		}
		if _, exists := m[key]; !exists {
			return fmt.Errorf("user %q not found", u.Username)
		}
		m[key] = u
		return s.save(ctx, m, sec)
	})
}

func (s *K8sUserStore) Delete(ctx context.Context, username string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	key := strings.ToLower(username)
	return retryOnConflictOrExists(func() error {
		m, sec, err := s.load(ctx)
		if err != nil {
			return err
		}
		if _, ok := m[key]; !ok {
			return fmt.Errorf("user %q not found", username)
		}
		delete(m, key)
		return s.save(ctx, m, sec)
	})
}
