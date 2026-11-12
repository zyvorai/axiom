//go:build kubernetes

package platform

import (
	"context"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/zyvorai/axiom/internal/auth"
)

const (
	revocationsConfigMapName = "zyvor-axiom-revocations"
	revocationsConfigMapKey  = "revocations.json"

	// podProxyRedeemedConfigMapName holds the ids of shared pod-proxy tickets
	// already redeemed (auth.PodProxyRedeemed), so a ticket redeemed on one
	// replica is refused on the others. Entries live for a ticket's 60 s.
	podProxyRedeemedConfigMapName = "zyvor-axiom-pod-proxy-redeemed"
	podProxyRedeemedConfigMapKey  = "redeemed.json"

	samlRequestConfigMapName = "zyvor-axiom-saml-authn"
	samlRequestConfigMapKey  = "saml-authn.json"

	streamTicketConfigMapName = "zyvor-axiom-stream-tickets"
	streamTicketConfigMapKey  = "tickets.json"

	loginThrottleConfigMapName = "zyvor-axiom-login-throttle"
	loginThrottleConfigMapKey  = "throttle.json"
)

// K8sRevocationStore keeps the session-revocation snapshot in a ConfigMap so a
// logout or "sign out everywhere" survives a restart. The snapshot is bounded
// (see auth.Revocations) and holds only random session IDs and usernames.
// The same ConfigMap-backed blob store, under another name, holds the shared
// set of redeemed pod-proxy tickets (NewK8sPodProxyRedeemedStore).
type K8sRevocationStore struct {
	client kubernetes.Interface
	ns     string
	name   string // ConfigMap name
	key    string // data key
}

func NewK8sRevocationStore(client kubernetes.Interface, ns string) *K8sRevocationStore {
	return newK8sBlobStore(client, ns, revocationsConfigMapName, revocationsConfigMapKey)
}

// NewK8sPodProxyRedeemedStore is the ConfigMap store of redeemed pod-proxy
// ticket ids.
func NewK8sPodProxyRedeemedStore(client kubernetes.Interface, ns string) *K8sRevocationStore {
	return newK8sBlobStore(client, ns, podProxyRedeemedConfigMapName, podProxyRedeemedConfigMapKey)
}

func newK8sBlobStore(client kubernetes.Interface, ns, name, key string) *K8sRevocationStore {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return &K8sRevocationStore{client: client, ns: ns, name: name, key: key}
}

// OpenPodProxyRedeemedStore returns the shared redeemed-ticket store in
// kubernetes mode and nil (this replica only) otherwise.
func OpenPodProxyRedeemedStore(mode string, provider Provider) auth.RevocationStore {
	if strings.ToLower(mode) != "kubernetes" {
		return nil
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return nil
	}
	return NewK8sPodProxyRedeemedStore(k.client, "")
}

// OpenSAMLRequestStore shares outstanding SAML AuthnRequest IDs across replicas.
func OpenSAMLRequestStore(mode string, provider Provider) auth.RevocationStore {
	if strings.ToLower(mode) != "kubernetes" {
		return nil
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return nil
	}
	return newK8sBlobStore(k.client, "", samlRequestConfigMapName, samlRequestConfigMapKey)
}

// OpenStreamTicketStore shares outstanding WebSocket/SSE tickets across replicas.
func OpenStreamTicketStore(mode string, provider Provider) auth.RevocationStore {
	if strings.ToLower(mode) != "kubernetes" {
		return nil
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return nil
	}
	return newK8sBlobStore(k.client, "", streamTicketConfigMapName, streamTicketConfigMapKey)
}

// OpenLoginThrottleStore shares failed-login counters across replicas.
func OpenLoginThrottleStore(mode string, provider Provider) auth.RevocationStore {
	if strings.ToLower(mode) != "kubernetes" {
		return nil
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return nil
	}
	return newK8sBlobStore(k.client, "", loginThrottleConfigMapName, loginThrottleConfigMapKey)
}

// OpenRevocationStore returns the persistent store in kubernetes mode and nil
// (in-memory only) otherwise.
func OpenRevocationStore(mode string, provider Provider) auth.RevocationStore {
	if strings.ToLower(mode) != "kubernetes" {
		return nil
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return nil
	}
	return NewK8sRevocationStore(k.client, "")
}

func (s *K8sRevocationStore) Load() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cm, err := s.client.CoreV1().ConfigMaps(s.ns).Get(ctx, s.name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(cm.Data[s.key]), nil
}

func (s *K8sRevocationStore) Save(raw []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: s.name, Namespace: s.ns, Labels: map[string]string{"app": "zyvor-axiom"}},
		Data:       map[string]string{s.key: string(raw)},
	}
	if _, err := s.client.CoreV1().ConfigMaps(s.ns).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			_, err = s.client.CoreV1().ConfigMaps(s.ns).Create(ctx, cm, metav1.CreateOptions{})
			return err
		}
		return err
	}
	return nil
}

// revocationMergeAttempts bounds Merge's retries on a write conflict; each
// retry waits a short, growing, jittered delay so racing replicas spread out.
const revocationMergeAttempts = 12

// Merge implements auth.MergingRevocationStore: a read-modify-write guarded
// by the ConfigMap's resourceVersion, retried when another replica wrote in
// between, so concurrent revocations on different replicas are all kept.
func (s *K8sRevocationStore) Merge(fn func(current []byte) ([]byte, error)) error {
	var lastErr error
	for attempt := 0; attempt < revocationMergeAttempts; attempt++ {
		if attempt > 0 {
			step := time.Duration(attempt) * 5 * time.Millisecond
			time.Sleep(step + time.Duration(rand.Int64N(int64(step))))
		}
		lastErr = s.mergeOnce(fn)
		if lastErr == nil || !(apierrors.IsConflict(lastErr) || apierrors.IsAlreadyExists(lastErr)) {
			return lastErr
		}
	}
	return lastErr
}

func (s *K8sRevocationStore) mergeOnce(fn func(current []byte) ([]byte, error)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cms := s.client.CoreV1().ConfigMaps(s.ns)
	cm, err := cms.Get(ctx, s.name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		raw, err := fn(nil)
		if err != nil {
			return err
		}
		// Create fails with AlreadyExists if another replica created it first;
		// Merge then retries against that copy.
		_, err = cms.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: s.name, Namespace: s.ns, Labels: map[string]string{"app": "zyvor-axiom"}},
			Data:       map[string]string{s.key: string(raw)},
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	raw, err := fn([]byte(cm.Data[s.key]))
	if err != nil {
		return err
	}
	cm = cm.DeepCopy()
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[s.key] = string(raw)
	// cm carries the resourceVersion that was read, so the API server rejects
	// this update with a Conflict if the ConfigMap changed since.
	_, err = cms.Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

var _ auth.MergingRevocationStore = (*K8sRevocationStore)(nil)
