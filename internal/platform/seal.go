package platform

import "context"

const (
	SealedAnnotation = "axiom.zyvor.dev/sealed"
	IntentAnnotation = "axiom.zyvor.dev/intent"
	SealPolicyPrefix = "axiom-seal-"
)

func sealPolicyName(workload string) string {
	return SealPolicyPrefix + workload
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
