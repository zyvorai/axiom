//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const remoteClustersSecret = "zyvor-axiom-remote-clusters"

// ProviderFromKubeconfigYAML builds a live KubernetesProvider from a kubeconfig document.
func ProviderFromKubeconfigYAML(kubeconfigYAML, name string) (Provider, string, error) {
	if strings.TrimSpace(kubeconfigYAML) == "" {
		return nil, "", fmt.Errorf("kubeconfig is required")
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfigYAML))
	if err != nil {
		return nil, "", fmt.Errorf("invalid kubeconfig: %w", err)
	}
	cfg.UserAgent = "zyvor-axiom/0.1"
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, "", err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, "", err
	}
	key := name
	if key == "" {
		key = cfg.Host
	}
	return &KubernetesProvider{client: client, config: cfg, dynamic: dyn, clusterKey: key}, cfg.Host, nil
}

func OpenRemoteClusterStore(mode string, provider Provider) RemoteClusterStore {
	if mode == "demo" {
		return NewMemoryRemoteClusterStore()
	}
	kp, ok := underlyingKubernetes(provider)
	if !ok || kp == nil || kp.client == nil {
		return NewMemoryRemoteClusterStore()
	}
	ns := axiomNamespace()
	return &k8sRemoteClusterStore{client: kp.client, namespace: ns}
}

func underlyingKubernetes(p Provider) (*KubernetesProvider, bool) {
	switch t := p.(type) {
	case *KubernetesProvider:
		return t, true
	case *SwitchableProvider:
		t.mu.RLock()
		defer t.mu.RUnlock()
		if e := t.entries[t.defaultActive]; e != nil {
			if kp, ok := e.provider.(*KubernetesProvider); ok {
				return kp, true
			}
		}
		for _, e := range t.entries {
			if kp, ok := e.provider.(*KubernetesProvider); ok {
				return kp, true
			}
		}
	}
	return nil, false
}

func axiomNamespace() string {
	if ns := strings.TrimSpace(getenv("AXIOM_NAMESPACE")); ns != "" {
		return ns
	}
	if ns := strings.TrimSpace(getenv("POD_NAMESPACE")); ns != "" {
		return ns
	}
	return "axiom-system"
}

type k8sRemoteClusterStore struct {
	client    kubernetes.Interface
	namespace string
}

func (s *k8sRemoteClusterStore) List(ctx context.Context) ([]RemoteCluster, error) {
	sec, err := s.client.CoreV1().Secrets(s.namespace).Get(ctx, remoteClustersSecret, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	raw := sec.Data["clusters.json"]
	if len(raw) == 0 {
		return nil, nil
	}
	var list []RemoteCluster
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *k8sRemoteClusterStore) Upsert(ctx context.Context, c RemoteCluster) error {
	list, err := s.List(ctx)
	if err != nil {
		// Never fall through with an empty list: save() would overwrite the
		// Secret with just this one cluster and silently drop the rest.
		return err
	}
	found := false
	for i := range list {
		if list[i].Name == c.Name {
			list[i] = c
			found = true
			break
		}
	}
	if !found {
		list = append(list, c)
	}
	return s.save(ctx, list)
}

func (s *k8sRemoteClusterStore) Delete(ctx context.Context, name string) error {
	list, err := s.List(ctx)
	if err != nil {
		return err
	}
	out := list[:0]
	for _, c := range list {
		if c.Name != name {
			out = append(out, c)
		}
	}
	return s.save(ctx, out)
}

func (s *k8sRemoteClusterStore) save(ctx context.Context, list []RemoteCluster) error {
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	sec, err := s.client.CoreV1().Secrets(s.namespace).Get(ctx, remoteClustersSecret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = s.client.CoreV1().Secrets(s.namespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: remoteClustersSecret, Namespace: s.namespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"clusters.json": raw},
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if sec.Data == nil {
		sec.Data = map[string][]byte{}
	}
	sec.Data["clusters.json"] = raw
	_, err = s.client.CoreV1().Secrets(s.namespace).Update(ctx, sec, metav1.UpdateOptions{})
	return err
}

func getenv(k string) string {
	// tiny indirection so tests can avoid importing os in every path
	return getenvOS(k)
}
