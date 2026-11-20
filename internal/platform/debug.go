package platform

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

// Ephemeral debug containers and the node shell (Workstream B4/B5). Shared,
// build-tag free configuration and types; the Kubernetes implementations are in
// debug_container_kubernetes.go and node_shell_kubernetes.go.

const (
	// DefaultDebugImage is the ephemeral container image when AXIOM_DEBUG_IMAGE is unset.
	DefaultDebugImage = "busybox"
	// DefaultNodeShellImage is the node-shell pod image when AXIOM_NODE_SHELL_IMAGE is unset.
	// It must provide nsenter (Alpine's busybox does).
	DefaultNodeShellImage = "alpine:3.20"
	// DefaultNodeShellNamespace is the dedicated namespace node-shell pods live in.
	DefaultNodeShellNamespace = "axiom-node-shell"
	// DefaultNodeShellTimeout is how long a node-shell pod may live.
	DefaultNodeShellTimeout = 15 * time.Minute
	maxNodeShellTimeout     = time.Hour
)

// ErrNodeShellDisabled is returned when the node-shell switch is off.
var ErrNodeShellDisabled = NewStatusError(http.StatusForbidden,
	errors.New("node shell is disabled: set AXIOM_ENABLE_NODE_SHELL=true on the Axiom deployment to enable it (admin-only, audited; see docs/DEPLOYMENT.md)"))

// DebugImage returns the image used for ephemeral debug containers.
func DebugImage() string {
	if v := strings.TrimSpace(os.Getenv("AXIOM_DEBUG_IMAGE")); v != "" {
		return v
	}
	return DefaultDebugImage
}

// NodeShellEnabled reports whether the node shell is switched on. It is off
// unless AXIOM_ENABLE_NODE_SHELL is exactly "true" (case-insensitive).
func NodeShellEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("AXIOM_ENABLE_NODE_SHELL")), "true")
}

// NodeShellNamespace returns the dedicated namespace for node-shell pods.
func NodeShellNamespace() string {
	if v := strings.TrimSpace(os.Getenv("AXIOM_NODE_SHELL_NAMESPACE")); v != "" {
		return v
	}
	return DefaultNodeShellNamespace
}

// NodeShellImage returns the node-shell pod image.
func NodeShellImage() string {
	if v := strings.TrimSpace(os.Getenv("AXIOM_NODE_SHELL_IMAGE")); v != "" {
		return v
	}
	return DefaultNodeShellImage
}

// NodeShellTimeout returns the node-shell session lifetime
// (AXIOM_NODE_SHELL_TIMEOUT as a Go duration, capped at one hour).
func NodeShellTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("AXIOM_NODE_SHELL_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= time.Minute {
			if d > maxNodeShellTimeout {
				return maxNodeShellTimeout
			}
			return d
		}
	}
	return DefaultNodeShellTimeout
}

// DebugCapabilities is the GET /debug/capabilities payload: what the UI may offer.
type DebugCapabilities struct {
	EphemeralContainers bool   `json:"ephemeralContainers"`
	DebugImage          string `json:"debugImage"`
	NodeShell           bool   `json:"nodeShell"`
	NodeShellNamespace  string `json:"nodeShellNamespace,omitempty"`
	NodeShellTimeoutSec int    `json:"nodeShellTimeoutSeconds,omitempty"`
	// Interactive is true when a live Kubernetes connection can attach terminals.
	Interactive bool `json:"interactive"`
}

// DebugRequest is the POST /pods/{ns}/{name}/debug body. The image is
// deliberately not client-settable: operators choose the toolbox image once,
// via AXIOM_DEBUG_IMAGE.
type DebugRequest struct {
	// Target is the container whose process namespace the debug container
	// shares (so it can see that container's processes). Optional.
	Target string `json:"target,omitempty"`
}

// DebugContainer describes the ephemeral container that was added to a pod.
type DebugContainer struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"` // pass this as ?container= to the terminal
	Image     string `json:"image"`
	Target    string `json:"target,omitempty"`
	// Ready is true once the container is running and a terminal can attach.
	Ready   bool   `json:"ready"`
	Message string `json:"message,omitempty"`
}

func newDebugContainerName() string { return "debugger-" + randHex(3) }

// randHex returns 2n hex characters of cryptographic randomness.
func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("0405.000")))[:2*n]
	}
	return hex.EncodeToString(b)
}
