package platform

import "context"

// GWCondition is one status condition surfaced in the Gateway API view.
type GWCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"` // True | False | Unknown
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// GWListener is one Gateway listener.
type GWListener struct {
	Name           string `json:"name"`
	Port           int64  `json:"port"`
	Protocol       string `json:"protocol"`
	Hostname       string `json:"hostname,omitempty"`
	Programmed     string `json:"programmed,omitempty"`
	Accepted       string `json:"accepted,omitempty"`
	AttachedRoutes int64  `json:"attachedRoutes"`
}

// GWGatewayInfo is one Gateway.
type GWGatewayInfo struct {
	Namespace  string        `json:"namespace"`
	Name       string        `json:"name"`
	Class      string        `json:"class"`
	Controller string        `json:"controller,omitempty"`
	Provider   string        `json:"provider,omitempty"`
	Listeners  []GWListener  `json:"listeners"`
	Addresses  []string      `json:"addresses"`
	Accepted   string        `json:"accepted"`
	Programmed string        `json:"programmed"`
	Conditions []GWCondition `json:"conditions"`
	Age        string        `json:"age,omitempty"`
}

// GWParentRef points at a parent Gateway (namespace is resolved).
type GWParentRef struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	SectionName  string `json:"sectionName,omitempty"`
	Accepted     string `json:"accepted,omitempty"`
	ResolvedRefs string `json:"resolvedRefs,omitempty"`
}

// GWBackendRef is one route backend.
type GWBackendRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Port      int64  `json:"port,omitempty"`
	Weight    int64  `json:"weight,omitempty"`
}

// GWRouteInfo is an HTTPRoute or GRPCRoute.
type GWRouteInfo struct {
	Kind         string         `json:"kind"` // HTTPRoute | GRPCRoute
	Namespace    string         `json:"namespace"`
	Name         string         `json:"name"`
	Hostnames    []string       `json:"hostnames"`
	ParentRefs   []GWParentRef  `json:"parentRefs"`
	BackendRefs  []GWBackendRef `json:"backendRefs"`
	Accepted     string         `json:"accepted"`
	ResolvedRefs string         `json:"resolvedRefs"`
	Age          string         `json:"age,omitempty"`
}

// GWReferenceGrant is a ReferenceGrant (cross-namespace reference permission).
type GWReferenceGrant struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	From      []string `json:"from"` // "Kind group/namespace"
	To        []string `json:"to"`   // "Kind[/name]"
	Age       string   `json:"age,omitempty"`
}

// GatewayAPIInventory is GET /api/v1/gateway-api.
type GatewayAPIInventory struct {
	Available       bool               `json:"available"`
	Note            string             `json:"note,omitempty"`
	Gateways        []GWGatewayInfo    `json:"gateways"`
	Routes          []GWRouteInfo      `json:"routes"`
	ReferenceGrants []GWReferenceGrant `json:"referenceGrants"`
}

func emptyGatewayAPI(note string) GatewayAPIInventory {
	return GatewayAPIInventory{Note: note, Gateways: []GWGatewayInfo{}, Routes: []GWRouteInfo{}, ReferenceGrants: []GWReferenceGrant{}}
}

// ListGatewayAPI returns a read-only Gateway API deep view.
func ListGatewayAPI(ctx context.Context, p Provider) (GatewayAPIInventory, error) {
	if l, ok := p.(interface {
		ListGatewayAPI(context.Context) (GatewayAPIInventory, error)
	}); ok {
		return l.ListGatewayAPI(ctx)
	}
	return emptyGatewayAPI("Gateway API view not available on this provider"), nil
}

func (d *DemoProvider) ListGatewayAPI(ctx context.Context) (GatewayAPIInventory, error) {
	return GatewayAPIInventory{
		Available: true,
		Note:      "Demo Gateway API inventory (simulated Envoy Gateway)",
		Gateways: []GWGatewayInfo{{
			Namespace: "envoy-gateway-system", Name: "eg", Class: "eg",
			Controller: "gateway.envoyproxy.io/gatewayclass-controller", Provider: "envoy",
			Listeners: []GWListener{
				{Name: "http", Port: 80, Protocol: "HTTP", Hostname: "*.example.com", Programmed: "True", Accepted: "True", AttachedRoutes: 2},
				{Name: "https", Port: 443, Protocol: "HTTPS", Hostname: "shop.example.com", Programmed: "True", Accepted: "True", AttachedRoutes: 1},
			},
			Addresses: []string{"203.0.113.10"}, Accepted: "True", Programmed: "True",
			Conditions: []GWCondition{
				{Type: "Accepted", Status: "True", Reason: "Accepted"},
				{Type: "Programmed", Status: "True", Reason: "Programmed"},
			},
			Age: "12d",
		}},
		Routes: []GWRouteInfo{
			{Kind: "HTTPRoute", Namespace: "shop", Name: "storefront", Hostnames: []string{"shop.example.com"},
				ParentRefs:  []GWParentRef{{Namespace: "envoy-gateway-system", Name: "eg", SectionName: "https", Accepted: "True", ResolvedRefs: "True"}},
				BackendRefs: []GWBackendRef{{Kind: "Service", Namespace: "shop", Name: "storefront", Port: 8080, Weight: 1}},
				Accepted:    "True", ResolvedRefs: "True", Age: "9d"},
			{Kind: "HTTPRoute", Namespace: "shop", Name: "payments", Hostnames: []string{"pay.example.com"},
				ParentRefs:  []GWParentRef{{Namespace: "envoy-gateway-system", Name: "eg", Accepted: "True", ResolvedRefs: "False"}},
				BackendRefs: []GWBackendRef{{Kind: "Service", Namespace: "payments", Name: "payments-api", Port: 9000, Weight: 1}},
				Accepted:    "True", ResolvedRefs: "False", Age: "3d"},
			{Kind: "GRPCRoute", Namespace: "shop", Name: "inventory", Hostnames: []string{"grpc.example.com"},
				ParentRefs:  []GWParentRef{{Namespace: "envoy-gateway-system", Name: "eg", SectionName: "http", Accepted: "True", ResolvedRefs: "True"}},
				BackendRefs: []GWBackendRef{{Kind: "Service", Namespace: "shop", Name: "inventory", Port: 50051, Weight: 1}},
				Accepted:    "True", ResolvedRefs: "True", Age: "5d"},
		},
		ReferenceGrants: []GWReferenceGrant{{
			Namespace: "payments", Name: "allow-shop-routes",
			From: []string{"HTTPRoute gateway.networking.k8s.io/shop"}, To: []string{"Service"}, Age: "3d",
		}},
	}, nil
}
