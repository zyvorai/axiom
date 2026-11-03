package platform

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Read-only RBAC viewer. The engine below is pure (no Kubernetes imports): the
// Kubernetes provider converts Roles/ClusterRoles/bindings/ServiceAccounts into
// an rbacSnapshot and the demo provider builds a fixed one, so both share one
// implementation of "what can this subject do" and "who can do this".
//
// Deliberately read-only: Axiom does not write ClusterRoles (see docs/GAPS.md).

const (
	rbacWhoCanMax = 500 // cap on who-can rows returned

	subjectUser           = "User"
	subjectGroup          = "Group"
	subjectServiceAccount = "ServiceAccount"
)

type rbacRoleDef struct {
	Namespace string // empty for ClusterRole
	Name      string
	Rules     []RBACRuleView
}

type rbacBindingDef struct {
	Namespace string // empty for ClusterRoleBinding
	Name      string
	Cluster   bool
	RoleKind  string // Role | ClusterRole
	RoleName  string
	Subjects  []RBACSubject
}

func (b rbacBindingDef) label() string {
	if b.Cluster {
		return "ClusterRoleBinding/" + b.Name
	}
	return "RoleBinding/" + b.Namespace + "/" + b.Name
}

type rbacSnapshot struct {
	ClusterRoles    map[string]rbacRoleDef // by name
	Roles           map[string]rbacRoleDef // by namespace/name
	Bindings        []rbacBindingDef
	ServiceAccounts []RBACSubject
}

var rbacTokenRE = regexp.MustCompile(`^[A-Za-z0-9*._:/-]{1,253}$`)

// normalizeSubjectKind canonicalises the {kind} path segment.
func normalizeSubjectKind(k string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "user", "users":
		return subjectUser, nil
	case "group", "groups":
		return subjectGroup, nil
	case "serviceaccount", "serviceaccounts", "sa":
		return subjectServiceAccount, nil
	}
	return "", NewStatusError(400, fmt.Errorf("invalid subject kind %q: must be User, Group or ServiceAccount", k))
}

func (s rbacSnapshot) role(b rbacBindingDef) (rbacRoleDef, bool) {
	if b.RoleKind == "ClusterRole" {
		r, ok := s.ClusterRoles[b.RoleName]
		return r, ok
	}
	r, ok := s.Roles[b.Namespace+"/"+b.RoleName]
	return r, ok
}

func roleRefLabel(b rbacBindingDef) string { return b.RoleKind + "/" + b.RoleName }

// subjectMatch reports whether a binding subject applies to target, and via
// which implicit group when the match is not a direct name match.
func subjectMatch(sub RBACSubject, bindingNS string, target RBACSubject) (via string, ok bool) {
	switch sub.Kind {
	case subjectGroup:
		switch {
		case target.Kind == subjectGroup && sub.Name == target.Name:
			return "", true
		case sub.Name == "system:authenticated" && (target.Kind == subjectUser || target.Kind == subjectServiceAccount):
			return sub.Name, true
		case target.Kind == subjectServiceAccount && sub.Name == "system:serviceaccounts":
			return sub.Name, true
		case target.Kind == subjectServiceAccount && sub.Name == "system:serviceaccounts:"+target.Namespace:
			return sub.Name, true
		}
	case subjectUser:
		if target.Kind == subjectUser && sub.Name == target.Name {
			return "", true
		}
	case subjectServiceAccount:
		ns := sub.Namespace
		if ns == "" {
			ns = bindingNS
		}
		if target.Kind == subjectServiceAccount && sub.Name == target.Name && ns == target.Namespace {
			return "", true
		}
	}
	return "", false
}

// grantsAdmin reports whether rules include a */*/* rule (cluster-admin shape).
func grantsAdmin(rules []RBACRuleView) bool {
	for _, r := range rules {
		if len(r.ResourceNames) == 0 && hasStar(r.Verbs) && hasStar(r.Resources) && hasStar(r.APIGroups) {
			return true
		}
	}
	return false
}

func hasStar(l []string) bool {
	for _, v := range l {
		if v == "*" {
			return true
		}
	}
	return false
}

func normSubject(s RBACSubject, bindingNS string) RBACSubject {
	if s.Kind == subjectServiceAccount && s.Namespace == "" {
		s.Namespace = bindingNS
	}
	if s.Kind != subjectServiceAccount {
		s.Namespace = ""
	}
	return s
}

func subjectKeyOf(s RBACSubject) string { return s.Kind + "\x00" + s.Namespace + "\x00" + s.Name }

