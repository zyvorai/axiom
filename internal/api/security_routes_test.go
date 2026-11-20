package api

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/platform"
)

func getJSON(t *testing.T, h http.Handler, path string, into any) int {
	t.Helper()
	w := request(t, h, "GET", path, "")
	if into != nil && w.Code == 200 {
		if err := json.Unmarshal(w.Body.Bytes(), into); err != nil {
			t.Fatalf("%s: bad JSON: %v\n%s", path, err, w.Body.String())
		}
	}
	return w.Code
}

func TestRBACViewerEndpoints(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")

	var subjects []platform.RBACSubjectSummary
	if code := getJSON(t, h, "/api/v1/rbac/subjects", &subjects); code != 200 || len(subjects) == 0 {
		t.Fatalf("subjects: %d %v", code, subjects)
	}
	var sawMasters, sawSA bool
	for _, s := range subjects {
		if s.Kind == "Group" && s.Name == "system:masters" && s.ClusterAdmin && s.ClusterWide {
			sawMasters = true
		}
		if s.Kind == "ServiceAccount" && s.Namespace == "payments" && s.Name == "payments-api" {
			sawSA = true
		}
	}
	if !sawMasters || !sawSA {
		t.Fatalf("expected system:masters (cluster admin) and the payments-api ServiceAccount: %+v", subjects)
	}

	var detail platform.RBACSubjectDetail
	if code := getJSON(t, h, "/api/v1/rbac/subjects/ServiceAccount/payments-api?namespace=payments", &detail); code != 200 {
		t.Fatalf("detail: %d", code)
	}
	if len(detail.Namespaced) == 0 || detail.Namespaced[0].Namespace != "payments" || detail.Namespaced[0].RoleRef != "Role/payments-operator" {
		t.Fatalf("payments-api should hold payments-operator in payments: %+v", detail)
	}

	for path, want := range map[string]int{
		"/api/v1/rbac/subjects/ServiceAccount/payments-api":             400, // namespace required
		"/api/v1/rbac/subjects/Robot/x":                                 400,
		"/api/v1/rbac/subjects/User/nobody@example.com":                 404,
		"/api/v1/rbac/who-can":                                          400,
		"/api/v1/rbac/who-can?verb=get":                                 400,
		"/api/v1/rbac/who-can?verb=get&resource=pods%0Aevil":            400,
		"/api/v1/rbac/who-can?verb=get&resource=pods&namespace=a%20b":   400,
		"/api/v1/rbac/subjects/User/ops-lead@zyvor.dev":                 200,
		"/api/v1/rbac/subjects/user/ops-lead%40zyvor.dev":               200, // kind is case-insensitive
		"/api/v1/rbac/subjects/Group/system:masters":                    200,
		"/api/v1/rbac/who-can?verb=delete&resource=deployments.apps":    200,
		"/api/v1/rbac/who-can?verb=get&resource=pods&namespace=nowhere": 200,
	} {
		if code := getJSON(t, h, path, nil); code != want {
			t.Errorf("GET %s = %d, want %d", path, code, want)
		}
	}
}

