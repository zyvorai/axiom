package platform

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Read-only aggregation of security scanner output that already lives in the
// cluster as CRs:
//
//   - aquasecurity.github.io VulnerabilityReport / ConfigAuditReport (Trivy Operator)
//   - wgpolicyk8s.io PolicyReport / ClusterPolicyReport (Kyverno, Gatekeeper-compatible)
//
// Axiom never scans anything itself and never writes these objects. The parsing
// is pure and works on the raw unstructured maps so the Kubernetes provider,
// the demo provider and the tests all exercise the same code.

const (
	secMaxWorkloadRows = 500
	secMaxVulnsDetail  = 300
	secMaxChecksDetail = 200
	secMaxPolicyDetail = 200

	secInstallHint = "Axiom reads scanner output but does not run scanners. Install Trivy Operator " +
		"(helm install trivy-operator aqua/trivy-operator -n trivy-system --create-namespace) for vulnerability and config-audit reports, " +
		"and/or Kyverno or Gatekeeper with its policy-report controller for policy reports, then grant Axiom read access to their CRDs (see docs/DEPLOYMENT.md)."
)

// secReportSet is the raw material for both views.
type secReportSet struct {
	Vuln    []map[string]any
	Config  []map[string]any
	Policy  []map[string]any
	Sources []SecuritySource
	// Truncated is set when the provider stopped paging early.
	Truncated bool
}

type secOwner struct{ Namespace, Kind, Name string }

func (o secOwner) key() string { return o.Namespace + "/" + o.Kind + "/" + o.Name }

// ---- tiny unstructured helpers -------------------------------------------

func uget(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func ustr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func uint0(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	}
	return 0
}

func ufloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	}
	return 0
}

func uslice(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, x := range raw {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// ---- severity -------------------------------------------------------------

func (c *SecuritySeverityCounts) add(sev string) {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "CRITICAL":
		c.Critical++
	case "HIGH":
		c.High++
	case "MEDIUM", "MODERATE":
		c.Medium++
	case "LOW":
		c.Low++
	default:
		c.Unknown++
	}
}

func (c *SecuritySeverityCounts) merge(o SecuritySeverityCounts) {
	c.Critical += o.Critical
	c.High += o.High
	c.Medium += o.Medium
	c.Low += o.Low
	c.Unknown += o.Unknown
}

func (c SecuritySeverityCounts) total() int {
	return c.Critical + c.High + c.Medium + c.Low + c.Unknown
}

func (c *SecurityPolicyCounts) merge(o SecurityPolicyCounts) {
	c.Pass += o.Pass
	c.Fail += o.Fail
	c.Warn += o.Warn
	c.Error += o.Error
	c.Skip += o.Skip
}

func (c *SecurityPolicyCounts) add(result string) {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "pass":
		c.Pass++
	case "fail":
		c.Fail++
	case "warn":
		c.Warn++
	case "error":
		c.Error++
	case "skip":
		c.Skip++
	}
}

func severityRank(s string) int {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return 5
	case "HIGH":
		return 4
	case "MEDIUM", "MODERATE":
		return 3
	case "LOW":
		return 2
	}
	return 1
}

// ---- owners ---------------------------------------------------------------

var replicaSetHashRE = regexp.MustCompile(`^(.+)-[a-z0-9]{8,10}$`)

// ownerOf works out which workload a Trivy report describes. Trivy Operator
// labels reports with the scanned controller; for a Deployment that controller
// is the current ReplicaSet, which is folded back to its Deployment (best
// effort, by stripping the pod-template-hash suffix).
func ownerOf(obj map[string]any) secOwner {
	labels, _ := uget(obj, "metadata", "labels").(map[string]any)
	o := secOwner{
		Namespace: firstNonEmptyStr(ustr(labels["trivy-operator.resource.namespace"]), ustr(uget(obj, "metadata", "namespace"))),
		Kind:      ustr(labels["trivy-operator.resource.kind"]),
		Name:      ustr(labels["trivy-operator.resource.name"]),
	}
	if o.Kind == "" || o.Name == "" {
		if refs := uslice(uget(obj, "metadata", "ownerReferences")); len(refs) > 0 {
			o.Kind, o.Name = ustr(refs[0]["kind"]), ustr(refs[0]["name"])
		}
	}
	return normalizeOwner(o)
}

