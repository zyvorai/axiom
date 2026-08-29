package platform

import (
	"errors"
	"net/http"
	"testing"
)

// HTTPStatus used to not exist at all — every provider error was reported
// to the client as a generic 500, including e.g. a real Kubernetes 403
// Forbidden from an RBAC-denied CRD list. These cover both the explicit
// StatusError path and the message-phrasing heuristic this (non-kubernetes)
// build falls back to for demo mode's plain fmt.Errorf values.
func TestHTTPStatusNil(t *testing.T) {
	if got := HTTPStatus(nil); got != http.StatusOK {
		t.Fatalf("HTTPStatus(nil) = %d, want %d", got, http.StatusOK)
	}
}

func TestHTTPStatusExplicitStatusError(t *testing.T) {
	inner := errors.New("brew failed")
	err := NewStatusError(http.StatusTeapot, inner)
	if got := HTTPStatus(err); got != http.StatusTeapot {
		t.Fatalf("HTTPStatus(explicit) = %d, want %d", got, http.StatusTeapot)
	}
	if err.Error() != "brew failed" {
		t.Fatalf("StatusError.Error() = %q, want %q", err.Error(), "brew failed")
	}
	if !errors.Is(err, inner) {
		t.Fatal("StatusError should unwrap to the wrapped error via errors.Is")
	}
}

func TestNewStatusErrorNilIsNil(t *testing.T) {
	if err := NewStatusError(http.StatusNotFound, nil); err != nil {
		t.Fatalf("NewStatusError(_, nil) = %v, want nil", err)
	}
}
