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

// K8sAgentRunStore persists Axiom-owned agent runs when the AgentRun CRD is absent.
type K8sAgentRunStore struct {
	client kubernetes.Interface
	ns     string
}

func NewK8sAgentRunStore(client kubernetes.Interface, ns string) *K8sAgentRunStore {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	}
	if ns == "" {
		ns = "axiom-system"
	}
	return &K8sAgentRunStore{client: client, ns: ns}
}

func (s *K8sAgentRunStore) load(ctx context.Context) (*corev1.ConfigMap, []AgentRun, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.ns).Get(ctx, agentRunConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, []AgentRun{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	runs := []AgentRun{}
	if raw := cm.Data[agentRunConfigMapKey]; strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &runs); err != nil {
			return nil, nil, fmt.Errorf("configmap %s/%s: invalid JSON: %w", s.ns, agentRunConfigMapName, err)
		}
	}
	return cm, runs, nil
}

func (s *K8sAgentRunStore) mutate(ctx context.Context, fn func([]AgentRun) ([]AgentRun, error)) ([]AgentRun, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for attempt := 0; ; attempt++ {
		cm, runs, err := s.load(ctx)
		if err != nil {
			return nil, err
		}
		next, err := fn(runs)
		if err != nil {
			return nil, err
		}
		if len(next) > agentRunMaxStored {
			next = next[len(next)-agentRunMaxStored:]
		}
		raw, err := json.Marshal(next)
		if err != nil {
			return nil, err
		}
		if cm == nil {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRunConfigMapName,
					Namespace: s.ns,
					Labels:    map[string]string{"app": "zyvor-axiom"},
				},
				Data: map[string]string{agentRunConfigMapKey: string(raw)},
			}
			_, err = s.client.CoreV1().ConfigMaps(s.ns).Create(ctx, cm, metav1.CreateOptions{})
		} else {
			cm = cm.DeepCopy()
			if cm.Data == nil {
				cm.Data = map[string]string{}
			}
			cm.Data[agentRunConfigMapKey] = string(raw)
			_, err = s.client.CoreV1().ConfigMaps(s.ns).Update(ctx, cm, metav1.UpdateOptions{})
		}
		if (apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)) && attempt < 4 {
			continue
		}
		if err != nil {
			return nil, err
		}
		return next, nil
	}
}

func (s *K8sAgentRunStore) List(ctx context.Context, ns string) ([]AgentRun, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, runs, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AgentRun, 0, len(runs))
	for _, r := range runs {
		if ns == "" || ns == "all" || r.Namespace == ns {
			cp := r
			agentRunDecideFlags(&cp)
			out = append(out, cp)
		}
	}
	return out, nil
}

func (s *K8sAgentRunStore) Get(ctx context.Context, ns, name string) (AgentRun, error) {
	list, err := s.List(ctx, ns)
	if err != nil {
		return AgentRun{}, err
	}
	for _, r := range list {
		if r.Namespace == ns && r.Name == name {
			return r, nil
		}
	}
	return AgentRun{}, NewStatusError(404, fmt.Errorf("agent run %s/%s not found", ns, name))
}

func (s *K8sAgentRunStore) Create(ctx context.Context, run AgentRun) (AgentRun, error) {
	var created AgentRun
	_, err := s.mutate(ctx, func(list []AgentRun) ([]AgentRun, error) {
		for _, existing := range list {
			if existing.Namespace == run.Namespace && existing.Name == run.Name {
				return nil, NewStatusError(409, fmt.Errorf("agent run %s/%s already exists", run.Namespace, run.Name))
			}
		}
		agentRunDecideFlags(&run)
		created = run
		return append(list, run), nil
	})
	return created, err
}

func (s *K8sAgentRunStore) Update(ctx context.Context, run AgentRun) (AgentRun, error) {
	var updated AgentRun
	_, err := s.mutate(ctx, func(list []AgentRun) ([]AgentRun, error) {
		for i, existing := range list {
			if existing.Namespace == run.Namespace && existing.Name == run.Name {
				agentRunDecideFlags(&run)
				list[i] = run
				updated = run
				return list, nil
			}
		}
		return nil, NewStatusError(404, fmt.Errorf("agent run %s/%s not found", run.Namespace, run.Name))
	})
	return updated, err
}
