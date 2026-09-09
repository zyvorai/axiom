package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

// vmVNC's only kubernetes-independent behavior is its fallback: it type-
// asserts the active provider for an unexported VMVNCProxy(...) error
// interface, and only *platform.KubernetesProvider (internal/platform/vm_vnc_kubernetes.go,
// gated by the `kubernetes` build tag) implements it. On this build no
// provider — demo or otherwise — satisfies that interface, so every request
// falls through to the StatusNotImplemented branch regardless of whether
// the namespace/name refer to a real VM: the handler never gets far enough
// to look the VM up. Actually driving the success path would require the
// kubernetes build tag plus a real (or hijackable) WebSocket upgrade target,
// which httptest.ResponseRecorder can't provide (it doesn't implement
// http.Hijacker) — so that path is not covered here.
func TestVMVNCNotImplementedOnDemoProvider(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "GET", "/api/v1/vms/apps/hello/vnc", "")
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 on a build without KubeVirt/kubernetes support, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "kubernetes build") {
		t.Fatalf("expected the kubernetes-build guidance message, got: %s", w.Body.String())
	}
}

func TestVMVNCNotImplementedForNonexistentVM(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "GET", "/api/v1/vms/does-not-exist/also-missing/vnc", "")
	// The handler gates purely on whether the active provider implements the
	// VMVNCProxy interface — it never looks up the VM/namespace at all — so
	// a nonexistent VM produces the same 501 as a real one on this build.
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVMVNCUsesSwitchableProviderCurrentCluster(t *testing.T) {
	h := New(platform.NewDemoSwitchableProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "GET", "/api/v1/vms/apps/hello/vnc", "")
	// Exercises the s.switchable()/CurrentProvider branch specifically; the
	// wrapped demo providers still don't implement VMVNCProxy, so the result
	// is the same fallback as the single-provider case.
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVMVNCRequiresAuth(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	r := httptest.NewRequest(http.MethodGet, "/api/v1/vms/apps/hello/vnc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without credentials, got %d: %s", w.Code, w.Body.String())
	}
}
