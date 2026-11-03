package platform

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Cluster health check (Workstream B2). Deterministic, rule-based analyzers —
// no LLM — in the spirit of k8sgpt: each analyzer inspects one class of
// Kubernetes object and emits findings with a severity and a suggested fix.
// The report types and helpers here are build-tag free so the Demo and
// Kubernetes providers (and their tests) share them; the Kubernetes analyzers
// themselves live in diagnose_kubernetes.go and the demo analyzers in
// diagnose_demo.go.

// Health finding severities, most severe first.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// Analyzer names (stable identifiers surfaced in the API and UI).
const (
	AnalyzerPodStatus       = "pod-status"
	AnalyzerServiceEndpoint = "service-endpoints"
	AnalyzerPVC             = "pvc-pending"
	AnalyzerIngress         = "ingress-backend"
	AnalyzerHPA             = "hpa"
	AnalyzerNodePressure    = "node-pressure"
)

// HealthAnalyzers lists every analyzer a report covers, in display order.
var HealthAnalyzers = []string{
	AnalyzerPodStatus, AnalyzerServiceEndpoint, AnalyzerPVC,
	AnalyzerIngress, AnalyzerHPA, AnalyzerNodePressure,
}

// HealthFinding is one problem the health check found.
type HealthFinding struct {
	ID         string `json:"id"`
	Analyzer   string `json:"analyzer"`
	Severity   string `json:"severity"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	Object     string `json:"object"` // "Kind/name", the form resourceHref understands
	Title      string `json:"title"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// HealthReport is the GET /diagnose payload.
type HealthReport struct {
	Namespace   string          `json:"namespace"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Status      string          `json:"status"` // healthy | warning | critical
	Counts      map[string]int  `json:"counts"`
	Analyzers   []string        `json:"analyzers"`
	Findings    []HealthFinding `json:"findings"`
	// Errors lists analyzers that could not run (typically RBAC) so a partial
	// report is never mistaken for a clean bill of health.
	Errors []string `json:"errors,omitempty"`
}

func severityRank(s string) int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

// newFinding fills the derived fields (ID, Object) of a finding.
func newFinding(analyzer, severity, kind, ns, name, title, message, suggestion string) HealthFinding {
	id := analyzer + ":" + kind + "/"
	if ns != "" {
		id += ns + "/"
	}
	id += name
	return HealthFinding{
		ID: id, Analyzer: analyzer, Severity: severity, Kind: kind, Namespace: ns, Name: name,
		Object: kind + "/" + name, Title: title, Message: message, Suggestion: suggestion,
	}
}

// NewHealthReport sorts the findings (most severe first, then stable by
// namespace/name), drops duplicate IDs and derives the overall status and
// per-severity counts.
func NewHealthReport(ns string, findings []HealthFinding, errs []string, now time.Time) HealthReport {
	seen := map[string]bool{}
	uniq := make([]HealthFinding, 0, len(findings))
	for _, f := range findings {
		if seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		uniq = append(uniq, f)
	}
	sort.SliceStable(uniq, func(i, j int) bool {
		a, b := uniq[i], uniq[j]
		if severityRank(a.Severity) != severityRank(b.Severity) {
			return severityRank(a.Severity) < severityRank(b.Severity)
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	counts := map[string]int{SeverityCritical: 0, SeverityWarning: 0, SeverityInfo: 0}
	for _, f := range uniq {
		counts[f.Severity]++
	}
	status := "healthy"
	switch {
	case counts[SeverityCritical] > 0:
		status = "critical"
	case counts[SeverityWarning] > 0 || len(errs) > 0:
		status = "warning"
	}
	if ns == "" {
		ns = "all"
	}
	return HealthReport{
		Namespace: ns, GeneratedAt: now.UTC(), Status: status, Counts: counts,
		Analyzers: append([]string(nil), HealthAnalyzers...), Findings: uniq, Errors: errs,
	}
}

// containerProblem classifies a container's waiting reason and last
// termination into a finding title and suggestion. ok is false when the
// container is healthy as far as these signals can tell.
func containerProblem(waitingReason, waitingMessage, lastTermReason string, lastExit int32, restarts int32, image string) (severity, title, message, suggestion string, ok bool) {
	oom := strings.EqualFold(lastTermReason, "OOMKilled")
	switch waitingReason {
	case "ImagePullBackOff", "ErrImagePull", "InvalidImageName", "ErrImageNeverPull":
		msg := fmt.Sprintf("Cannot pull image %q: %s", image, firstNonEmpty(waitingMessage, waitingReason))
		return SeverityCritical, "Image pull failing", msg,
			"Check the image name and tag, that the registry is reachable from the nodes, and that an imagePullSecret with valid credentials is referenced by the pod or its ServiceAccount.", true
	case "CreateContainerConfigError", "CreateContainerError", "RunContainerError":
		return SeverityCritical, "Container cannot be created", firstNonEmpty(waitingMessage, waitingReason),
			"Usually a missing ConfigMap/Secret key referenced by env or volumes, or an invalid securityContext. Describe the pod and check its events.", true
	case "CrashLoopBackOff":
		if oom {
			return SeverityCritical, "Container is OOMKilled and crash-looping",
				fmt.Sprintf("The container was killed for exceeding its memory limit and has restarted %d times.", restarts),
				"Raise resources.limits.memory (or fix the leak); compare against the pod's metrics before choosing a value.", true
		}
		msg := fmt.Sprintf("The container keeps exiting (last exit code %d) and has restarted %d times.", lastExit, restarts)
		if lastTermReason != "" {
			msg = fmt.Sprintf("The container keeps exiting (%s, exit code %d) and has restarted %d times.", lastTermReason, lastExit, restarts)
		}
		return SeverityCritical, "Container is crash-looping", msg,
			"Read the previous container's logs (Logs, with the previous toggle on) for the startup error; check config, probes and dependencies.", true
	}
	if oom && restarts > 0 {
		return SeverityWarning, "Container was OOMKilled",
			fmt.Sprintf("The container's last termination was OOMKilled (%d restarts).", restarts),
			"The memory limit is too low for the workload. Raise resources.limits.memory or investigate memory growth.", true
	}
	return "", "", "", "", false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
