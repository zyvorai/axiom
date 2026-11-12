package platform

import "strings"

// gatewayProvider classifies a GatewayClass controller / class name for the UI
// (Envoy Gateway is the recommended replacement for retiring Ingress NGINX).
func gatewayProvider(controller, className string) string {
	c := strings.ToLower(strings.TrimSpace(controller))
	class := strings.ToLower(strings.TrimSpace(className))
	s := strings.TrimSpace(c + " " + class)
	switch {
	case s == "":
		return ""
	case strings.Contains(s, "envoyproxy.io") || strings.Contains(s, "envoy") ||
		class == "eg" || class == "envoy-gateway":
		return "envoy"
	case strings.Contains(s, "nginx") || strings.Contains(s, "ngf") || strings.Contains(s, "nginx.org"):
		return "nginx"
	case strings.Contains(s, "istio"):
		return "istio"
	case strings.Contains(s, "traefik"):
		return "traefik"
	case strings.Contains(s, "cilium"):
		return "cilium"
	default:
		return "other"
	}
}
