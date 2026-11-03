package auth

import (
	"net/http"
	"strings"
)

// isRBACViewRead reports whether the request is one of the read-only RBAC
// viewer routes (who is bound to what, and who can do X). They enumerate the
// cluster's whole access graph, so viewers are denied and namespace-restricted
// identities are denied by the "/api/v1/rbac" restrictedSensitivePrefixes
// entry. Operators may read them (but, as before, still cannot write RBAC).
func isRBACViewRead(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	p := r.URL.Path
	return p == "/api/v1/rbac/subjects" || strings.HasPrefix(p, "/api/v1/rbac/subjects/") || p == "/api/v1/rbac/who-can"
}

// Routes added by the security views whose first path segment after the prefix
// is the namespace. Registered here (not in the pathNamespacePrefixes literal)
// to keep the shared map's diff empty; the guard treats them identically.
func init() {
	pathNamespacePrefixes["/api/v1/networking/policies"] = true // /{namespace}/analysis
	pathNamespacePrefixes["/api/v1/security/reports"] = true    // /{namespace}/{kind}/{name}
}
