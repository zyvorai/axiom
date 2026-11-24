package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

// Cluster hygiene audits (GET /api/v1/audits/...): four read-only reports in
// the spirit of Popeye and similar cluster sanitizers, built only from lists
// the Provider already serves. Nothing here writes to the cluster.

const (
	auditMaxFindings = 300

	QuotaDefaultThreshold = 80

	AuditQuotaExhausted = "exhausted"
	AuditQuotaNear      = "near-limit"

	AuditImageLatest = "latest-tag"
	AuditImageNoTag  = "no-tag"

	AuditHPACannotScale   = "cannot-scale"
	AuditHPAAtMax         = "at-max"
	AuditHPAMetricMissing = "metric-missing"
	AuditHPAFixedSize     = "fixed-size"

	AuditSvcNoMatch  = "selector-matches-nothing"
	AuditSvcNotReady = "no-ready-endpoints"
)

// AuditFinding is one object that needs a look.
type AuditFinding struct {
	Kind      string `json:"kind"`
	Severity  string `json:"severity"` // high | medium | low
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Detail    string `json:"detail,omitempty"` // for example the quota resource or the image
	Reason    string `json:"reason"`
	Suggest   string `json:"suggestion"`
}

// AuditReport is the shared shape of the four reports.
type AuditReport struct {
	Summary struct {
		Total   int            `json:"total"`
		Scanned int            `json:"scanned"`
		ByKind  map[string]int `json:"byKind"`
	} `json:"summary"`
	Findings  []AuditFinding `json:"findings"`
	Truncated bool           `json:"truncated,omitempty"`
	Note      string         `json:"note"`
}

func finishAudit(out []AuditFinding, scanned int, note string) AuditReport {
	rank := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if rank[a.Severity] != rank[b.Severity] {
			return rank[a.Severity] < rank[b.Severity]
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Detail < b.Detail
	})
	rep := AuditReport{Findings: out, Note: note}
	rep.Summary.Total = len(out)
	rep.Summary.Scanned = scanned
	rep.Summary.ByKind = map[string]int{}
	for _, f := range out {
		rep.Summary.ByKind[f.Kind]++
	}
	if len(rep.Findings) > auditMaxFindings {
		rep.Findings = rep.Findings[:auditMaxFindings]
		rep.Truncated = true
	}
	if rep.Findings == nil {
		rep.Findings = []AuditFinding{}
	}
	return rep
}

func auditInScope(namespace, ns string) bool {
	return namespace == "" || namespace == "all" || ns == namespace
}

func auditClip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// ---- 1. Quota headroom ----

// DetectQuotaHeadroom reports ResourceQuota entries whose usage is at or
// above threshold percent of the hard limit. threshold must be 1..100.
func DetectQuotaHeadroom(quotas []ResourceQuotaSummary, namespace string, threshold int) AuditReport {
	var out []AuditFinding
	scanned := 0
	for _, q := range quotas {
		if !auditInScope(namespace, q.Namespace) {
			continue
		}
		scanned++
		keys := make([]string, 0, len(q.Hard))
		for k := range q.Hard {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			hard, err1 := resource.ParseQuantity(q.Hard[k])
			used, err2 := resource.ParseQuantity(q.Used[k])
			if err1 != nil || err2 != nil {
				continue
			}
			h, u := hard.AsApproximateFloat64(), used.AsApproximateFloat64()
			if h <= 0 && u <= 0 {
				continue
			}
			f := AuditFinding{Namespace: q.Namespace, Name: q.Name, Detail: k}
			pct := 100.0
			if h > 0 {
				pct = u / h * 100
			}
			switch {
			case h <= 0 || u >= h:
				f.Kind, f.Severity = AuditQuotaExhausted, "high"
				f.Reason = fmt.Sprintf("%s is fully used (%s of %s): new objects or Pods that need it are rejected.", k, q.Used[k], q.Hard[k])
				f.Suggest = "Raise the quota, or free the resource (delete unused objects, rightsize requests)."
			case pct >= float64(threshold):
				f.Kind, f.Severity = AuditQuotaNear, "medium"
				f.Reason = fmt.Sprintf("%s is at %.0f%% (%s of %s), above the %d%% threshold.", k, pct, q.Used[k], q.Hard[k], threshold)
				f.Suggest = "Plan a quota increase before the next deploy or scale-up is refused."
			default:
				continue
			}
			out = append(out, f)
		}
	}
	return finishAudit(out, scanned, "Compares each ResourceQuota used value with its hard limit. Quotas count requests and objects, not live consumption. Axiom changes nothing from this report.")
}