func normalizeOwner(o secOwner) secOwner {
	if o.Kind == "ReplicaSet" {
		if m := replicaSetHashRE.FindStringSubmatch(o.Name); m != nil {
			o.Kind, o.Name = "Deployment", m[1]
		}
	}
	return o
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func vulnReportImage(obj map[string]any) string {
	repo := ustr(uget(obj, "report", "artifact", "repository"))
	if repo == "" {
		return ""
	}
	if reg := ustr(uget(obj, "report", "registry", "server")); reg != "" && !strings.HasPrefix(repo, reg) {
		repo = reg + "/" + repo
	}
	if tag := ustr(uget(obj, "report", "artifact", "tag")); tag != "" {
		repo += ":" + tag
	}
	return repo
}

func summaryCounts(obj map[string]any, items func() SecuritySeverityCounts) SecuritySeverityCounts {
	sum, ok := uget(obj, "report", "summary").(map[string]any)
	if ok {
		return SecuritySeverityCounts{
			Critical: uint0(sum["criticalCount"]), High: uint0(sum["highCount"]), Medium: uint0(sum["mediumCount"]),
			Low: uint0(sum["lowCount"]), Unknown: uint0(sum["unknownCount"]),
		}
	}
	return items()
}

func vulnCountsOf(obj map[string]any) SecuritySeverityCounts {
	return summaryCounts(obj, func() SecuritySeverityCounts {
		var c SecuritySeverityCounts
		for _, v := range uslice(uget(obj, "report", "vulnerabilities")) {
			c.add(ustr(v["severity"]))
		}
		return c
	})
}

func configCountsOf(obj map[string]any) SecuritySeverityCounts {
	return summaryCounts(obj, func() SecuritySeverityCounts {
		var c SecuritySeverityCounts
		for _, ch := range uslice(uget(obj, "report", "checks")) {
			if ok, _ := ch["success"].(bool); !ok {
				c.add(ustr(ch["severity"]))
			}
		}
		return c
	})
}

// policyOwners returns the workloads a PolicyReport result applies to and its
// per-owner counts. A report with a scope counts wholly against that scope;
// otherwise each result is attributed to its own resources.
func policyContributions(obj map[string]any) map[string]struct {
	Owner  secOwner
	Counts SecurityPolicyCounts
} {
	out := map[string]struct {
		Owner  secOwner
		Counts SecurityPolicyCounts
	}{}
	ns := ustr(uget(obj, "metadata", "namespace"))
	results := uslice(obj["results"])
	if scope, ok := obj["scope"].(map[string]any); ok && ustr(scope["name"]) != "" {
		o := normalizeOwner(secOwner{Namespace: firstNonEmptyStr(ustr(scope["namespace"]), ns), Kind: ustr(scope["kind"]), Name: ustr(scope["name"])})
		var c SecurityPolicyCounts
		if sum, ok := obj["summary"].(map[string]any); ok {
			c = SecurityPolicyCounts{Pass: uint0(sum["pass"]), Fail: uint0(sum["fail"]), Warn: uint0(sum["warn"]), Error: uint0(sum["error"]), Skip: uint0(sum["skip"])}
		} else {
			for _, r := range results {
				c.add(ustr(r["result"]))
			}
		}
		out[o.key()] = struct {
			Owner  secOwner
			Counts SecurityPolicyCounts
		}{o, c}
		return out
	}
	for _, r := range results {
		for _, res := range uslice(r["resources"]) {
			o := normalizeOwner(secOwner{Namespace: firstNonEmptyStr(ustr(res["namespace"]), ns), Kind: ustr(res["kind"]), Name: ustr(res["name"])})
			if o.Name == "" {
				continue
			}
			e := out[o.key()]
			e.Owner = o
			e.Counts.add(ustr(r["result"]))
			out[o.key()] = e
		}
	}
	return out
}

// ---- summary --------------------------------------------------------------

func (s secReportSet) summary() SecurityReportSummary {
	sum := SecurityReportSummary{Sources: s.Sources, Workloads: []SecurityWorkloadRow{}, Truncated: s.Truncated}
	for _, src := range s.Sources {
		if src.Available {
			sum.Available = true
		}
	}
	if sum.Sources == nil {
		sum.Sources = []SecuritySource{}
	}
	if !sum.Available {
		sum.InstallHint = secInstallHint
		return sum
	}
	rows := map[string]*SecurityWorkloadRow{}
	row := func(o secOwner) *SecurityWorkloadRow {
		r := rows[o.key()]
		if r == nil {
			r = &SecurityWorkloadRow{Namespace: o.Namespace, Kind: o.Kind, Name: o.Name}
			rows[o.key()] = r
		}
		return r
	}
	imgSeen := map[string]bool{}
	for _, obj := range s.Vuln {
		o := ownerOf(obj)
		if o.Name == "" {
			continue
		}
		c := vulnCountsOf(obj)
		r := row(o)
		r.Vulnerabilities.merge(c)
		sum.Vulnerabilities.merge(c)
		if img := vulnReportImage(obj); img != "" && !imgSeen[o.key()+"|"+img] {
			imgSeen[o.key()+"|"+img] = true
			r.Images = append(r.Images, img)
		}
	}
	for _, obj := range s.Config {
		o := ownerOf(obj)
		if o.Name == "" {
			continue
		}
		c := configCountsOf(obj)
		row(o).Misconfigurations.merge(c)
		sum.Misconfigurations.merge(c)
	}
	for _, obj := range s.Policy {
		for _, contrib := range policyContributions(obj) {
			row(contrib.Owner).Policy.merge(contrib.Counts)
			sum.Policy.merge(contrib.Counts)
		}
	}
	for _, r := range rows {
		sort.Strings(r.Images)
		sum.Workloads = append(sum.Workloads, *r)
	}
	sort.Slice(sum.Workloads, func(i, j int) bool {
		a, b := sum.Workloads[i], sum.Workloads[j]
		switch {
		case a.Vulnerabilities.Critical != b.Vulnerabilities.Critical:
			return a.Vulnerabilities.Critical > b.Vulnerabilities.Critical
		case a.Vulnerabilities.High != b.Vulnerabilities.High:
			return a.Vulnerabilities.High > b.Vulnerabilities.High
		case a.Policy.Fail != b.Policy.Fail:
			return a.Policy.Fail > b.Policy.Fail
		case a.Misconfigurations.total() != b.Misconfigurations.total():
			return a.Misconfigurations.total() > b.Misconfigurations.total()
		case a.Namespace != b.Namespace:
			return a.Namespace < b.Namespace
		case a.Kind != b.Kind:
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
	if len(sum.Workloads) > secMaxWorkloadRows {
		sum.Workloads = sum.Workloads[:secMaxWorkloadRows]
		sum.Truncated = true
	}
	return sum
}

// ---- drill-down -----------------------------------------------------------

func sameOwner(a, b secOwner) bool {
	return strings.EqualFold(a.Namespace, b.Namespace) && strings.EqualFold(a.Kind, b.Kind) && a.Name == b.Name
}

func (s secReportSet) detail(ns, kind, name string) WorkloadSecurityDetail {
	target := normalizeOwner(secOwner{Namespace: ns, Kind: kind, Name: name})
	d := WorkloadSecurityDetail{
		Namespace: ns, Kind: kind, Name: name,
		Vulnerabilities: []SecurityVulnerability{}, Misconfigurations: []SecurityCheck{}, PolicyResults: []SecurityPolicyResult{},
	}
	seenVuln := map[string]bool{}
	imgSeen := map[string]bool{}
	for _, obj := range s.Vuln {
		if !sameOwner(ownerOf(obj), target) {
			continue
		}
		img := vulnReportImage(obj)
		if img != "" && !imgSeen[img] {
			imgSeen[img] = true
			d.Images = append(d.Images, img)
		}
		for _, v := range uslice(uget(obj, "report", "vulnerabilities")) {
			id := ustr(v["vulnerabilityID"])
			k := id + "|" + ustr(v["resource"]) + "|" + ustr(v["installedVersion"]) + "|" + img
			if seenVuln[k] {
				continue
			}
			seenVuln[k] = true
			d.VulnerabilityCnt.add(ustr(v["severity"]))
			d.Vulnerabilities = append(d.Vulnerabilities, SecurityVulnerability{
				ID: id, Severity: strings.ToUpper(ustr(v["severity"])), Package: ustr(v["resource"]),
				Installed: ustr(v["installedVersion"]), Fixed: ustr(v["fixedVersion"]), Title: ustr(v["title"]),
				Image: img, Score: ufloat(v["score"]), Link: safeLink(ustr(v["primaryLink"])),
			})
		}
	}
	sort.Slice(d.Vulnerabilities, func(i, j int) bool {
		a, b := d.Vulnerabilities[i], d.Vulnerabilities[j]
		if ra, rb := severityRank(a.Severity), severityRank(b.Severity); ra != rb {
			return ra > rb
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.ID < b.ID
	})
	if len(d.Vulnerabilities) > secMaxVulnsDetail {
		d.Vulnerabilities = d.Vulnerabilities[:secMaxVulnsDetail]
		d.Truncated = true
	}
	sort.Strings(d.Images)

	for _, obj := range s.Config {
		if !sameOwner(ownerOf(obj), target) {
			continue
		}
		for _, ch := range uslice(uget(obj, "report", "checks")) {
			if ok, _ := ch["success"].(bool); ok {
				continue
			}
			var msgs []string
			if raw, ok := ch["messages"].([]any); ok {
				for _, m := range raw {
					if s, ok := m.(string); ok {
						msgs = append(msgs, s)
					}
				}
			}
			d.MisconfigCnt.add(ustr(ch["severity"]))
			d.Misconfigurations = append(d.Misconfigurations, SecurityCheck{
				ID: ustr(ch["checkID"]), Severity: strings.ToUpper(ustr(ch["severity"])), Title: ustr(ch["title"]),
				Messages: msgs, Category: ustr(ch["category"]),
			})
		}
	}
	sort.Slice(d.Misconfigurations, func(i, j int) bool {
		a, b := d.Misconfigurations[i], d.Misconfigurations[j]
		if ra, rb := severityRank(a.Severity), severityRank(b.Severity); ra != rb {
			return ra > rb
		}
		return a.ID < b.ID
	})
	if len(d.Misconfigurations) > secMaxChecksDetail {
		d.Misconfigurations = d.Misconfigurations[:secMaxChecksDetail]
		d.Truncated = true
	}

	for _, obj := range s.Policy {
		ns := ustr(uget(obj, "metadata", "namespace"))
		scoped := false
		if scope, ok := obj["scope"].(map[string]any); ok && ustr(scope["name"]) != "" {
			scoped = true
			if !sameOwner(normalizeOwner(secOwner{Namespace: firstNonEmptyStr(ustr(scope["namespace"]), ns), Kind: ustr(scope["kind"]), Name: ustr(scope["name"])}), target) {
				continue
			}
		}
		for _, r := range uslice(obj["results"]) {
			if !scoped {
				hit := false
				for _, res := range uslice(r["resources"]) {
					if sameOwner(normalizeOwner(secOwner{Namespace: firstNonEmptyStr(ustr(res["namespace"]), ns), Kind: ustr(res["kind"]), Name: ustr(res["name"])}), target) {
						hit = true
					}
				}
				if !hit {
					continue
				}
			}
			res := strings.ToLower(ustr(r["result"]))
			d.PolicyCnt.add(res)
			// Passing results are counted but not listed: they bury the findings.
			if res == "pass" {
				continue
			}
			d.PolicyResults = append(d.PolicyResults, SecurityPolicyResult{
				Policy: ustr(r["policy"]), Rule: ustr(r["rule"]), Result: res,
				Severity: strings.ToLower(ustr(r["severity"])), Message: ustr(r["message"]), Source: ustr(r["source"]),
			})
		}
	}
	sort.Slice(d.PolicyResults, func(i, j int) bool {
		a, b := d.PolicyResults[i], d.PolicyResults[j]
		if a.Result != b.Result {
			return a.Result < b.Result
		}
		return a.Policy+a.Rule < b.Policy+b.Rule
	})
	if len(d.PolicyResults) > secMaxPolicyDetail {
		d.PolicyResults = d.PolicyResults[:secMaxPolicyDetail]
		d.Truncated = true
	}
	return d
}

// safeLink keeps only http(s) URLs: the value comes from a CR any scanner (or
// anyone with write access to the CRD) controls and is rendered as an href.
func safeLink(u string) string {
	l := strings.ToLower(strings.TrimSpace(u))
	if strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://") {
		return strings.TrimSpace(u)
	}
	return ""
}

func validateWorkloadRef(ns, kind, name string) error {
	for _, v := range []string{ns, kind, name} {
		if strings.TrimSpace(v) == "" || len(v) > 253 {
			return NewStatusError(400, fmt.Errorf("namespace, kind and name are required"))
		}
	}
	return nil
}
