package auth

import (
	"net/http"
	"strings"
)

// isHelmCatalogOperatorForbidden reports a write to the Helm chart catalog's
// repository list (POST /helm/repos, DELETE /helm/repos/{name}). Adding a
// repository makes the server fetch a URL of the caller's choosing, so it is
// admin-only. Catalog reads (/helm/repos GET, /helm/charts) stay open.
func isHelmCatalogOperatorForbidden(r *http.Request) bool {
	p := r.URL.Path
	if p != "/api/v1/helm/repos" && !strings.HasPrefix(p, "/api/v1/helm/repos/") {
		return false
	}
	return isMutatingMethod(r.Method)
}