func QuotaHeadroom(ctx context.Context, p Provider, namespace string, threshold int) (AuditReport, error) {
	qs, err := p.ListResourceQuotas(ctx, "")
	if err != nil {
		return AuditReport{}, err
	}
	return DetectQuotaHeadroom(qs, namespace, threshold), nil
}

// ---- 2. Image tag hygiene ----

// splitImageRef returns the tag and whether the reference is digest-pinned.
// A registry port ("host:5000/app") is not a tag.
func splitImageRef(ref string) (tag string, pinned bool) {
	if strings.Contains(ref, "@") {
		return "", true
	}
	last := ref
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		last = ref[i+1:]
	}
	if i := strings.Index(last, ":"); i >= 0 {
		return last[i+1:], false
	}
	return "", false
}

// DetectImageHygiene flags workloads that run a mutable image: the latest tag
// or no tag at all (which means latest). Digest-pinned images pass.
func DetectImageHygiene(ws []Workload, namespace string) AuditReport {
	var out []AuditFinding
	scanned := 0
	for _, w := range ws {
		if !auditInScope(namespace, w.Namespace) {
			continue
		}
		scanned++
		for _, ref := range strings.FieldsFunc(w.Image, func(r rune) bool { return r == ',' || r == ' ' }) {
			tag, pinned := splitImageRef(ref)
			if pinned {
				continue
			}
			f := AuditFinding{Namespace: w.Namespace, Name: w.Name, Detail: auditClip(ref, 200)}
			switch {
			case tag == "":
				f.Kind, f.Severity = AuditImageNoTag, "medium"
				f.Reason = "The image has no tag, so the runtime pulls latest: what runs can change between restarts."
			case strings.EqualFold(tag, "latest"):
				f.Kind, f.Severity = AuditImageLatest, "medium"
				f.Reason = "The image uses the latest tag, which is mutable: a restart on another node can run different code."
			default:
				continue
			}
			f.Suggest = "Pin a version tag, or better a digest (image@sha256:...), so rollouts and rollbacks are reproducible."
			out = append(out, f)
		}
	}
	return finishAudit(out, scanned, "Reads the image the workload summary reports (the first container). Tags other than latest are not judged, and a digest-pinned image always passes. Axiom changes nothing from this report.")
}

func ImageHygiene(ctx context.Context, p Provider, namespace string) (AuditReport, error) {
	ws, err := p.ListWorkloads(ctx, "")
	if err != nil {
		return AuditReport{}, err
	}
	return DetectImageHygiene(ws, namespace), nil
}

// ---- 3. Autoscaler saturation ----

