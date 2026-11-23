package api

import (
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func TestBackupCoverageEndpoint(t *testing.T) {
	e := newCostEnv(t, platform.NewMemoryCostBudgetStore())
	w := e.do("GET", "/api/v1/backups/coverage", "", asCost("viewer"))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("coverage: %d %s", w.Code, w.Body.String())
	}
	var rep platform.BackupCoverageReport
	decodeInto(t, w, &rep)
	if rep.MaxAgeDays != 7 || len(rep.Items) == 0 {
		t.Fatalf("report %+v", rep.Summary)
	}
	for _, bad := range []string{"0", "-1", "abc", "366", "1e2"} {
		if w := e.do("GET", "/api/v1/backups/coverage?maxAgeDays="+bad, "", asCost("viewer")); w.Code != 400 {
			t.Errorf("maxAgeDays=%q: %d", bad, w.Code)
		}
	}
	if w := e.do("GET", "/api/v1/backups/coverage", "", asCost("scoped")); w.Code != 403 {
		t.Fatalf("restricted account: %d", w.Code)
	}
}

func TestDisruptionAuditEndpoint(t *testing.T) {
	e := newCostEnv(t, platform.NewMemoryCostBudgetStore())
	w := e.do("GET", "/api/v1/disruption/audit", "", asCost("viewer"))
	if w.Code != 200 {
		t.Fatalf("audit: %d %s", w.Code, w.Body.String())
	}
	var rep platform.DisruptionReport
	decodeInto(t, w, &rep)
	if rep.Summary.PDBs == 0 {
		t.Fatalf("summary %+v", rep.Summary)
	}
	if w := e.do("GET", "/api/v1/disruption/audit?namespace=Bad_NS", "", asCost("viewer")); w.Code != 400 {
		t.Fatalf("bad namespace: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/disruption/audit", "", asCost("scoped")); w.Code != 403 {
		t.Fatalf("restricted account: %d", w.Code)
	}
}

func TestManifestDeprecationsEndpoint(t *testing.T) {
	e := newCostEnv(t, platform.NewMemoryCostBudgetStore())
	yaml := "apiVersion: batch/v1beta1\nkind: CronJob\nmetadata:\n  name: x\n"
	w := e.do("POST", "/api/v1/manifests/deprecations", yaml, asCost("operator"))
	if w.Code != 200 {
		t.Fatalf("raw yaml: %d %s", w.Code, w.Body.String())
	}
	var rep platform.APIDeprecationReport
	decodeInto(t, w, &rep)
	if rep.Removed != 1 || rep.Findings[0].RemovedIn != "1.25" {
		t.Fatalf("report %+v", rep)
	}
	if w := e.do("POST", "/api/v1/manifests/deprecations", yaml, asCost("viewer")); w.Code != 403 {
		t.Fatalf("viewer POST must be refused: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/manifests/deprecations", "", asCost("operator")); w.Code != 400 {
		t.Fatalf("empty body: %d", w.Code)
	}
	big := strings.Repeat("a", platform.APIDeprecationMaxBytes+8192)
	if w := e.do("POST", "/api/v1/manifests/deprecations", big, asCost("operator")); w.Code != 413 {
		t.Fatalf("oversized body: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/manifests/deprecations", yaml, asCost("scopedop")); w.Code != 403 {
		t.Fatalf("restricted operator: %d", w.Code)
	}
}

func TestReadinessMCPTools(t *testing.T) {
	e := newCostEnv(t, platform.NewMemoryCostBudgetStore())
	w := e.do("GET", "/api/v1/mcp/tools", "", asCost("viewer"))
	for _, name := range []string{"backup_coverage", "disruption_audit", "check_api_deprecations"} {
		if w.Code != 200 || !strings.Contains(w.Body.String(), name) {
			t.Errorf("tool %s missing: %d", name, w.Code)
		}
	}
}

func TestManifestDeprecationsEnvelope(t *testing.T) {
	e := newCostEnv(t, platform.NewMemoryCostBudgetStore())
	body := `{"manifest":"apiVersion: extensions/v1beta1\nkind: Ingress\nmetadata:\n  name: a\n"}`
	w := e.do("POST", "/api/v1/manifests/deprecations", body, asCost("operator"))
	var rep platform.APIDeprecationReport
	decodeInto(t, w, &rep)
	if w.Code != 200 || rep.Removed != 1 {
		t.Fatalf("envelope: %d %+v", w.Code, rep)
	}
	// JSON that is itself a Kubernetes object is checked as the manifest.
	obj := `{"apiVersion":"policy/v1beta1","kind":"PodDisruptionBudget","metadata":{"name":"p"}}`
	w = e.do("POST", "/api/v1/manifests/deprecations", obj, asCost("operator"))
	decodeInto(t, w, &rep)
	if w.Code != 200 || rep.Removed != 1 {
		t.Fatalf("object: %d %+v", w.Code, rep)
	}
}
