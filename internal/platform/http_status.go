package platform

import (
	"errors"
	"fmt"
	"net/http"
)

// StatusError wraps err with an explicit HTTP status code, for providers
// that need to report something more specific than a generic failure
// without internal/api needing to know provider-specific error shapes.
type StatusError struct {
	Code int
	Err  error
}

func (e *StatusError) Error() string { return e.Err.Error() }
func (e *StatusError) Unwrap() error { return e.Err }

// NewStatusError wraps err so HTTPStatus reports code for it. Returns nil
// if err is nil, so it's safe to call unconditionally around a return.
func NewStatusError(code int, err error) error {
	if err == nil {
		return nil
	}
	return &StatusError{Code: code, Err: err}
}

// notFoundf is fmt.Errorf that reports 404. The Kubernetes build only classifies real
// *apierrors.StatusError values, so a hand-written "X not found" from a provider that
// looked something up itself (a node in a list, a user in a ConfigMap) would otherwise
// fall through to 500 and read as a server outage to the client.
func notFoundf(format string, a ...any) error {
	return NewStatusError(http.StatusNotFound, fmt.Errorf(format, a...))
}

// HTTPStatus picks the HTTP status code internal/api should report for a
// provider error: an explicit StatusError wins, then a build-specific
// classification (httpStatusFromKnownError — real apierrors under the
// kubernetes build tag, a message-phrasing heuristic otherwise), falling
// back to 500 for anything unrecognized. Without this, every provider
// error — including a real Kubernetes 403 Forbidden from an RBAC-denied
// CRD list — was reported to the client as a generic 500.
func HTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code
	}
	if code, ok := httpStatusFromKnownError(err); ok {
		return code
	}
	return http.StatusInternalServerError
}
