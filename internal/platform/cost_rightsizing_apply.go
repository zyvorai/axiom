package platform

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Applying a rightsizing recommendation (POST /api/v1/cost/rightsizing/apply)
//
// The client names one container and the resources to change; the server
// recomputes the recommendation for exactly that container with the same
// pipeline and window as the GET, so the client never supplies the numbers.
// Only resources.requests of that container are written, plus a limit when an
// existing limit would fall below the new request (it is raised to the
// request, never lowered, and never added when there was none).

// RightsizingApplyRequest is the body of POST /api/v1/cost/rightsizing/apply.
type RightsizingApplyRequest struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Workload  string `json:"workload"`
	Container string `json:"container"`
	// Resources is a non-empty subset of ["cpu", "memory"].
	Resources []string `json:"resources"`
	// Window is the usage window, as in the GET ("" means 7d).
	Window string `json:"window,omitempty"`
	// DryRun computes and returns the changes without writing anything.
	DryRun bool `json:"dryRun,omitempty"`
	// Observed is the request the caller saw; when given and different from
	// the live request the apply is refused (stale view).
	Observed *RightsizingObserved `json:"observed,omitempty"`
}

// RightsizingObserved is the request the caller's view showed (the GET's
// cpu.requestMilli / memory.requestMiB). Each field is optional.
type RightsizingObserved struct {
	CPURequestMilli  *int64 `json:"cpuRequestMilli,omitempty"`
	MemoryRequestMiB *int64 `json:"memoryRequestMiB,omitempty"`
}

// RightsizingChange is one field the apply changes (or would change).
type RightsizingChange struct {
	Resource string `json:"resource"` // cpu or memory
	Field    string `json:"field"`    // requests or limits
	From     string `json:"from"`     // previous quantity, "none" when unset
	To       string `json:"to"`
	Note     string `json:"note,omitempty"`
}

// RightsizingApplyResult is the response of the apply route (dry run or not).
type RightsizingApplyResult struct {
	OK         bool                `json:"ok"`
	DryRun     bool                `json:"dryRun"`
	Changes    []RightsizingChange `json:"changes"`
	Rollout    string              `json:"rollout"`
	Verdict    string              `json:"verdict"`
	Source     string              `json:"source"`
	Confidence string              `json:"confidence"`
}

// Summary is the one-line description recorded in the audit log, e.g.
// "cpu requests 500m->250m, memory limits 256Mi->288Mi".
func (r RightsizingApplyResult) Summary() string {
	parts := make([]string, 0, len(r.Changes))
	for _, c := range r.Changes {
		parts = append(parts, c.Resource+" "+c.Field+" "+c.From+"->"+c.To)
	}
	return strings.Join(parts, ", ")
}

const (
	rightsizingLimitRaisedNote = "limit raised to stay >= the request"
	rightsizingNone            = "none"
)

var (
	rightsizingLabelRe     = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	rightsizingSubdomainRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
)

func rightsizingBadRequest(format string, a ...any) error {
	return NewStatusError(http.StatusBadRequest, fmt.Errorf(format, a...))
}

func rightsizingConflict(format string, a ...any) error {
	return NewStatusError(http.StatusConflict, fmt.Errorf(format, a...))
}

// NormalizeRightsizingApplyRequest validates req (400 on bad input) and
// returns it with the window defaulted and resources deduplicated in the
// canonical order cpu, memory.
func NormalizeRightsizingApplyRequest(req RightsizingApplyRequest) (RightsizingApplyRequest, error) {
	if err := ValidateCostNamespace(req.Namespace); err != nil {
		return req, err
	}
	switch req.Kind {
	case "Deployment", "StatefulSet", "DaemonSet":
	default:
		return req, rightsizingBadRequest("kind must be Deployment, StatefulSet or DaemonSet")
	}
	if len(req.Workload) == 0 || len(req.Workload) > 253 || !rightsizingSubdomainRe.MatchString(req.Workload) {
		return req, rightsizingBadRequest("workload must be a DNS-1123 name (lowercase letters, digits, '-' and '.', at most 253 characters)")
	}
	if len(req.Container) == 0 || len(req.Container) > 63 || !rightsizingLabelRe.MatchString(req.Container) {
		return req, rightsizingBadRequest("container must be a DNS-1123 label (lowercase letters, digits and '-', at most 63 characters)")
	}
	var cpu, mem bool
	for _, r := range req.Resources {
		switch r {
		case "cpu":
			cpu = true
		case "memory":
			mem = true
		default:
			return req, rightsizingBadRequest("resources must be a non-empty subset of [\"cpu\", \"memory\"]")
		}
	}
	if !cpu && !mem {
		return req, rightsizingBadRequest("resources must be a non-empty subset of [\"cpu\", \"memory\"]")
	}
	req.Resources = nil
	if cpu {
		req.Resources = append(req.Resources, "cpu")
	}
	if mem {
		req.Resources = append(req.Resources, "memory")
	}
	w, err := NormalizeRightsizingWindow(req.Window)
	if err != nil {
		return req, err
	}
	req.Window = w
	if o := req.Observed; o != nil {
		if (o.CPURequestMilli != nil && *o.CPURequestMilli < 0) || (o.MemoryRequestMiB != nil && *o.MemoryRequestMiB < 0) {
			return req, rightsizingBadRequest("observed requests must not be negative")
		}
	}
	return req, nil
}

