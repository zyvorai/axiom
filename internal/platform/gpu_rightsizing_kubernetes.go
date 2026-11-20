//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplyGPURightsizing lowers nvidia.com/gpu container requests to 1 when currently ≥2.
func (k *KubernetesProvider) ApplyGPURightsizing(ctx context.Context, req GPURightsizingApplyRequest) (GPURightsizingApplyResult, error) {
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Name = strings.TrimSpace(req.Name)
	req.Container = strings.TrimSpace(req.Container)
	if req.Namespace == "" || req.Name == "" {
		return GPURightsizingApplyResult{}, rightsizingBadRequest("namespace and name are required")
	}
	if req.Kind == "" {
		req.Kind = "Deployment"
	}
	tpl, rv, err := k.rightsizingPodTemplate(ctx, RightsizingApplyRequest{
		Namespace: req.Namespace, Kind: req.Kind, Workload: req.Name, Container: firstNonEmpty(req.Container, req.Name),
	})
	if err != nil {
		return GPURightsizingApplyResult{}, err
	}
	_ = rv
	idx := -1
	from := int64(0)
	for i := range tpl.Spec.Containers {
		c := &tpl.Spec.Containers[i]
		if req.Container != "" && c.Name != req.Container {
			continue
		}
		if q, ok := c.Resources.Requests[corev1.ResourceName(nvidiaGPUResource)]; ok && q.Value() >= 2 {
			idx = i
			from = q.Value()
			if req.Container == "" {
				req.Container = c.Name
			}
			break
		}
	}
	if idx < 0 {
		return GPURightsizingApplyResult{}, rightsizingConflict("no container with nvidia.com/gpu request ≥ 2")
	}
	to := int64(1)
	res := GPURightsizingApplyResult{
		OK: true, DryRun: req.DryRun,
		From: strconv.FormatInt(from, 10), To: strconv.FormatInt(to, 10),
		Note:    fmt.Sprintf("lower %s requests.nvidia.com/gpu on container %s", req.Kind, req.Container),
		Rollout: "RollingUpdate",
	}
	if req.DryRun {
		return res, nil
	}
	qty := resource.NewQuantity(to, resource.DecimalSI)
	tpl.Spec.Containers[idx].Resources.Requests[corev1.ResourceName(nvidiaGPUResource)] = *qty
	if lim, ok := tpl.Spec.Containers[idx].Resources.Limits[corev1.ResourceName(nvidiaGPUResource)]; ok && lim.Value() < to {
		tpl.Spec.Containers[idx].Resources.Limits[corev1.ResourceName(nvidiaGPUResource)] = *qty
	}
	switch req.Kind {
	case "Deployment":
		d, err := k.client.AppsV1().Deployments(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
		if err != nil {
			return GPURightsizingApplyResult{}, err
		}
		d.Spec.Template = *tpl
		_, err = k.client.AppsV1().Deployments(req.Namespace).Update(ctx, d, metav1.UpdateOptions{})
		return res, err
	case "StatefulSet":
		s, err := k.client.AppsV1().StatefulSets(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
		if err != nil {
			return GPURightsizingApplyResult{}, err
		}
		s.Spec.Template = *tpl
		_, err = k.client.AppsV1().StatefulSets(req.Namespace).Update(ctx, s, metav1.UpdateOptions{})
		return res, err
	default:
		return GPURightsizingApplyResult{}, rightsizingBadRequest("kind must be Deployment or StatefulSet for GPU rightsizing")
	}
}
