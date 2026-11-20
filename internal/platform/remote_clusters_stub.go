//go:build !kubernetes

package platform

import (
	"context"
	"fmt"
	"strings"
)

// ProviderFromKubeconfigYAML registers a demo stand-in when not built with the
// kubernetes tag — useful so the Settings → Add cluster UI works in lab/demo.
func ProviderFromKubeconfigYAML(kubeconfigYAML, name string) (Provider, string, error) {
	if strings.TrimSpace(kubeconfigYAML) == "" {
		return nil, "", fmt.Errorf("kubeconfig is required")
	}
	if strings.TrimSpace(name) == "" {
		return nil, "", fmt.Errorf("name is required")
	}
	p := NewDemoProviderForCluster(name)
	server := "demo://" + name
	return p, server, nil
}

func OpenRemoteClusterStore(mode string, provider Provider) RemoteClusterStore {
	return NewMemoryRemoteClusterStore()
}

// NewDemoProviderForCluster returns an isolated demo provider labeled for a
// remote cluster registration in demo builds.
func NewDemoProviderForCluster(name string) Provider {
	_ = context.Background()
	return NewDemoProvider()
}
