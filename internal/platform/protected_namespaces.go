package platform

import (
	"os"
	"strings"
)

// protectedNamespace reports namespaces that must never be deleted through
// Axiom: the cluster's own system namespaces, `default`, and Axiom's own
// namespace (deleting it would destroy the console mid-request).
func protectedNamespace(name string) bool {
	switch strings.TrimSpace(name) {
	case "kube-system", "kube-public", "kube-node-lease", "default", "axiom-system":
		return true
	}
	for _, env := range []string{"AXIOM_NAMESPACE", "POD_NAMESPACE"} {
		if own := strings.TrimSpace(os.Getenv(env)); own != "" && own == strings.TrimSpace(name) {
			return true
		}
	}
	return false
}
