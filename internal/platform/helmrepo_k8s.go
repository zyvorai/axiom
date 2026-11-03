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

// K8sHelmRepoStore keeps the chart repository list as JSON in the ConfigMap
// zyvor-axiom-helm-repos (key repos.json). Repository URLs carry no
// credentials (query strings and userinfo are refused), so a ConfigMap is
// enough.
type K8sHelmRepoStore struct {
	client kubernetes.Interface
	ns     string
}

func NewK8sHelmRepoStore(client kubernetes.Interface, ns string) *K8sHelmRepoStore {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return &K8sHelmRepoStore{client: client, ns: ns}
}

// OpenHelmRepoStore returns the ConfigMap store in kubernetes mode and an
// in-memory store otherwise.
func OpenHelmRepoStore(mode string, provider Provider) HelmRepoStore {
	if strings.ToLower(mode) != "kubernetes" {
		return NewMemoryHelmRepoStore()
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return NewMemoryHelmRepoStore()
	}
	return NewK8sHelmRepoStore(k.client, "")
}

func (s *K8sHelmRepoStore) load(ctx context.Context) (*corev1.ConfigMap, []HelmRepo, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.ns).Get(ctx, helmRepoConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, []HelmRepo{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	repos := []HelmRepo{}
	if raw := cm.Data[helmRepoConfigMapKey]; strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &repos); err != nil {
			return nil, nil, fmt.Errorf("configmap %s/%s: %s is not valid JSON: %w", s.ns, helmRepoConfigMapName, helmRepoConfigMapKey, err)
		}
	}
	return cm, repos, nil
}

func (s *K8sHelmRepoStore) List(ctx context.Context) ([]HelmRepo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, repos, err := s.load(ctx)
	return repos, err
}

// mutate applies fn to the stored list and writes it back, retrying on a
// resourceVersion conflict so two replicas cannot lose each other's change.
func (s *K8sHelmRepoStore) mutate(ctx context.Context, fn func([]HelmRepo) ([]HelmRepo, error)) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for attempt := 0; ; attempt++ {
		cm, repos, err := s.load(ctx)
		if err != nil {
			return err
		}
		next, err := fn(repos)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(next)
		if err != nil {
			return err
		}
		if cm == nil {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: helmRepoConfigMapName, Namespace: s.ns, Labels: map[string]string{"app": "zyvor-axiom"}},
				Data:       map[string]string{helmRepoConfigMapKey: string(raw)},
			}
			_, err = s.client.CoreV1().ConfigMaps(s.ns).Create(ctx, cm, metav1.CreateOptions{})
		} else {
			cm = cm.DeepCopy()
			if cm.Data == nil {
				cm.Data = map[string]string{}
			}
			cm.Data[helmRepoConfigMapKey] = string(raw)
			_, err = s.client.CoreV1().ConfigMaps(s.ns).Update(ctx, cm, metav1.UpdateOptions{})
		}
		if (apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)) && attempt < 4 {
			continue
		}
		return err
	}
}

func (s *K8sHelmRepoStore) Add(ctx context.Context, repo HelmRepo) error {
	return s.mutate(ctx, func(list []HelmRepo) ([]HelmRepo, error) { return helmRepoListAdd(list, repo) })
}

func (s *K8sHelmRepoStore) Remove(ctx context.Context, name string) error {
	return s.mutate(ctx, func(list []HelmRepo) ([]HelmRepo, error) { return helmRepoListRemove(list, name) })
}