func TestRBACWhoCanSemantics(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	who := func(path string) map[string]bool {
		var res platform.RBACWhoCanResult
		if code := getJSON(t, h, path, &res); code != 200 {
			t.Fatalf("%s: %d", path, code)
		}
		out := map[string]bool{}
		for _, e := range res.Allowed {
			out[e.Subject.Kind+"/"+e.Subject.Namespace+"/"+e.Subject.Name+"@"+e.Scope+":"+e.Namespace] = true
		}
		return out
	}

	// cluster-admin matches everything; scoped Roles only inside their namespace.
	got := who("/api/v1/rbac/who-can?verb=create&resource=pods%2Fexec&namespace=payments")
	if !got["Group//system:masters@cluster:"] {
		t.Errorf("cluster-admin missing: %v", got)
	}
	if !got["User//alice@zyvor.dev@namespace:payments"] || !got["ServiceAccount/payments/payments-api@namespace:payments"] {
		t.Errorf("payments-operator subjects missing for pods/exec create: %v", got)
	}
	if got["User//bob@zyvor.dev@namespace:default"] {
		t.Errorf("a binding in another namespace must not appear: %v", got)
	}
	// Same question, different namespace: payments-operator no longer applies.
	got = who("/api/v1/rbac/who-can?verb=create&resource=pods%2Fexec&namespace=default")
	if got["User//alice@zyvor.dev@namespace:payments"] {
		t.Errorf("namespace filter ignored: %v", got)
	}
	// A rule restricted by resourceNames only answers for that object.
	got = who("/api/v1/rbac/who-can?verb=get&resource=secrets&namespace=payments")
	if got["User//alice@zyvor.dev@namespace:payments"] {
		t.Errorf("resourceNames-restricted rule must not match an unnamed secrets query: %v", got)
	}
	got = who("/api/v1/rbac/who-can?verb=get&resource=secrets&namespace=payments&name=payments-db")
	if !got["User//alice@zyvor.dev@namespace:payments"] {
		t.Errorf("named query should match the resourceNames rule: %v", got)
	}
	got = who("/api/v1/rbac/who-can?verb=get&resource=secrets&namespace=payments&name=other")
	if got["User//alice@zyvor.dev@namespace:payments"] {
		t.Errorf("a different object name must not match: %v", got)
	}
	// Read-only view role: get yes, delete no.
	if !who("/api/v1/rbac/who-can?verb=get&resource=pods")["Group//devs@cluster:"] {
		t.Error("devs should be able to get pods via the view ClusterRole")
	}
	if who("/api/v1/rbac/who-can?verb=delete&resource=pods")["Group//devs@cluster:"] {
		t.Error("devs must not be able to delete pods")
	}
	// Subresource wildcard semantics: view grants pods/log but not pods/exec.
	if !who("/api/v1/rbac/who-can?verb=get&resource=pods%2Flog")["Group//devs@cluster:"] {
		t.Error("pods/log should be readable by devs")
	}
	if who("/api/v1/rbac/who-can?verb=get&resource=pods%2Fexec")["Group//devs@cluster:"] {
		t.Error("pods/exec must not be readable by devs")
	}
}

func TestNetworkPolicyAnalysisEndpoint(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")

	var commerce platform.NetworkPolicyAnalysis
	if code := getJSON(t, h, "/api/v1/networking/policies/commerce/analysis", &commerce); code != 200 {
		t.Fatalf("commerce: %d", code)
	}
	if len(commerce.Pods) == 0 || commerce.Policies != 1 {
		t.Fatalf("commerce analysis: %+v", commerce)
	}
	for _, p := range commerce.Pods {
		if !p.IngressIsolated || !p.EgressIsolated || len(p.Ingress) != 2 || len(p.Egress) != 2 {
			t.Fatalf("checkout pod %s should be isolated both ways with 2 ingress / 2 egress peers: %+v", p.Pod, p)
		}
	}

	var payments platform.NetworkPolicyAnalysis
	getJSON(t, h, "/api/v1/networking/policies/payments/analysis", &payments)
	if !payments.DefaultDenyIngress || payments.DefaultDenyEgress {
		t.Fatalf("payments is default-deny ingress only: %+v", payments)
	}
	for _, p := range payments.Pods {
		if !p.IngressIsolated || p.EgressIsolated || len(p.Ingress) != 0 {
			t.Fatalf("payments pod %s: %+v", p.Pod, p)
		}
	}

	var data platform.NetworkPolicyAnalysis
	getJSON(t, h, "/api/v1/networking/policies/data/analysis", &data)
	if len(data.Uncovered) != len(data.Pods) || len(data.Uncovered) == 0 || len(data.Warnings) == 0 {
		t.Fatalf("data has no policy: every pod must be reported uncovered: %+v", data)
	}

	if code := getJSON(t, h, "/api/v1/networking/policies/no-such-ns/analysis", nil); code != 404 {
		t.Errorf("unknown namespace: %d, want 404", code)
	}
	if code := getJSON(t, h, "/api/v1/networking/policies/all/analysis", nil); code != 400 {
		t.Errorf("namespace=all: %d, want 400", code)
	}
}

