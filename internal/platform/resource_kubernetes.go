//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (k *KubernetesProvider) GetResource(ctx context.Context, ns, kind, name string) (ResourceDetail, error) {
	detail := ResourceDetail{Namespace: ns, Kind: kind, Name: name}
	switch strings.ToLower(kind) {
	case "deployment":
		obj, err := k.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return detail, err
		}
		detail.Labels = obj.Labels
		detail.Annotations = obj.Annotations
		detail.Manifest = objectToMap(obj)
		detail.ContainerPorts = containerPortsFromPodTemplate(&obj.Spec.Template)
	case "statefulset":
		obj, err := k.client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return detail, err
		}
		detail.Labels = obj.Labels
		detail.Annotations = obj.Annotations
		detail.Manifest = objectToMap(obj)
		detail.ContainerPorts = containerPortsFromPodTemplate(&obj.Spec.Template)
	case "daemonset":
		obj, err := k.client.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return detail, err
		}
		detail.Labels = obj.Labels
		detail.Annotations = obj.Annotations
		detail.Manifest = objectToMap(obj)
		detail.ContainerPorts = containerPortsFromPodTemplate(&obj.Spec.Template)
	default:
		return detail, fmt.Errorf("unsupported kind: %s", kind)
	}

	pods, err := k.ListPods(ctx, ns)
	if err == nil {
		for _, p := range pods {
			if strings.HasPrefix(p.Name, name+"-") || strings.Contains(p.Name, name) {
				detail.Pods = append(detail.Pods, p)
			}
		}
	}
	events, err := k.ListEvents(ctx, ns)
	if err == nil {
		for _, e := range events {
			if strings.Contains(e.Object, name) {
				detail.RelatedEvents = append(detail.RelatedEvents, e)
				if len(detail.RelatedEvents) >= 20 {
					break
				}
			}
		}
	}
	return detail, nil
}

func objectToMap(obj runtime.Object) map[string]any {
	raw, err := json.Marshal(obj)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func containerPortsFromPodTemplate(t *corev1.PodTemplateSpec) []map[string]any {
	var ports []map[string]any
	if t == nil {
		return ports
	}
	for _, c := range t.Spec.Containers {
		for _, p := range c.Ports {
			ports = append(ports, map[string]any{
				"container": c.Name,
				"port":      p.ContainerPort,
				"protocol":  string(p.Protocol),
				"name":      p.Name,
			})
		}
	}
	return ports
}

func (k *KubernetesProvider) GetNode(ctx context.Context, name string) (Node, error) {
	nodes, err := k.ListNodes(ctx)
	if err != nil {
		return Node{}, err
	}
	for _, n := range nodes {
		if n.Name == name {
			return n, nil
		}
	}
	return Node{}, fmt.Errorf("node %s not found", name)
}

func (k *KubernetesProvider) WorkloadNetworkGuard(ctx context.Context, ns, kind, name string) (NetworkGuardSummary, error) {
	return workloadNetworkGuardFromPW(ctx, ns, name)
}

var _ = appsv1.SchemeGroupVersion
