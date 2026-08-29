//go:build !kubernetes

package platform

import (
	"net/http"
	"strings"
)

// httpStatusFromKnownError applies a lightweight message-phrasing
// heuristic for the demo build, where provider errors are plain
// fmt.Errorf values with no structured type. Matches the phrasing
// demo.go and friends already use consistently ("X not found", "X is
// required", "X already exists", "cannot ...") so demo mode reports the
// same class of status code a real cluster would, without retrofitting
// every call site individually.
func httpStatusFromKnownError(err error) (int, bool) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		return http.StatusNotFound, true
	case strings.Contains(msg, "already exists"):
		return http.StatusConflict, true
	case strings.Contains(msg, "required") ||
		strings.Contains(msg, "must be") ||
		strings.Contains(msg, "unsupported") ||
		strings.Contains(msg, "not supported"):
		return http.StatusBadRequest, true
	case strings.HasPrefix(msg, "cannot "):
		return http.StatusForbidden, true
	}
	return 0, false
}
