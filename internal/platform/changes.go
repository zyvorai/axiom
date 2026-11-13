package platform

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Change correlation ("what changed before this?"): merges the change-like
// signals already available through existing Provider methods into one list,
// ranked by relevance to a namespace/workload and by closeness in time.

// Change sources.
const (
	ChangeGitOps  = "gitops"
	ChangeHelm    = "helm"
	ChangeRollout = "rollout"
	ChangeConfig  = "config"
	ChangeNode    = "node"
	ChangeHPA     = "hpa"
	ChangeAudit   = "audit"
)

const (
	DefaultChangeWindow = 60 * time.Minute
	MaxChangeWindow     = 7 * 24 * time.Hour
	maxChangeItems      = 100
)

// ChangeCandidate is a raw change signal before scoring.
type ChangeCandidate struct {
	Source    string
	Time      time.Time
	Namespace string // empty for cluster-scoped changes
	Object    string // "Kind/name"
	Actor     string
	Summary   string
}

// ChangeItem is one ranked row of GET /changes.
type ChangeItem struct {
	Source    string    `json:"source"`
	Time      time.Time `json:"time"`
	Namespace string    `json:"namespace,omitempty"`
	Object    string    `json:"object,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	Summary   string    `json:"summary"`
	Score     int       `json:"score"`            // 0-100, higher is more relevant
	Scope     string    `json:"scope"`            // workload | namespace | cluster
	Before    string    `json:"before,omitempty"` // time before the reference point, e.g. "12m"
}

// ChangeReport is the GET /changes response.
type ChangeReport struct {
	Namespace string       `json:"namespace,omitempty"`
	Workload  string       `json:"workload,omitempty"`
	At        time.Time    `json:"at"`
	Window    string       `json:"window"`
	Changes   []ChangeItem `json:"changes"`
}

func objectName(object string) string {
	if i := strings.LastIndexByte(object, '/'); i >= 0 {
		return object[i+1:]
	}
	return object
}

func objectMatchesWorkload(object, workload string) bool {
	if workload == "" || object == "" {
		return false
	}
	n := strings.ToLower(objectName(object))
	w := strings.ToLower(workload)
	return n == w || strings.HasPrefix(n, w+"-")
}

// RankChanges filters candidates to [at-window, at] and scores them. Pure.
// Scope weight: same workload 1.0, same namespace 0.6, cluster-scoped 0.3
// (when a namespace is given, other namespaces are dropped). Score is
// 100*(0.7*scope + 0.3*proximity), proximity falling linearly to 0 at the
// window edge. Ties break on newest first.
func RankChanges(cands []ChangeCandidate, namespace, workload string, at time.Time, window time.Duration) []ChangeItem {
	if window <= 0 {
		window = DefaultChangeWindow
	}
	from := at.Add(-window)
	out := make([]ChangeItem, 0, len(cands))
	for _, c := range cands {
		if c.Time.IsZero() || c.Time.Before(from) || c.Time.After(at) {
			continue
		}
		var scope string
		var w float64
		switch {
		case namespace != "" && c.Namespace != "" && !strings.EqualFold(c.Namespace, namespace):
			continue
		case workload != "" && objectMatchesWorkload(c.Object, workload) &&
			(c.Namespace == "" || namespace == "" || strings.EqualFold(c.Namespace, namespace)):
			scope, w = "workload", 1.0
		case c.Namespace == "":
			scope, w = "cluster", 0.3
		default:
			scope, w = "namespace", 0.6
		}
		dt := at.Sub(c.Time)
		prox := 1 - float64(dt)/float64(window)
		score := int(math.Round(100 * (0.7*w + 0.3*prox)))
		out = append(out, ChangeItem{
			Source: c.Source, Time: c.Time, Namespace: c.Namespace, Object: c.Object, Actor: c.Actor,
			Summary: c.Summary, Score: score, Scope: scope, Before: shortDuration(dt),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Time.After(out[j].Time)
	})
	if len(out) > maxChangeItems {
		out = out[:maxChangeItems]
	}
	return out
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	default:
		return strconv.Itoa(int(d/time.Hour)) + "h" + strconv.Itoa(int(d%time.Hour/time.Minute)) + "m"
	}
}

// changeEventReasons maps Kubernetes event reasons that signal a change to a source.
var changeEventReasons = map[string]string{
	"ScalingReplicaSet": ChangeRollout, "SuccessfulRescale": ChangeHPA,
	"NodeNotReady": ChangeNode, "NodeReady": ChangeNode, "Cordon": ChangeNode, "NodeNotSchedulable": ChangeNode,
	"NodeSchedulable": ChangeNode, "Drain": ChangeNode, "Drained": ChangeNode,
}

func auditChangeSource(action string) string {
	prefix := action
	if i := strings.IndexByte(prefix, '.'); i >= 0 {
		prefix = prefix[:i]
	}
	switch prefix {
	case "gitops":
		return ChangeGitOps
	case "helm":
		return ChangeHelm
	case "secret", "configmap":
		return ChangeConfig
	case "node":
		return ChangeNode
	case "hpa":
		return ChangeHPA
	}
	if strings.Contains(action, "rollout") || strings.Contains(action, "restart") || strings.Contains(action, "scale") || strings.Contains(action, "image") {
		return ChangeRollout
	}
	return ChangeAudit
}

// CollectChanges gathers candidates through existing read-only Provider
// methods (no new RBAC) and ranks them. Individual source failures are skipped.
func CollectChanges(ctx context.Context, p Provider, namespace, workload string, at time.Time, window time.Duration) ChangeReport {
	if window <= 0 {
		window = DefaultChangeWindow
	}
	now := time.Now()
	if at.IsZero() {
		at = now
	}
	var cands []ChangeCandidate
	inWindow := func(t time.Time) bool { return !t.Before(at.Add(-window)) && !t.After(at) }

	if evs, err := p.ListEvents(ctx, namespace); err == nil {
		for _, e := range evs {
			src, ok := changeEventReasons[e.Reason]
			if !ok || !inWindow(e.Time) {
				continue
			}
			t := TimelineFromEvent(e)
			cands = append(cands, ChangeCandidate{Source: src, Time: e.Time, Namespace: e.Namespace, Object: e.Object, Summary: t.Summary})
		}
	}
	for _, a := range ListAxiomAudit() {
		if !inWindow(a.Time) {
			continue
		}
		if a.Action == "pod.exec" || strings.HasPrefix(a.Action, "pod.terminal") || strings.HasPrefix(a.Action, "pod.window") {
			continue
		}
		t := TimelineFromAudit(a)
		cands = append(cands, ChangeCandidate{Source: auditChangeSource(a.Action), Time: a.Time, Namespace: t.Namespace, Object: t.Object, Actor: a.User, Summary: t.Summary})
	}
	if items, err := p.ListGitOps(ctx); err == nil {
		for _, g := range items {
			t := AgeToTime(g.Age, now)
			if g.Revision == "" || !inWindow(t) {
				continue
			}
			cands = append(cands, ChangeCandidate{Source: ChangeGitOps, Time: t, Namespace: g.Namespace, Object: g.Kind + "/" + g.Name,
				Summary: g.Kind + " " + g.Name + " synced revision " + g.Revision})
		}
	}
	if rels, err := p.ListHelmReleases(ctx, namespace); err == nil {
		for _, r := range rels {
			t := AgeToTime(r.Age, now)
			if !inWindow(t) {
				continue
			}
			s := "Helm release " + r.Name + " revision " + strconv.Itoa(r.Revision) + " (" + r.Chart + ") " + r.Status
			cands = append(cands, ChangeCandidate{Source: ChangeHelm, Time: t, Namespace: r.Namespace, Object: "HelmRelease/" + r.Name, Summary: s})
		}
	}
	if hpas, err := p.ListHPAs(ctx, namespace); err == nil {
		for _, h := range hpas {
			if h.LastScaleTime == "" {
				continue
			}
			t, err := time.Parse(time.RFC3339, h.LastScaleTime)
			if err != nil {
				t = AgeToTime(h.LastScaleTime, now)
			}
			if !inWindow(t) {
				continue
			}
			cands = append(cands, ChangeCandidate{Source: ChangeHPA, Time: t, Namespace: h.Namespace, Object: h.TargetKind + "/" + h.TargetName,
				Summary: "HPA " + h.Name + " scaled to " + strconv.Itoa(int(h.CurrentReplicas)) + " replicas"})
		}
	}
	// Rollout revisions: the named workload, or the namespace's Deployments (capped).
	if namespace != "" {
		var names []string
		if workload != "" {
			names = []string{workload}
		} else if wls, err := p.ListWorkloads(ctx, namespace); err == nil {
			for _, w := range wls {
				if w.Kind == "Deployment" && len(names) < 25 {
					names = append(names, w.Name)
				}
			}
		}
		for _, n := range names {
			st, err := p.GetRolloutStatus(ctx, namespace, "Deployment", n)
			if err != nil {
				continue
			}
			for _, rev := range st.History {
				te := TimelineFromRollout(namespace, "Deployment", n, rev, now)
				if !inWindow(te.Time) {
					continue
				}
				cands = append(cands, ChangeCandidate{Source: ChangeRollout, Time: te.Time, Namespace: namespace, Object: te.Object, Summary: te.Summary})
			}
		}
	}
	if sd, ok := p.(changeSeeder); ok {
		cands = append(cands, sd.seedChanges(now)...)
	}
	return ChangeReport{Namespace: namespace, Workload: workload, At: at, Window: window.String(),
		Changes: RankChanges(cands, namespace, workload, at, window)}
}
