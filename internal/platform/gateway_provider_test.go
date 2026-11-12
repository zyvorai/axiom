package platform

import "testing"

func TestGatewayProvider(t *testing.T) {
	cases := []struct {
		controller, class, want string
	}{
		{"gateway.envoyproxy.io/gatewayclass-controller", "eg", "envoy"},
		{"", "eg", "envoy"},
		{"", "envoy-gateway", "envoy"},
		{"k8s.io/ingress-nginx", "nginx", "nginx"},
		{"istio.io/gateway-controller", "istio", "istio"},
		{"", "", ""},
		{"example.com/custom", "custom", "other"},
	}
	for _, c := range cases {
		if got := gatewayProvider(c.controller, c.class); got != c.want {
			t.Errorf("gatewayProvider(%q,%q)=%q want %q", c.controller, c.class, got, c.want)
		}
	}
}
