package auth

import (
	"net/http"
	"strings"
)

// isVeleroRestoreWrite reports whether the request previews or creates a
// Velero Restore (anything but a read under /api/v1/velero/restores). A
// restore can overwrite live objects, so it is admin-only; the preview is a
// POST and is gated the same way.
func isVeleroRestoreWrite(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return false
	}
	p := strings.TrimSuffix(r.URL.Path, "/")
	return p == "/api/v1/velero/restores" || strings.HasPrefix(p, "/api/v1/velero/restores/")
}
