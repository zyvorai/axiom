//go:build kubernetes

package platform

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	clienttesting "k8s.io/client-go/testing"
)

func TestKubernetesRBACViewer(t *testing.T) {
	read := []string{"get", "list", "watch"}
	p, _, _ := newFakeFullProvider(nil, nil,
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "cluster-admin"}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}}}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "view"}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods", "pods/log"}, Verbs: read}}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "deployer", Namespace: "team-a"}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "patch"}}}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "root"}, RoleRef: rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
			Subjects: []rbacv1.Subject{{Kind: "User", Name: "root@x"}}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "team-a"}, RoleRef: rbacv1.RoleRef{Kind: "Role", Name: "deployer"},
			Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "ci", Namespace: "team-a"}, {Kind: "Group", Name: "devs"}}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "view-b", Namespace: "team-b"}, RoleRef: rbacv1.RoleRef{Kind: "ClusterRole", Name: "view"},
			Subjects: []rbacv1.Subject{{Kind: "Group", Name: "devs"}}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "ci", Namespace: "team-a"}, Secrets: []corev1.ObjectReference{{Name: "s"}}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-a"}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team-b"}},
	)
	ctx := context.Background()

	subs, err := p.ListRBACSubjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	idx := map[string]RBACSubjectSummary{}
	for _, s := range subs {
		idx[s.Kind+"/"+s.Namespace+"/"+s.Name] = s
	}
	if s := idx["User//root@x"]; !s.ClusterAdmin || !s.ClusterWide {
		t.Errorf("root: %+v", s)
	}
	if s := idx["Group//devs"]; s.Bindings != 2 || s.ClusterWide {
		t.Errorf("devs holds two namespaced bindings: %+v", s)
	}
	if _, ok := idx["ServiceAccount/team-b/default"]; !ok {
		t.Errorf("unbound ServiceAccounts must be listed: %v", len(idx))
	}

	det, err := p.GetRBACSubject(ctx, "Group", "devs", "")
	if err != nil || len(det.Namespaced) != 2 || det.Namespaced[0].Namespace != "team-a" || det.Namespaced[1].RoleRef != "ClusterRole/view" {
		t.Fatalf("devs detail: %+v err=%v", det, err)
	}

	res, err := p.RBACWhoCan(ctx, RBACWhoCanQuery{Verb: "patch", Resource: "deployments", Group: "apps", Namespace: "team-a"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range res.Allowed {
		got[e.Subject.Kind+"/"+e.Subject.Name+"@"+e.Scope] = true
	}
	if !got["User/root@x@cluster"] || !got["ServiceAccount/ci@namespace"] || !got["Group/devs@namespace"] || len(got) != 3 {
		t.Errorf("who can patch deployments in team-a: %v", got)
	}
	// team-b only has the read-only view binding.
	res, _ = p.RBACWhoCan(ctx, RBACWhoCanQuery{Verb: "patch", Resource: "deployments", Namespace: "team-b"})
	if len(res.Allowed) != 1 || res.Allowed[0].Subject.Name != "root@x" {
		t.Errorf("team-b patch: %+v", res.Allowed)
	}

	sas, err := p.ListServiceAccounts(ctx, "team-a")
	if err != nil || len(sas) != 2 || sas[0].Name != "ci" || sas[0].Secrets != 1 || sas[0].Bindings != 1 {
		t.Fatalf("service accounts: %+v err=%v", sas, err)
	}
}

func TestKubernetesRBACViewerFailsClosedOnPartialData(t *testing.T) {
	p, cs, _ := newFakeFullProvider(nil, nil)
	cs.PrependReactor("list", "rolebindings", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "rolebindings"}, "", nil)
	})
	_, err := p.RBACWhoCan(context.Background(), RBACWhoCanQuery{Verb: "get", Resource: "pods"})
	if err == nil || HTTPStatus(err) != 403 {
		t.Fatalf("a snapshot that cannot read every binding must not answer who-can (err=%v)", err)
	}
}

