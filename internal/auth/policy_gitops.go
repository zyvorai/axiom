package auth

import (
	"net/http"
	"strings"
)

// isGitOpsCreate reports whether the request creates (or previews creating) an
// Argo CD / Flux application: POST /api/v1/gitops and /api/v1/gitops/preview,
// or deletes one (DELETE /api/v1/gitops/{kind}/{ns}/{name}).
// That deploys arbitrary workloads from a repository — the same privilege as a
// Helm install — so it is admin-only. Listing (GET /api/v1/gitops) and the
// sync action (POST /api/v1/gitops/{kind}/{ns}/{name}/sync) stay operator-usable.
//
// Editing (POST {kind}/{ns}/{name}/edit/preview, PUT {kind}/{ns}/{name}) falls
// under "every other write" below; GET {kind}/{ns}/{name}/edit is a read like
// the delete preview. Repository credentials are admin-only for every method
// (isGitOpsCredentials), including the listing.
func isGitOpsCreate(r *http.Request) bool {
	p := strings.TrimSuffix(r.URL.Path, "/")
	if p == "/api/v1/gitops/preview" || isGitOpsCredentials(r) {
		return true
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return false // listing and GET .../delete-preview are reads
	}
	if p == "/api/v1/gitops" {
		return true
	}
	// Every other write under /api/v1/gitops/ — DELETE {kind}/{ns}/{name}
	// (which can cascade-delete an application's workloads) and anything
	// added later — is admin-only, except exactly POST {kind}/{ns}/{name}/sync.
	if rest, ok := strings.CutPrefix(p, "/api/v1/gitops/"); ok {
		segs := strings.Split(rest, "/")
		isSync := r.Method == http.MethodPost && len(segs) == 4 && segs[3] == "sync"
		return !isSync
	}
	return false
}

// isGitOpsCredentials reports whether the request touches GitOps repository
// credentials (/api/v1/gitops/credentials and below). Every method, GET
// included, is admin-only: even the listing reveals which repositories are
// used and with what kind of access. The guard denies viewers and operators.
func isGitOpsCredentials(r *http.Request) bool {
	p := strings.TrimSuffix(r.URL.Path, "/")
	return p == "/api/v1/gitops/credentials" || strings.HasPrefix(p, "/api/v1/gitops/credentials/")
}
