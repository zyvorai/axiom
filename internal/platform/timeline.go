package platform

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Change / event timeline (Workstream B1): one chronological feed that merges
// Kubernetes events, Axiom audit entries, alert warnings and rollout revisions
// so "what changed before it broke" is a single read.

// Timeline entry sources.
const (
	TimelineEvent   = "event"
	TimelineAudit   = "audit"
	TimelineAlert   = "alert"
	TimelineRollout = "rollout"
)

// TimelineEntry is one row of GET /timeline.
type TimelineEntry struct {
	Time      time.Time `json:"time"`
	Source    string    `json:"source"`   // event | audit | alert | rollout
	Severity  string    `json:"severity"` // info | warning | critical
	Namespace string    `json:"namespace,omitempty"`
	Object    string    `json:"object"` // "Kind/name" (resourceHref form); empty for cluster-wide audit actions
	Summary   string    `json:"summary"`
	Actor     string    `json:"actor,omitempty"` // audit entries: the Axiom user
}

// TimelineFromEvent maps a Kubernetes event. Warning events are alerts.
func TimelineFromEvent(e Event) TimelineEntry {
	src, sev := TimelineEvent, SeverityInfo
	if strings.EqualFold(e.Type, "Warning") {
		sev = SeverityWarning
	}
	summary := e.Reason
	if e.Message != "" {
		summary += ": " + e.Message
	}
	if e.Count > 1 {
		summary += " (x" + strconv.Itoa(int(e.Count)) + ")"
	}
	return TimelineEntry{Time: e.Time, Source: src, Severity: sev, Namespace: e.Namespace, Object: e.Object, Summary: summary}
}

// TimelineFromAlert maps a Warning event surfaced by the alerts view.
func TimelineFromAlert(e Event) TimelineEntry {
	t := TimelineFromEvent(e)
	t.Source, t.Severity = TimelineAlert, SeverityWarning
	return t
}

// auditKinds maps an audit action's resource prefix ("pod.delete" -> "pod")
// to the Kubernetes kind used for the timeline object.
var auditKinds = map[string]string{
	"pod": "Pod", "workload": "", "secret": "Secret", "configmap": "ConfigMap", "job": "Job",
	"cronjob": "CronJob", "service": "Service", "ingress": "Ingress", "hpa": "HorizontalPodAutoscaler",
	"namespace": "Namespace", "node": "Node", "volume": "PersistentVolumeClaim", "vm": "VirtualMachine",
	"backup": "PersistentVolumeClaim", "networkpolicy": "NetworkPolicy", "quota": "ResourceQuota",
	"gitops": "", "helm": "", "agentrun": "", "rbac": "",
}

// TimelineFromAudit maps an Axiom audit entry. Targets look like "ns/name"
// (namespaced resource), "ns/Kind/name" (workloads) or just "name" (nodes,
// namespaces); anything unrecognised keeps the raw target as its object.
func TimelineFromAudit(a AuditEntry) TimelineEntry {
	prefix := a.Action
	if i := strings.IndexByte(prefix, '.'); i >= 0 {
		prefix = prefix[:i]
	}
	kind := auditKinds[prefix]
	parts := strings.Split(a.Target, "/")
	var ns, object string
	switch {
	case len(parts) == 3:
		ns, object = parts[0], canonicalKind(parts[1])+"/"+parts[2]
	case len(parts) == 2 && kind != "":
		ns, object = parts[0], kind+"/"+parts[1]
	case len(parts) == 1 && a.Target != "" && kind != "":
		object = kind + "/" + a.Target
		if kind == "Namespace" {
			ns = a.Target
		}
	default:
		object = a.Target
		if len(parts) == 2 {
			ns = parts[0]
		}
	}
	if strings.HasPrefix(a.Action, "cost.budget_") && len(parts) == 1 && a.Target != "" {
		ns = a.Target // budget targets are namespaces
	}
	sev := SeverityInfo
	switch {
	case strings.HasPrefix(a.Action, "auth."):
		sev = SeverityWarning
	case a.Action == "fleet.cluster_unreachable", a.Action == "fleet.cluster_degraded",
		a.Action == "cost.budget_over", a.Action == "cost.budget_warning":
		sev = SeverityWarning
	case strings.HasSuffix(a.Action, ".delete"), strings.HasSuffix(a.Action, ".drain"), strings.HasSuffix(a.Action, ".evict"),
		strings.HasSuffix(a.Action, ".stop"), strings.HasSuffix(a.Action, ".rollback"), strings.HasSuffix(a.Action, ".uninstall"),
		a.Action == "pod.debug", a.Action == "node.shell":
		sev = SeverityWarning
	}
	summary := a.Action
	// Operator presence (“who poked it”): keep stable action prefixes tests/UI match on.
	switch a.Action {
	case "pod.terminal":
		summary = "pod.terminal " + a.Target + " — operator opened terminal"
		sev = SeverityInfo
	case "pod.window":
		summary = "pod.window " + a.Target + " — operator opened pod window"
		sev = SeverityInfo
	case "pod.exec":
		// Detail (command) is intentionally omitted from the summary elsewhere;
		// keep the "pod.exec " prefix and never append Detail for this action.
		summary = "pod.exec " + a.Target
	case "workload.seal.receipt":
		summary = "workload.seal.receipt " + a.Target
	case "node.shell":
		summary = "node.shell " + a.Target + " — operator opened node shell"
	default:
		if a.Target != "" {
			summary += " " + a.Target
		}
	}
	if a.Detail != "" && a.Action != "pod.exec" {
		summary += " (" + a.Detail + ")"
	}
	return TimelineEntry{Time: a.Time, Source: TimelineAudit, Severity: sev, Namespace: ns, Object: object, Summary: summary, Actor: a.User}
}

