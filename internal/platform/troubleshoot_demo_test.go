package platform

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func findingFor(r HealthReport, analyzer, name string) *HealthFinding {
	for i := range r.Findings {
		if r.Findings[i].Analyzer == analyzer && r.Findings[i].Name == name {
			return &r.Findings[i]
		}
	}
	return nil
}

func TestDemoClusterHealth(t *testing.T) {
	d := NewDemoProvider()
	r, err := d.ClusterHealth(context.Background(), "all")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "critical" || r.Counts[SeverityCritical] < 1 {
		t.Fatalf("expected a critical report, got status=%s counts=%v", r.Status, r.Counts)
	}
	crash := findingFor(r, AnalyzerPodStatus, "otel-collector-04")
	if crash == nil || crash.Severity != SeverityCritical || crash.Suggestion == "" || crash.Object != "Pod/otel-collector-04" {
		t.Fatalf("missing crash-loop finding: %+v", crash)
	}
	if hpa := findingFor(r, AnalyzerHPA, "payments-api"); hpa == nil || hpa.Severity != SeverityWarning || hpa.Namespace != "default" {
		t.Fatalf("expected the demo HPA with a missing target to be flagged, got %+v", hpa)
	}
	if pvc := findingFor(r, AnalyzerPVC, "qa-scratch"); pvc == nil || pvc.Severity != SeverityInfo {
		t.Fatalf("expected the WaitForFirstConsumer PVC as info, got %+v", pvc)
	}
	// Critical findings sort first.
	if r.Findings[0].Severity != SeverityCritical {
		t.Fatalf("findings not sorted by severity: first is %s", r.Findings[0].Severity)
	}
	if len(r.Analyzers) != len(HealthAnalyzers) {
		t.Fatalf("analyzers = %v", r.Analyzers)
	}
}

func TestDemoClusterHealthIsNamespaceScoped(t *testing.T) {
	d := NewDemoProvider()
	r, err := d.ClusterHealth(context.Background(), "payments")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.Namespace != "payments" {
			t.Fatalf("finding leaked from namespace %q: %+v", f.Namespace, f)
		}
		if f.Analyzer == AnalyzerNodePressure {
			t.Fatalf("node findings must not appear in a namespaced check: %+v", f)
		}
	}
	if r.Status != "healthy" {
		t.Fatalf("payments is healthy in the demo, got %s: %+v", r.Status, r.Findings)
	}
	obs, _ := d.ClusterHealth(context.Background(), "observability")
	if findingFor(obs, AnalyzerPodStatus, "otel-collector-04") == nil {
		t.Fatal("observability should contain the crash-loop finding")
	}
}

func TestDemoClusterHealthFlagsServiceWithoutEndpoints(t *testing.T) {
	d := NewDemoProvider()
	if err := d.Stop(context.Background(), "commerce", "Deployment", "checkout"); err != nil {
		t.Fatal(err)
	}
	r, _ := d.ClusterHealth(context.Background(), "commerce")
	f := findingFor(r, AnalyzerServiceEndpoint, "checkout")
	if f == nil || f.Severity != SeverityWarning {
		t.Fatalf("stopped workload's Service should have no endpoints, got %+v (report %+v)", f, r.Findings)
	}
}

func TestNewHealthReportSortsAndDedupes(t *testing.T) {
	a := newFinding(AnalyzerHPA, SeverityInfo, "HorizontalPodAutoscaler", "z", "h", "t", "m", "s")
	b := newFinding(AnalyzerPodStatus, SeverityCritical, "Pod", "a", "p", "t", "m", "s")
	c := newFinding(AnalyzerPVC, SeverityWarning, "PersistentVolumeClaim", "a", "c", "t", "m", "s")
	r := NewHealthReport("", []HealthFinding{a, b, c, b}, nil, time.Unix(0, 0))
	if len(r.Findings) != 3 {
		t.Fatalf("duplicates not removed: %d findings", len(r.Findings))
	}
	if r.Findings[0].ID != b.ID || r.Findings[1].ID != c.ID || r.Findings[2].ID != a.ID {
		t.Fatalf("bad order: %v", []string{r.Findings[0].ID, r.Findings[1].ID, r.Findings[2].ID})
	}
	if r.Namespace != "all" || r.Status != "critical" {
		t.Fatalf("namespace=%q status=%q", r.Namespace, r.Status)
	}
	clean := NewHealthReport("x", nil, nil, time.Now())
	if clean.Status != "healthy" || clean.Findings == nil {
		t.Fatalf("clean report: %+v", clean)
	}
	partial := NewHealthReport("x", nil, []string{"hpa: forbidden"}, time.Now())
	if partial.Status != "warning" {
		t.Fatalf("a report with failed analyzers must not read as healthy, got %q", partial.Status)
	}
}

