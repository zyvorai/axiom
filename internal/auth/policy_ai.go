package auth

import "net/http"

// isReadOnlyPost reports POST routes that only read cluster state and must stay
// available to the read-only viewer role. Today that is the AI explain route:
// it never mutates anything, and whether it may run at all is gated by the
// admin-controlled "enabled" setting plus the same namespace authorization as
// the object reads it summarizes.
func isReadOnlyPost(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == "/api/v1/ai/explain"
}
