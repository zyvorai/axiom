//go:build kubernetes

package platform

import (
	"context"
	"sort"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ruleViews(rules []rbacv1.PolicyRule) []RBACRuleView {
	out := make([]RBACRuleView, 0, len(rules))
	for _, r := range rules {
		out = append(out, RBACRuleView{
			APIGroups: r.APIGroups, Resources: r.Resources, ResourceNames: r.ResourceNames,
			NonResourceURLs: r.NonResourceURLs, Verbs: r.Verbs,
		})
	}
	return out
}

func rbacSubjects(in []rbacv1.Subject) []RBACSubject {
	out := make([]RBACSubject, 0, len(in))
	for _, s := range in {
		out = append(out, RBACSubject{Kind: s.Kind, Name: s.Name, Namespace: s.Namespace})
	}
	return out
}

// loadRBACSnapshot reads every Role, ClusterRole, binding and ServiceAccount the
// ServiceAccount of Axiom can list. Each list is a hard requirement: a partial
// snapshot would silently under-report who can do what, so any error is
// returned rather than ignored.
func (k *KubernetesProvider) loadRBACSnapshot(ctx context.Context) (rbacSnapshot, error) {
	snap := rbacSnapshot{ClusterRoles: map[string]rbacRoleDef{}, Roles: map[string]rbacRoleDef{}}
	crs, err := k.client.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		return snap, err
	}
	for _, r := range crs.Items {
		snap.ClusterRoles[r.Name] = rbacRoleDef{Name: r.Name, Rules: ruleViews(r.Rules)}
	}
	roles, err := k.client.RbacV1().Roles(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return snap, err
	}
	for _, r := range roles.Items {
		snap.Roles[r.Namespace+"/"+r.Name] = rbacRoleDef{Namespace: r.Namespace, Name: r.Name, Rules: ruleViews(r.Rules)}
	}
	crbs, err := k.client.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return snap, err
	}
	for _, b := range crbs.Items {
		snap.Bindings = append(snap.Bindings, rbacBindingDef{
			Name: b.Name, Cluster: true, RoleKind: b.RoleRef.Kind, RoleName: b.RoleRef.Name, Subjects: rbacSubjects(b.Subjects),
		})
	}
	rbs, err := k.client.RbacV1().RoleBindings(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return snap, err
	}
	for _, b := range rbs.Items {
		snap.Bindings = append(snap.Bindings, rbacBindingDef{
			Namespace: b.Namespace, Name: b.Name, RoleKind: b.RoleRef.Kind, RoleName: b.RoleRef.Name, Subjects: rbacSubjects(b.Subjects),
		})
	}
	sas, err := k.client.CoreV1().ServiceAccounts(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return snap, err
	}
	for _, sa := range sas.Items {
		snap.ServiceAccounts = append(snap.ServiceAccounts, RBACSubject{Kind: subjectServiceAccount, Name: sa.Name, Namespace: sa.Namespace})
	}
	return snap, nil
}

func (k *KubernetesProvider) ListRBACSubjects(ctx context.Context) ([]RBACSubjectSummary, error) {
	snap, err := k.loadRBACSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return snap.Subjects(), nil
}

func (k *KubernetesProvider) GetRBACSubject(ctx context.Context, kind, name, namespace string) (RBACSubjectDetail, error) {
	snap, err := k.loadRBACSnapshot(ctx)
	if err != nil {
		return RBACSubjectDetail{}, err
	}
	return snap.Subject(kind, name, namespace)
}

func (k *KubernetesProvider) RBACWhoCan(ctx context.Context, q RBACWhoCanQuery) (RBACWhoCanResult, error) {
	snap, err := k.loadRBACSnapshot(ctx)
	if err != nil {
		return RBACWhoCanResult{}, err
	}
	return snap.WhoCan(q)
}

func (k *KubernetesProvider) ListServiceAccounts(ctx context.Context, ns string) ([]ServiceAccountSummary, error) {
	list, err := k.client.CoreV1().ServiceAccounts(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	if snap, err := k.loadRBACSnapshot(ctx); err == nil {
		counts = snap.saBindingCounts()
	}
	out := make([]ServiceAccountSummary, 0, len(list.Items))
	for _, sa := range list.Items {
		out = append(out, ServiceAccountSummary{
			Namespace: sa.Namespace, Name: sa.Name, Secrets: len(sa.Secrets),
			Bindings: counts[sa.Namespace+"/"+sa.Name], Age: age(sa.CreationTimestamp.Time),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
