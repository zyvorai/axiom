//go:build kubernetes

package platform

import (
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// httpStatusFromKnownError classifies real Kubernetes API errors into the
// matching HTTP status. apierrors.Is* walks the error chain via errors.As,
// so this still finds the underlying *apierrors.StatusError even through
// fmt.Errorf("...: %w", err) wrapping — no call site needs to change.
func httpStatusFromKnownError(err error) (int, bool) {
	switch {
	case apierrors.IsNotFound(err):
		return http.StatusNotFound, true
	case apierrors.IsForbidden(err):
		return http.StatusForbidden, true
	case apierrors.IsUnauthorized(err):
		return http.StatusUnauthorized, true
	case apierrors.IsConflict(err), apierrors.IsAlreadyExists(err):
		return http.StatusConflict, true
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		return http.StatusBadRequest, true
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return http.StatusGatewayTimeout, true
	case apierrors.IsTooManyRequests(err):
		return http.StatusTooManyRequests, true
	}
	// Hand-written validation errors ("X is required", "unsupported kind") are not
	// apierrors; without this they read as HTTP 500 — the same request the demo build
	// answers with 400.
	return httpStatusFromMessage(err)
}
