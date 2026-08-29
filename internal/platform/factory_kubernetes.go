//go:build kubernetes

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func NewProvider(mode, kubeconfig, clusterName string) (Provider, error) {
	if mode == "demo" {
		return NewDemoSwitchableProvider(), nil
	}
	return newKubernetesClusterRegistry(kubeconfig, clusterName)
}

// newKubernetesClusterRegistry builds a SwitchableProvider for kubernetes
// mode. Running in-cluster (the normal production deploy, via ServiceAccount
// token) has no notion of "other contexts", so exactly one cluster is
// registered — labeled by clusterName when set, so operators can give their
// single real cluster a meaningful name instead of a generic placeholder.
// When a kubeconfig file with multiple contexts is available (e.g. running
// the binary directly against ~/.kube/config), one provider is registered
// per context and switching between them is real.
func newKubernetesClusterRegistry(kubeconfigPath, clusterName string) (*SwitchableProvider, error) {
	sp := NewSwitchableProvider()
	clusterName = strings.TrimSpace(clusterName)

	if _, err := rest.InClusterConfig(); err == nil {
		p, err := NewKubernetesProvider(kubeconfigPath)
		if err != nil {
			return nil, err
		}
		name := clusterName
		if name == "" {
			name = "in-cluster"
		}
		sp.Register(name, name, p, "in-cluster")
		return sp, nil
	}

	path := kubeconfigPath
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".kube", "config")
		}
	}
	raw, err := clientcmd.LoadFromFile(path)
	if err != nil || len(raw.Contexts) == 0 {
		p, perr := NewKubernetesProvider(kubeconfigPath)
		if perr != nil {
			return nil, perr
		}
		name := clusterName
		if name == "" {
			name = "default"
		}
		sp.Register(name, name, p, "")
		return sp, nil
	}

	for name := range raw.Contexts {
		cfg, err := clientcmd.NewNonInteractiveClientConfig(*raw, name, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
		if err != nil {
			continue
		}
		cfg.UserAgent = "zyvor-axiom/0.1"
		client, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			continue
		}
		dyn, err := dynamic.NewForConfig(cfg)
		if err != nil {
			continue
		}
		label := name
		if clusterName != "" && name == raw.CurrentContext {
			label = clusterName
		}
		sp.Register(name, label, &KubernetesProvider{client: client, config: cfg, dynamic: dyn, clusterKey: name}, cfg.Host)
	}
	if sp.Count() == 0 {
		return nil, fmt.Errorf("no usable contexts found in kubeconfig %q", path)
	}
	if raw.CurrentContext != "" {
		_ = sp.SetActive(raw.CurrentContext)
	}
	return sp, nil
}
