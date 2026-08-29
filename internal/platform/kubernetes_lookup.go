//go:build kubernetes

package platform

import "context"

// CurrentKubernetesProvider unwraps a SwitchableProvider to the
// *KubernetesProvider backing its currently active cluster, or returns the
// provider itself if it already is one. Used anywhere that needs the real
// client-go client directly (e.g. the interactive terminal WebSocket, or a
// persistence layer backed by a k8s Secret/ConfigMap) rather than going
// through the generic Provider interface.
func CurrentKubernetesProvider(ctx context.Context, p Provider) (*KubernetesProvider, bool) {
	switch v := p.(type) {
	case *SwitchableProvider:
		kp, ok := v.current(ctx).(*KubernetesProvider)
		return kp, ok
	case *KubernetesProvider:
		return v, true
	default:
		return nil, false
	}
}
