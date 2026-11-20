package platform

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Read-only per-image vulnerability and SBOM view built from Trivy Operator
// VulnerabilityReports and SbomReports (and ClusterSbomReports when present).
// Pure helpers only; providers just fetch the raw unstructured maps.

const (
	vulnMaxImages = 200
	vulnMaxTopCVE = 50
	vulnMaxFixes  = 10
	vulnMaxLic    = 12

	vulnInstallHint = "Axiom reads Trivy Operator output and does not scan. Install Trivy Operator " +
		"(helm install trivy-operator aqua/trivy-operator -n trivy-system --create-namespace); enable " +
		"sbomGenerationEnabled for SBOM data, then grant Axiom read access to its CRDs (see docs/DEPLOYMENT.md)."
)

// VulnFixSuggestion is the lowest fixed version offered for one package.
type VulnFixSuggestion struct {
	Package   string `json:"package"`
	Installed string `json:"installed"`
	FixedIn   string `json:"fixedIn"`
	Fixes     int    `json:"fixes"` // CVEs in this package that upgrading resolves
}

// VulnWorkloadRef is a workload using an image.
type VulnWorkloadRef struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

// VulnLicenseCount is a license and how many components carry it.
type VulnLicenseCount struct {
	License string `json:"license"`
	Count   int    `json:"count"`
}

// VulnImageRollup is one image's vulnerability summary.
type VulnImageRollup struct {
	Image      string                 `json:"image"`
	Counts     SecuritySeverityCounts `json:"counts"`
	Fixable    int                    `json:"fixable"`
	Workloads  []VulnWorkloadRef      `json:"workloads"`
	Fixes      []VulnFixSuggestion    `json:"fixes"`
	Components int                    `json:"components"` // SBOM component count, 0 when no SBOM
	HasSBOM    bool                   `json:"hasSbom"`
	Licenses   []VulnLicenseCount     `json:"licenses,omitempty"`
}

// VulnCVE is one CVE rolled up across images.
type VulnCVE struct {
	ID        string   `json:"id"`
	Severity  string   `json:"severity"`
	Score     float64  `json:"score,omitempty"`
	Package   string   `json:"package"`
	Installed string   `json:"installed"`
	FixedIn   string   `json:"fixedIn,omitempty"`
	Title     string   `json:"title,omitempty"`
	Link      string   `json:"link,omitempty"`
	Images    []string `json:"images"`
}

// VulnSBOMImage is the SBOM component summary of one image.
type VulnSBOMImage struct {
	Image      string             `json:"image"`
	Components int                `json:"components"`
	Licenses   []VulnLicenseCount `json:"licenses"`
	Unlicensed int                `json:"unlicensed"`
}

// VulnerabilityView is GET /api/v1/security/vulnerabilities.
type VulnerabilityView struct {
	Available     bool                   `json:"available"`
	SBOMAvailable bool                   `json:"sbomAvailable"`
	InstallHint   string                 `json:"installHint,omitempty"`
	Sources       []SecuritySource       `json:"sources"`
	Totals        SecuritySeverityCounts `json:"totals"`
	Fixable       int                    `json:"fixable"`
	Images        []VulnImageRollup      `json:"images"`
	TopCVEs       []VulnCVE              `json:"topCves"`
	SBOM          []VulnSBOMImage        `json:"sbom"`
	Truncated     bool                   `json:"truncated,omitempty"`
}

// vulnSet is the raw input of the view.
type vulnSet struct {
	Vuln      []map[string]any
	Sbom      []map[string]any
	Sources   []SecuritySource
	Truncated bool
}

// parseSeverityFilter turns "critical,high" into an upper-case set; empty means all.
func parseSeverityFilter(s string) (map[string]bool, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "all") {
		return nil, nil
	}
	out := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.ToUpper(strings.TrimSpace(p))
		switch p {
		case "CRITICAL", "HIGH", "MEDIUM", "LOW", "UNKNOWN":
			out[p] = true
		case "":
		default:
			return nil, NewStatusError(400, fmt.Errorf("invalid severity %q (use critical, high, medium, low, unknown)", p))
		}
	}
	return out, nil
}

func normSev(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "MODERATE" {
		return "MEDIUM"
	}
	switch s {
	case "CRITICAL", "HIGH", "MEDIUM", "LOW":
		return s
	}
	return "UNKNOWN"
}

// versionParts splits a version into comparable numeric/alpha chunks.
func versionParts(v string) []string {
	v = strings.TrimLeft(strings.TrimSpace(v), "vV")
	var parts []string
	var cur strings.Builder
	digit := false
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	for _, r := range v {
		isD := r >= '0' && r <= '9'
		isA := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !isD && !isA {
			flush()
			continue
		}
		if cur.Len() > 0 && isD != digit {
			flush()
		}
		digit = isD
		cur.WriteRune(r)
	}
	flush()
	return parts
}

