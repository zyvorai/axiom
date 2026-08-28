//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const usersSecretName = "zyvor-axiom-users"

// K8sUserStore persists local accounts as a single JSON blob in a Secret,
// mirroring K8sIntegrationStore's pattern.
type K8sUserStore struct {
	client kubernetes.Interface
	ns     string
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
	var k *KubernetesProvider
	switch p := provider.(type) {
	case *SwitchableProvider:
		if kp, ok := p.current().(*KubernetesProvider); ok {
			k = kp
		}
	case *KubernetesProvider:
		k = p
	}
	if k == nil || k.client == nil {
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
		_, err = s.client.CoreV1().Secrets(s.ns).Update(ctx, sec, metav1.UpdateOptions{})
	} else {
		_, err = s.client.CoreV1().Secrets(s.ns).Create(ctx, sec, metav1.CreateOptions{})
	}
	return err
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

func (s *K8sUserStore) Create(ctx context.Context, u User) error {
	m, sec, err := s.load(ctx)
	if err != nil {
		return err
	}
	key := strings.ToLower(u.Username)
	if _, exists := m[key]; exists {
		return fmt.Errorf("user %q already exists", u.Username)
	}
	m[key] = u
	return s.save(ctx, m, sec)
}

func (s *K8sUserStore) Update(ctx context.Context, u User) error {
	m, sec, err := s.load(ctx)
	if err != nil {
		return err
	}
	key := strings.ToLower(u.Username)
	if _, exists := m[key]; !exists {
		return fmt.Errorf("user %q not found", u.Username)
	}
	m[key] = u
	return s.save(ctx, m, sec)
}

func (s *K8sUserStore) Delete(ctx context.Context, username string) error {
	m, sec, err := s.load(ctx)
	if err != nil {
		return err
	}
	key := strings.ToLower(username)
	if _, ok := m[key]; !ok {
		return fmt.Errorf("user %q not found", username)
	}
	delete(m, key)
	return s.save(ctx, m, sec)
}
