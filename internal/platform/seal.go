package platform

import (
	"context"
	"strings"
)

const (
	SealedAnnotation = "axiom.zyvor.dev/sealed"
	IntentAnnotation = "axiom.zyvor.dev/intent"
	SealPolicyPrefix = "axiom-seal-"
)

// sealPolicyName includes workload kind so Deployment/foo and StatefulSet/foo
// do not collide on the same NetworkPolicy name.
func sealPolicyName(kind, workload string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	if k == "" {
		k = "workload"
	}
	return SealPolicyPrefix + k + "-" + workload
}

// sealPolicyNames returns the current name plus the legacy name (pre-kind).
func sealPolicyNames(kind, workload string) []string {
	cur := sealPolicyName(kind, workload)
	legacy := SealPolicyPrefix + workload
	if cur == legacy {
		return []string{cur}
	}
	return []string{cur, legacy}
}

func IsSealedAnnotation(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}
	return annotations[SealedAnnotation] == "true"
}

// SealedWorkloadsInNamespace is implemented by the Kubernetes and demo providers.
type SealedWorkloadsInNamespace interface {
	SealedWorkloads(context.Context, string) (map[string]bool, error)
}