// compareVersions is a best-effort comparison good enough to pick the lowest
// fix among a package's fixed versions (numeric chunks numerically).
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		if i >= len(pa) {
			return -1
		}
		if i >= len(pb) {
			return 1
		}
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		switch {
		case ea == nil && eb == nil:
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
		case ea == nil:
			return 1
		case eb == nil:
			return -1
		default:
			if c := strings.Compare(pa[i], pb[i]); c != 0 {
				return c
			}
		}
	}
	return 0
}

// lowestFixed returns the lowest entry of a Trivy fixedVersion field, which may
// list several comma-separated branches ("1.2.3, 1.3.1").
func lowestFixed(fixed string) string {
	best := ""
	for _, f := range strings.Split(fixed, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if best == "" || compareVersions(f, best) < 0 {
			best = f
		}
	}
	return best
}

type vulnImgAcc struct {
	vulns     map[string]map[string]any // id|pkg|installed -> vuln
	summary   SecuritySeverityCounts
	workloads map[string]secOwner
}

func sbomLicenses(comp map[string]any) []string {
	var out []string
	for _, l := range uslice(comp["licenses"]) {
		if e := ustr(l["expression"]); e != "" {
			out = append(out, e)
			continue
		}
		if lic, ok := l["license"].(map[string]any); ok {
			if n := firstNonEmptyStr(ustr(lic["id"]), ustr(lic["name"])); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// sbomComponents returns the CycloneDX component list of an SbomReport.
func sbomComponents(obj map[string]any) []map[string]any {
	return uslice(uget(obj, "report", "components", "components"))
}

func topLicenses(m map[string]int) []VulnLicenseCount {
	out := make([]VulnLicenseCount, 0, len(m))
	for k, v := range m {
		out = append(out, VulnLicenseCount{License: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].License < out[j].License
	})
	if len(out) > vulnMaxLic {
		out = out[:vulnMaxLic]
	}
	return out
}

func (s vulnSet) view(sev map[string]bool) VulnerabilityView {
	v := VulnerabilityView{Sources: s.Sources, Images: []VulnImageRollup{}, TopCVEs: []VulnCVE{}, SBOM: []VulnSBOMImage{}, Truncated: s.Truncated}
	if v.Sources == nil {
		v.Sources = []SecuritySource{}
	}
	for _, src := range s.Sources {
		if src.Available {
			if strings.Contains(src.Name, "Vulnerability") {
				v.Available = true
			} else {
				v.SBOMAvailable = true
			}
		}
	}
	if !v.Available && !v.SBOMAvailable {
		v.InstallHint = vulnInstallHint
		return v
	}
	imgs := map[string]*vulnImgAcc{}
	for _, obj := range s.Vuln {
		img := vulnReportImage(obj)
		if img == "" {
			continue
		}
		a := imgs[img]
		if a == nil {
			a = &vulnImgAcc{vulns: map[string]map[string]any{}, workloads: map[string]secOwner{}}
			imgs[img] = a
		}
		if o := ownerOf(obj); o.Name != "" {
			a.workloads[o.key()] = o
		}
		list := uslice(uget(obj, "report", "vulnerabilities"))
		if len(list) == 0 && a.summary.total() == 0 {
			a.summary = vulnCountsOf(obj)
		}
		for _, x := range list {
			a.vulns[ustr(x["vulnerabilityID"])+"|"+ustr(x["resource"])+"|"+ustr(x["installedVersion"])] = x
		}
	}
	cves := map[string]*VulnCVE{}
	for name, a := range imgs {
		row := VulnImageRollup{Image: name, Workloads: []VulnWorkloadRef{}, Fixes: []VulnFixSuggestion{}}
		fixByPkg := map[string]*VulnFixSuggestion{}
		matched := 0
		for _, x := range a.vulns {
			sv := normSev(ustr(x["severity"]))
			row.Counts.add(sv)
			fixed := lowestFixed(ustr(x["fixedVersion"]))
			if fixed != "" {
				row.Fixable++
			}
			if sev != nil && !sev[sv] {
				continue
			}
			matched++
			key := ustr(x["vulnerabilityID"]) + "|" + ustr(x["resource"])
			c := cves[key]
			if c == nil {
				c = &VulnCVE{ID: ustr(x["vulnerabilityID"]), Severity: sv, Score: ufloat(x["score"]), Package: ustr(x["resource"]),
					Installed: ustr(x["installedVersion"]), Title: ustr(x["title"]), Link: ustr(x["primaryLink"])}
				cves[key] = c
			}
			if fixed != "" && (c.FixedIn == "" || compareVersions(fixed, c.FixedIn) < 0) {
				c.FixedIn = fixed
			}
			c.Images = append(c.Images, name)
			if fixed != "" {
				f := fixByPkg[ustr(x["resource"])]
				if f == nil {
					f = &VulnFixSuggestion{Package: ustr(x["resource"]), Installed: ustr(x["installedVersion"]), FixedIn: fixed}
					fixByPkg[f.Package] = f
				}
				if compareVersions(fixed, f.FixedIn) < 0 {
					f.FixedIn = fixed
				}
				f.Fixes++
			}
		}
		if len(a.vulns) == 0 {
			row.Counts = a.summary
		}
		if sev != nil && matched == 0 {
			continue
		}
		for _, o := range a.workloads {
			row.Workloads = append(row.Workloads, VulnWorkloadRef{Namespace: o.Namespace, Kind: o.Kind, Name: o.Name})
		}
		sort.Slice(row.Workloads, func(i, j int) bool {
			x, y := row.Workloads[i], row.Workloads[j]
			return x.Namespace+"/"+x.Kind+"/"+x.Name < y.Namespace+"/"+y.Kind+"/"+y.Name
		})
		for _, f := range fixByPkg {
			row.Fixes = append(row.Fixes, *f)
		}
		sort.Slice(row.Fixes, func(i, j int) bool {
			if row.Fixes[i].Fixes != row.Fixes[j].Fixes {
				return row.Fixes[i].Fixes > row.Fixes[j].Fixes
			}
			return row.Fixes[i].Package < row.Fixes[j].Package
		})
		if len(row.Fixes) > vulnMaxFixes {
			row.Fixes = row.Fixes[:vulnMaxFixes]
		}
		v.Images = append(v.Images, row)
	}
	sort.Slice(v.Images, func(i, j int) bool {
		a, b := v.Images[i], v.Images[j]
		switch {
		case a.Counts.Critical != b.Counts.Critical:
			return a.Counts.Critical > b.Counts.Critical
		case a.Counts.High != b.Counts.High:
			return a.Counts.High > b.Counts.High
		case a.Counts.total() != b.Counts.total():
			return a.Counts.total() > b.Counts.total()
		}
		return a.Image < b.Image
	})
	if len(v.Images) > vulnMaxImages {
		v.Images = v.Images[:vulnMaxImages]
		v.Truncated = true
	}
	for _, c := range cves {
		sort.Strings(c.Images)
		v.TopCVEs = append(v.TopCVEs, *c)
	}
	sort.Slice(v.TopCVEs, func(i, j int) bool {
		a, b := v.TopCVEs[i], v.TopCVEs[j]
		switch {
		case vulnSeverityRank(a.Severity) != vulnSeverityRank(b.Severity):
			return vulnSeverityRank(a.Severity) > vulnSeverityRank(b.Severity)
		case a.Score != b.Score:
			return a.Score > b.Score
		case len(a.Images) != len(b.Images):
			return len(a.Images) > len(b.Images)
		}
		return a.ID < b.ID
	})
	if len(v.TopCVEs) > vulnMaxTopCVE {
		v.TopCVEs = v.TopCVEs[:vulnMaxTopCVE]
	}

	// SBOM: one entry per image, keeping the largest report when an image has several.
	sboms := map[string]*VulnSBOMImage{}
	for _, obj := range s.Sbom {
		img := vulnReportImage(obj)
		if img == "" {
			continue
		}
		comps := sbomComponents(obj)
		n := len(comps)
		if n == 0 {
			n = uint0(uget(obj, "report", "summary", "componentsCount"))
		}
		if prev := sboms[img]; prev != nil && prev.Components >= n {
			continue
		}
		lic := map[string]int{}
		unl := 0
		for _, c := range comps {
			ls := sbomLicenses(c)
			if len(ls) == 0 {
				unl++
			}
			for _, l := range ls {
				lic[l]++
			}
		}
		sboms[img] = &VulnSBOMImage{Image: img, Components: n, Licenses: topLicenses(lic), Unlicensed: unl}
	}
	for i := range v.Images {
		if sb := sboms[v.Images[i].Image]; sb != nil {
			v.Images[i].HasSBOM, v.Images[i].Components, v.Images[i].Licenses = true, sb.Components, sb.Licenses
		}
	}
	for _, sb := range sboms {
		v.SBOM = append(v.SBOM, *sb)
	}
	sort.Slice(v.SBOM, func(i, j int) bool {
		if v.SBOM[i].Components != v.SBOM[j].Components {
			return v.SBOM[i].Components > v.SBOM[j].Components
		}
		return v.SBOM[i].Image < v.SBOM[j].Image
	})
	for _, r := range v.Images {
		v.Totals.merge(r.Counts)
		v.Fixable += r.Fixable
	}
	return v
}

// ListVulnerabilities returns the per-image vulnerability + SBOM view.
func ListVulnerabilities(ctx context.Context, p Provider, ns, severity string) (VulnerabilityView, error) {
	if _, err := parseSeverityFilter(severity); err != nil {
		return VulnerabilityView{}, err
	}
	if sp, ok := p.(*SwitchableProvider); ok {
		return sp.ListVulnerabilities(ctx, ns, severity)
	}
	if l, ok := p.(interface {
		ListVulnerabilities(context.Context, string, string) (VulnerabilityView, error)
	}); ok {
		return l.ListVulnerabilities(ctx, ns, severity)
	}
	return emptyVulnView(), nil
}

func emptyVulnView() VulnerabilityView {
	return VulnerabilityView{Sources: []SecuritySource{}, Images: []VulnImageRollup{}, TopCVEs: []VulnCVE{}, SBOM: []VulnSBOMImage{}, InstallHint: vulnInstallHint}
}

// ---- demo -----------------------------------------------------------------

func demoSbomReport(ns, kind, name, repo, tag string, comps ...[2]string) map[string]any {
	list := make([]any, 0, len(comps))
	for _, c := range comps {
		m := map[string]any{"name": c[0], "version": "1.0", "type": "library"}
		if c[1] != "" {
			m["licenses"] = []any{map[string]any{"license": map[string]any{"id": c[1]}}}
		}
		list = append(list, m)
	}
	return map[string]any{
		"metadata": map[string]any{"name": strings.ToLower(kind) + "-" + name, "namespace": ns, "labels": map[string]any{
			"trivy-operator.resource.kind": kind, "trivy-operator.resource.name": name, "trivy-operator.resource.namespace": ns}},
		"report": map[string]any{
			"artifact":   map[string]any{"repository": repo, "tag": tag},
			"registry":   map[string]any{"server": "ghcr.io"},
			"summary":    map[string]any{"componentsCount": int64(len(comps))},
			"components": map[string]any{"bomFormat": "CycloneDX", "components": list},
		},
	}
}

func demoVulnSet() vulnSet {
	base := demoSecurityReports()
	sbom := []map[string]any{
		demoSbomReport("payments", "ReplicaSet", "payments-api-7d9f8c6b5", "zyvorai/payments", "v2.8.4",
			[2]string{"golang.org/x/crypto", "BSD-3-Clause"}, [2]string{"golang.org/x/net", "BSD-3-Clause"}, [2]string{"stdlib", "BSD-3-Clause"},
			[2]string{"libssl3", "Apache-2.0"}, [2]string{"perl-base", "GPL-1.0-or-later"}, [2]string{"busybox", "GPL-2.0-only"}, [2]string{"ca-certificates", ""}),
		demoSbomReport("commerce", "ReplicaSet", "checkout-5c8b7d9f4", "zyvorai/checkout", "v1.14.2",
			[2]string{"openssh-client", "BSD-2-Clause"}, [2]string{"libssl3", "Apache-2.0"}, [2]string{"zlib1g", "Zlib"}, [2]string{"tzdata", ""}),
		demoSbomReport("data", "StatefulSet", "redis", "library/redis", "7.4",
			[2]string{"redis", "BSD-3-Clause"}, [2]string{"gpgv", "GPL-3.0-or-later"}, [2]string{"apt", "GPL-2.0-or-later"}),
	}
	return vulnSet{
		Vuln: base.Vuln, Sbom: sbom,
		Sources: []SecuritySource{
			{Name: "Trivy VulnerabilityReports", Group: "aquasecurity.github.io", Available: true, Reports: len(base.Vuln)},
			{Name: "Trivy SbomReports", Group: "aquasecurity.github.io", Available: true, Reports: len(sbom)},
		},
	}
}

func filterVulnNamespace(set vulnSet, ns string) vulnSet {
	if ns == "" || ns == "all" {
		return set
	}
	pick := func(in []map[string]any) []map[string]any {
		var out []map[string]any
		for _, o := range in {
			if ustr(uget(o, "metadata", "namespace")) == ns {
				out = append(out, o)
			}
		}
		return out
	}
	set.Vuln, set.Sbom = pick(set.Vuln), pick(set.Sbom)
	srcs := append([]SecuritySource(nil), set.Sources...)
	for i := range srcs {
		if strings.Contains(srcs[i].Name, "Vulnerability") {
			srcs[i].Reports = len(set.Vuln)
		} else {
			srcs[i].Reports = len(set.Sbom)
		}
	}
	set.Sources = srcs
	return set
}

func (d *DemoProvider) ListVulnerabilities(ctx context.Context, ns, severity string) (VulnerabilityView, error) {
	sev, err := parseSeverityFilter(severity)
	if err != nil {
		return VulnerabilityView{}, err
	}
	return filterVulnNamespace(demoVulnSet(), ns).view(sev), nil
}