// containerKey matches rightsizingContainer.key().
func (req RightsizingApplyRequest) containerKey() string {
	return req.Namespace + "/" + req.Kind + "/" + req.Workload + "/" + req.Container
}

func (req RightsizingApplyRequest) workloadRef() string {
	return req.Kind + " " + req.Namespace + "/" + req.Workload
}

// rightsizingCurrentResources is a container's resources as the workload's
// pod template declares them. 0 means not set.
type rightsizingCurrentResources struct {
	CPURequestMilli, CPULimitMilli int64
	MemRequestBytes, MemLimitBytes int64
}

// rightsizingPlan is what an apply writes. A zero New* value leaves that
// field untouched.
type rightsizingPlan struct {
	Changes []RightsizingChange

	NewCPURequestMilli, NewCPULimitMilli int64
	NewMemRequestBytes, NewMemLimitBytes int64
}

// planRightsizingChanges computes the changes that bring the chosen resources'
// requests to the recommendation. A resource whose recommendation is unknown
// (0) or already equals the request is left alone. An existing limit below the
// new request is raised to it; limits are never lowered and never added.
func planRightsizingChanges(cur rightsizingCurrentResources, item RightsizingItem, resources []string) rightsizingPlan {
	var p rightsizingPlan
	want := map[string]bool{}
	for _, r := range resources {
		want[r] = true
	}
	if target := item.CPU.RecommendedMilli; want["cpu"] && target > 0 && target != cur.CPURequestMilli {
		p.NewCPURequestMilli = target
		p.Changes = append(p.Changes, RightsizingChange{Resource: "cpu", Field: "requests", From: formatCPUMilliOrNone(cur.CPURequestMilli), To: formatCPUMilli(target)})
		if cur.CPULimitMilli > 0 && cur.CPULimitMilli < target {
			p.NewCPULimitMilli = target
			p.Changes = append(p.Changes, RightsizingChange{Resource: "cpu", Field: "limits", From: formatCPUMilli(cur.CPULimitMilli), To: formatCPUMilli(target), Note: rightsizingLimitRaisedNote})
		}
	}
	if target := item.Memory.RecommendedMiB * mib; want["memory"] && target > 0 && target != cur.MemRequestBytes {
		p.NewMemRequestBytes = target
		p.Changes = append(p.Changes, RightsizingChange{Resource: "memory", Field: "requests", From: formatMemoryBytesOrNone(cur.MemRequestBytes), To: formatMemoryBytes(target)})
		if cur.MemLimitBytes > 0 && cur.MemLimitBytes < target {
			p.NewMemLimitBytes = target
			p.Changes = append(p.Changes, RightsizingChange{Resource: "memory", Field: "limits", From: formatMemoryBytes(cur.MemLimitBytes), To: formatMemoryBytes(target), Note: rightsizingLimitRaisedNote})
		}
	}
	return p
}

// applyTo returns cur with the plan's new values written over it.
func (p rightsizingPlan) applyTo(cur rightsizingCurrentResources) rightsizingCurrentResources {
	if p.NewCPURequestMilli > 0 {
		cur.CPURequestMilli = p.NewCPURequestMilli
	}
	if p.NewCPULimitMilli > 0 {
		cur.CPULimitMilli = p.NewCPULimitMilli
	}
	if p.NewMemRequestBytes > 0 {
		cur.MemRequestBytes = p.NewMemRequestBytes
	}
	if p.NewMemLimitBytes > 0 {
		cur.MemLimitBytes = p.NewMemLimitBytes
	}
	return cur
}

// patchJSON is the strategic-merge patch for the workload: containers merge
// by name, so only the named container's touched resource keys change.
func (p rightsizingPlan) patchJSON(container string) ([]byte, error) {
	requests := map[string]string{}
	limits := map[string]string{}
	if p.NewCPURequestMilli > 0 {
		requests["cpu"] = formatCPUMilli(p.NewCPURequestMilli)
	}
	if p.NewMemRequestBytes > 0 {
		requests["memory"] = formatMemoryBytes(p.NewMemRequestBytes)
	}
	if p.NewCPULimitMilli > 0 {
		limits["cpu"] = formatCPUMilli(p.NewCPULimitMilli)
	}
	if p.NewMemLimitBytes > 0 {
		limits["memory"] = formatMemoryBytes(p.NewMemLimitBytes)
	}
	res := map[string]any{}
	if len(requests) > 0 {
		res["requests"] = requests
	}
	if len(limits) > 0 {
		res["limits"] = limits
	}
	return json.Marshal(map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
		"containers": []any{map[string]any{"name": container, "resources": res}},
	}}}})
}

