//go:build kubernetes

package platform

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const gatewayAPIGroup = "gateway.networking.k8s.io"

// listGWResource tries each version in turn; ok is false when none worked.
func (k *KubernetesProvider) listGWResource(ctx context.Context, resource string, versions []string) ([]unstructured.Unstructured, bool) {
	for _, ver := range versions {
		gvr := schema.GroupVersionResource{Group: gatewayAPIGroup, Version: ver, Resource: resource}
		list, err := k.dynamic.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 500})
		if err == nil && list != nil {
			return list.Items, true
		}
	}
	return nil, false
}

func (k *KubernetesProvider) ListGatewayAPI(ctx context.Context) (GatewayAPIInventory, error) {
	inv := emptyGatewayAPI("")
	if k.dynamic == nil {
		inv.Note = "Gateway API view not available"
		return inv, nil
	}
	versions := []string{"v1", "v1beta1"}
	controllers := map[string]string{}
	for _, ver := range versions {
		gvr := schema.GroupVersionResource{Group: gatewayAPIGroup, Version: ver, Resource: "gatewayclasses"}
		if list, err := k.dynamic.Resource(gvr).List(ctx, metav1.ListOptions{}); err == nil {
			for _, it := range list.Items {
				c, _, _ := unstructured.NestedString(it.Object, "spec", "controllerName")
				controllers[it.GetName()] = c
			}
			break
		}
	}
	gws, ok1 := k.listGWResource(ctx, "gateways", versions)
	for _, it := range gws {
		g := parseGateway(it, controllers)
		g.Age = age(it.GetCreationTimestamp().Time)
		inv.Gateways = append(inv.Gateways, g)
	}
	hr, ok2 := k.listGWResource(ctx, "httproutes", versions)
	for _, it := range hr {
		r := parseRoute("HTTPRoute", it)
		r.Age = age(it.GetCreationTimestamp().Time)
		inv.Routes = append(inv.Routes, r)
	}
	gr, ok3 := k.listGWResource(ctx, "grpcroutes", versions)
	for _, it := range gr {
		r := parseRoute("GRPCRoute", it)
		r.Age = age(it.GetCreationTimestamp().Time)
		inv.Routes = append(inv.Routes, r)
	}
	rg, ok4 := k.listGWResource(ctx, "referencegrants", []string{"v1", "v1beta1", "v1alpha2"})
	for _, it := range rg {
		g := parseReferenceGrant(it)
		g.Age = age(it.GetCreationTimestamp().Time)
		inv.ReferenceGrants = append(inv.ReferenceGrants, g)
	}
	inv.Available = ok1 || ok2 || ok3 || ok4
	if !inv.Available {
		inv.Note = "gateway.networking.k8s.io CRDs not installed"
	}
	sortGatewayAPI(&inv)
	return inv, nil
}
