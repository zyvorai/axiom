package auth

import (
	"net/http"
	"strings"
)

// isAdminOnlyRead lists GET routes only an administrator may read, whatever
// the method-based rules below would otherwise allow (operators and viewers
// may read most of the API).
func isAdminOnlyRead(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	// Which directory (LDAP) accounts have a second factor: the Users page's
	// account-security column. No secret material, but it is account admin.
	return strings.TrimSuffix(r.URL.Path, "/") == "/api/v1/users/external-mfa"
}
