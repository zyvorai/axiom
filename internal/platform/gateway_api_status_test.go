package platform

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func cond(t, s, r string) map[string]any {
	return map[string]any{"type": t, "status": s, "reason": r}
}

func TestParseGateway(t *testing.T) {
	u := unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "eg", "namespace": "gw"},
		"spec": map[string]any{
			"gatewayClassName": "eg",
			"listeners": []any{
				map[string]any{"name": "http", "port": int64(80), "protocol": "HTTP", "hostname": "*.example.com"},
			},
		},
		"status": map[string]any{
			"conditions": []any{cond("Accepted", "True", "Accepted"), cond("Programmed", "False", "Pending")},
			"addresses":  []any{map[string]any{"type": "IPAddress", "value": "10.0.0.1"}},
			"listeners": []any{map[string]any{
				"name": "http", "attachedRoutes": int64(3),
				"conditions": []any{cond("Programmed", "True", "Programmed"), cond("Accepted", "True", "Accepted")},
			}},
		},
	}}
	g := parseGateway(u, map[string]string{"eg": "gateway.envoyproxy.io/gatewayclass-controller"})
	if g.Accepted != "True" || g.Programmed != "False" || g.Provider != "envoy" {
		t.Fatalf("%+v", g)
	}
	if len(g.Listeners) != 1 || g.Listeners[0].AttachedRoutes != 3 || g.Listeners[0].Programmed != "True" || g.Listeners[0].Port != 80 {
		t.Fatalf("listeners %+v", g.Listeners)
	}
	if len(g.Addresses) != 1 || g.Addresses[0] != "10.0.0.1" {
		t.Fatalf("addr %v", g.Addresses)
	}
}

func TestParseRoute(t *testing.T) {
	u := unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "r", "namespace": "app"},
		"spec": map[string]any{
			"hostnames":  []any{"a.example.com"},
			"parentRefs": []any{map[string]any{"name": "eg", "namespace": "gw", "sectionName": "http"}},
			"rules": []any{map[string]any{"backendRefs": []any{
				map[string]any{"name": "svc", "port": int64(8080), "weight": int64(5)},
				map[string]any{"name": "svc", "port": int64(8080)},
				map[string]any{"name": "other", "namespace": "x", "port": int64(1)},
			}}},
		},
		"status": map[string]any{"parents": []any{map[string]any{
			"parentRef":  map[string]any{"name": "eg", "namespace": "gw", "sectionName": "http"},
			"conditions": []any{cond("Accepted", "True", "Accepted"), cond("ResolvedRefs", "False", "BackendNotFound")},
		}}},
	}}
	r := parseRoute("HTTPRoute", u)
	if r.Accepted != "True" || r.ResolvedRefs != "False" {
		t.Fatalf("%+v", r)
	}
	if len(r.ParentRefs) != 1 || r.ParentRefs[0].Namespace != "gw" || r.ParentRefs[0].ResolvedRefs != "False" {
		t.Fatalf("parents %+v", r.ParentRefs)
	}
	if len(r.BackendRefs) != 2 || r.BackendRefs[0].Namespace != "app" || r.BackendRefs[0].Weight != 5 || r.BackendRefs[1].Namespace != "x" {
		t.Fatalf("backends %+v", r.BackendRefs)
	}
}

func TestParseRouteNoStatus(t *testing.T) {
	u := unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "r", "namespace": "app"},
		"spec":     map[string]any{"parentRefs": []any{map[string]any{"name": "eg"}}},
	}}
	r := parseRoute("GRPCRoute", u)
	if r.ParentRefs[0].Namespace != "app" || r.Accepted != "Unknown" {
		t.Fatalf("%+v", r)
	}
}

func TestParseReferenceGrant(t *testing.T) {
	u := unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "g", "namespace": "p"},
		"spec": map[string]any{
			"from": []any{map[string]any{"group": "gateway.networking.k8s.io", "kind": "HTTPRoute", "namespace": "shop"}},
			"to":   []any{map[string]any{"group": "", "kind": "Service", "name": "api"}},
		},
	}}
	g := parseReferenceGrant(u)
	if g.From[0] != "HTTPRoute gateway.networking.k8s.io/shop" || g.To[0] != "Service/api" {
		t.Fatalf("%+v", g)
	}
}