// Subjects lists every distinct subject named by a binding, plus every known
// ServiceAccount, with a direct-bindings summary.
func (s rbacSnapshot) Subjects() []RBACSubjectSummary {
	type acc struct {
		sum   RBACSubjectSummary
		roles map[string]bool
	}
	m := map[string]*acc{}
	get := func(sub RBACSubject) *acc {
		k := subjectKeyOf(sub)
		a := m[k]
		if a == nil {
			a = &acc{sum: RBACSubjectSummary{Kind: sub.Kind, Name: sub.Name, Namespace: sub.Namespace, Roles: []string{}}, roles: map[string]bool{}}
			m[k] = a
		}
		return a
	}
	for _, sa := range s.ServiceAccounts {
		get(normSubject(sa, sa.Namespace))
	}
	for _, b := range s.Bindings {
		role, found := s.role(b)
		for _, raw := range b.Subjects {
			sub := normSubject(raw, b.Namespace)
			if sub.Kind != subjectUser && sub.Kind != subjectGroup && sub.Kind != subjectServiceAccount {
				continue
			}
			a := get(sub)
			a.sum.Bindings++
			if b.Cluster {
				a.sum.ClusterWide = true
			}
			a.roles[roleRefLabel(b)] = true
			if found && b.Cluster && grantsAdmin(role.Rules) {
				a.sum.ClusterAdmin = true
			}
		}
	}
	out := make([]RBACSubjectSummary, 0, len(m))
	for _, a := range m {
		for r := range a.roles {
			a.sum.Roles = append(a.sum.Roles, r)
		}
		sort.Strings(a.sum.Roles)
		out = append(out, a.sum)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Subject resolves the effective permissions of one subject. A User or Group
// has no namespace; a ServiceAccount requires one.
func (s rbacSnapshot) Subject(kind, name, namespace string) (RBACSubjectDetail, error) {
	k, err := normalizeSubjectKind(kind)
	if err != nil {
		return RBACSubjectDetail{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 253 || strings.ContainsAny(name, "\x00\r\n") || (k != subjectUser && !rbacTokenRE.MatchString(name)) {
		return RBACSubjectDetail{}, NewStatusError(400, fmt.Errorf("invalid subject name %q", name))
	}
	if k == subjectServiceAccount {
		if namespace == "" || namespace == "all" {
			return RBACSubjectDetail{}, NewStatusError(400, fmt.Errorf("namespace is required for a ServiceAccount"))
		}
	} else {
		namespace = ""
	}
	target := RBACSubject{Kind: k, Name: name, Namespace: namespace}

	detail := RBACSubjectDetail{Cluster: []RBACGrant{}, Namespaced: []RBACGrant{}}
	var summary RBACSubjectSummary
	for _, cand := range s.Subjects() {
		if cand.Kind == target.Kind && cand.Name == target.Name && cand.Namespace == target.Namespace {
			summary = cand
		}
	}
	known := summary.Name != ""
	if !known {
		summary = RBACSubjectSummary{Kind: k, Name: name, Namespace: namespace, Roles: []string{}}
	}

	for _, b := range s.Bindings {
		for _, sub := range b.Subjects {
			via, ok := subjectMatch(sub, b.Namespace, target)
			if !ok {
				continue
			}
			role, found := s.role(b)
			g := RBACGrant{
				Binding: b.label(), RoleRef: roleRefLabel(b), Via: via, Dangling: !found,
				Rules: append([]RBACRuleView{}, role.Rules...),
			}
			g.RuleSummary = fmt.Sprintf("%d rule(s)", len(g.Rules))
			if b.Cluster {
				g.Scope = "cluster"
				detail.Cluster = append(detail.Cluster, g)
				if found && grantsAdmin(role.Rules) {
					summary.ClusterAdmin = true
				}
			} else {
				g.Scope, g.Namespace = "namespace", b.Namespace
				detail.Namespaced = append(detail.Namespaced, g)
			}
			break // one grant per binding even if several subjects match
		}
	}
	byBinding := func(gs []RBACGrant) {
		sort.Slice(gs, func(i, j int) bool {
			if gs[i].Namespace != gs[j].Namespace {
				return gs[i].Namespace < gs[j].Namespace
			}
			return gs[i].Binding < gs[j].Binding
		})
	}
	byBinding(detail.Cluster)
	byBinding(detail.Namespaced)
	summary.ClusterWide = len(detail.Cluster) > 0
	summary.Bindings = len(detail.Cluster) + len(detail.Namespaced)
	detail.Subject = summary

	switch k {
	case subjectServiceAccount:
		detail.Notes = append(detail.Notes, "Includes bindings granted through the implicit groups system:serviceaccounts, system:serviceaccounts:"+namespace+" and system:authenticated.")
	case subjectUser:
		detail.Notes = append(detail.Notes, "Group memberships asserted by your identity provider are not visible to Kubernetes; only bindings naming this user (and system:authenticated) are shown.")
	default:
		detail.Notes = append(detail.Notes, "Shows bindings that name this group. Which users belong to it is decided by your identity provider.")
	}
	if !known && len(detail.Cluster)+len(detail.Namespaced) == 0 {
		return detail, notFoundf("subject %s %q has no bindings", k, name)
	}
	return detail, nil
}

// ruleAllows reports whether a single rule permits verb on resource.
func ruleAllows(r RBACRuleView, q RBACWhoCanQuery) bool {
	if !matchesAny(r.Verbs, q.Verb) {
		return false
	}
	if q.Group != "" {
		want := q.Group
		if want == "core" {
			want = "" // the core API group is spelled "" in a PolicyRule
		}
		if !matchesAny(r.APIGroups, want) {
			return false
		}
	}
	if !resourceMatches(r.Resources, q.Resource) {
		return false
	}
	if len(r.ResourceNames) > 0 {
		// A rule restricted to named objects only answers a question about
		// one of those objects.
		if q.Name == "" {
			return false
		}
		return matchesAny(r.ResourceNames, q.Name)
	}
	return true
}

func matchesAny(list []string, want string) bool {
	for _, v := range list {
		if v == "*" || v == want {
			return true
		}
	}
	return false
}

// resourceMatches implements RBAC resource matching: exact, "*", "pods/*"
// (every subresource) and "*/scale" (a subresource of any resource).
func resourceMatches(rules []string, res string) bool {
	for _, r := range rules {
		switch {
		case r == "*" || r == res:
			return true
		case strings.HasSuffix(r, "/*") && strings.HasPrefix(res, strings.TrimSuffix(r, "*")):
			return true
		case strings.HasPrefix(r, "*/") && strings.HasSuffix(res, strings.TrimPrefix(r, "*")):
			return true
		}
	}
	return false
}

// WhoCan lists every subject allowed to perform the query via any binding.
func (s rbacSnapshot) WhoCan(q RBACWhoCanQuery) (RBACWhoCanResult, error) {
	q.Verb = strings.ToLower(strings.TrimSpace(q.Verb))
	q.Resource = strings.ToLower(strings.TrimSpace(q.Resource))
	q.Group = strings.TrimSpace(q.Group)
	q.Name = strings.TrimSpace(q.Name)
	q.Namespace = strings.TrimSpace(q.Namespace)
	if q.Verb == "" || q.Resource == "" {
		return RBACWhoCanResult{}, NewStatusError(400, fmt.Errorf("verb and resource are required"))
	}
	for _, v := range []string{q.Verb, q.Resource, q.Group, q.Name, q.Namespace} {
		if v != "" && !rbacTokenRE.MatchString(v) {
			return RBACWhoCanResult{}, NewStatusError(400, fmt.Errorf("invalid query value %q", v))
		}
	}
	if q.Namespace == "all" {
		q.Namespace = ""
	}
	res := RBACWhoCanResult{Query: q, Allowed: []RBACWhoCanEntry{}}
	seen := map[string]bool{}
	for _, b := range s.Bindings {
		if !b.Cluster && q.Namespace != "" && b.Namespace != q.Namespace {
			continue
		}
		role, found := s.role(b)
		if !found {
			continue
		}
		allowed := false
		for _, rule := range role.Rules {
			if ruleAllows(rule, q) {
				allowed = true
				break
			}
		}
		if !allowed {
			continue
		}
		for _, raw := range b.Subjects {
			sub := normSubject(raw, b.Namespace)
			if sub.Kind != subjectUser && sub.Kind != subjectGroup && sub.Kind != subjectServiceAccount {
				continue
			}
			k := subjectKeyOf(sub) + "\x00" + b.label()
			if seen[k] {
				continue
			}
			seen[k] = true
			e := RBACWhoCanEntry{Subject: sub, Binding: b.label(), RoleRef: roleRefLabel(b)}
			if b.Cluster {
				e.Scope = "cluster"
			} else {
				e.Scope, e.Namespace = "namespace", b.Namespace
			}
			res.Allowed = append(res.Allowed, e)
		}
	}
	sort.Slice(res.Allowed, func(i, j int) bool {
		a, b := res.Allowed[i], res.Allowed[j]
		if a.Scope != b.Scope {
			return a.Scope < b.Scope // cluster before namespace
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Subject.Kind != b.Subject.Kind {
			return a.Subject.Kind < b.Subject.Kind
		}
		if a.Subject.Namespace != b.Subject.Namespace {
			return a.Subject.Namespace < b.Subject.Namespace
		}
		if a.Subject.Name != b.Subject.Name {
			return a.Subject.Name < b.Subject.Name
		}
		return a.Binding < b.Binding
	})
	if len(res.Allowed) > rbacWhoCanMax {
		res.Allowed = res.Allowed[:rbacWhoCanMax]
		res.Truncated = true
	}
	return res, nil
}

// bindingCounts returns how many bindings name each ServiceAccount
// (namespace/name), used by the ServiceAccount list.
func (s rbacSnapshot) saBindingCounts() map[string]int {
	out := map[string]int{}
	for _, b := range s.Bindings {
		seen := map[string]bool{}
		for _, raw := range b.Subjects {
			sub := normSubject(raw, b.Namespace)
			if sub.Kind != subjectServiceAccount {
				continue
			}
			k := sub.Namespace + "/" + sub.Name
			if !seen[k] {
				seen[k] = true
				out[k]++
			}
		}
	}
	return out
}
