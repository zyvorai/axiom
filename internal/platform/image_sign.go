package platform

import (
	"strings"
)

// imageSignBadge returns a coarse supply-chain badge from image digest / labels.
// Never contacts a registry — annotation/label presence only.
func imageSignBadge(image string, labels map[string]string) string {
	if labels != nil {
		for _, k := range []string{
			"cosign.sigstore.dev/signature",
			"containers.k8s.io/signed",
			"org.opencontainers.image.created",
		} {
			if labels[k] != "" {
				return "signed"
			}
		}
		if labels["slsa.dev/provenance"] != "" || labels["provenance.slsa.dev"] != "" {
			return "signed"
		}
	}
	img := strings.TrimSpace(image)
	if img == "" {
		return "unknown"
	}
	// Mutable tags without digest are unsigned-by-default for twin compare.
	if strings.Contains(img, "@sha256:") {
		return "unknown" // digest pinned; signature still unknown without cosign verify
	}
	if strings.HasSuffix(img, ":latest") || !strings.Contains(img, ":") {
		return "unsigned"
	}
	return "unknown"
}
