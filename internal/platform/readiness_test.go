package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAgeHours(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"now", 0, true}, {"30m", 0.5, true}, {"5h", 5, true}, {"3d", 72, true},
		{"", 0, false}, {"-", 0, false}, {"d", 0, false}, {"-3d", 0, false}, {"3x", 0, false},
		{"99999999999d", 0, false}, {"1e3d", 0, false}, {"3d; drop", 0, false},
	}
	for _, c := range cases {
		got, ok := ageHours(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ageHours(%q) = %v,%v want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestBackupIncludes(t *testing.T) {
	cases := []struct {
		inc, ns string
		want    bool
	}{
		{"", "a", true}, {"*", "a", true}, {"a,b", "b", true}, {" a , b ", "b", true},
		{"a,b", "c", false}, {"ab", "a", false}, {"a,*", "zz", true},
	}
	for _, c := range cases {
		if got := backupIncludes(c.inc, c.ns); got != c.want {
			t.Errorf("backupIncludes(%q,%q)=%v", c.inc, c.ns, got)
		}
	}
}

func TestDetectBackupCoverage(t *testing.T) {
	in := BackupCoverageInput{
		Volumes: []Volume{
			{Namespace: "shop", Name: "db", Capacity: "10Gi"},
			{Namespace: "blog", Name: "db", Capacity: "5Gi"},
			{Namespace: "wiki", Name: "db", Capacity: "20Gi"},
			{Namespace: "mail", Name: "db", Capacity: "1Gi"},
			{Namespace: "kube-system", Name: "x", Capacity: "1Gi"},
		},
		Workloads: []Workload{{Namespace: "shop", Name: "web"}, {Namespace: "stateless", Name: "api"}, {Namespace: "axiom-system", Name: "axiom"}},
		Backups: []VeleroBackup{
			{Name: "ok", Phase: "Completed", Namespaces: "shop", Age: "2d"},
			{Name: "old", Phase: "Completed", Namespaces: "blog", Age: "30d"},
			{Name: "bad", Phase: "PartiallyFailed", Namespaces: "wiki", Age: "1d"},
			{Name: "junk", Phase: "Completed", Namespaces: "mail", Age: "garbage"},
			{Name: "inprog", Phase: "InProgress", Namespaces: "*", Age: "now"},
		},
		Schedules: []VeleroSchedule{{Name: "a"}, {Name: "b", Paused: true}},
	}
	rep := DetectBackupCoverage(in, 0)
	if rep.MaxAgeDays != BackupCoverageDefaultMaxAgeDays {
		t.Fatalf("default age %d", rep.MaxAgeDays)
	}
	got := map[string]string{}
	for _, it := range rep.Items {
		got[it.Namespace] = it.Status
	}
	want := map[string]string{"shop": CoverageProtected, "blog": CoverageStale, "wiki": CoverageFailing, "mail": CoverageUnprotected, "stateless": CoverageProtected}
	// "*" with an in-progress backup never counts as good; stateless has no
	// completed backup so it is unprotected.
	want["stateless"] = CoverageUnprotected
	for ns, st := range want {
		if got[ns] != st {
			t.Errorf("%s: got %q want %q", ns, got[ns], st)
		}
	}
	for _, ns := range []string{"kube-system", "axiom-system"} {
		if _, ok := got[ns]; ok {
			t.Errorf("%s must be skipped", ns)
		}
	}
	if rep.Items[0].Status != CoverageUnprotected {
		t.Errorf("worst first, got %q", rep.Items[0].Status)
	}
	if rep.Summary.Schedules != 2 || rep.Summary.Paused != 1 || rep.Summary.Namespaces != 5 {
		t.Errorf("summary %+v", rep.Summary)
	}
	if rep.Summary.DataAtRisk != 26 { // blog 5 + wiki 20 + mail 1 (stale, failing, unprotected)
		t.Errorf("at risk %v", rep.Summary.DataAtRisk)
	}
	// A wildcard completed backup protects everything, and the age limit clamps.
	rep = DetectBackupCoverage(BackupCoverageInput{Volumes: in.Volumes, Backups: []VeleroBackup{{Phase: "Completed", Age: "1h"}}}, 99999)
	if rep.MaxAgeDays != BackupCoverageMaxAgeDaysCap || rep.Summary.ByStatus[CoverageProtected] != 4 {
		t.Errorf("wildcard %+v", rep.Summary)
	}
}

func TestDetectBackupCoverageCapsOutput(t *testing.T) {
	var vols []Volume
	for i := 0; i < backupCoverageMaxNamespaces+50; i++ {
		vols = append(vols, Volume{Namespace: "ns" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + itoa(i), Capacity: "1Gi"})
	}
	rep := DetectBackupCoverage(BackupCoverageInput{Volumes: vols}, 7)
	if len(rep.Items) != backupCoverageMaxNamespaces || !rep.Truncated {
		t.Fatalf("len %d truncated %v", len(rep.Items), rep.Truncated)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for ; i > 0; i /= 10 {
		s = string(rune('0'+i%10)) + s
	}
	return s
}

func TestParsePDBValue(t *testing.T) {
	cases := []struct {
		in      string
		n       int
		pct, ok bool
	}{
		{"1", 1, false, true}, {"50%", 50, true, true}, {"0", 0, false, true}, {"100%", 100, true, true},
		{"", 0, false, false}, {"%", 0, false, false}, {"101%", 0, false, false}, {"-1", 0, false, false},
		{"abc", 0, false, false}, {"1.5", 0, false, false}, {"123456789", 0, false, false}, {"5%%", 0, false, false},
	}
	for _, c := range cases {
		n, pct, ok := parsePDBValue(c.in)
		if n != c.n || pct != c.pct || ok != c.ok {
			t.Errorf("parsePDBValue(%q)=%d,%v,%v", c.in, n, pct, ok)
		}
	}
}

func TestDetectDisruption(t *testing.T) {
	in := DisruptionInput{
		PDBs: []PDBSummary{
			{Namespace: "a", Name: "ghost", MinAvailable: "1", ExpectedPods: 0},
			{Namespace: "a", Name: "single", MinAvailable: "1", ExpectedPods: 1, CurrentHealthy: 1, DesiredHealthy: 1},
			{Namespace: "a", Name: "zero", MaxUnavailable: "0", ExpectedPods: 3, CurrentHealthy: 3, DesiredHealthy: 3},
			{Namespace: "a", Name: "all", MinAvailable: "100%", ExpectedPods: 3, CurrentHealthy: 3, DesiredHealthy: 3},
			{Namespace: "b", Name: "sick", MinAvailable: "2", ExpectedPods: 3, CurrentHealthy: 1, DesiredHealthy: 2},
			{Namespace: "b", Name: "tight", MinAvailable: "2", ExpectedPods: 3, CurrentHealthy: 2, DesiredHealthy: 2},
			{Namespace: "b", Name: "fine", MinAvailable: "1", ExpectedPods: 3, CurrentHealthy: 3, DesiredHealthy: 1},
			{Namespace: "b", Name: "garbage", MinAvailable: "x;y", ExpectedPods: 3, CurrentHealthy: 3, DesiredHealthy: 1},
		},
		Workloads: []Workload{
			{Namespace: "c", Name: "web", Kind: "Deployment", Desired: 3},
			{Namespace: "c", Name: "db", Kind: "StatefulSet", Desired: 2},
			{Namespace: "c", Name: "one", Kind: "Deployment", Desired: 1},
			{Namespace: "c", Name: "job", Kind: "CronJob", Desired: 5},
			{Namespace: "b", Name: "covered", Kind: "Deployment", Desired: 3},
			{Namespace: "kube-system", Name: "dns", Kind: "Deployment", Desired: 2},
		},
	}
	rep := DetectDisruption(in, "")
	kinds := map[string]string{}
	for _, f := range rep.Findings {
		kinds[f.Namespace+"/"+f.Name] = f.Kind
	}
	want := map[string]string{
		"a/ghost": DisruptionNoMatch, "a/single": DisruptionUnsatisfiable, "a/zero": DisruptionUnsatisfiable,
		"a/all": DisruptionUnsatisfiable, "b/sick": DisruptionUnhealthy, "b/tight": DisruptionBlocking,
		"c/": DisruptionNoBudget,
	}
	for k, v := range want {
		if kinds[k] != v {
			t.Errorf("%s: got %q want %q", k, kinds[k], v)
		}
	}
	if len(rep.Findings) != len(want) {
		t.Errorf("unexpected findings: %v", kinds)
	}
	if rep.Findings[0].Severity != "high" || rep.Findings[len(rep.Findings)-1].Kind != DisruptionNoBudget {
		t.Errorf("order: %+v", rep.Findings)
	}
	for _, f := range rep.Findings {
		if f.Kind == DisruptionNoBudget && (len(f.Workloads) != 2 || f.Workloads[0] != "db") {
			t.Errorf("no-budget workloads %v", f.Workloads)
		}
	}
	if only := DetectDisruption(in, "b"); only.Summary.Total != 2 {
		t.Errorf("namespace filter: %+v", only.Summary)
	}
}

func TestDetectDisruptionCapsOutput(t *testing.T) {
	var pdbs []PDBSummary
	for i := 0; i < disruptionMaxFindings+20; i++ {
		pdbs = append(pdbs, PDBSummary{Namespace: "n", Name: "p" + itoa(i), MinAvailable: "1", ExpectedPods: 0})
	}
	rep := DetectDisruption(DisruptionInput{PDBs: pdbs}, "")
	if len(rep.Findings) != disruptionMaxFindings || !rep.Truncated || rep.Summary.Total != disruptionMaxFindings+20 {
		t.Fatalf("cap: %d %v %d", len(rep.Findings), rep.Truncated, rep.Summary.Total)
	}
}

func TestCheckAPIDeprecations(t *testing.T) {
	manifest := `apiVersion: extensions/v1beta1
kind: Ingress
metadata: {name: old, namespace: web}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: fine}
---
# comment only

---
apiVersion: batch/v1beta1
kind: CronJob
metadata: {name: nightly}
---
apiVersion: v1
kind: Endpoints
metadata: {name: ep}
---
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata: {name: crb}
---
apiVersion: v1
kind: List
items:
  - apiVersion: policy/v1beta1
    kind: PodDisruptionBudget
    metadata: {name: inlist}
  - not-a-map
---
- just
- a list
---
kind: NoVersion
`
	rep, err := CheckAPIDeprecations(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Removed != 4 || rep.Deprecated != 1 {
		t.Fatalf("counts removed=%d deprecated=%d: %+v", rep.Removed, rep.Deprecated, rep.Findings)
	}
	if rep.Findings[0].Status != "removed" || rep.Findings[len(rep.Findings)-1].Status != "deprecated" {
		t.Errorf("removed must sort first: %+v", rep.Findings)
	}
	var ing APIDeprecationFinding
	for _, f := range rep.Findings {
		if f.Kind == "Ingress" {
			ing = f
		}
	}
	if ing.RemovedIn != "1.22" || ing.Name != "old" || ing.Namespace != "web" || !strings.Contains(ing.Replacement, "networking.k8s.io/v1") {
		t.Errorf("ingress finding %+v", ing)
	}
	if len(rep.Skipped) != 2 {
		t.Errorf("skipped %v", rep.Skipped)
	}
}

func TestCheckAPIDeprecationsHostile(t *testing.T) {
	bad := map[string]string{
		"empty":         "  \n",
		"nul":           "kind: A\x00",
		"too large":     strings.Repeat("a", APIDeprecationMaxBytes+1),
		"too many docs": strings.Repeat("kind: A\n---\n", apiDeprecationMaxDocs+5),
	}
	for name, in := range bad {
		if _, err := CheckAPIDeprecations(in); !errors.Is(err, ErrAPIDeprecationInput) {
			t.Errorf("%s: err %v", name, err)
		}
	}
	// None of these may panic or hang; each either parses or is skipped.
	hostile := []string{
		"{{{{",
		"a: *b",
		"apiVersion: [1,2]\nkind: {x: y}",
		"apiVersion: extensions/v1beta1\nkind: Ingress\nmetadata: 5",
		"apiVersion: extensions/v1beta1\nkind: Ingress\nmetadata: {name: " + strings.Repeat("n", 5000) + "}",
		"\xef\xbb\xbfapiVersion: v1\nkind: Endpoints",
		"apiVersion: \"" + strings.Repeat("v", 100000) + "\"\nkind: Pod",
	}
	// YAML alias bomb ("billion laughs") well under the size cap.
	bomb := "a: &a [x,x,x,x,x,x,x,x,x]\n"
	prev := "a"
	for _, n := range []string{"b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		bomb += n + ": &" + n + " [*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + ",*" + prev + "]\n"
		prev = n
	}
	hostile = append(hostile, bomb)
	for i, in := range hostile {
		rep, err := CheckAPIDeprecations(in)
		if err != nil {
			t.Errorf("case %d: unexpected error %v", i, err)
			continue
		}
		for _, f := range rep.Findings {
			if len(f.Name) > 253 || len(f.APIVersion) > 100 {
				t.Errorf("case %d: unclipped finding", i)
			}
		}
	}
	// Findings are capped.
	var b strings.Builder
	for i := 0; i < apiDeprecationMaxHits+30; i++ {
		b.WriteString("apiVersion: batch/v1beta1\nkind: CronJob\n---\n")
	}
	if _, err := CheckAPIDeprecations(b.String()); !errors.Is(err, ErrAPIDeprecationInput) {
		t.Errorf("document cap not enforced: %v", err)
	}
	b.Reset()
	b.WriteString("apiVersion: v1\nkind: List\nitems:\n")
	for i := 0; i < apiDeprecationMaxItems+50; i++ {
		b.WriteString("  - {apiVersion: batch/v1beta1, kind: CronJob}\n")
	}
	rep, err := CheckAPIDeprecations(b.String())
	if err != nil || len(rep.Findings) != apiDeprecationMaxItems || len(rep.Skipped) != 1 {
		t.Errorf("item cap: err=%v findings=%d skipped=%v", err, len(rep.Findings), rep.Skipped)
	}
}

func TestReadinessOverDemoProvider(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if rep, err := BackupCoverage(ctx, d, 7); err != nil || len(rep.Items) == 0 {
		t.Fatalf("coverage: %v %+v", err, rep.Summary)
	}
	if rep, err := DisruptionAudit(ctx, d, ""); err != nil || rep.Summary.PDBs == 0 {
		t.Fatalf("disruption: %v %+v", err, rep.Summary)
	}
}

func TestAPIDeprecationTableSane(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range apiDeprecationTable {
		k := d.APIVersion + "|" + d.Kind
		if seen[k] {
			t.Errorf("duplicate %s", k)
		}
		seen[k] = true
		if d.Replacement == "" || d.DeprecatedIn == "" || d.APIVersion == "" {
			t.Errorf("incomplete row %+v", d)
		}
	}
}
