package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/platform"
)

func getTimeline(t *testing.T, h http.Handler, query string) []platform.TimelineEntry {
	t.Helper()
	w := request(t, h, "GET", "/api/v1/timeline"+query, "")
	if w.Code != 200 {
		t.Fatalf("timeline%s: %d %s", query, w.Code, w.Body.String())
	}
	var out []platform.TimelineEntry
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	return out
}

func TestTimelineMergesSources(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	// Generate an audit entry for a namespace so the audit source is present.
	if w := request(t, h, "POST", "/api/v1/workloads/payments/Deployment/payments-api/restart", ""); w.Code != 200 {
		t.Fatalf("restart: %d", w.Code)
	}
	entries := getTimeline(t, h, "")
	sources := map[string]bool{}
	for i, e := range entries {
		sources[e.Source] = true
		if i > 0 && e.Time.After(entries[i-1].Time) {
			t.Fatalf("timeline not newest-first at %d", i)
		}
		if e.Summary == "" {
			t.Fatalf("entry without summary: %+v", e)
		}
	}
	for _, src := range []string{"event", "audit", "alert", "rollout"} {
		if !sources[src] {
			t.Errorf("source %q missing from the merged timeline (have %v)", src, sources)
		}
	}
	// The BackOff warning is reported once, as an alert, not twice.
	backoff := 0
	for _, e := range entries {
		if strings.Contains(e.Summary, "BackOff") {
			backoff++
		}
	}
	if backoff != 1 {
		t.Fatalf("BackOff warning appears %d times, want 1 (alert supersedes event)", backoff)
	}
}

func TestTimelineFilters(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	request(t, h, "POST", "/api/v1/workloads/payments/Deployment/payments-api/restart", "")
	request(t, h, "POST", "/api/v1/nodes/axiom-01/cordon", "")

	for _, e := range getTimeline(t, h, "?namespace=payments") {
		if e.Namespace != "payments" {
			t.Fatalf("namespace filter leaked %+v", e)
		}
		if e.Object == "Node/axiom-01" {
			t.Fatalf("cluster-wide audit entry leaked into a namespaced timeline: %+v", e)
		}
	}
	// Admin sees the cordon on the unfiltered timeline.
	seen := false
	for _, e := range getTimeline(t, h, "") {
		seen = seen || (e.Source == "audit" && e.Object == "Node/axiom-01")
	}
	if !seen {
		t.Fatal("unfiltered timeline should include the node.cordon audit entry")
	}

	obj := getTimeline(t, h, "?namespace=payments&object=Deployment/payments-api")
	if len(obj) == 0 {
		t.Fatal("expected entries for Deployment/payments-api")
	}
	var haveRollout, haveAudit bool
	for _, e := range obj {
		if !platform.TimelineObjectMatches(e.Object, "Deployment/payments-api") {
			t.Fatalf("object filter leaked %+v", e)
		}
		haveRollout = haveRollout || e.Source == "rollout"
		haveAudit = haveAudit || e.Source == "audit"
	}
	if !haveRollout || !haveAudit {
		t.Fatalf("object timeline missing rollout=%v audit=%v", haveRollout, haveAudit)
	}

	if got := getTimeline(t, h, "?limit=1"); len(got) != 1 {
		t.Fatalf("limit=1 returned %d", len(got))
	}
	if got := getTimeline(t, h, "?since=1s&namespace=observability"); len(got) != 0 {
		t.Fatalf("since=1s should exclude the demo's minutes-old events, got %d", len(got))
	}
	for _, bad := range []string{"?since=yesterday", "?limit=0", "?limit=abc"} {
		if w := request(t, h, "GET", "/api/v1/timeline"+bad, ""); w.Code != 400 {
			t.Fatalf("timeline%s: got %d, want 400", bad, w.Code)
		}
	}
}

