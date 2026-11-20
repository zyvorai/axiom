package platform

import (
	"context"
	"strings"
	"testing"
)

// ---- RBAC viewer engine -----------------------------------------------------

func TestRBACSubjectsAndImplicitGroups(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	subs, err := d.ListRBACSubjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	idx := map[string]RBACSubjectSummary{}
	for _, s := range subs {
		idx[s.Kind+"/"+s.Namespace+"/"+s.Name] = s
	}
	if s := idx["Group//system:masters"]; !s.ClusterAdmin || !s.ClusterWide || s.Bindings != 1 {
		t.Errorf("system:masters: %+v", s)
	}
	if s := idx["User//bob@zyvor.dev"]; s.ClusterAdmin || s.ClusterWide || len(s.Roles) != 1 || s.Roles[0] != "Role/app-editor" {
		t.Errorf("bob: %+v", s)
	}
	// A ServiceAccount with no bindings of its own is still listed.
	if s, ok := idx["ServiceAccount/data/redis"]; !ok || s.Bindings != 0 {
		t.Errorf("unbound ServiceAccount should be listed with 0 bindings: %+v ok=%v", s, ok)
	}

	// Implicit groups: a binding to system:serviceaccounts:payments reaches the SA.
	snap := rbacSnapshot{
		ClusterRoles: map[string]rbacRoleDef{"view": {Name: "view", Rules: []RBACRuleView{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}}}},
		Roles:        map[string]rbacRoleDef{},
		Bindings: []rbacBindingDef{
			{Name: "ns-wide", Cluster: true, RoleKind: "ClusterRole", RoleName: "view", Subjects: []RBACSubject{{Kind: "Group", Name: "system:serviceaccounts:payments"}}},
			{Name: "all-authn", Cluster: true, RoleKind: "ClusterRole", RoleName: "view", Subjects: []RBACSubject{{Kind: "Group", Name: "system:authenticated"}}},
		},
		ServiceAccounts: []RBACSubject{{Kind: "ServiceAccount", Name: "api", Namespace: "payments"}, {Kind: "ServiceAccount", Name: "api", Namespace: "other"}},
	}
	det, err := snap.Subject("ServiceAccount", "api", "payments")
	if err != nil || len(det.Cluster) != 2 {
		t.Fatalf("payments/api should match both implicit groups: %+v err=%v", det, err)
	}
	det, _ = snap.Subject("ServiceAccount", "api", "other")
	if len(det.Cluster) != 1 || det.Cluster[0].Via != "system:authenticated" {
		t.Fatalf("other/api only matches system:authenticated: %+v", det.Cluster)
	}
}

func TestRBACDanglingAndRoleBindingToClusterRole(t *testing.T) {
	snap := rbacSnapshot{
		ClusterRoles: map[string]rbacRoleDef{"edit": {Name: "edit", Rules: []RBACRuleView{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"*"}}}}},
		Roles:        map[string]rbacRoleDef{},
		Bindings: []rbacBindingDef{
			{Namespace: "a", Name: "rb", RoleKind: "ClusterRole", RoleName: "edit", Subjects: []RBACSubject{{Kind: "User", Name: "u"}}},
			{Namespace: "a", Name: "ghost", RoleKind: "Role", RoleName: "gone", Subjects: []RBACSubject{{Kind: "User", Name: "u"}}},
		},
	}
	det, err := snap.Subject("User", "u", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(det.Cluster) != 0 || len(det.Namespaced) != 2 {
		t.Fatalf("RoleBinding->ClusterRole must stay namespaced: %+v", det)
	}
	var dangling int
	for _, g := range det.Namespaced {
		if g.Dangling {
			dangling++
		}
	}
	if dangling != 1 || det.Subject.ClusterWide || det.Subject.ClusterAdmin {
		t.Fatalf("dangling=%d summary=%+v", dangling, det.Subject)
	}
	// A dangling role grants nothing in who-can.
	res, _ := snap.WhoCan(RBACWhoCanQuery{Verb: "get", Resource: "pods", Namespace: "a"})
	if len(res.Allowed) != 1 || res.Allowed[0].Binding != "RoleBinding/a/rb" {
		t.Fatalf("who-can: %+v", res)
	}
	// A RoleBinding's ServiceAccount subject without a namespace defaults to the binding's.
	snap.Bindings = append(snap.Bindings, rbacBindingDef{Namespace: "a", Name: "sa", RoleKind: "ClusterRole", RoleName: "edit", Subjects: []RBACSubject{{Kind: "ServiceAccount", Name: "bot"}}})
	if det, err := snap.Subject("ServiceAccount", "bot", "a"); err != nil || len(det.Namespaced) != 1 {
		t.Fatalf("default namespace for SA subject: %+v err=%v", det, err)
	}
}

