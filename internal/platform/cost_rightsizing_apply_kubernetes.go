//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ApplyRightsizing recomputes the rightsizing recommendation for one
// workload container (same pipeline and window as ListRightsizing) and, unless
// req.DryRun, writes it to the workload's pod template with a strategic-merge
// patch. The workload controller then rolls its pods as usual.
func (k *KubernetesProvider) ApplyRightsizing(ctx context.Context, req RightsizingApplyRequest) (RightsizingApplyResult, error) {
	req, err := NormalizeRightsizingApplyRequest(req)
	if err != nil {
		return RightsizingApplyResult{}, err
	}
	tmpl, rollout, err := k.rightsizingPodTemplate(ctx, req)
	if err != nil {
		return RightsizingApplyResult{}, err
	}
	var cur *corev1.Container
	for i := range tmpl.Spec.Containers {
		if tmpl.Spec.Containers[i].Name == req.Container {
			cur = &tmpl.Spec.Containers[i]
			break
		}
	}
	if cur == nil {
		return RightsizingApplyResult{}, NewStatusError(http.StatusNotFound, fmt.Errorf("container %q not found in %s", req.Container, req.workloadRef()))
	}

	in, err := k.gatherRightsizingInputs(ctx, req.Namespace)
	if err != nil {
		return RightsizingApplyResult{}, err
	}
	item, src, ok := recommendOneContainer(ctx, req.Window, k.rightsizingSources(), in, req.containerKey())
	if !ok {
		return RightsizingApplyResult{}, rightsizingNoUsage(req)
	}
	plan, err := prepareRightsizingApply(req, item, templateContainerResources(cur))
	if err != nil {
		return RightsizingApplyResult{}, err
	}
	res := RightsizingApplyResult{
		OK: true, DryRun: req.DryRun, Changes: plan.Changes, Rollout: rollout,
		Verdict: item.Verdict, Source: src.name(), Confidence: src.confidence(req.Window),
	}
	if req.DryRun {
		return res, nil
	}
	patch, err := plan.patchJSON(req.Container)
	if err != nil {
		return RightsizingApplyResult{}, err
	}
	opts := metav1.PatchOptions{FieldManager: axiomFieldManager}
	switch req.Kind {
	case "Deployment":
		_, err = k.client.AppsV1().Deployments(req.Namespace).Patch(ctx, req.Workload, types.StrategicMergePatchType, patch, opts)
	case "StatefulSet":
		_, err = k.client.AppsV1().StatefulSets(req.Namespace).Patch(ctx, req.Workload, types.StrategicMergePatchType, patch, opts)
	case "DaemonSet":
		_, err = k.client.AppsV1().DaemonSets(req.Namespace).Patch(ctx, req.Workload, types.StrategicMergePatchType, patch, opts)
	}
	if err != nil {
		return RightsizingApplyResult{}, rightsizingPatchError(err)
	}
	return res, nil
}

// rightsizingPodTemplate reads the workload and returns its pod template and
// the rollout note for its update strategy. A missing workload is a 404.
func (k *KubernetesProvider) rightsizingPodTemplate(ctx context.Context, req RightsizingApplyRequest) (*corev1.PodTemplateSpec, string, error) {
	var (
		tmpl    *corev1.PodTemplateSpec
		rollout string
		err     error
	)
	switch req.Kind {
	case "Deployment":
		d, e := k.client.AppsV1().Deployments(req.Namespace).Get(ctx, req.Workload, metav1.GetOptions{})
		if err = e; e == nil {
			tmpl, rollout = &d.Spec.Template, rightsizingRolloutNote(req.Kind, "", d.Spec.Paused)
		}
	case "StatefulSet":
		s, e := k.client.AppsV1().StatefulSets(req.Namespace).Get(ctx, req.Workload, metav1.GetOptions{})
		if err = e; e == nil {
			tmpl, rollout = &s.Spec.Template, rightsizingRolloutNote(req.Kind, string(s.Spec.UpdateStrategy.Type), false)
		}
	case "DaemonSet":
		ds, e := k.client.AppsV1().DaemonSets(req.Namespace).Get(ctx, req.Workload, metav1.GetOptions{})
		if err = e; e == nil {
			tmpl, rollout = &ds.Spec.Template, rightsizingRolloutNote(req.Kind, string(ds.Spec.UpdateStrategy.Type), false)
		}
	}
	if apierrors.IsNotFound(err) {
		return nil, "", NewStatusError(http.StatusNotFound, fmt.Errorf("%s not found", req.workloadRef()))
	}
	if err != nil {
		return nil, "", err
	}
	return tmpl, rollout, nil
}

// templateContainerResources reads a pod template container's requests and limits.
func templateContainerResources(c *corev1.Container) rightsizingCurrentResources {
	var cur rightsizingCurrentResources
	if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
		cur.CPURequestMilli = q.MilliValue()
	}
	if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
		cur.CPULimitMilli = q.MilliValue()
	}
	if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
		cur.MemRequestBytes = q.Value()
	}
	if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
		cur.MemLimitBytes = q.Value()
	}
	return cur
}

// rightsizingPatchError surfaces the API server's refusal with its message:
// an invalid object (e.g. a request above a limit we may not raise) is 422, a
// conflict or a quota refusal is 409. Anything else keeps its usual mapping.
func rightsizingPatchError(err error) error {
	switch {
	case apierrors.IsInvalid(err):
		return NewStatusError(http.StatusUnprocessableEntity, err)
	case apierrors.IsConflict(err), apierrors.IsForbidden(err) && strings.Contains(strings.ToLower(err.Error()), "quota"):
		return NewStatusError(http.StatusConflict, err)
	}
	return err
}
