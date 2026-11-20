package platform

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// Helm release history, revision diff and rollback review. Everything in this
// file is pure (no Kubernetes client, no build tag) so it is unit-tested.

// HelmRevision is one entry of a release's history.
type HelmRevision struct {
	Revision     int    `json:"revision"`
	Status       string `json:"status"`
	Chart        string `json:"chart"`
	ChartVersion string `json:"chartVersion"`
	AppVersion   string `json:"appVersion"`
	Updated      string `json:"updated"`
	Description  string `json:"description"`
}

// HelmRevisionData is the decoded, already-redacted content of a revision.
type HelmRevisionData struct {
	HelmRevision
	Values   string `json:"-"` // redacted "a.b: value" rendering of the release's own values
	Manifest string `json:"-"` // redacted rendered manifest
	Hooks    int    `json:"-"`
}

// HelmDiffLine is one line of a diff. Op is " ", "+" or "-".
type HelmDiffLine struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// HelmRevisionDiff compares two revisions of one release.
type HelmRevisionDiff struct {
	Release   string         `json:"release"`
	Namespace string         `json:"namespace"`
	From      int            `json:"from"`
	To        int            `json:"to"`
	Values    []HelmDiffLine `json:"values"`
	Manifest  []HelmDiffLine `json:"manifest"`
	Changed   bool           `json:"changed"`
}

// HelmRollbackPreview is the review step before a rollback.
type HelmRollbackPreview struct {
	Release       string         `json:"release"`
	Namespace     string         `json:"namespace"`
	Current       int            `json:"current"`
	Target        int            `json:"target"`
	Warnings      []string       `json:"warnings"`
	Added         int            `json:"added"`
	Removed       int            `json:"removed"`
	ValuesChanged bool           `json:"valuesChanged"`
	Manifest      []HelmDiffLine `json:"manifest"`
	Values        []HelmDiffLine `json:"values"`
	ConfirmName   string         `json:"confirmName"`
}

// HelmRollbackRequest is the input of the rollback (revision and confirm come
// from the body, namespace and name from the path).
type HelmRollbackRequest struct {
	Namespace string `json:"-"`
	Name      string `json:"-"`
	Revision  int    `json:"revision"`
	Confirm   string `json:"confirm"`
}

const helmRedacted = "<redacted>"

var (
	helmSecretKey = regexp.MustCompile(`(?i)(password|passwd|token|secret|key|credential)`)
	helmKVLine    = regexp.MustCompile(`^(\s*(?:-\s+)?)([A-Za-z0-9_.\-/]+)(\s*:\s*)(\S.*)$`)
	helmKindSec   = regexp.MustCompile(`(?m)^kind:\s*Secret\s*$`)
)

