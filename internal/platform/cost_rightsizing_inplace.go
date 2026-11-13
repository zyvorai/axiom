package platform

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// In-place pod resize (Kubernetes >= 1.33 beta, 1.35 stable): the same
// recommendation, written to the running pods' resize subresource instead of
// the workload's pod template. Same limit rule as the template apply: a limit
// is only raised (to stay >= the request), never lowered or added.

// RightsizingPodResize is the outcome for one pod of an inPlace apply.
type RightsizingPodResize struct {
	Name string `json:"name"`
	// Status is "resized" (patch accepted; the kubelet then reports
	// PodResizeInProgress / PodResizePending on the pod), "unchanged" (the pod
	// already has the values) or "skipped" (see Note).
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

const (
	inPlaceResized   = "resized"
	inPlaceUnchanged = "unchanged"
	inPlaceSkipped   = "skipped"

	// InPlaceUnsupportedMessage is the clear error when the cluster has no
	// pods/resize subresource.
	InPlaceUnsupportedMessage = "in-place pod resize is not supported by this cluster (needs Kubernetes 1.33+ with InPlacePodVerticalScaling, stable in 1.35); retry without inPlace to roll the workload instead"

	inPlaceRestartNote = "its resizePolicy restarts the container to apply the change"
)

// rightsizingInPlaceRollout is the Rollout text of an inPlace result.
const rightsizingInPlaceRollout = "Running pods are resized in place without a restart; the pod template is unchanged, so a later rollout restores the old values until the manifest is updated"

// planInPlacePod computes the resize of one running pod. guaranteed is the
// pod's QoS class == Guaranteed: the resize API refuses a change of QoS class,
// so a Guaranteed pod must keep request == limit. Because limits are never
// lowered, a request decrease on such a pod is refused here with a clear
// reason instead of a late API error. Returns an empty plan when the pod
// already has the recommended values.
func planInPlacePod(cur rightsizingCurrentResources, item RightsizingItem, resources []string, guaranteed bool) (rightsizingPlan, error) {
	p := planRightsizingChanges(cur, item, resources)
	if guaranteed {
		next := p.applyTo(cur)
		if (cur.CPULimitMilli > 0 && p.NewCPURequestMilli > 0 && next.CPURequestMilli != next.CPULimitMilli) ||
			(cur.MemLimitBytes > 0 && p.NewMemRequestBytes > 0 && next.MemRequestBytes != next.MemLimitBytes) {
			return rightsizingPlan{}, fmt.Errorf("the pod has the Guaranteed QoS class (requests equal limits) and limits are never lowered, so lowering a request would change its QoS class, which a resize cannot do")
		}
	}
	return p, nil
}

// inPlaceRestartsContainer reports whether a resizePolicy asks for a
// container restart for any of the changed resources. policies maps the
// resource name ("cpu", "memory") to its restartPolicy.
func inPlaceRestartsContainer(p rightsizingPlan, policies map[string]string) bool {
	changedCPU := p.NewCPURequestMilli > 0 || p.NewCPULimitMilli > 0
	changedMem := p.NewMemRequestBytes > 0 || p.NewMemLimitBytes > 0
	return (changedCPU && policies["cpu"] == "RestartContainer") || (changedMem && policies["memory"] == "RestartContainer")
}

// InPlaceSummary condenses per-pod outcomes for the audit log.
func InPlaceSummary(pods []RightsizingPodResize) string {
	counts := map[string]int{}
	for _, p := range pods {
		counts[p.Status]++
	}
	var parts []string
	for _, s := range []string{inPlaceResized, inPlaceUnchanged, inPlaceSkipped} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	return strings.Join(parts, ", ")
}
