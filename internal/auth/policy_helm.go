package auth

import (
	"net/http"
	"strings"
)

// isHelmCatalogOperatorForbidden reports a write to the Helm chart catalog's
// repository list (POST /helm/repos, DELETE /helm/repos/{name}). Adding a
// repository makes the server fetch a URL of the caller's choosing, so it is
// admin-only. Catalog reads (/helm/repos GET, /helm/charts) stay open.
// Registry credentials are admin-only for every method (isHelmRegistryCreds).
func isHelmCatalogOperatorForbidden(r *http.Request) bool {
	if isHelmRegistryCreds(r) {
		return true
	}
	p := r.URL.Path
	if p != "/api/v1/helm/repos" && !strings.HasPrefix(p, "/api/v1/helm/repos/") {
		return false
	}
	return isMutatingMethod(r.Method)
}

// isHelmRegistryCreds reports whether the request touches OCI registry
// credentials (/api/v1/helm/registry-credentials and below). Every method,
// GET included, is admin-only: even the listing reveals which registries are
// used and with which accounts. The guard denies viewers and operators.
func isHelmRegistryCreds(r *http.Request) bool {
	p := strings.TrimSuffix(r.URL.Path, "/")
	return p == "/api/v1/helm/registry-credentials" || strings.HasPrefix(p, "/api/v1/helm/registry-credentials/")
}