// checkRightsizingObserved refuses (409) when the caller's view of the
// request no longer matches the live one the recommendation was computed from.
func checkRightsizingObserved(obs *RightsizingObserved, item RightsizingItem) error {
	if obs == nil {
		return nil
	}
	var diffs []string
	if obs.CPURequestMilli != nil && *obs.CPURequestMilli != item.CPU.RequestMilli {
		diffs = append(diffs, fmt.Sprintf("cpu request is now %s, you saw %s", formatCPUMilliOrNone(item.CPU.RequestMilli), formatCPUMilliOrNone(*obs.CPURequestMilli)))
	}
	if obs.MemoryRequestMiB != nil && *obs.MemoryRequestMiB != item.Memory.RequestMiB {
		diffs = append(diffs, fmt.Sprintf("memory request is now %s, you saw %s", formatMemoryBytesOrNone(item.Memory.RequestMiB*mib), formatMemoryBytesOrNone(*obs.MemoryRequestMiB*mib)))
	}
	if len(diffs) == 0 {
		return nil
	}
	return rightsizingConflict("the live requests changed since you looked (%s); reload the recommendations and review them again", strings.Join(diffs, "; "))
}

// prepareRightsizingApply runs the checks both providers share once the
// recommendation for the container is known: stale view, already right-sized,
// and nothing to change for the chosen resources.
func prepareRightsizingApply(req RightsizingApplyRequest, item RightsizingItem, cur rightsizingCurrentResources) (rightsizingPlan, error) {
	if err := checkRightsizingObserved(req.Observed, item); err != nil {
		return rightsizingPlan{}, err
	}
	if item.Verdict == "ok" {
		return rightsizingPlan{}, rightsizingConflict("container %q of %s is already right-sized", req.Container, req.workloadRef())
	}
	p := planRightsizingChanges(cur, item, req.Resources)
	if len(p.Changes) == 0 {
		return rightsizingPlan{}, rightsizingConflict("container %q of %s is already right-sized for %s", req.Container, req.workloadRef(), strings.Join(req.Resources, " and "))
	}
	return p, nil
}

// rightsizingNoUsage is the 409 for a container that exists but has nothing
// to recommend from (no running pods, or no usage samples for it).
func rightsizingNoUsage(req RightsizingApplyRequest) error {
	return rightsizingConflict("no usage data is available for container %q of %s, so there is no recommendation to apply", req.Container, req.workloadRef())
}

// rightsizingRolloutNote tells the caller what the patch sets in motion.
// strategy is the StatefulSet / DaemonSet update strategy type ("" = default).
func rightsizingRolloutNote(kind, strategy string, paused bool) string {
	switch kind {
	case "Deployment":
		if paused {
			return "The Deployment is paused: its pods keep the old values until it is resumed, then roll out with a rolling update"
		}
		return "Deployments restart their pods with the new values; expect a rolling update"
	case "StatefulSet":
		if strategy == "OnDelete" {
			return "The StatefulSet uses the OnDelete update strategy: each pod keeps the old values until it is deleted"
		}
		return "StatefulSets replace their pods one at a time, highest ordinal first, with the new values"
	case "DaemonSet":
		if strategy == "OnDelete" {
			return "The DaemonSet uses the OnDelete update strategy: each pod keeps the old values until it is deleted"
		}
		return "DaemonSets replace the pod on each node with the new values; expect a rolling update"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Quantity rendering (Kubernetes canonical-looking strings)

// formatCPUMilli renders millicores as whole cores when exact ("2"), else "250m".
func formatCPUMilli(m int64) string {
	if m%1000 == 0 {
		return strconv.FormatInt(m/1000, 10)
	}
	return strconv.FormatInt(m, 10) + "m"
}

func formatCPUMilliOrNone(m int64) string {
	if m <= 0 {
		return rightsizingNone
	}
	return formatCPUMilli(m)
}

// formatMemoryBytes renders bytes as Gi or Mi when exact, then the decimal
// G / M a manifest likely used, then Ki, else plain bytes.
func formatMemoryBytes(b int64) string {
	switch {
	case b != 0 && b%(1024*mib) == 0:
		return strconv.FormatInt(b/(1024*mib), 10) + "Gi"
	case b != 0 && b%mib == 0:
		return strconv.FormatInt(b/mib, 10) + "Mi"
	case b != 0 && b%1_000_000_000 == 0:
		return strconv.FormatInt(b/1_000_000_000, 10) + "G"
	case b != 0 && b%1_000_000 == 0:
		return strconv.FormatInt(b/1_000_000, 10) + "M"
	case b != 0 && b%1024 == 0:
		return strconv.FormatInt(b/1024, 10) + "Ki"
	}
	return strconv.FormatInt(b, 10)
}

func formatMemoryBytesOrNone(b int64) string {
	if b <= 0 {
		return rightsizingNone
	}
	return formatMemoryBytes(b)
}