func TestRBACResourceMatching(t *testing.T) {
	cases := []struct {
		rules []string
		res   string
		want  bool
	}{
		{[]string{"pods"}, "pods", true},
		{[]string{"pods"}, "pods/log", false},
		{[]string{"pods/*"}, "pods/log", true},
		{[]string{"pods/*"}, "pods", false},
		{[]string{"*"}, "anything/sub", true},
		{[]string{"*/scale"}, "deployments/scale", true},
		{[]string{"*/scale"}, "deployments", false},
		{[]string{"pods/logs"}, "pods/log", false},
		{nil, "pods", false},
	}
	for _, c := range cases {
		if got := resourceMatches(c.rules, c.res); got != c.want {
			t.Errorf("resourceMatches(%v,%q)=%v want %v", c.rules, c.res, got, c.want)
		}
	}
}

func TestRBACWhoCanValidation(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	for _, q := range []RBACWhoCanQuery{
		{}, {Verb: "get"}, {Resource: "pods"},
		{Verb: "get", Resource: "pods\nx"}, {Verb: "g et", Resource: "pods"},
		{Verb: "get", Resource: "pods", Namespace: strings.Repeat("a", 300)},
	} {
		if _, err := d.RBACWhoCan(ctx, q); err == nil || HTTPStatus(err) != 400 {
			t.Errorf("query %+v: err=%v status=%d, want 400", q, err, HTTPStatus(err))
		}
	}
	if _, err := d.GetRBACSubject(ctx, "Robot", "x", ""); HTTPStatus(err) != 400 {
		t.Errorf("bad kind: %v", err)
	}
	if _, err := d.GetRBACSubject(ctx, "ServiceAccount", "x", ""); HTTPStatus(err) != 400 {
		t.Errorf("SA without namespace: %v", err)
	}
	if _, err := d.GetRBACSubject(ctx, "User", "stranger", ""); HTTPStatus(err) != 404 {
		t.Errorf("unknown subject: %v", err)
	}
	res, err := d.RBACWhoCan(ctx, RBACWhoCanQuery{Verb: "GET", Resource: "Pods", Namespace: "all"})
	if err != nil || res.Query.Verb != "get" || res.Query.Resource != "pods" || res.Query.Namespace != "" || len(res.Allowed) == 0 {
		t.Errorf("normalisation: %+v err=%v", res, err)
	}
}

func TestDemoServiceAccounts(t *testing.T) {
	d := NewDemoProvider()
	all, _ := d.ListServiceAccounts(context.Background(), "all")
	pay, _ := d.ListServiceAccounts(context.Background(), "payments")
	if len(all) < 5 || len(pay) != 1 || pay[0].Name != "payments-api" || pay[0].Bindings != 1 {
		t.Fatalf("all=%d payments=%+v", len(all), pay)
	}
}

// ---- NetworkPolicy engine ---------------------------------------------------

func sel(kv ...string) npSelector {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return npSelector{MatchLabels: m}
}

