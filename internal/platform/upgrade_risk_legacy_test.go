package platform

import "testing"

func TestIngressNginxFindings(t *testing.T) {
	if got := ingressNginxFindings([]IngressSummary{{Namespace: "a", Name: "x", Class: "traefik"}}); got != nil {
		t.Fatalf("expected none, got %v", got)
	}
	got := ingressNginxFindings([]IngressSummary{
		{Namespace: "b", Name: "y", Class: "nginx"},
		{Namespace: "a", Name: "x", Class: "NGINX"},
	})
	if len(got) != 1 || got[0].Code != "ingress-nginx-retiring" || got[0].Severity != "warning" {
		t.Fatalf("unexpected: %+v", got)
	}
}
