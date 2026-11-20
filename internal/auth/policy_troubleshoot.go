package auth

import "strings"

// isNodeShellPath reports whether the path is the node shell,
// GET /api/v1/nodes/{name}/shell (a WebSocket giving root on a node). It is
// admin-only and never available to namespace-restricted identities.
func isNodeShellPath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/api/v1/nodes/")
	if !ok {
		return false
	}
	name, tail, found := strings.Cut(rest, "/")
	return found && name != "" && strings.TrimSuffix(tail, "/") == "shell"
}
