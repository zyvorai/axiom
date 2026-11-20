//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (k *KubernetesProvider) PodProxyTarget(ctx context.Context, ns, podName string, port int32) (string, error) {
	p, err := k.client.CoreV1().Pods(ns).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if p.Status.PodIP == "" {
		return "", fmt.Errorf("pod has no IP yet")
	}
	if p.Spec.HostNetwork {
		// A hostNetwork pod's IP is the node's: proxying would reach any
		// node-local service (kubelet, etcd, CNI debug ports).
		return "", NewStatusError(http.StatusForbidden, fmt.Errorf("pod uses the host network and cannot be proxied"))
	}
	declared := false
	for _, c := range p.Spec.Containers {
		for _, cp := range c.Ports {
			if cp.ContainerPort == port {
				declared = true
			}
		}
	}
	if !declared {
		return "", NewStatusError(http.StatusForbidden, fmt.Errorf("port %d is not a declared container port of this pod", port))
	}
	return fmt.Sprintf("http://%s:%d", p.Status.PodIP, port), nil
}

func (k *KubernetesProvider) WorkloadVolumes(ctx context.Context, ns, workload string) ([]string, error) {
	pods, err := k.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, p := range pods.Items {
		wl := workloadNameFromPod(p)
		if wl != workload {
			continue
		}
		for _, vol := range p.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				name := vol.PersistentVolumeClaim.ClaimName
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					out = append(out, name)
				}
			}
		}
	}
	return out, nil
}