func TestTimelineHidesExecCommands(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "POST", "/api/v1/pods/payments/payments-api-01/exec", `{"command":"echo hunter2-secret"}`)
	if w.Code != 200 {
		t.Fatalf("exec: %d %s", w.Code, w.Body.String())
	}
	found := false
	for _, e := range getTimeline(t, h, "?namespace=payments") {
		if strings.Contains(e.Summary, "hunter2-secret") {
			t.Fatalf("exec command line leaked into the timeline: %+v", e)
		}
		found = found || strings.HasPrefix(e.Summary, "pod.exec ")
	}
	if !found {
		t.Fatal("pod.exec should still appear (without its command)")
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"": {}, "90m": now.Add(-90 * time.Minute), "7d": now.Add(-7 * 24 * time.Hour),
		"2026-02-28T12:00:00Z": time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC),
	} {
		got, err := parseSince(in, now)
		if err != nil || !got.Equal(want) {
			t.Errorf("parseSince(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"soon", "-5h", "d", "1x"} {
		if _, err := parseSince(bad, now); err == nil {
			t.Errorf("parseSince(%q) should fail", bad)
		}
	}
}

func TestDiagnoseEndpoint(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "GET", "/api/v1/diagnose", "")
	if w.Code != 200 {
		t.Fatalf("diagnose: %d %s", w.Code, w.Body.String())
	}
	var rep platform.HealthReport
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Status != "critical" || len(rep.Findings) == 0 || rep.Findings[0].Suggestion == "" {
		t.Fatalf("unexpected report: %+v", rep)
	}
	w = request(t, h, "GET", "/api/v1/diagnose?namespace=payments", "")
	var scoped platform.HealthReport
	_ = json.Unmarshal(w.Body.Bytes(), &scoped)
	if scoped.Namespace != "payments" || scoped.Status != "healthy" {
		t.Fatalf("scoped report: %+v", scoped)
	}
}

func TestWorkloadLogsEndpoint(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "GET", "/api/v1/workloads/observability/Deployment/otel-collector/logs?tail=3", "")
	if w.Code != 200 {
		t.Fatalf("logs: %d %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q", ct)
	}
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) != 12 { // 4 pods x tail 3
		t.Fatalf("got %d lines: %s", len(lines), w.Body.String())
	}
	if !strings.HasPrefix(lines[0], "[otel-collector-") {
		t.Fatalf("missing pod prefix: %q", lines[0])
	}
	if w := request(t, h, "GET", "/api/v1/workloads/observability/Deployment/otel-collector/logs?previous=true", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "otel-collector-04/") {
		t.Fatalf("previous logs: %d %s", w.Code, w.Body.String())
	}
	if w := request(t, h, "GET", "/api/v1/workloads/payments/Deployment/missing/logs", ""); w.Code != 404 {
		t.Fatalf("missing workload: %d", w.Code)
	}
	if w := request(t, h, "GET", "/api/v1/workloads/payments/CronJob/payments-api/logs", ""); w.Code != 400 {
		t.Fatalf("unsupported kind: %d", w.Code)
	}
}

func TestDebugPodEndpointIsAudited(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "POST", "/api/v1/pods/payments/payments-api-02/debug", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("debug: %d %s", w.Code, w.Body.String())
	}
	var v platform.DebugContainer
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || !strings.HasPrefix(v.Container, "debugger-") || v.Pod != "payments-api-02" {
		t.Fatalf("response %+v err=%v", v, err)
	}
	audited := false
	for _, e := range platform.ListAxiomAudit() {
		if e.Action == "pod.debug" && e.Target == "payments/payments-api-02" && strings.Contains(e.Detail, v.Container) && e.User != "" {
			audited = true
		}
	}
	if !audited {
		t.Fatal("pod.debug was not recorded in the audit log")
	}
	if w := request(t, h, "POST", "/api/v1/pods/payments/payments-api-02/debug", `{"target":"payments-api"}`); w.Code != http.StatusCreated {
		t.Fatalf("debug with target: %d %s", w.Code, w.Body.String())
	}
	if w := request(t, h, "POST", "/api/v1/pods/payments/payments-api-02/debug", `{"image":"evil/root"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a client-supplied image must be rejected, got %d", w.Code)
	}
	if w := request(t, h, "POST", "/api/v1/pods/payments/ghost/debug", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown pod: %d", w.Code)
	}
}

func TestNodeShellRefusedByDefault(t *testing.T) {
	t.Setenv("AXIOM_ENABLE_NODE_SHELL", "")
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "GET", "/api/v1/nodes/axiom-01/shell", "")
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "AXIOM_ENABLE_NODE_SHELL") {
		t.Fatalf("disabled node shell must refuse with a clear 403, got %d %s", w.Code, w.Body.String())
	}
	var c platform.DebugCapabilities
	w = request(t, h, "GET", "/api/v1/debug/capabilities", "")
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil || c.NodeShell || !c.EphemeralContainers || c.DebugImage == "" {
		t.Fatalf("capabilities %+v err=%v", c, err)
	}
	// Enabled but running against the demo provider: still no shell, clear reason.
	t.Setenv("AXIOM_ENABLE_NODE_SHELL", "true")
	w = request(t, h, "GET", "/api/v1/nodes/axiom-01/shell", "")
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("enabled node shell on the demo provider: %d %s", w.Code, w.Body.String())
	}
	w = request(t, h, "GET", "/api/v1/debug/capabilities", "")
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	if !c.NodeShell || c.NodeShellNamespace == "" {
		t.Fatalf("capabilities should advertise the enabled node shell: %+v", c)
	}
}