func TestContainerProblem(t *testing.T) {
	cases := []struct {
		name                       string
		waiting, term              string
		restarts                   int32
		wantOK                     bool
		wantSeverity, wantTitleHas string
	}{
		{"image pull", "ImagePullBackOff", "", 0, true, SeverityCritical, "Image pull"},
		{"err image pull", "ErrImagePull", "", 0, true, SeverityCritical, "Image pull"},
		{"oom crashloop", "CrashLoopBackOff", "OOMKilled", 5, true, SeverityCritical, "OOMKilled"},
		{"plain crashloop", "CrashLoopBackOff", "Error", 3, true, SeverityCritical, "crash-looping"},
		{"config error", "CreateContainerConfigError", "", 0, true, SeverityCritical, "cannot be created"},
		{"oom but running", "", "OOMKilled", 2, true, SeverityWarning, "OOMKilled"},
		{"healthy", "", "", 0, false, "", ""},
		{"completed restart is fine", "", "Completed", 1, false, "", ""},
		{"container creating", "ContainerCreating", "", 0, false, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sev, title, _, fix, ok := containerProblem(tc.waiting, "", tc.term, 137, tc.restarts, "img:1")
			if ok != tc.wantOK {
				t.Fatalf("ok = %v", ok)
			}
			if !ok {
				return
			}
			if sev != tc.wantSeverity || !strings.Contains(title, tc.wantTitleHas) || fix == "" {
				t.Fatalf("sev=%q title=%q fix=%q", sev, title, fix)
			}
		})
	}
}

func TestNodeFindings(t *testing.T) {
	got := nodeFindings(Node{Name: "n1", Conditions: []NodeCondition{
		{Type: "Ready", Status: "False"}, {Type: "MemoryPressure", Status: "True"}, {Type: "DiskPressure", Status: "False"},
	}})
	if len(got) != 2 || got[0].Severity != SeverityCritical || got[1].Severity != SeverityWarning {
		t.Fatalf("findings = %+v", got)
	}
	if len(nodeFindings(Node{Name: "ok", Conditions: []NodeCondition{{Type: "Ready", Status: "True"}}})) != 0 {
		t.Fatal("a healthy node produced findings")
	}
}

func readAll(t *testing.T, rc io.ReadCloser) []string {
	t.Helper()
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func TestDemoWorkloadLogsMergeAcrossPods(t *testing.T) {
	d := NewDemoProvider()
	rc, err := d.WorkloadLogs(context.Background(), "payments", "Deployment", "payments-api", WorkloadLogOptions{Tail: 2})
	if err != nil {
		t.Fatal(err)
	}
	lines := readAll(t, rc)
	// 12 replicas x 1 container x tail 2.
	if len(lines) != 24 {
		t.Fatalf("got %d lines, want 24", len(lines))
	}
	pods := map[string]bool{}
	prev := ""
	for _, l := range lines {
		if !strings.HasPrefix(l, "[payments-api-") || !strings.Contains(l, "/payments-api] ") {
			t.Fatalf("line lacks [pod/container] prefix: %q", l)
		}
		pods[l[1:strings.Index(l, "/")]] = true
		ts := timestampOf(l[strings.Index(l, "] ")+2:])
		if ts == "" || ts < prev {
			t.Fatalf("lines not in chronological order at %q (prev %q)", l, prev)
		}
		prev = ts
	}
	if len(pods) != 12 {
		t.Fatalf("expected lines from 12 pods, got %d", len(pods))
	}
}

func TestDemoWorkloadLogsPreviousAndFilters(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	rc, err := d.WorkloadLogs(ctx, "observability", "Deployment", "otel-collector", WorkloadLogOptions{Previous: true})
	if err != nil {
		t.Fatal(err)
	}
	lines := readAll(t, rc)
	if len(lines) == 0 {
		t.Fatal("expected previous logs from the crash-looping pod")
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "[otel-collector-04/") {
			t.Fatalf("only the restarted pod has previous logs, got %q", l)
		}
	}
	if _, err := d.WorkloadLogs(ctx, "payments", "Deployment", "payments-api", WorkloadLogOptions{Previous: true}); HTTPStatus(err) != 404 {
		t.Fatalf("no previous container should be a 404, got %v", err)
	}
	if _, err := d.WorkloadLogs(ctx, "payments", "Deployment", "payments-api", WorkloadLogOptions{Container: "nope"}); HTTPStatus(err) != 404 {
		t.Fatalf("unknown container should yield a 404, got %v", err)
	}
	if _, err := d.WorkloadLogs(ctx, "payments", "Deployment", "missing", WorkloadLogOptions{}); HTTPStatus(err) != 404 {
		t.Fatalf("missing workload: %v", err)
	}
	if _, err := d.WorkloadLogs(ctx, "payments", "Pod", "payments-api", WorkloadLogOptions{}); HTTPStatus(err) != 400 {
		t.Fatalf("unsupported kind should be a 400, got %v", err)
	}
}

