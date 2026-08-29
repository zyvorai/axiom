//go:build kubernetes

package platform

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// The kubernetes build classifies real client-go errors via apierrors.Is*
// instead of message-sniffing. This is the exact bug this fix addresses:
// a real RBAC-denied CRD list returned apierrors.NewForbidden, and before
// HTTPStatus existed, internal/api reported it to the client as a plain
// 500 instead of 403.
func TestHTTPStatusRealAPIErrors(t *testing.T) {
	gr := schema.GroupResource{Group: "bmo.metal3.io", Resource: "alertpolicies"}
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", apierrors.NewNotFound(gr, "demo"), http.StatusNotFound},
		{"forbidden", apierrors.NewForbidden(gr, "demo", errors.New("rbac")), http.StatusForbidden},
		{"conflict", apierrors.NewConflict(gr, "demo", errors.New("stale")), http.StatusConflict},
		{"already exists", apierrors.NewAlreadyExists(gr, "demo"), http.StatusConflict},
		{"unauthorized", apierrors.NewUnauthorized("no token"), http.StatusUnauthorized},
		{"too many requests", apierrors.NewTooManyRequests("slow down", 5), http.StatusTooManyRequests},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HTTPStatus(c.err); got != c.want {
				t.Errorf("HTTPStatus(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

// apierrors.Is* uses errors.As, so wrapping with fmt.Errorf("...: %w", err)
// (the pattern used throughout kubernetes.go, e.g. Drain's "cordon node: %w")
// must not defeat the classification.
func TestHTTPStatusRealAPIErrorWrapped(t *testing.T) {
	gr := schema.GroupResource{Group: "", Resource: "nodes"}
	inner := apierrors.NewForbidden(gr, "axiom-01", errors.New("rbac"))
	wrapped := fmt.Errorf("cordon node: %w", inner)
	if got := HTTPStatus(wrapped); got != http.StatusForbidden {
		t.Fatalf("HTTPStatus(wrapped forbidden) = %d, want %d", got, http.StatusForbidden)
	}
}

// A plain non-Kubernetes error (e.g. an internal bug) still falls back to 500.
func TestHTTPStatusUnclassifiedFallsBackTo500(t *testing.T) {
	if got := HTTPStatus(errors.New("something totally unrelated broke")); got != http.StatusInternalServerError {
		t.Fatalf("HTTPStatus(plain error) = %d, want %d", got, http.StatusInternalServerError)
	}
}
