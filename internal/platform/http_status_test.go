package platform

import (
	"errors"
	"fmt"
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

// The Kubernetes build only recognises real apierrors, so provider code that reports a
// hand-written "not found" (e.g. GetNode scanning a node list) must say 404 explicitly,
// or a missing resource looks like a 500 server error to the client.
func TestNotFoundfReports404AndKeepsMessage(t *testing.T) {
	err := notFoundf("node %s not found", "worker-9")
	if got := HTTPStatus(err); got != http.StatusNotFound {
		t.Fatalf("HTTPStatus(notFoundf) = %d, want %d", got, http.StatusNotFound)
	}
	if err.Error() != "node worker-9 not found" {
		t.Fatalf("message = %q, want %q", err.Error(), "node worker-9 not found")
	}
	wrapped := fmt.Errorf("get node: %w", err)
	if got := HTTPStatus(wrapped); got != http.StatusNotFound {
		t.Fatalf("HTTPStatus(wrapped notFoundf) = %d, want %d", got, http.StatusNotFound)
	}
}
