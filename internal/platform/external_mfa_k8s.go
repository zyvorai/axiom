//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	externalMFASecretName = "zyvor-axiom-external-mfa"
	externalMFASecretKey  = "records.json"
	// externalMFAMutateAttempts bounds retries on a write conflict, as for the
	// revocations ConfigMap.
	externalMFAMutateAttempts = 12
)

// K8sExternalMFABackend keeps directory users' second factors as JSON in the
// Secret zyvor-axiom-external-mfa (key records.json). It holds TOTP secrets,
// so it is a Secret, not a ConfigMap. Writes are a read-modify-write guarded
// by the Secret's resourceVersion and retried on conflict, so replicas never
// lose each other's change (or let a code be replayed on another replica).
type K8sExternalMFABackend struct {
	client kubernetes.Interface
	ns     string
}

func NewK8sExternalMFABackend(client kubernetes.Interface, ns string) *K8sExternalMFABackend {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return &K8sExternalMFABackend{client: client, ns: ns}
}

// OpenExternalMFAStore returns the Secret-backed store in kubernetes mode and an
// in-memory store otherwise.
func OpenExternalMFAStore(mode string, provider Provider) *ExternalMFAStore {
	if strings.ToLower(mode) != "kubernetes" {
		return NewMemoryExternalMFAStore()
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return NewMemoryExternalMFAStore()
	}
	return NewExternalMFAStore(NewK8sExternalMFABackend(k.client, ""))
}

func (b *K8sExternalMFABackend) decode(sec *corev1.Secret) (map[string]ExternalMFARecord, error) {
	recs := map[string]ExternalMFARecord{}
	if sec == nil {
		return recs, nil
	}
	if raw := sec.Data[externalMFASecretKey]; len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &recs); err != nil {
			return nil, fmt.Errorf("secret %s/%s: %s is not valid JSON: %w", b.ns, externalMFASecretName, externalMFASecretKey, err)
		}
	}
	return recs, nil
}

func (b *K8sExternalMFABackend) Load(ctx context.Context) (map[string]ExternalMFARecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	sec, err := b.client.CoreV1().Secrets(b.ns).Get(ctx, externalMFASecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return map[string]ExternalMFARecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	return b.decode(sec)
}

// Mutate implements ExternalMFABackend.
func (b *K8sExternalMFABackend) Mutate(ctx context.Context, fn func(map[string]ExternalMFARecord) (bool, error)) error {
	var lastErr error
	for attempt := 0; attempt < externalMFAMutateAttempts; attempt++ {
		if attempt > 0 {
			step := time.Duration(attempt) * 5 * time.Millisecond
			time.Sleep(step + time.Duration(rand.Int64N(int64(step))))
		}
		lastErr = b.mutateOnce(ctx, fn)
		if lastErr == nil || !(apierrors.IsConflict(lastErr) || apierrors.IsAlreadyExists(lastErr)) {
			return lastErr
		}
	}
	return lastErr
}

func (b *K8sExternalMFABackend) mutateOnce(ctx context.Context, fn func(map[string]ExternalMFARecord) (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	secrets := b.client.CoreV1().Secrets(b.ns)
	sec, err := secrets.Get(ctx, externalMFASecretName, metav1.GetOptions{})
	notFound := apierrors.IsNotFound(err)
	if err != nil && !notFound {
		return err
	}
	if notFound {
		sec = nil
	}
	recs, err := b.decode(sec)
	if err != nil {
		return err
	}
	changed, err := fn(recs)
	if err != nil || !changed {
		return err
	}
	raw, err := json.Marshal(recs)
	if err != nil {
		return err
	}
	if sec == nil {
		// AlreadyExists when another replica created it first: Mutate retries
		// against that copy.
		_, err = secrets.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: externalMFASecretName, Namespace: b.ns, Labels: map[string]string{"app": "zyvor-axiom"}},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{externalMFASecretKey: raw},
		}, metav1.CreateOptions{})
		return err
	}
	sec = sec.DeepCopy()
	if sec.Data == nil {
		sec.Data = map[string][]byte{}
	}
	sec.Data[externalMFASecretKey] = raw
	// sec carries the resourceVersion that was read, so the API server rejects
	// this update with a Conflict if the Secret changed since.
	_, err = secrets.Update(ctx, sec, metav1.UpdateOptions{})
	return err
}

var _ ExternalMFABackend = (*K8sExternalMFABackend)(nil)