// DetectHPASaturation reports HorizontalPodAutoscalers that cannot do their job.
func DetectHPASaturation(hpas []HPASummary, namespace string) AuditReport {
	var out []AuditFinding
	scanned := 0
	for _, h := range hpas {
		if !auditInScope(namespace, h.Namespace) {
			continue
		}
		scanned++
		base := AuditFinding{Namespace: h.Namespace, Name: h.Name, Detail: h.TargetKind + "/" + h.TargetName}
		broken := false
		for _, c := range h.Conditions {
			if (c.Type == "AbleToScale" || c.Type == "ScalingActive") && c.Status == "False" {
				f := base
				f.Kind, f.Severity = AuditHPACannotScale, "high"
				f.Reason = fmt.Sprintf("%s is False (%s): %s", c.Type, c.Reason, auditClip(c.Message, 200))
				f.Suggest = "Fix the target reference, metrics-server or the custom metric source; the HPA is not scaling anything meanwhile."
				out = append(out, f)
				broken = true
				break
			}
		}
		if !broken {
			for _, m := range h.Metrics {
				if m.Current == "" {
					f := base
					f.Kind, f.Severity = AuditHPAMetricMissing, "medium"
					f.Detail = base.Detail + " " + m.Name
					f.Reason = fmt.Sprintf("Metric %q has no current value, so it cannot drive scaling.", m.Name)
					f.Suggest = "Check that the metrics adapter serves this metric and that Pods report it."
					out = append(out, f)
					break
				}
			}
		}
		if h.MaxReplicas > 0 && h.CurrentReplicas >= h.MaxReplicas && h.MinReplicas != h.MaxReplicas {
			f := base
			f.Kind, f.Severity = AuditHPAAtMax, "medium"
			f.Reason = fmt.Sprintf("Running %d of a maximum of %d replicas: load beyond this cannot be absorbed.", h.CurrentReplicas, h.MaxReplicas)
			f.Suggest = "Raise maxReplicas if the load is real, and check node capacity or quotas can host the extra Pods."
			out = append(out, f)
		}
		if h.MinReplicas == h.MaxReplicas && h.MaxReplicas > 0 {
			f := base
			f.Kind, f.Severity = AuditHPAFixedSize, "low"
			f.Reason = fmt.Sprintf("minReplicas and maxReplicas are both %d: the HPA can never scale.", h.MaxReplicas)
			f.Suggest = "Widen the range, or remove the HPA and set the replica count directly."
			out = append(out, f)
		}
	}
	return finishAudit(out, scanned, "Reads HorizontalPodAutoscaler status as the API server reports it. A workload at its maximum may be fine at peak; check it against the load. Axiom changes nothing from this report.")
}

func HPASaturation(ctx context.Context, p Provider, namespace string) (AuditReport, error) {
	hs, err := p.ListHPAs(ctx, "")
	if err != nil {
		return AuditReport{}, err
	}
	return DetectHPASaturation(hs, namespace), nil
}

// ---- 4. Service wiring ----

func selectorMatches(sel, labels map[string]string) bool {
	if len(sel) == 0 {
		return false
	}
	for k, v := range sel {
		if lv, ok := labels[k]; !ok || lv != v {
			return false
		}
	}
	return true
}

// DetectServiceWiring reports Services that have a selector but no endpoints:
// either no Pod carries the selector's labels (a typo or a deleted workload),
// or matching Pods exist but none is ready.
func DetectServiceWiring(svcs []Service, pods []Pod, namespace string) AuditReport {
	byNS := map[string][]Pod{}
	for _, p := range pods {
		byNS[p.Namespace] = append(byNS[p.Namespace], p)
	}
	var out []AuditFinding
	scanned := 0
	for _, s := range svcs {
		if !auditInScope(namespace, s.Namespace) || s.Type == "ExternalName" || len(s.Selector) == 0 {
			continue
		}
		scanned++
		if len(s.Endpoints) > 0 {
			continue
		}
		matched := 0
		for _, p := range byNS[s.Namespace] {
			if selectorMatches(s.Selector, p.Labels) {
				matched++
			}
		}
		f := AuditFinding{Namespace: s.Namespace, Name: s.Name, Detail: s.Type}
		if matched == 0 {
			f.Kind, f.Severity = AuditSvcNoMatch, "high"
			f.Reason = "No Pod in the namespace carries the labels in the Service selector, so traffic to it fails."
			f.Suggest = "Compare the selector with the Pod labels (a typo is common), or delete the Service if its workload is gone."
		} else {
			f.Kind, f.Severity = AuditSvcNotReady, "medium"
			f.Reason = fmt.Sprintf("%d Pod(s) match the selector but none is ready, so the Service has no endpoints.", matched)
			f.Suggest = "Look at the Pods' readiness probes and status; the Service is working as designed."
		}
		out = append(out, f)
	}
	return finishAudit(out, scanned, "Matches the Service selector against Pod labels in the same namespace. Services without a selector (manual Endpoints) and ExternalName Services are skipped. Axiom changes nothing from this report.")
}

func ServiceWiring(ctx context.Context, p Provider, namespace string) (AuditReport, error) {
	svcs, err := p.ListServices(ctx, "")
	if err != nil {
		return AuditReport{}, err
	}
	pods, err := p.ListPods(ctx, "")
	if err != nil {
		return AuditReport{}, err
	}
	return DetectServiceWiring(svcs, pods, namespace), nil
}
