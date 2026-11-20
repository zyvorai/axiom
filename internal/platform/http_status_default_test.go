//go:build !kubernetes

package platform

import (
	"errors"
	"net/http"
	"testing"
)

// The demo build has no structured Kubernetes error type to inspect, so
// httpStatusFromKnownError falls back to matching demo.go's consistent
// error phrasing. These pin that heuristic against the actual message
// vocabulary used across internal/platform/*_demo.go.
func TestHTTPStatusDemoHeuristic(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{errors.New("workload not found"), http.StatusNotFound},
		{errors.New("secret default/app-config not found"), http.StatusNotFound},
		{errors.New("user \"alice\" already exists"), http.StatusConflict},
		{errors.New("namespace and name are required"), http.StatusBadRequest},
		{errors.New("replicas must be >= 0"), http.StatusBadRequest},
		{errors.New("unsupported rollout action \"bogus\""), http.StatusBadRequest},
		{errors.New("cannot delete default namespace in demo"), http.StatusForbidden},
		{errors.New("something totally unrelated broke"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		if got := HTTPStatus(c.err); got != c.want {
			t.Errorf("HTTPStatus(%q) = %d, want %d", c.err.Error(), got, c.want)
		}
	}
}

func TestHTTPStatusWrappedStillClassified(t *testing.T) {
	inner := errors.New("workload not found")
	wrapped := errors.New("scale: " + inner.Error())
	if got := HTTPStatus(wrapped); got != http.StatusNotFound {
		t.Fatalf("HTTPStatus(wrapped not-found) = %d, want %d", got, http.StatusNotFound)
	}
}
