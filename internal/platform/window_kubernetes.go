//go:build kubernetes

package platform

import (
	"context"
	"fmt"

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