func TestNetpolSelectorsAndUnion(t *testing.T) {
	in := npInput{
		Namespace:  "app",
		Namespaces: []npNamespace{{Name: "app", Labels: map[string]string{"team": "a"}}, {Name: "mon", Labels: map[string]string{"team": "obs"}}},
		Pods: []npPod{
			{Name: "web-1", Labels: map[string]string{"app": "web", "tier": "front"}},
			{Name: "db-1", Labels: map[string]string{"app": "db"}},
			{Name: "free-1", Labels: map[string]string{"app": "free"}},
		},
	}
	webSel := sel("app", "web")
	in.Policies = []npPolicy{
		{Namespace: "app", Name: "p-web-1", PodSelector: webSel, Ingress: []npRule{{Peers: []npPeer{{Pod: &npSelector{MatchLabels: map[string]string{"app": "db"}}}}, Ports: []npPort{{Protocol: "TCP", Port: "80"}}}}},
		{Namespace: "app", Name: "p-web-2", PodSelector: npSelector{Exprs: []npExpr{{Key: "tier", Op: "In", Values: []string{"front", "edge"}}}},
			Ingress: []npRule{{Peers: []npPeer{{Namespace: &npSelector{MatchLabels: map[string]string{"team": "obs"}}}}}}},
		{Namespace: "app", Name: "p-db", PodSelector: sel("app", "db"), Types: []string{"Ingress", "Egress"}},
	}
	res := analyzeNetworkPolicies(in)
	byName := map[string]PodNetworkAnalysis{}
	for _, p := range res.Pods {
		byName[p.Pod] = p
	}
	web := byName["web-1"]
	if !web.IngressIsolated || web.EgressIsolated || len(web.Policies) != 2 || len(web.Ingress) != 2 {
		t.Fatalf("web-1 (union of two policies): %+v", web)
	}
	if web.Ingress[0].Kind != "pods" || web.Ingress[0].Pods[0] != "db-1" || web.Ingress[0].Ports[0] != "TCP/80" {
		t.Errorf("first rule should resolve the db pod on TCP/80: %+v", web.Ingress[0])
	}
	if web.Ingress[1].Kind != "namespace" || web.Ingress[1].Namespaces[0] != "mon" || web.Ingress[1].Ports[0] != "all ports" {
		t.Errorf("second rule should be every pod in namespace mon on all ports: %+v", web.Ingress[1])
	}
	db := byName["db-1"]
	if !db.IngressIsolated || !db.EgressIsolated || len(db.Ingress) != 0 || len(db.Egress) != 0 {
		t.Fatalf("db-1 is denied both ways: %+v", db)
	}
	if len(res.Uncovered) != 1 || res.Uncovered[0] != "free-1" || res.DefaultDenyIngress {
		t.Fatalf("uncovered=%v defaultDenyIngress=%v", res.Uncovered, res.DefaultDenyIngress)
	}
}

func TestNetpolEmptyRuleAllowsAllAndTypesDefault(t *testing.T) {
	in := npInput{Namespace: "n", Pods: []npPod{{Name: "a", Labels: map[string]string{}}},
		Policies: []npPolicy{{Namespace: "n", Name: "allow-all-in", Ingress: []npRule{{}}}}}
	res := analyzeNetworkPolicies(in)
	a := res.Pods[0]
	if !a.IngressIsolated || a.EgressIsolated || len(a.Ingress) != 1 || a.Ingress[0].Kind != "any" || a.Ingress[0].Ports[0] != "all ports" {
		t.Fatalf("empty ingress rule allows every peer/port; egress untouched when only ingress rules exist: %+v", a)
	}
	// policyTypes omitted but egress rules present -> Egress is isolated too.
	in.Policies = []npPolicy{{Namespace: "n", Name: "eg", Egress: []npRule{{Ports: []npPort{{Protocol: "UDP", Port: "53"}}}}}}
	a = analyzeNetworkPolicies(in).Pods[0]
	if !a.IngressIsolated || !a.EgressIsolated || a.Egress[0].Ports[0] != "UDP/53" {
		t.Fatalf("default policyTypes: %+v", a)
	}
}

