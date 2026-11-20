package platform

import (
	"context"
	"sort"
	"strings"
)

// demoRBACSnapshot is a fixed, deterministic RBAC graph for demo mode and e2e.
// It is independent of the mutable demoRoles/demoRoleBindings used by the
// Roles tab so that "who can" answers never change under a test.
func demoRBACSnapshot() rbacSnapshot {
	read := []string{"get", "list", "watch"}
	allVerbs := []string{"get", "list", "watch", "create", "update", "patch", "delete"}
	sa := func(ns, name string) RBACSubject {
		return RBACSubject{Kind: subjectServiceAccount, Name: name, Namespace: ns}
	}
	return rbacSnapshot{
		ClusterRoles: map[string]rbacRoleDef{
			"cluster-admin": {Name: "cluster-admin", Rules: []RBACRuleView{{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}}}},
			"view": {Name: "view", Rules: []RBACRuleView{
				{APIGroups: []string{""}, Resources: []string{"pods", "pods/log", "services", "endpoints", "configmaps", "namespaces"}, Verbs: read},
				{APIGroups: []string{"apps"}, Resources: []string{"deployments", "statefulsets", "daemonsets", "replicasets"}, Verbs: read},
			}},
			"edit": {Name: "edit", Rules: []RBACRuleView{
				{APIGroups: []string{""}, Resources: []string{"pods", "pods/exec", "services", "configmaps", "secrets"}, Verbs: allVerbs},
				{APIGroups: []string{"apps"}, Resources: []string{"deployments", "statefulsets", "daemonsets"}, Verbs: allVerbs},
			}},
			"zyvor-axiom": {Name: "zyvor-axiom", Rules: []RBACRuleView{
				{APIGroups: []string{""}, Resources: []string{"pods", "pods/log", "services", "nodes", "namespaces", "events", "serviceaccounts"}, Verbs: read},
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: append([]string{"create", "update", "delete"}, read...)},
				{APIGroups: []string{"apps"}, Resources: []string{"deployments", "statefulsets", "daemonsets"}, Verbs: []string{"get", "list", "watch", "patch", "update"}},
				{APIGroups: []string{"rbac.authorization.k8s.io"}, Resources: []string{"roles", "rolebindings", "clusterroles", "clusterrolebindings"}, Verbs: read},
			}},
		},
		Roles: map[string]rbacRoleDef{
			"default/app-editor": {Namespace: "default", Name: "app-editor", Rules: []RBACRuleView{
				{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: allVerbs},
				{APIGroups: []string{""}, Resources: []string{"configmaps", "services"}, Verbs: allVerbs},
			}},
			"platform/ops-reader": {Namespace: "platform", Name: "ops-reader", Rules: []RBACRuleView{
				{APIGroups: []string{""}, Resources: []string{"pods", "pods/log", "events"}, Verbs: read},
			}},
			"payments/payments-operator": {Namespace: "payments", Name: "payments-operator", Rules: []RBACRuleView{
				{APIGroups: []string{""}, Resources: []string{"pods", "pods/log", "pods/exec"}, Verbs: []string{"get", "list", "create"}},
				{APIGroups: []string{""}, Resources: []string{"secrets"}, ResourceNames: []string{"payments-db"}, Verbs: []string{"get"}},
			}},
		},
		Bindings: []rbacBindingDef{
			{Name: "cluster-admin", Cluster: true, RoleKind: "ClusterRole", RoleName: "cluster-admin", Subjects: []RBACSubject{
				{Kind: subjectGroup, Name: "system:masters"}, {Kind: subjectUser, Name: "ops-lead@zyvor.dev"},
			}},
			{Name: "zyvor-axiom", Cluster: true, RoleKind: "ClusterRole", RoleName: "zyvor-axiom", Subjects: []RBACSubject{sa("axiom-system", "zyvor-axiom")}},
			{Name: "devs-view", Cluster: true, RoleKind: "ClusterRole", RoleName: "view", Subjects: []RBACSubject{{Kind: subjectGroup, Name: "devs"}}},
			{Namespace: "default", Name: "app-editor-binding", RoleKind: "Role", RoleName: "app-editor", Subjects: []RBACSubject{
				{Kind: subjectUser, Name: "bob@zyvor.dev"}, {Kind: subjectGroup, Name: "devs"},
			}},
			{Namespace: "platform", Name: "ops-reader-binding", RoleKind: "Role", RoleName: "ops-reader", Subjects: []RBACSubject{{Kind: subjectGroup, Name: "sre"}}},
			{Namespace: "payments", Name: "payments-operator-binding", RoleKind: "Role", RoleName: "payments-operator", Subjects: []RBACSubject{
				{Kind: subjectUser, Name: "alice@zyvor.dev"}, sa("payments", "payments-api"),
			}},
			{Namespace: "commerce", Name: "checkout-edit", RoleKind: "ClusterRole", RoleName: "edit", Subjects: []RBACSubject{sa("commerce", "checkout")}},
		},
		ServiceAccounts: []RBACSubject{
			sa("axiom-system", "zyvor-axiom"), sa("payments", "payments-api"), sa("commerce", "checkout"),
			sa("default", "default"), sa("data", "redis"), sa("observability", "otel-collector"),
		},
	}
}

func (d *DemoProvider) ListRBACSubjects(ctx context.Context) ([]RBACSubjectSummary, error) {
	return demoRBACSnapshot().Subjects(), nil
}

func (d *DemoProvider) GetRBACSubject(ctx context.Context, kind, name, namespace string) (RBACSubjectDetail, error) {
	return demoRBACSnapshot().Subject(kind, name, namespace)
}

func (d *DemoProvider) RBACWhoCan(ctx context.Context, q RBACWhoCanQuery) (RBACWhoCanResult, error) {
	return demoRBACSnapshot().WhoCan(q)
}

func (d *DemoProvider) ListServiceAccounts(ctx context.Context, ns string) ([]ServiceAccountSummary, error) {
	snap := demoRBACSnapshot()
	counts := snap.saBindingCounts()
	out := []ServiceAccountSummary{}
	for _, sa := range snap.ServiceAccounts {
		if ns != "" && ns != "all" && sa.Namespace != ns {
			continue
		}
		out = append(out, ServiceAccountSummary{Namespace: sa.Namespace, Name: sa.Name, Secrets: 0, Bindings: counts[sa.Namespace+"/"+sa.Name], Age: "30d"})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return strings.Compare(out[i].Name, out[j].Name) < 0
	})
	return out, nil
}
