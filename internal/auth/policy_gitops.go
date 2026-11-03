package auth

import (
	"net/http"
	"strings"
)

// isGitOpsCreate reports whether the request creates (or previews creating) an
// Argo CD / Flux application: POST /api/v1/gitops and /api/v1/gitops/preview.
// That deploys arbitrary workloads from a repository — the same privilege as a
// Helm install — so it is admin-only. Listing (GET /api/v1/gitops) and the
// sync action (POST /api/v1/gitops/{kind}/{ns}/{name}/sync) stay operator-usable.
func isGitOpsCreate(r *http.Request) bool {
	p := strings.TrimSuffix(r.URL.Path, "/")
	if p == "/api/v1/gitops/preview" {
		return true
	}
	return p == "/api/v1/gitops" && r.Method != http.MethodGet && r.Method != http.MethodHead
}