func TestSecurityReportsEndpoints(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	var sum platform.SecurityReportSummary
	if code := getJSON(t, h, "/api/v1/security/reports", &sum); code != 200 {
		t.Fatalf("reports: %d", code)
	}
	if !sum.Available || sum.InstallHint != "" || len(sum.Sources) != 3 {
		t.Fatalf("demo has all three report sources installed: %+v", sum)
	}
	if sum.Vulnerabilities.Critical != 1 || sum.Vulnerabilities.High != 5 || sum.Policy.Fail != 1 {
		t.Fatalf("unexpected totals: %+v", sum)
	}
	if len(sum.Workloads) == 0 || sum.Workloads[0].Name != "payments-api" || sum.Workloads[0].Kind != "Deployment" {
		t.Fatalf("riskiest workload (ReplicaSet folded into its Deployment) should be first: %+v", sum.Workloads)
	}

	var scoped platform.SecurityReportSummary
	getJSON(t, h, "/api/v1/security/reports?namespace=data", &scoped)
	if len(scoped.Workloads) != 1 || scoped.Workloads[0].Name != "redis" || scoped.Vulnerabilities.Critical != 0 {
		t.Fatalf("namespace filter: %+v", scoped)
	}

	var d platform.WorkloadSecurityDetail
	if code := getJSON(t, h, "/api/v1/security/reports/payments/Deployment/payments-api", &d); code != 200 {
		t.Fatalf("drill-down: %d", code)
	}
	if len(d.Vulnerabilities) != 7 || d.Vulnerabilities[0].ID != "CVE-2024-45337" || d.Vulnerabilities[0].Severity != "CRITICAL" {
		t.Fatalf("vulns should be sorted critical-first: %+v", d.Vulnerabilities)
	}
	if len(d.Misconfigurations) != 2 || d.PolicyCnt.Warn != 1 || len(d.Images) != 1 {
		t.Fatalf("detail: %+v", d)
	}
	// The same workload name in another namespace must not leak across.
	var other platform.WorkloadSecurityDetail
	getJSON(t, h, "/api/v1/security/reports/commerce/Deployment/payments-api", &other)
	if len(other.Vulnerabilities) != 0 {
		t.Fatalf("cross-namespace leak: %+v", other)
	}
}

func TestWSTicketEndToEnd(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	srv := httptest.NewServer(h)
	defer srv.Close()
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("demo:demo"))

	mint := func(body, authz string) (*http.Response, map[string]any) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/ws-ticket", strings.NewReader(body))
		if authz != "" {
			req.Header.Set("Authorization", authz)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res, out
	}

	if res, _ := mint(`{"path":"/api/v1/events/stream"}`, ""); res.StatusCode != 401 {
		t.Fatalf("unauthenticated mint: %d", res.StatusCode)
	}
	for _, bad := range []string{`{"path":"/api/v1/secrets/ns/s"}`, `{"path":""}`, `{}`, `{"path":"/api/v1/pods/a/b/terminal","x":1}`, `not json`} {
		if res, _ := mint(bad, basic); res.StatusCode != 400 {
			t.Errorf("mint %s: %d, want 400", bad, res.StatusCode)
		}
	}
	res, out := mint(`{"path":"/api/v1/events/stream"}`, basic)
	if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("mint: %d cache=%q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	ticket, _ := out["ticket"].(string)
	if len(ticket) < 40 || out["expiresIn"].(float64) != 30 {
		t.Fatalf("ticket response: %v", out)
	}

	// The ticket opens the SSE stream with NO credentials of any kind...
	open := func(path string) *http.Response {
		r, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	sse := open("/api/v1/events/stream?ticket=" + ticket)
	if sse.StatusCode != 200 {
		t.Fatalf("ticketed stream: %d", sse.StatusCode)
	}
	line := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(sse.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "data:") {
				line <- sc.Text()
				return
			}
		}
		line <- ""
	}()
	select {
	case l := <-line:
		if !strings.Contains(l, "connected") {
			t.Fatalf("unexpected first SSE frame %q", l)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no SSE frame")
	}
	sse.Body.Close()

	// ...exactly once.
	if r := open("/api/v1/events/stream?ticket=" + ticket); r.StatusCode != 401 {
		t.Fatalf("reused ticket: %d, want 401", r.StatusCode)
	}
	// A ticket for one path is useless on another.
	_, out = mint(`{"path":"/api/v1/events/stream"}`, basic)
	if r := open("/api/v1/pods/payments/payments-api-01/logs?ticket=" + out["ticket"].(string)); r.StatusCode != 401 {
		t.Fatalf("ticket on a different path: %d, want 401", r.StatusCode)
	}
}