func TestKubernetesNetworkPolicyAnalysis(t *testing.T) {
	tcp := corev1.ProtocolTCP
	port := intstr.FromInt32(8080)
	p, _, _ := newFakeFullProvider(nil, nil,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shop"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ingress", Labels: map[string]string{"role": "edge"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "shop", Labels: map[string]string{"app": "web"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "shop", Labels: map[string]string{"app": "api"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "stray-1", Namespace: "shop", Labels: map[string]string{"app": "stray"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "done-1", Namespace: "shop", Labels: map[string]string{"app": "job"}}, Status: corev1.PodStatus{Phase: corev1.PodSucceeded}},
		&netv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "deny-all", Namespace: "shop"}, Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"web", "api"}}}},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress, netv1.PolicyTypeEgress},
		}},
		&netv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "web-in", Namespace: "shop"}, Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			Ingress: []netv1.NetworkPolicyIngressRule{
				{From: []netv1.NetworkPolicyPeer{
					{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "edge"}}},
					{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}},
					{IPBlock: &netv1.IPBlock{CIDR: "10.0.0.0/8", Except: []string{"10.9.0.0/16"}}},
				}, Ports: []netv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port}}},
			},
		}},
		// A malformed peer (neither selector nor ipBlock) must NOT widen to "any".
		&netv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "shop"}, Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Ingress:     []netv1.NetworkPolicyIngressRule{{From: []netv1.NetworkPolicyPeer{{}}}},
		}},
	)
	a, err := p.AnalyzeNetworkPolicies(context.Background(), "shop")
	if err != nil {
		t.Fatal(err)
	}
	if a.Policies != 3 || len(a.Pods) != 3 || len(a.Uncovered) != 1 || a.Uncovered[0] != "stray-1" {
		t.Fatalf("policies/pods/uncovered: %+v", a)
	}
	// deny-all selects only web/api through a matchExpression, so it is NOT a
	// namespace-wide default deny.
	if a.DefaultDenyIngress || a.DefaultDenyEgress {
		t.Fatalf("a policy with a podSelector expression is not a namespace-wide default deny: %+v", a)
	}
	by := map[string]PodNetworkAnalysis{}
	for _, pod := range a.Pods {
		by[pod.Pod] = pod
	}
	web := by["web-1"]
	if !web.IngressIsolated || !web.EgressIsolated || len(web.Policies) != 2 || len(web.Ingress) != 3 {
		t.Fatalf("web-1: %+v", web)
	}
	if web.Ingress[0].Namespaces[0] != "ingress" || web.Ingress[1].Pods[0] != "api-1" || web.Ingress[2].CIDR != "10.0.0.0/8" || web.Ingress[0].Ports[0] != "TCP/8080" {
		t.Errorf("peers: %+v", web.Ingress)
	}
	api := by["api-1"]
	if len(api.Ingress) != 1 || len(api.Ingress[0].Pods) != 0 || api.Ingress[0].PodCount != 0 {
		t.Errorf("the malformed peer must match nothing: %+v", api.Ingress)
	}
	if _, ok := by["done-1"]; ok {
		t.Errorf("completed pods must be excluded")
	}
	if _, err := p.AnalyzeNetworkPolicies(context.Background(), "all"); HTTPStatus(err) != 400 {
		t.Errorf("all: %v", err)
	}
	if _, err := p.AnalyzeNetworkPolicies(context.Background(), "ghost"); HTTPStatus(err) != 404 {
		t.Errorf("ghost: %v", err)
	}
}

func reportObj(gv, kind, ns, name string, labels map[string]any, body map[string]any) *unstructured.Unstructured {
	meta := map[string]any{"name": name, "namespace": ns}
	if labels != nil {
		meta["labels"] = labels
	}
	obj := map[string]any{"apiVersion": gv, "kind": kind, "metadata": meta}
	for k, v := range body {
		obj[k] = v
	}
	return &unstructured.Unstructured{Object: obj}
}

