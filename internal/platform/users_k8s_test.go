//go:build kubernetes

package platform

import (
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// retryOnConflictOrExists is the fix for a real bug: Create/Update/Delete
// all load-modify-save a single shared Secret, so two concurrent calls can
// race on either a resourceVersion conflict (a concurrent Update raced
// ahead) or an AlreadyExists error (both found the Secret missing and both
// tried to Create it). retry.RetryOnConflict alone only retries the first
// case; this must retry both, or one of two logically-independent
// concurrent requests fails outright.
//
// Not verified against client-go's fake clientset here: it doesn't assign
// or check resourceVersion at all (confirmed empirically — Create leaves it
// "" and Update never rejects a stale one), so it can't simulate the
// conflict this fix handles. A real Kubernetes API server does enforce
// this; see the real-cluster verification in the accompanying commit.
func TestRetryOnConflictOrExistsRetriesConflict(t *testing.T) {
	gr := schema.GroupResource{Group: "", Resource: "secrets"}
	attempts := 0
	err := retryOnConflictOrExists(func() error {
		attempts++
		if attempts < 3 {
			return apierrors.NewConflict(gr, "zyvor-axiom-users", errors.New("stale resourceVersion"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryOnConflictOrExistsRetriesAlreadyExists(t *testing.T) {
	gr := schema.GroupResource{Group: "", Resource: "secrets"}
	attempts := 0
	err := retryOnConflictOrExists(func() error {
		attempts++
		if attempts < 2 {
			return apierrors.NewAlreadyExists(gr, "zyvor-axiom-users")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetryOnConflictOrExistsDoesNotRetryOtherErrors(t *testing.T) {
	attempts := 0
	wantErr := errors.New("user \"alice\" not found")
	err := retryOnConflictOrExists(func() error {
		attempts++
		return wantErr
	})
	if err != wantErr {
		t.Fatalf("expected the business-logic error to pass through unchanged, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected exactly 1 attempt for a non-retriable error, got %d", attempts)
	}
}

// Regression coverage for a real bug: podMetricsHist (ops_kubernetes.go) is
// a package-level map shared by every KubernetesProvider in the process —
// more than one real cluster in the multi-context kubeconfig case. Without
// scoping the key by cluster, two clusters both having e.g. "default/nginx"
// would silently share (and corrupt) each other's metric history.
func TestMetricsKeyIsolatesClusters(t *testing.T) {
	a := &KubernetesProvider{clusterKey: "cluster-a"}
	b := &KubernetesProvider{clusterKey: "cluster-b"}
	single := &KubernetesProvider{}

	if a.metricsKey("default/nginx") == b.metricsKey("default/nginx") {
		t.Fatal("expected two different clusters' same-named pod to get different metrics keys")
	}
	if single.metricsKey("default/nginx") != "default/nginx" {
		t.Fatalf("expected an unscoped (single-cluster) provider to leave the key unchanged, got %q", single.metricsKey("default/nginx"))
	}
}
