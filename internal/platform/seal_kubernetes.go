//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (k *KubernetesProvider) workloadSelector(ctx context.Context, ns, kind, name string) (map[string]string, error) {
	switch strings.ToLower(kind) {
	case "deployment":
		obj, err := k.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if obj.Spec.Selector != nil && len(obj.Spec.Selector.MatchLabels) > 0 {
			return obj.Spec.Selector.MatchLabels, nil
		}
		return map[string]string{"app.kubernetes.io/name": name}, nil
	case "statefulset":
		obj, err := k.client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if obj.Spec.Selector != nil && len(obj.Spec.Selector.MatchLabels) > 0 {
			return obj.Spec.Selector.MatchLabels, nil
		}
		return map[string]string{"app.kubernetes.io/name": name}, nil
	default:
		return nil, fmt.Errorf("unsupported workload kind %q", kind)
	}
}

func (k *KubernetesProvider) setWorkloadAnnotation(ctx context.Context, ns, kind, name, key, value string) error {
	patch := map[string]any{"metadata": map[string]any{"annotations": map[string]string{key: value}}}
	raw, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	switch strings.ToLower(kind) {
	case "deployment":
		_, err := k.client.AppsV1().Deployments(ns).Patch(ctx, name, types.MergePatchType, raw, metav1.PatchOptions{})
		return err
	case "statefulset":
		_, err := k.client.AppsV1().StatefulSets(ns).Patch(ctx, name, types.MergePatchType, raw, metav1.PatchOptions{})
		return err
	default:
		return fmt.Errorf("unsupported workload kind %q", kind)
	}
}

func (k *KubernetesProvider) SealWorkload(ctx context.Context, ns, kind, name string) error {
	labels, err := k.workloadSelector(ctx, ns, kind, name)
	if err != nil {
		return err
	}
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sealPolicyName(name),
			Namespace: ns,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "zyvor-axiom"},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: labels},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		},
	}
	if _, err := k.client.NetworkingV1().NetworkPolicies(ns).Create(ctx, policy, metav1.CreateOptions{}); err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	return k.setWorkloadAnnotation(ctx, ns, kind, name, SealedAnnotation, "true")
}

func (k *KubernetesProvider) UnsealWorkload(ctx context.Context, ns, kind, name string) error {
	_ = k.client.NetworkingV1().NetworkPolicies(ns).Delete(ctx, sealPolicyName(name), metav1.DeleteOptions{})
	patch := map[string]any{"metadata": map[string]any{"annotations": map[string]string{SealedAnnotation: ""}}}
	raw, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	switch strings.ToLower(kind) {
	case "deployment":
		_, err := k.client.AppsV1().Deployments(ns).Patch(ctx, name, types.MergePatchType, raw, metav1.PatchOptions{})
		return err
	case "statefulset":
		_, err := k.client.AppsV1().StatefulSets(ns).Patch(ctx, name, types.MergePatchType, raw, metav1.PatchOptions{})
		return err
	default:
		return fmt.Errorf("unsupported workload kind %q", kind)
	}
}

func (k *KubernetesProvider) SealedWorkloads(ctx context.Context, ns string) (map[string]bool, error) {
	out := map[string]bool{}
	selectorNS := ns
	if ns == "" || ns == "all" {
		selectorNS = metav1.NamespaceAll
	}
	deps, err := k.client.AppsV1().Deployments(selectorNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, d := range deps.Items {
		if IsSealedAnnotation(d.Annotations) {
			out[d.Namespace+"/"+d.Name] = true
		}
	}
	sts, err := k.client.AppsV1().StatefulSets(selectorNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, s := range sts.Items {
		if IsSealedAnnotation(s.Annotations) {
			out[s.Namespace+"/"+s.Name] = true
		}
	}
	return out, nil
}

func workloadNameFromPod(p corev1.Pod) string {
	if v := p.Labels["app.kubernetes.io/name"]; v != "" {
		return v
	}
	if v := p.Labels["app"]; v != "" {
		return v
	}
	for _, ref := range p.OwnerReferences {
		if ref.Kind == "ReplicaSet" || ref.Kind == "StatefulSet" {
			name := ref.Name
			if ref.Kind == "ReplicaSet" {
				if idx := strings.LastIndex(name, "-"); idx > 0 {
					return name[:idx]
				}
			}
			return name
		}
	}
	return ""
}

func podQOS(p corev1.Pod) string {
	if p.Status.QOSClass == "" {
		return "BestEffort"
	}
	return string(p.Status.QOSClass)
}

func containerLimits(p corev1.Pod) (cpu, mem string) {
	cpu, mem = "—", "—"
	for _, c := range p.Spec.Containers {
		if c.Resources.Limits != nil {
			if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				cpu = q.String()
			}
			if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				mem = q.String()
			}
			if cpu != "—" || mem != "—" {
				return cpu, mem
			}
		}
	}
	return cpu, mem
}

func workloadIntent(ctx context.Context, k *KubernetesProvider, ns, wl string) string {
	if wl == "" {
		return ""
	}
	d, err := k.client.AppsV1().Deployments(ns).Get(ctx, wl, metav1.GetOptions{})
	if err == nil && d.Annotations != nil {
		return d.Annotations[IntentAnnotation]
	}
	s, err := k.client.AppsV1().StatefulSets(ns).Get(ctx, wl, metav1.GetOptions{})
	if err == nil && s.Annotations != nil {
		return s.Annotations[IntentAnnotation]
	}
	return ""
}

func podContainerInfo(p corev1.Pod) []PodContainerStatus {
	out := make([]PodContainerStatus, 0)
	statusByName := map[string]corev1.ContainerStatus{}
	for _, cs := range p.Status.InitContainerStatuses {
		statusByName[cs.Name] = cs
	}
	for _, cs := range p.Status.ContainerStatuses {
		statusByName[cs.Name] = cs
	}
	for _, c := range p.Spec.InitContainers {
		st := "Unknown"
		ready := false
		if cs, ok := statusByName[c.Name]; ok {
			ready = cs.Ready
			st = containerStateReason(cs)
		}
		out = append(out, PodContainerStatus{Name: c.Name, Init: true, Ready: ready, State: st})
	}
	for _, c := range p.Spec.Containers {
		st := "Unknown"
		ready := false
		if cs, ok := statusByName[c.Name]; ok {
			ready = cs.Ready
			st = containerStateReason(cs)
		}
		out = append(out, PodContainerStatus{Name: c.Name, Init: false, Ready: ready, State: st})
	}
	return out
}

func containerStateReason(cs corev1.ContainerStatus) string {
	if cs.State.Waiting != nil {
		return cs.State.Waiting.Reason
	}
	if cs.State.Running != nil {
		return "Running"
	}
	if cs.State.Terminated != nil {
		return cs.State.Terminated.Reason
	}
	return "Unknown"
}
