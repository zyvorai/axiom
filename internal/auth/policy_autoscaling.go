package auth

import (
	"context"
	"net/http"
)

// bodyNamespaceRoutes are POST routes whose target namespace is in the JSON
// body, not the URL: VPA / KEDA ScaledObject create and preview. The guard
// cannot read the body, so for namespace-restricted identities it lets these
// through (they are still subject to the role checks: viewers are refused as
// for any write) and the handler MUST call NamespaceAllowed on the body's
// namespace before doing anything else.
var bodyNamespaceRoutes = map[string]bool{
	"/api/v1/vpas":                  true,
	"/api/v1/vpas/preview":          true,
	"/api/v1/scaledobjects":         true,
	"/api/v1/scaledobjects/preview": true,
}

func isBodyNamespaceRoute(r *http.Request) bool {
	return r.Method == http.MethodPost && bodyNamespaceRoutes[r.URL.Path]
}

// NamespaceAllowed reports whether the authenticated caller may act in ns.
// Unrestricted identities may act anywhere; a namespace-restricted one only in
// its own namespaces (never "" or "all"). Without an identity (no guard
// installed, i.e. authentication disabled) there are no restrictions to apply.
func NamespaceAllowed(ctx context.Context, ns string) bool {
	id, ok := IdentityFromContext(ctx)
	if !ok || !id.namespaceRestricted() {
		return true
	}
	return ns != "" && ns != "all" && id.allowsNamespace(ns)
}
