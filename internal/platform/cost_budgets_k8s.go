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

// K8sCostBudgetStore keeps the per-namespace cost budgets as JSON in the
// ConfigMap zyvor-axiom-cost-budgets (key budgets.json). Budgets are plain
// numbers, so a ConfigMap is enough.
type K8sCostBudgetStore struct {
	client kubernetes.Interface
	ns     string
}

func NewK8sCostBudgetStore(client kubernetes.Interface, ns string) *K8sCostBudgetStore {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return &K8sCostBudgetStore{client: client, ns: ns}
}

// OpenCostBudgetStore returns the ConfigMap store in kubernetes mode and an
// in-memory store otherwise.
func OpenCostBudgetStore(mode string, provider Provider) CostBudgetStore {
	if strings.ToLower(mode) != "kubernetes" {
		return NewMemoryCostBudgetStore()
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil || k.client == nil {
		return NewMemoryCostBudgetStore()
	}
	return NewK8sCostBudgetStore(k.client, "")
}

func (s *K8sCostBudgetStore) load(ctx context.Context) (*corev1.ConfigMap, []CostBudget, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.ns).Get(ctx, costBudgetConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, []CostBudget{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	budgets := []CostBudget{}
	if raw := cm.Data[costBudgetConfigMapKey]; strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &budgets); err != nil {
			return nil, nil, fmt.Errorf("configmap %s/%s: %s is not valid JSON: %w", s.ns, costBudgetConfigMapName, costBudgetConfigMapKey, err)
		}
	}
	return cm, budgets, nil
}

func (s *K8sCostBudgetStore) List(ctx context.Context) ([]CostBudget, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, budgets, err := s.load(ctx)
	return budgets, err
}

// mutate applies fn to the stored list and writes it back, retrying on a
// resourceVersion conflict so two replicas cannot lose each other's change.
func (s *K8sCostBudgetStore) mutate(ctx context.Context, fn func([]CostBudget) ([]CostBudget, error)) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for attempt := 0; ; attempt++ {
		cm, budgets, err := s.load(ctx)
		if err != nil {
			return err
		}
		next, err := fn(budgets)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(next)
		if err != nil {
			return err
		}
		if cm == nil {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: costBudgetConfigMapName, Namespace: s.ns, Labels: map[string]string{"app": "zyvor-axiom"}},
				Data:       map[string]string{costBudgetConfigMapKey: string(raw)},
			}
			_, err = s.client.CoreV1().ConfigMaps(s.ns).Create(ctx, cm, metav1.CreateOptions{})
		} else {
			cm = cm.DeepCopy()
			if cm.Data == nil {
				cm.Data = map[string]string{}
			}
			cm.Data[costBudgetConfigMapKey] = string(raw)
			_, err = s.client.CoreV1().ConfigMaps(s.ns).Update(ctx, cm, metav1.UpdateOptions{})
		}
		if (apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)) && attempt < 4 {
			continue
		}
		return err
	}
}

func (s *K8sCostBudgetStore) Set(ctx context.Context, b CostBudget) error {
	return s.mutate(ctx, func(list []CostBudget) ([]CostBudget, error) { return costBudgetListSet(list, b) })
}

func (s *K8sCostBudgetStore) Delete(ctx context.Context, ns string) error {
	return s.mutate(ctx, func(list []CostBudget) ([]CostBudget, error) { return costBudgetListDelete(list, ns) })
}