var (
	vulnGVR   = schema.GroupVersionResource{Group: "aquasecurity.github.io", Version: "v1alpha1", Resource: "vulnerabilityreports"}
	configGVR = schema.GroupVersionResource{Group: "aquasecurity.github.io", Version: "v1alpha1", Resource: "configauditreports"}
	polGVR    = schema.GroupVersionResource{Group: "wgpolicyk8s.io", Version: "v1alpha2", Resource: "policyreports"}
	polBetaGV = schema.GroupVersionResource{Group: "wgpolicyk8s.io", Version: "v1beta1", Resource: "policyreports"}
	cpolGVR   = schema.GroupVersionResource{Group: "wgpolicyk8s.io", Version: "v1alpha2", Resource: "clusterpolicyreports"}
	cpolBeta  = schema.GroupVersionResource{Group: "wgpolicyk8s.io", Version: "v1beta1", Resource: "clusterpolicyreports"}
)

func allReportKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		vulnGVR: "VulnerabilityReportList", configGVR: "ConfigAuditReportList",
		polGVR: "PolicyReportList", polBetaGV: "PolicyReportList", cpolGVR: "ClusterPolicyReportList", cpolBeta: "ClusterPolicyReportList",
	}
}

func TestKubernetesSecurityReportsAggregation(t *testing.T) {
	lbl := func(kind, name string) map[string]any {
		return map[string]any{"trivy-operator.resource.kind": kind, "trivy-operator.resource.name": name}
	}
	vr := reportObj("aquasecurity.github.io/v1alpha1", "VulnerabilityReport", "shop", "rs-web", lbl("ReplicaSet", "web-6f7d9c8b5d"), map[string]any{
		"report": map[string]any{
			"artifact": map[string]any{"repository": "shop/web", "tag": "2.0"},
			"summary":  map[string]any{"criticalCount": int64(2), "highCount": int64(1), "mediumCount": int64(0), "lowCount": int64(0), "unknownCount": int64(0)},
			"vulnerabilities": []any{
				map[string]any{"vulnerabilityID": "CVE-A", "severity": "CRITICAL", "resource": "openssl", "installedVersion": "1", "fixedVersion": "2", "score": 9.8},
				map[string]any{"vulnerabilityID": "CVE-B", "severity": "CRITICAL", "resource": "zlib", "installedVersion": "1"},
				map[string]any{"vulnerabilityID": "CVE-C", "severity": "HIGH", "resource": "curl", "installedVersion": "7"},
			},
		}})
	vrOther := reportObj("aquasecurity.github.io/v1alpha1", "VulnerabilityReport", "other", "rs-x", lbl("Deployment", "x"), map[string]any{
		"report": map[string]any{"summary": map[string]any{"criticalCount": int64(9), "highCount": int64(0), "mediumCount": int64(0), "lowCount": int64(0), "unknownCount": int64(0)}}})
	cr := reportObj("aquasecurity.github.io/v1alpha1", "ConfigAuditReport", "shop", "dep-web", lbl("Deployment", "web"), map[string]any{
		"report": map[string]any{"summary": map[string]any{"criticalCount": int64(0), "highCount": int64(1), "mediumCount": int64(0), "lowCount": int64(0)},
			"checks": []any{map[string]any{"checkID": "KSV017", "severity": "HIGH", "title": "Privileged", "success": false, "messages": []any{"set privileged=false"}}}}})
	pr := reportObj("wgpolicyk8s.io/v1alpha2", "PolicyReport", "shop", "polr-web", nil, map[string]any{
		"scope":   map[string]any{"kind": "Deployment", "name": "web", "namespace": "shop"},
		"summary": map[string]any{"pass": int64(3), "fail": int64(1), "warn": int64(0), "error": int64(0), "skip": int64(0)},
		"results": []any{map[string]any{"policy": "no-latest", "rule": "tag", "result": "fail", "severity": "medium", "message": "uses :latest"}}})
	cpr := reportObj("wgpolicyk8s.io/v1alpha2", "ClusterPolicyReport", "", "cpolr", nil, map[string]any{
		"results": []any{map[string]any{"policy": "ns-labels", "result": "fail", "resources": []any{map[string]any{"kind": "Namespace", "name": "shop"}}}}})

	p, _, _ := newFakeFullProvider(allReportKinds(), []runtime.Object{vr, vrOther, cr, pr, cpr})
	ctx := context.Background()

	all, err := p.ListSecurityReports(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if !all.Available || len(all.Sources) != 3 || all.InstallHint != "" {
		t.Fatalf("sources: %+v", all.Sources)
	}
	if all.Vulnerabilities.Critical != 11 || all.Misconfigurations.High != 1 || all.Policy.Fail != 2 {
		t.Fatalf("totals: %+v", all)
	}
	if all.Workloads[0].Name != "x" || all.Workloads[1].Name != "web" || all.Workloads[1].Kind != "Deployment" {
		t.Fatalf("ordering/normalisation: %+v", all.Workloads)
	}

	scoped, _ := p.ListSecurityReports(ctx, "shop")
	if scoped.Vulnerabilities.Critical != 2 || len(scoped.Workloads) != 1 {
		t.Fatalf("namespace scope must exclude other namespaces and cluster-scoped reports: %+v", scoped)
	}
	if scoped.Policy.Fail != 1 {
		t.Fatalf("cluster policy reports are all-namespaces only: %+v", scoped.Policy)
	}

	d, err := p.GetWorkloadSecurity(ctx, "shop", "Deployment", "web")
	if err != nil || len(d.Vulnerabilities) != 3 || d.Vulnerabilities[0].ID != "CVE-A" || d.Vulnerabilities[0].Fixed != "2" ||
		len(d.Misconfigurations) != 1 || len(d.PolicyResults) != 1 || d.Images[0] != "shop/web:2.0" {
		t.Fatalf("drill-down: %+v err=%v", d, err)
	}
	if _, err := p.GetWorkloadSecurity(ctx, "all", "Deployment", "web"); HTTPStatus(err) != 400 {
		t.Errorf("namespace=all drill-down: %v", err)
	}
}

func TestKubernetesSecurityReportsCRDsAbsent(t *testing.T) {
	p, _, dyn := newFakeFullProvider(allReportKinds(), nil)
	dyn.PrependReactor("list", "*", func(a clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: a.GetResource().Group, Resource: a.GetResource().Resource}, "")
	})
	s, err := p.ListSecurityReports(context.Background(), "")
	if err != nil {
		t.Fatalf("missing CRDs are an empty state, not an error: %v", err)
	}
	if s.Available || s.InstallHint == "" || len(s.Workloads) != 0 || len(s.Sources) != 3 {
		t.Fatalf("empty state: %+v", s)
	}
	for _, src := range s.Sources {
		if src.Available || src.Message == "" {
			t.Errorf("source %+v", src)
		}
	}
}

func TestKubernetesSecurityReportsForbiddenIsExplained(t *testing.T) {
	p, _, dyn := newFakeFullProvider(allReportKinds(), nil)
	dyn.PrependReactor("list", "vulnerabilityreports", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "aquasecurity.github.io", Resource: "vulnerabilityreports"}, "", nil)
	})
	dyn.PrependReactor("list", "*", func(a clienttesting.Action) (bool, runtime.Object, error) {
		if a.GetResource().Resource == "vulnerabilityreports" {
			return false, nil, nil
		}
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: a.GetResource().Resource}, "")
	})
	s, err := p.ListSecurityReports(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, src := range s.Sources {
		if src.Name == "Trivy VulnerabilityReports" {
			found = !src.Available && src.Message != ""
		}
	}
	if !found {
		t.Fatalf("a Forbidden list must surface as an unavailable source with a hint: %+v", s.Sources)
	}
}
