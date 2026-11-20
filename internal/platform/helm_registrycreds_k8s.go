//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// K8sHelmRegistryCredStore keeps every registry credential in one Secret,
// zyvor-axiom-helm-registry-creds (key credentials.json, a JSON map name ->
// {registry, username, password, createdAt}), in the Axiom namespace. Writes
// are read-modify-write guarded by the Secret's resourceVersion and retried
// on a conflict, so two replicas cannot lose each other's change. A Secret
// whose JSON does not parse is reported and never overwritten.
type K8sHelmRegistryCredStore struct {
	client kubernetes.Interface
	ns     string
	now    func() time.Time
}

func NewK8sHelmRegistryCredStore(client kubernetes.Interface, ns string) *K8sHelmRegistryCredStore {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return &K8sHelmRegistryCredStore{client: client, ns: ns, now: time.Now}
}

// OpenHelmRegistryCredStore returns the Secret store in kubernetes mode and
// an in-memory store otherwise.
func OpenHelmRegistryCredStore(mode string, provider Provider) HelmRegistryCredStore {
	if strings.ToLower(mode) != "kubernetes" {
		return NewMemoryHelmRegistryCredStore()
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return NewMemoryHelmRegistryCredStore()
	}
	return NewK8sHelmRegistryCredStore(k.client, "")
}

func (s *K8sHelmRegistryCredStore) load(ctx context.Context) (*corev1.Secret, map[string]helmRegistryCred, error) {
	sec, err := s.client.CoreV1().Secrets(s.ns).Get(ctx, HelmRegistryCredSecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, map[string]helmRegistryCred{}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read registry credentials: %w", err)
	}
	m := map[string]helmRegistryCred{}
	if raw := sec.Data[helmRegistryCredSecretKey]; len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			// The JSON error could quote part of the data: never wrap it.
			return nil, nil, fmt.Errorf("secret %s/%s: %s is not valid JSON; fix or delete it by hand (it is never overwritten)", s.ns, HelmRegistryCredSecretName, helmRegistryCredSecretKey)
		}
		if m == nil {
			m = map[string]helmRegistryCred{}
		}
	}
	return sec, m, nil
}

func (s *K8sHelmRegistryCredStore) List(ctx context.Context) ([]HelmRegistryCredInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, m, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	return helmRegistryCredInfos(m), nil
}

func (s *K8sHelmRegistryCredStore) get(ctx context.Context, name string) (helmRegistryCred, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, m, err := s.load(ctx)
	if err != nil {
		return helmRegistryCred{}, err
	}
	c, ok := m[name]
	if !ok {
		return helmRegistryCred{}, helmRegistryCredNotFound(name)
	}
	return c, nil
}

// mutate applies fn to the stored map and writes it back.
func (s *K8sHelmRegistryCredStore) mutate(ctx context.Context, fn func(map[string]helmRegistryCred) error) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	secrets := s.client.CoreV1().Secrets(s.ns)
	for attempt := 0; ; attempt++ {
		sec, m, err := s.load(ctx)
		if err != nil {
			return err
		}
		if err := fn(m); err != nil {
			return err
		}
		raw, err := json.Marshal(m)
		if err != nil {
			return err
		}
		if sec == nil {
			_, err = secrets.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: HelmRegistryCredSecretName, Namespace: s.ns,
					Labels: map[string]string{"app": "zyvor-axiom", "app.kubernetes.io/managed-by": "zyvor-axiom"},
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{helmRegistryCredSecretKey: raw},
			}, metav1.CreateOptions{})
		} else {
			sec = sec.DeepCopy()
			if sec.Data == nil {
				sec.Data = map[string][]byte{}
			}
			sec.Data[helmRegistryCredSecretKey] = raw
			// sec carries the resourceVersion that was read: a concurrent
			// writer makes this a Conflict, and the loop starts over.
			_, err = secrets.Update(ctx, sec, metav1.UpdateOptions{})
		}
		for i := range raw {
			raw[i] = 0
		}
		if (apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)) && attempt < 4 {
			continue
		}
		if err != nil {
			return fmt.Errorf("write registry credentials: %w", err)
		}
		return nil
	}
}

func (s *K8sHelmRegistryCredStore) Add(ctx context.Context, req HelmRegistryCredRequest) (HelmRegistryCredInfo, error) {
	name, c, err := validateHelmRegistryCredRequest(req, s.now())
	if err != nil {
		return HelmRegistryCredInfo{}, err
	}
	if err := s.mutate(ctx, func(m map[string]helmRegistryCred) error { return helmRegistryCredMapAdd(m, name, c) }); err != nil {
		return HelmRegistryCredInfo{}, err
	}
	return HelmRegistryCredInfo{Name: name, Registry: c.Registry, Username: c.Username, CreatedAt: c.CreatedAt}, nil
}

func (s *K8sHelmRegistryCredStore) Delete(ctx context.Context, name string) (HelmRegistryCredInfo, error) {
	if err := checkHelmRegistryCredName(name); err != nil {
		return HelmRegistryCredInfo{}, err
	}
	var gone helmRegistryCred
	err := s.mutate(ctx, func(m map[string]helmRegistryCred) error {
		c, err := helmRegistryCredMapDelete(m, name)
		gone = c
		return err
	})
	if err != nil {
		return HelmRegistryCredInfo{}, err
	}
	return HelmRegistryCredInfo{Name: name, Registry: gone.Registry, Username: gone.Username, CreatedAt: gone.CreatedAt}, nil
}
