package platform

import (
	"context"
	"strings"
	"testing"
)

func TestDetectQuotaHeadroom(t *testing.T) {
	qs := []ResourceQuotaSummary{
		{Namespace: "a", Name: "q", Hard: map[string]string{"pods": "10", "requests.cpu": "4", "requests.memory": "8Gi", "count/x": "0"},
			Used: map[string]string{"pods": "10", "requests.cpu": "3500m", "requests.memory": "1Gi", "count/x": "0"}},
		{Namespace: "b", Name: "q", Hard: map[string]string{"pods": "bogus", "services": "5"}, Used: map[string]string{"pods": "1", "services": "5"}},
		{Namespace: "c", Name: "q", Hard: map[string]string{"pods": "0"}, Used: map[string]string{"pods": "1"}},
		{Namespace: "d", Name: "q", Hard: map[string]string{"pods": "10"}}, // no Used entry parses as zero
	}
	r := DetectQuotaHeadroom(qs, "", 80)
	if r.Summary.Scanned != 4 || r.Summary.ByKind[AuditQuotaExhausted] != 3 || r.Summary.ByKind[AuditQuotaNear] != 1 {
		t.Fatalf("summary %+v", r.Summary)
	}
	if r.Findings[0].Severity != "high" {
		t.Fatalf("high findings must sort first: %+v", r.Findings[0])
	}
	if got := DetectQuotaHeadroom(qs, "a", 95).Summary.ByKind[AuditQuotaNear]; got != 0 {
		t.Errorf("95%% threshold should not flag 87.5%%: %d", got)
	}
	if got := DetectQuotaHeadroom(qs, "nope", 80).Summary.Total; got != 0 {
		t.Errorf("unknown namespace: %d", got)
	}
}

func TestSplitImageRef(t *testing.T) {
	cases := []struct {
		ref    string
		tag    string
		pinned bool
	}{
		{"nginx", "", false}, {"nginx:latest", "latest", false}, {"nginx:1.27", "1.27", false},
		{"registry:5000/app", "", false}, {"registry:5000/app:v2", "v2", false},
		{"app@sha256:abcd", "", true}, {"app:1.0@sha256:abcd", "", true},
	}
	for _, c := range cases {
		tag, pinned := splitImageRef(c.ref)
		if tag != c.tag || pinned != c.pinned {
			t.Errorf("%q: %q %v", c.ref, tag, pinned)
		}
	}
}

func TestDetectImageHygiene(t *testing.T) {
	ws := []Workload{
		{Namespace: "a", Name: "one", Image: "nginx:latest"},
		{Namespace: "a", Name: "two", Image: "registry:5000/app"},
		{Namespace: "a", Name: "three", Image: "app:1.2.3"},
		{Namespace: "a", Name: "four", Image: "app@sha256:abc"},
		{Namespace: "a", Name: "five", Image: "x:LATEST, y:2"},
		{Namespace: "b", Name: "six", Image: ""},
	}
	r := DetectImageHygiene(ws, "")
	if r.Summary.ByKind[AuditImageLatest] != 2 || r.Summary.ByKind[AuditImageNoTag] != 1 || r.Summary.Scanned != 6 {
		t.Fatalf("summary %+v", r.Summary)
	}
	if DetectImageHygiene(ws, "b").Summary.Total != 0 {
		t.Error("empty image must not be flagged")
	}
}

func TestDetectHPASaturation(t *testing.T) {
	hs := []HPASummary{
		{Namespace: "a", Name: "broken", TargetKind: "Deployment", TargetName: "x", MinReplicas: 1, MaxReplicas: 5, CurrentReplicas: 2,
			Conditions: []AutoscalerCondition{{Type: "AbleToScale", Status: "False", Reason: "FailedGetScale", Message: strings.Repeat("m", 500)}}},
		{Namespace: "a", Name: "max", MinReplicas: 1, MaxReplicas: 5, CurrentReplicas: 5, Metrics: []HPAMetric{{Name: "cpu", Current: "90%"}}},
		{Namespace: "a", Name: "nometric", MinReplicas: 1, MaxReplicas: 5, CurrentReplicas: 2, Metrics: []HPAMetric{{Name: "lag"}}},
		{Namespace: "a", Name: "fixed", MinReplicas: 3, MaxReplicas: 3, CurrentReplicas: 3},
		{Namespace: "a", Name: "fine", MinReplicas: 1, MaxReplicas: 5, CurrentReplicas: 2, Metrics: []HPAMetric{{Name: "cpu", Current: "10%"}}},
	}
	r := DetectHPASaturation(hs, "")
	want := map[string]int{AuditHPACannotScale: 1, AuditHPAAtMax: 1, AuditHPAMetricMissing: 1, AuditHPAFixedSize: 1}
	for k, v := range want {
		if r.Summary.ByKind[k] != v {
			t.Errorf("%s = %d want %d", k, r.Summary.ByKind[k], v)
		}
	}
	if r.Summary.Total != 4 || r.Summary.Scanned != 5 {
		t.Fatalf("summary %+v", r.Summary)
	}
	if len(r.Findings[0].Reason) > 300 {
		t.Errorf("hostile condition message not clipped: %d", len(r.Findings[0].Reason))
	}
}

func TestDetectServiceWiring(t *testing.T) {
	svcs := []Service{
		{Namespace: "a", Name: "typo", Type: "ClusterIP", Selector: map[string]string{"app": "wbe"}},
		{Namespace: "a", Name: "notready", Type: "ClusterIP", Selector: map[string]string{"app": "web"}},
		{Namespace: "a", Name: "ok", Type: "ClusterIP", Selector: map[string]string{"app": "web"}, Endpoints: []string{"web-1"}},
		{Namespace: "a", Name: "manual", Type: "ClusterIP"},
		{Namespace: "a", Name: "ext", Type: "ExternalName", Selector: map[string]string{"app": "q"}},
		{Namespace: "b", Name: "other-ns", Type: "ClusterIP", Selector: map[string]string{"app": "web"}},
	}
	pods := []Pod{{Namespace: "a", Name: "web-1", Labels: map[string]string{"app": "web", "tier": "x"}}}
	r := DetectServiceWiring(svcs, pods, "")
	if r.Summary.ByKind[AuditSvcNoMatch] != 2 || r.Summary.ByKind[AuditSvcNotReady] != 1 || r.Summary.Scanned != 4 {
		t.Fatalf("summary %+v findings %+v", r.Summary, r.Findings)
	}
	if DetectServiceWiring(svcs, pods, "a").Summary.Total != 2 {
		t.Error("namespace filter")
	}
}

func TestAuditCapAndDemo(t *testing.T) {
	var ws []Workload
	for i := 0; i < auditMaxFindings+20; i++ {
		ws = append(ws, Workload{Namespace: "a", Name: "w" + strings.Repeat("x", i%7) + string(rune('a'+i%26)), Image: "x:latest"})
	}
	r := DetectImageHygiene(ws, "")
	if !r.Truncated || len(r.Findings) != auditMaxFindings || r.Summary.Total != len(ws) {
		t.Fatalf("cap: %v %d %d", r.Truncated, len(r.Findings), r.Summary.Total)
	}
	d := NewDemoProvider()
	ctx := context.Background()
	if _, err := QuotaHeadroom(ctx, d, "", 80); err != nil {
		t.Error(err)
	}
	if _, err := ImageHygiene(ctx, d, ""); err != nil {
		t.Error(err)
	}
	if _, err := HPASaturation(ctx, d, ""); err != nil {
		t.Error(err)
	}
	if _, err := ServiceWiring(ctx, d, ""); err != nil {
		t.Error(err)
	}
}