func TestMergeLogStreamsFollowPrefixesAndCloses(t *testing.T) {
	streams := []labeledLogStream{
		{label: "a/x", rc: io.NopCloser(strings.NewReader("2026-01-01T00:00:02Z two\n"))},
		{label: "b/y", rc: io.NopCloser(strings.NewReader("2026-01-01T00:00:01Z one\n"))},
	}
	lines := readAll(t, mergeLogStreams(streams, true))
	if len(lines) != 2 {
		t.Fatalf("lines = %v", lines)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "[a/x] ") && !strings.HasPrefix(l, "[b/y] ") {
			t.Fatalf("missing prefix: %q", l)
		}
	}
}

func TestTimestampOf(t *testing.T) {
	if got := timestampOf("2026-01-02T03:04:05.123456789Z hello world"); got != "2026-01-02T03:04:05.123456789Z" {
		t.Fatalf("got %q", got)
	}
	for _, in := range []string{"plain text line", "", "short ts", "no-timestamp T here"} {
		if got := timestampOf(in); got != "" {
			t.Fatalf("timestampOf(%q) = %q", in, got)
		}
	}
}

func TestDemoCreateDebugContainer(t *testing.T) {
	d := NewDemoProvider()
	t.Setenv("AXIOM_DEBUG_IMAGE", "")
	v, err := d.CreateDebugContainer(context.Background(), "payments", "payments-api-01", DebugRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if v.Image != "busybox" || !strings.HasPrefix(v.Container, "debugger-") || v.Ready || v.Message == "" {
		t.Fatalf("unexpected result %+v", v)
	}
	t.Setenv("AXIOM_DEBUG_IMAGE", "nicolaka/netshoot")
	v, _ = d.CreateDebugContainer(context.Background(), "payments", "payments-api-01", DebugRequest{Target: "payments-api"})
	if v.Image != "nicolaka/netshoot" || v.Target != "payments-api" {
		t.Fatalf("env image / target not honoured: %+v", v)
	}
	if _, err := d.CreateDebugContainer(context.Background(), "payments", "payments-api-01", DebugRequest{Target: "ghost"}); HTTPStatus(err) != 404 {
		t.Fatalf("unknown target should be 404, got %v", err)
	}
	if _, err := d.CreateDebugContainer(context.Background(), "payments", "nope", DebugRequest{}); HTTPStatus(err) != 404 {
		t.Fatalf("unknown pod should be 404, got %v", err)
	}
}

func TestNodeShellConfigDefaultsOff(t *testing.T) {
	t.Setenv("AXIOM_ENABLE_NODE_SHELL", "")
	if NodeShellEnabled() {
		t.Fatal("node shell must be off by default")
	}
	for _, v := range []string{"1", "yes", "enabled", "false"} {
		t.Setenv("AXIOM_ENABLE_NODE_SHELL", v)
		if NodeShellEnabled() {
			t.Fatalf("%q must not enable the node shell", v)
		}
	}
	t.Setenv("AXIOM_ENABLE_NODE_SHELL", "TRUE")
	if !NodeShellEnabled() {
		t.Fatal("true should enable it")
	}
	if HTTPStatus(ErrNodeShellDisabled) != 403 {
		t.Fatalf("disabled error status = %d", HTTPStatus(ErrNodeShellDisabled))
	}
	t.Setenv("AXIOM_NODE_SHELL_TIMEOUT", "24h")
	if NodeShellTimeout() != time.Hour {
		t.Fatalf("timeout must be capped at one hour, got %s", NodeShellTimeout())
	}
	t.Setenv("AXIOM_NODE_SHELL_TIMEOUT", "garbage")
	if NodeShellTimeout() != DefaultNodeShellTimeout {
		t.Fatalf("bad timeout must fall back to the default")
	}
}

func TestTimelineMappers(t *testing.T) {
	now := time.Now()
	cases := []struct {
		audit               AuditEntry
		ns, object, sev, in string
	}{
		{AuditEntry{Action: "pod.delete", Target: "payments/payments-api-01", User: "bob"}, "payments", "Pod/payments-api-01", SeverityWarning, "pod.delete"},
		{AuditEntry{Action: "workload.scale", Target: "payments/Deployment/payments-api", Detail: "replicas=3"}, "payments", "Deployment/payments-api", SeverityInfo, "replicas=3"},
		{AuditEntry{Action: "workload.restart", Target: "ns1/statefulset/db"}, "ns1", "StatefulSet/db", SeverityInfo, ""},
		{AuditEntry{Action: "node.cordon", Target: "axiom-01"}, "", "Node/axiom-01", SeverityInfo, ""},
		{AuditEntry{Action: "namespace.delete", Target: "tmp"}, "tmp", "Namespace/tmp", SeverityWarning, ""},
		{AuditEntry{Action: "auth.login_failed", Target: "1.2.3.4"}, "", "1.2.3.4", SeverityWarning, ""},
	}
	for _, tc := range cases {
		got := TimelineFromAudit(tc.audit)
		if got.Namespace != tc.ns || got.Object != tc.object || got.Severity != tc.sev || got.Source != TimelineAudit || !strings.Contains(got.Summary, tc.in) {
			t.Errorf("%+v -> %+v", tc.audit, got)
		}
	}
	ev := TimelineFromEvent(Event{Namespace: "n", Type: "Warning", Reason: "BackOff", Object: "Pod/p", Message: "restarting", Count: 3, Time: now})
	if ev.Source != TimelineEvent || ev.Severity != SeverityWarning || !strings.Contains(ev.Summary, "(x3)") {
		t.Fatalf("event mapping %+v", ev)
	}
	if al := TimelineFromAlert(Event{Type: "Warning"}); al.Source != TimelineAlert {
		t.Fatalf("alert mapping %+v", al)
	}
	ro := TimelineFromRollout("n", "Deployment", "d", RolloutRevision{Revision: 2, Status: "active", Age: "3h"}, now)
	if ro.Source != TimelineRollout || ro.Object != "Deployment/d" || now.Sub(ro.Time) < 2*time.Hour {
		t.Fatalf("rollout mapping %+v", ro)
	}
}

func TestTimelineObjectMatches(t *testing.T) {
	yes := [][2]string{
		{"Deployment/web", "Deployment/web"},
		{"deployment/WEB", "Deployment/web"},
		{"Pod/web-7d9f-abcde", "Deployment/web"},
		{"ReplicaSet/web-7d9f", "Deployment/web"},
		{"Pod/anything", ""},
	}
	no := [][2]string{
		{"Pod/webhook-1", "Deployment/web"}, // a prefix of the name alone is not ownership
		{"Pod/other-1", "Deployment/web"},
		{"Service/web-svc", "Deployment/web"},
		{"Node/web-1", "Pod/web"},
		{"", "Deployment/web"},
	}
	for _, c := range yes {
		if !TimelineObjectMatches(c[0], c[1]) {
			t.Errorf("%q should match %q", c[0], c[1])
		}
	}
	for _, c := range no {
		if TimelineObjectMatches(c[0], c[1]) {
			t.Errorf("%q must not match %q", c[0], c[1])
		}
	}
}

func TestAgeToTime(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"now": now, "": now, "-": now, "5m": now.Add(-5 * time.Minute), "3h": now.Add(-3 * time.Hour),
		"11d": now.Add(-11 * 24 * time.Hour), "30s": now.Add(-30 * time.Second), "weird": now,
	} {
		if got := AgeToTime(in, now); !got.Equal(want) {
			t.Errorf("AgeToTime(%q) = %v, want %v", in, got, want)
		}
	}
}
