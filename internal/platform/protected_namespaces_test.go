package platform

import "testing"

func TestProtectedNamespace(t *testing.T) {
	t.Setenv("AXIOM_NAMESPACE", "custom-ns")
	for _, ns := range []string{"kube-system", "kube-public", "kube-node-lease", "default", "axiom-system", "custom-ns"} {
		if !protectedNamespace(ns) {
			t.Errorf("%s should be protected", ns)
		}
	}
	for _, ns := range []string{"team-a", "e2e-ns-abc", ""} {
		if protectedNamespace(ns) {
			t.Errorf("%s should not be protected", ns)
		}
	}
}

func TestDemoDeleteNamespaceRefusesProtected(t *testing.T) {
	d := NewDemoProvider()
	for _, ns := range []string{"kube-system", "default"} {
		if err := d.DeleteNamespace(nil, ns); err == nil || HTTPStatus(err) != 403 {
			t.Errorf("%s: want 403 error, got %v", ns, err)
		}
	}
}