func TestNetpolSelectorOperators(t *testing.T) {
	labels := map[string]string{"env": "prod", "tier": "web"}
	cases := []struct {
		s    npSelector
		want bool
	}{
		{npSelector{}, true},
		{npSelector{Exprs: []npExpr{{Key: "env", Op: "NotIn", Values: []string{"dev"}}}}, true},
		{npSelector{Exprs: []npExpr{{Key: "env", Op: "NotIn", Values: []string{"prod"}}}}, false},
		{npSelector{Exprs: []npExpr{{Key: "missing", Op: "NotIn", Values: []string{"x"}}}}, true},
		{npSelector{Exprs: []npExpr{{Key: "tier", Op: "Exists"}}}, true},
		{npSelector{Exprs: []npExpr{{Key: "x", Op: "DoesNotExist"}}}, true},
		{npSelector{Exprs: []npExpr{{Key: "tier", Op: "DoesNotExist"}}}, false},
		{npSelector{Exprs: []npExpr{{Key: "env", Op: "Bogus"}}}, false}, // unknown operator selects nothing
		{npSelector{MatchLabels: map[string]string{"env": "prod"}, Exprs: []npExpr{{Key: "tier", Op: "In", Values: []string{"api"}}}}, false},
	}
	for i, c := range cases {
		if got := c.s.matches(labels); got != c.want {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}

func TestNetpolAnalysisDemoReflectsDeletion(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	a, err := d.AnalyzeNetworkPolicies(ctx, "payments")
	if err != nil || a.Policies != 1 || !a.DefaultDenyIngress {
		t.Fatalf("payments: %+v err=%v", a, err)
	}
	if err := d.DeleteNetworkPolicy(ctx, "payments", "deny-all-ingress"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		demoNetPolMu.Lock()
		demoNetPols["payments/deny-all-ingress"] = NetworkPolicySummary{Namespace: "payments", Name: "deny-all-ingress", PodSelector: "(all pods)", PolicyTypes: []string{"Ingress"}, Age: "40d"}
		demoNetPolMu.Unlock()
	})
	a, _ = d.AnalyzeNetworkPolicies(ctx, "payments")
	if a.Policies != 0 || len(a.Uncovered) != len(a.Pods) || len(a.Pods) == 0 {
		t.Fatalf("after deleting the only policy every pod is uncovered: %+v", a)
	}
}

// ---- Security reports -------------------------------------------------------

func TestSecurityReportsAbsentCRDsGiveInstallHint(t *testing.T) {
	set := secReportSet{Sources: []SecuritySource{
		{Name: "Trivy VulnerabilityReports", Group: "aquasecurity.github.io", Message: "CRD not installed"},
		{Name: "PolicyReports", Group: "wgpolicyk8s.io", Message: "CRD not installed"},
	}}
	s := set.summary()
	if s.Available || s.InstallHint == "" || len(s.Workloads) != 0 || len(s.Sources) != 2 {
		t.Fatalf("empty state: %+v", s)
	}
	if !strings.Contains(s.InstallHint, "trivy-operator") || !strings.Contains(s.InstallHint, "Kyverno") {
		t.Errorf("install hint should name the operators: %q", s.InstallHint)
	}
	if (secReportSet{}).summary().Sources == nil {
		t.Error("sources must serialise as [] not null")
	}
}

func TestSecuritySummaryFromRealisticObjects(t *testing.T) {
	// No precomputed summary: counts must be derived from the findings, and a
	// ReplicaSet owner must fold into its Deployment.
	vr := map[string]any{
		"metadata": map[string]any{"namespace": "ns", "labels": map[string]any{
			"trivy-operator.resource.kind": "ReplicaSet", "trivy-operator.resource.name": "api-5d8f7c9b6c",
		}},
		"report": map[string]any{"artifact": map[string]any{"repository": "lib/api", "tag": "1"}, "vulnerabilities": []any{
			map[string]any{"vulnerabilityID": "CVE-1", "severity": "CRITICAL", "resource": "a", "installedVersion": "1", "primaryLink": "javascript:alert(1)"},
			map[string]any{"vulnerabilityID": "CVE-1", "severity": "CRITICAL", "resource": "a", "installedVersion": "1"}, // duplicate across containers
			map[string]any{"vulnerabilityID": "CVE-2", "severity": "weird", "resource": "b", "installedVersion": "2"},
		}},
	}
	cr := map[string]any{
		"metadata": map[string]any{"namespace": "ns", "ownerReferences": []any{map[string]any{"kind": "Deployment", "name": "api"}}},
		"report": map[string]any{"checks": []any{
			map[string]any{"checkID": "K1", "severity": "HIGH", "success": false, "title": "t", "messages": []any{"m"}},
			map[string]any{"checkID": "K2", "severity": "HIGH", "success": true},
		}},
	}
	pr := map[string]any{
		"metadata": map[string]any{"namespace": "ns"},
		"results": []any{
			map[string]any{"policy": "p", "result": "fail", "resources": []any{map[string]any{"kind": "Deployment", "name": "api"}}},
			map[string]any{"policy": "q", "result": "pass", "resources": []any{map[string]any{"kind": "Deployment", "name": "api"}, map[string]any{"kind": "Deployment", "name": "other"}}},
		},
	}
	set := secReportSet{Vuln: []map[string]any{vr}, Config: []map[string]any{cr}, Policy: []map[string]any{pr},
		Sources: []SecuritySource{{Name: "x", Available: true}}}
	s := set.summary()
	if len(s.Workloads) != 2 {
		t.Fatalf("workloads: %+v", s.Workloads)
	}
	api := s.Workloads[0]
	if api.Name != "api" || api.Kind != "Deployment" || api.Vulnerabilities.Critical != 2 || api.Vulnerabilities.Unknown != 1 ||
		api.Misconfigurations.High != 1 || api.Policy.Fail != 1 || api.Policy.Pass != 1 || len(api.Images) != 1 || api.Images[0] != "lib/api:1" {
		t.Fatalf("api row: %+v", api)
	}
	d := set.detail("ns", "Deployment", "api")
	if len(d.Vulnerabilities) != 2 || d.VulnerabilityCnt.Critical != 1 {
		t.Fatalf("duplicate findings must be collapsed: %+v", d.Vulnerabilities)
	}
	for _, v := range d.Vulnerabilities {
		if v.Link != "" {
			t.Errorf("non-http link must be dropped, got %q", v.Link)
		}
	}
	if len(d.Misconfigurations) != 1 || d.Misconfigurations[0].ID != "K1" {
		t.Errorf("passing checks must not be listed: %+v", d.Misconfigurations)
	}
	if d.PolicyCnt.Pass != 1 || len(d.PolicyResults) != 1 || d.PolicyResults[0].Result != "fail" {
		t.Errorf("policy detail: %+v", d)
	}
	if other := set.detail("ns", "Deployment", "other"); other.PolicyCnt.Pass != 1 || len(other.Vulnerabilities) != 0 {
		t.Errorf("other: %+v", other)
	}
	if missing := set.detail("different-ns", "Deployment", "api"); len(missing.Vulnerabilities)+len(missing.PolicyResults) != 0 {
		t.Errorf("namespace must be part of the match: %+v", missing)
	}
}

func TestDemoSecurityValidation(t *testing.T) {
	d := NewDemoProvider()
	if _, err := d.GetWorkloadSecurity(context.Background(), "", "Deployment", "x"); HTTPStatus(err) != 400 {
		t.Errorf("empty namespace: %v", err)
	}
	s, _ := d.ListSecurityReports(context.Background(), "nowhere")
	if !s.Available || len(s.Workloads) != 0 || s.Vulnerabilities.total() != 0 {
		t.Errorf("filtering to an unknown namespace yields an empty (but available) summary: %+v", s)
	}
}