// redactHelmValues replaces every value whose key looks like a credential,
// recursively, and renders the result as sorted "a.b.c: value" lines.
func redactHelmValues(v map[string]any) string {
	var lines []string
	var walk func(prefix string, x any)
	walk = func(prefix string, x any) {
		switch t := x.(type) {
		case map[string]any:
			if len(t) == 0 && prefix != "" {
				lines = append(lines, prefix+": {}")
			}
			for k, c := range t {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				if helmSecretKey.MatchString(k) {
					lines = append(lines, p+": "+helmRedacted)
					continue
				}
				walk(p, c)
			}
		case []any:
			if len(t) == 0 {
				lines = append(lines, prefix+": []")
			}
			for i, c := range t {
				walk(fmt.Sprintf("%s[%d]", prefix, i), c)
			}
		default:
			b, _ := json.Marshal(t)
			lines = append(lines, prefix+": "+string(b))
		}
	}
	walk("", v)
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// redactHelmManifest masks credential-looking values in a rendered manifest
// and every value under data/stringData of Secret documents.
func redactHelmManifest(m string) string {
	docs := strings.Split(m, "\n---")
	for i, doc := range docs {
		isSecret := helmKindSec.MatchString(doc)
		lines := strings.Split(doc, "\n")
		inData, dataIndent := false, 0
		for j, ln := range lines {
			indent := len(ln) - len(strings.TrimLeft(ln, " "))
			trim := strings.TrimSpace(ln)
			if isSecret && inData {
				if trim == "" {
					continue
				}
				if indent > dataIndent {
					if kv := helmKVLine.FindStringSubmatch(ln); kv != nil {
						lines[j] = kv[1] + kv[2] + kv[3] + helmRedacted
					} else {
						lines[j] = strings.Repeat(" ", indent) + helmRedacted
					}
					continue
				}
				inData = false
			}
			if isSecret && (trim == "data:" || trim == "stringData:") {
				inData, dataIndent = true, indent
				continue
			}
			if kv := helmKVLine.FindStringSubmatch(ln); kv != nil && helmSecretKey.MatchString(kv[2]) {
				lines[j] = kv[1] + kv[2] + kv[3] + helmRedacted
			}
		}
		docs[i] = strings.Join(lines, "\n")
	}
	return strings.Join(docs, "\n---")
}

type helmFullRelease struct {
	Version  int            `json:"version"`
	Manifest string         `json:"manifest"`
	Config   map[string]any `json:"config"`
	Hooks    []any          `json:"hooks"`
	Info     struct {
		Status       string `json:"status"`
		Description  string `json:"description"`
		LastDeployed string `json:"last_deployed"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
	} `json:"chart"`
}

// decodeHelmRevision decodes a sh.helm.release.v1.* payload (base64, gzip,
// JSON) into redacted revision data. Only the release's own values and
// manifest are retained; nothing else from the Secret leaves this function.
func decodeHelmRevision(data []byte) (HelmRevisionData, error) {
	raw := data
	if dec, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(data))); err == nil {
		raw = dec
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return HelmRevisionData{}, fmt.Errorf("decode helm release: %w", err)
	}
	defer zr.Close()
	plain, err := io.ReadAll(io.LimitReader(zr, 64<<20))
	if err != nil {
		return HelmRevisionData{}, fmt.Errorf("decode helm release: %w", err)
	}
	var r helmFullRelease
	if err := json.Unmarshal(plain, &r); err != nil {
		return HelmRevisionData{}, fmt.Errorf("decode helm release: %w", err)
	}
	md := r.Chart.Metadata
	chart := md.Name
	if md.Version != "" {
		chart += "-" + md.Version
	}
	return HelmRevisionData{
		HelmRevision: HelmRevision{
			Revision: r.Version, Status: r.Info.Status, Chart: chart,
			ChartVersion: md.Version, AppVersion: md.AppVersion,
			Updated: r.Info.LastDeployed, Description: r.Info.Description,
		},
		Values:   redactHelmValues(r.Config),
		Manifest: redactHelmManifest(r.Manifest),
		Hooks:    len(r.Hooks),
	}, nil
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// diffHelmText is an LCS line diff; unchanged lines are kept for context.
func diffHelmText(a, b string) []HelmDiffLine {
	al, bl := splitLines(a), splitLines(b)
	n, m := len(al), len(bl)
	if n*m > 4_000_000 { // pathological size: show a full replace
		out := make([]HelmDiffLine, 0, n+m)
		for _, l := range al {
			out = append(out, HelmDiffLine{"-", l})
		}
		for _, l := range bl {
			out = append(out, HelmDiffLine{"+", l})
		}
		return out
	}
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case al[i] == bl[j]:
				dp[i][j] = dp[i+1][j+1] + 1
			case dp[i+1][j] >= dp[i][j+1]:
				dp[i][j] = dp[i+1][j]
			default:
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var out []HelmDiffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case al[i] == bl[j]:
			out = append(out, HelmDiffLine{" ", al[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			out = append(out, HelmDiffLine{"-", al[i]})
			i++
		default:
			out = append(out, HelmDiffLine{"+", bl[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, HelmDiffLine{"-", al[i]})
	}
	for ; j < m; j++ {
		out = append(out, HelmDiffLine{"+", bl[j]})
	}
	return out
}

func helmDiffChanged(d []HelmDiffLine) bool {
	for _, l := range d {
		if l.Op != " " {
			return true
		}
	}
	return false
}

func helmDiffCounts(d []HelmDiffLine) (added, removed int) {
	for _, l := range d {
		switch l.Op {
		case "+":
			added++
		case "-":
			removed++
		}
	}
	return
}

func helmManifestHas(m, kind string) bool {
	return regexp.MustCompile(`(?m)^kind:\s*` + regexp.QuoteMeta(kind) + `\s*$`).MatchString(m)
}

func buildHelmDiff(ns, name string, from, to HelmRevisionData) HelmRevisionDiff {
	v := diffHelmText(from.Values, to.Values)
	m := diffHelmText(from.Manifest, to.Manifest)
	return HelmRevisionDiff{Release: name, Namespace: ns, From: from.Revision, To: to.Revision,
		Values: v, Manifest: m, Changed: helmDiffChanged(v) || helmDiffChanged(m)}
}

// previousHelmRevision returns the highest revision below current (0 if none).
func previousHelmRevision(revs []HelmRevision, current int) int {
	best := 0
	for _, r := range revs {
		if r.Revision < current && r.Revision > best {
			best = r.Revision
		}
	}
	return best
}

// liveHelmRevision is the highest "deployed" revision, else the highest one.
func liveHelmRevision(revs []HelmRevision) int {
	live, max := 0, 0
	for _, r := range revs {
		if r.Revision > max {
			max = r.Revision
		}
		if r.Status == "deployed" && r.Revision > live {
			live = r.Revision
		}
	}
	if live == 0 {
		return max
	}
	return live
}

// buildHelmRollbackPreview computes what a rollback from cur to target changes
// and what it cannot undo: Helm does not revert data on PVCs or migrate
// custom resources back, and the target's hooks run again.
func buildHelmRollbackPreview(ns, name string, cur, target HelmRevisionData) HelmRollbackPreview {
	d := buildHelmDiff(ns, name, cur, target)
	add, rem := helmDiffCounts(d.Manifest)
	p := HelmRollbackPreview{
		Release: name, Namespace: ns, Current: cur.Revision, Target: target.Revision,
		Added: add, Removed: rem, ValuesChanged: helmDiffChanged(d.Values),
		Manifest: d.Manifest, Values: d.Values, ConfirmName: name, Warnings: []string{},
	}
	if target.Revision == cur.Revision {
		p.Warnings = append(p.Warnings, "same-revision")
	}
	if target.Status == "failed" {
		p.Warnings = append(p.Warnings, "target-failed")
	}
	if helmManifestHas(cur.Manifest, "PersistentVolumeClaim") || helmManifestHas(target.Manifest, "PersistentVolumeClaim") {
		p.Warnings = append(p.Warnings, "pvc")
	}
	if helmManifestHas(cur.Manifest, "CustomResourceDefinition") || helmManifestHas(target.Manifest, "CustomResourceDefinition") {
		p.Warnings = append(p.Warnings, "crd")
	}
	if cur.Hooks > 0 || target.Hooks > 0 {
		p.Warnings = append(p.Warnings, "hooks")
	}
	return p
}

// checkHelmRollbackConfirm enforces the typed confirmation server-side.
func checkHelmRollbackConfirm(p HelmRollbackPreview, confirm string) error {
	if confirm != p.ConfirmName {
		return gitOpsBad("rolling back replaces the live release; pass confirm=%s", p.ConfirmName)
	}
	return nil
}
