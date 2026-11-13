package platform

import (
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func condList(raw []any) []GWCondition {
	out := []GWCondition{}
	for _, c := range raw {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		cc := GWCondition{}
		cc.Type, _ = m["type"].(string)
		cc.Status, _ = m["status"].(string)
		cc.Reason, _ = m["reason"].(string)
		cc.Message, _ = m["message"].(string)
		if cc.Type != "" {
			out = append(out, cc)
		}
	}
	return out
}

// condStatus returns the status of a condition type, or "" when absent.
func condStatus(cs []GWCondition, typ string) string {
	for _, c := range cs {
		if c.Type == typ {
			if c.Status == "" {
				return "Unknown"
			}
			return c.Status
		}
	}
	return ""
}

func asInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

// parseGateway converts a Gateway object. classControllers maps class name to
// controllerName (may be nil).
func parseGateway(it unstructured.Unstructured, classControllers map[string]string) GWGatewayInfo {
	class, _, _ := unstructured.NestedString(it.Object, "spec", "gatewayClassName")
	g := GWGatewayInfo{
		Namespace: it.GetNamespace(), Name: it.GetName(), Class: class,
		Controller: classControllers[class], Listeners: []GWListener{}, Addresses: []string{},
	}
	g.Provider = gatewayProvider(g.Controller, class)
	gc, _, _ := unstructured.NestedSlice(it.Object, "status", "conditions")
	g.Conditions = condList(gc)
	g.Accepted = condStatus(g.Conditions, "Accepted")
	g.Programmed = condStatus(g.Conditions, "Programmed")

	lst, _, _ := unstructured.NestedSlice(it.Object, "status", "listeners")
	type lstat struct {
		prog, acc string
		attached  int64
	}
	stats := map[string]lstat{}
	for _, l := range lst {
		m, ok := l.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		cs, _ := m["conditions"].([]any)
		cl := condList(cs)
		stats[name] = lstat{prog: condStatus(cl, "Programmed"), acc: condStatus(cl, "Accepted"), attached: asInt(m["attachedRoutes"])}
	}
	spec, _, _ := unstructured.NestedSlice(it.Object, "spec", "listeners")
	for _, l := range spec {
		m, ok := l.(map[string]any)
		if !ok {
			continue
		}
		li := GWListener{}
		li.Name, _ = m["name"].(string)
		li.Port = asInt(m["port"])
		li.Protocol, _ = m["protocol"].(string)
		li.Hostname, _ = m["hostname"].(string)
		s := stats[li.Name]
		li.Programmed, li.Accepted, li.AttachedRoutes = s.prog, s.acc, s.attached
		g.Listeners = append(g.Listeners, li)
	}
	addrs, _, _ := unstructured.NestedSlice(it.Object, "status", "addresses")
	for _, a := range addrs {
		if m, ok := a.(map[string]any); ok {
			if v, _ := m["value"].(string); v != "" {
				g.Addresses = append(g.Addresses, v)
			}
		}
	}
	return g
}

// parseRoute converts an HTTPRoute or GRPCRoute. Parent refs default to the
// route's namespace; per-parent Accepted/ResolvedRefs come from
// status.parents[].conditions.
func parseRoute(kind string, it unstructured.Unstructured) GWRouteInfo {
	r := GWRouteInfo{Kind: kind, Namespace: it.GetNamespace(), Name: it.GetName(),
		Hostnames: []string{}, ParentRefs: []GWParentRef{}, BackendRefs: []GWBackendRef{}}
	hn, _, _ := unstructured.NestedStringSlice(it.Object, "spec", "hostnames")
	if hn != nil {
		r.Hostnames = hn
	}
	type pstat struct{ acc, res string }
	pstats := map[string]pstat{}
	key := func(ns, name, section string) string { return ns + "/" + name + "/" + section }
	sp, _, _ := unstructured.NestedSlice(it.Object, "status", "parents")
	for _, p := range sp {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		pr, _ := m["parentRef"].(map[string]any)
		ns, _ := pr["namespace"].(string)
		if ns == "" {
			ns = it.GetNamespace()
		}
		name, _ := pr["name"].(string)
		sec, _ := pr["sectionName"].(string)
		cs, _ := m["conditions"].([]any)
		cl := condList(cs)
		pstats[key(ns, name, sec)] = pstat{condStatus(cl, "Accepted"), condStatus(cl, "ResolvedRefs")}
	}
	var accs, ress []string
	prs, _, _ := unstructured.NestedSlice(it.Object, "spec", "parentRefs")
	for _, p := range prs {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		ref := GWParentRef{}
		ref.Namespace, _ = m["namespace"].(string)
		if ref.Namespace == "" {
			ref.Namespace = it.GetNamespace()
		}
		ref.Name, _ = m["name"].(string)
		ref.SectionName, _ = m["sectionName"].(string)
		s, ok := pstats[key(ref.Namespace, ref.Name, ref.SectionName)]
		if !ok {
			s = pstats[key(ref.Namespace, ref.Name, "")]
		}
		ref.Accepted, ref.ResolvedRefs = s.acc, s.res
		accs = append(accs, s.acc)
		ress = append(ress, s.res)
		r.ParentRefs = append(r.ParentRefs, ref)
	}
	r.Accepted, r.ResolvedRefs = rollup(accs), rollup(ress)

	rules, _, _ := unstructured.NestedSlice(it.Object, "spec", "rules")
	seen := map[string]bool{}
	for _, rule := range rules {
		rm, ok := rule.(map[string]any)
		if !ok {
			continue
		}
		bs, _ := rm["backendRefs"].([]any)
		for _, b := range bs {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			br := GWBackendRef{Kind: "Service"}
			if k, _ := bm["kind"].(string); k != "" {
				br.Kind = k
			}
			br.Namespace, _ = bm["namespace"].(string)
			if br.Namespace == "" {
				br.Namespace = it.GetNamespace()
			}
			br.Name, _ = bm["name"].(string)
			br.Port = asInt(bm["port"])
			br.Weight = asInt(bm["weight"])
			id := fmt.Sprintf("%s/%s/%s/%d", br.Kind, br.Namespace, br.Name, br.Port)
			if br.Name == "" || seen[id] {
				continue
			}
			seen[id] = true
			r.BackendRefs = append(r.BackendRefs, br)
		}
	}
	return r
}

// rollup: "" when no data, "False" if any False, "Unknown" if any is not
// explicitly True, else "True".
func rollup(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	res := "True"
	for _, v := range vals {
		switch v {
		case "False":
			return "False"
		case "True":
		default:
			res = "Unknown"
		}
	}
	return res
}

func parseReferenceGrant(it unstructured.Unstructured) GWReferenceGrant {
	g := GWReferenceGrant{Namespace: it.GetNamespace(), Name: it.GetName(), From: []string{}, To: []string{}}
	from, _, _ := unstructured.NestedSlice(it.Object, "spec", "from")
	for _, f := range from {
		if m, ok := f.(map[string]any); ok {
			k, _ := m["kind"].(string)
			grp, _ := m["group"].(string)
			ns, _ := m["namespace"].(string)
			g.From = append(g.From, fmt.Sprintf("%s %s/%s", k, grp, ns))
		}
	}
	to, _, _ := unstructured.NestedSlice(it.Object, "spec", "to")
	for _, t := range to {
		if m, ok := t.(map[string]any); ok {
			k, _ := m["kind"].(string)
			if n, _ := m["name"].(string); n != "" {
				k += "/" + n
			}
			g.To = append(g.To, k)
		}
	}
	return g
}

func sortGatewayAPI(inv *GatewayAPIInventory) {
	sort.Slice(inv.Gateways, func(i, j int) bool {
		a, b := inv.Gateways[i], inv.Gateways[j]
		return a.Namespace+"/"+a.Name < b.Namespace+"/"+b.Name
	})
	sort.Slice(inv.Routes, func(i, j int) bool {
		a, b := inv.Routes[i], inv.Routes[j]
		return a.Kind+a.Namespace+"/"+a.Name < b.Kind+b.Namespace+"/"+b.Name
	})
	sort.Slice(inv.ReferenceGrants, func(i, j int) bool {
		a, b := inv.ReferenceGrants[i], inv.ReferenceGrants[j]
		return a.Namespace+"/"+a.Name < b.Namespace+"/"+b.Name
	})
}