func canonicalKind(k string) string {
	switch strings.ToLower(k) {
	case "deployment":
		return "Deployment"
	case "statefulset":
		return "StatefulSet"
	case "daemonset":
		return "DaemonSet"
	}
	if k == "" {
		return k
	}
	return strings.ToUpper(k[:1]) + k[1:]
}

// TimelineFromRollout maps a Deployment revision to a rollout entry. When the
// provider knows only the revision's age ("3d"), the time is derived from it.
func TimelineFromRollout(ns, kind, name string, rev RolloutRevision, now time.Time) TimelineEntry {
	t := rev.Time
	if t.IsZero() {
		t = AgeToTime(rev.Age, now)
	}
	summary := "Rollout revision " + strconv.Itoa(rev.Revision) + " created"
	if rev.Status == "active" {
		summary += " (active)"
	}
	return TimelineEntry{Time: t, Source: TimelineRollout, Severity: SeverityInfo, Namespace: ns, Object: kind + "/" + name, Summary: summary}
}

// AgeToTime converts the short age strings the providers emit ("now", "5m",
// "3h", "11d") to an approximate timestamp. Unparseable input yields now.
func AgeToTime(age string, now time.Time) time.Time {
	age = strings.TrimSpace(age)
	if age == "" || age == "now" || age == "-" {
		return now
	}
	i := len(age)
	for i > 0 && !unicode.IsDigit(rune(age[i-1])) {
		i--
	}
	n, err := strconv.Atoi(age[:i])
	if err != nil {
		return now
	}
	switch age[i:] {
	case "s":
		return now.Add(-time.Duration(n) * time.Second)
	case "m":
		return now.Add(-time.Duration(n) * time.Minute)
	case "h":
		return now.Add(-time.Duration(n) * time.Hour)
	case "d":
		return now.Add(-time.Duration(n) * 24 * time.Hour)
	}
	return now
}

// TimelineObjectMatches reports whether an entry's object belongs to the
// requested "Kind/name" filter. Beyond an exact (case-insensitive) match, a
// workload filter also matches the objects it owns — its ReplicaSets and pods,
// whose names are "<workload>-<suffix>" — so a Deployment's "What changed" tab
// includes its pods' BackOff events.
func TimelineObjectMatches(entryObject, filter string) bool {
	if filter == "" {
		return true
	}
	if strings.EqualFold(entryObject, filter) {
		return true
	}
	fk, fname, ok := strings.Cut(filter, "/")
	if !ok {
		return false
	}
	ek, ename, ok := strings.Cut(entryObject, "/")
	if !ok {
		return false
	}
	switch strings.ToLower(fk) {
	case "deployment", "statefulset", "daemonset", "replicaset", "job", "cronjob":
		switch strings.ToLower(ek) {
		case "pod", "replicaset", "job":
			return strings.HasPrefix(ename, fname+"-")
		}
	}
	return false
}

// SortTimeline orders entries newest first.
func SortTimeline(entries []TimelineEntry) {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Time.After(entries[j].Time) })
}
