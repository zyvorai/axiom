package auth

import (
	"net/http"
	"strings"
)

// axiomNamespace is where Axiom keeps its own state (users, remote-cluster
// kubeconfigs, audit). Secret *values* there are admin-only.
const axiomNamespace = "axiom-system"

// isInteractivePath reports whether a request opens an interactive session
// into a workload: a pod shell, a VM console, or the pod "window" proxy. These
// are GET/WebSocket routes, so a method check alone would let the read-only
// viewer role through — they require operator or admin.
func isInteractivePath(path string) bool {
	if strings.HasPrefix(path, "/api/v1/pods/") {
		rest := strings.TrimPrefix(path, "/api/v1/pods/")
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) == 3 {
			return parts[2] == "terminal" || parts[2] == "exec" || parts[2] == "debug" || strings.HasPrefix(parts[2], "window/") || parts[2] == "window"
		}
		return false
	}
	if strings.HasPrefix(path, "/api/v1/vms/") {
		return strings.HasSuffix(path, "/vnc") || strings.HasSuffix(path, "/console")
	}
	return isNodeShellPath(path)
}

// secretValuesTarget reports whether the request reads decoded Secret values,
// and the namespace of the Secret.
func secretValuesTarget(r *http.Request) (ns string, ok bool) {
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/v1/secrets/") {
		return "", false
	}
	q := r.URL.Query()
	if q.Get("values") != "true" && q.Get("decode") != "true" {
		return "", false
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/secrets/")
	ns, _, _ = strings.Cut(rest, "/")
	return ns, true
}

// isOperatorForbidden lists routes an operator must not use, for any method:
// privilege-escalation surfaces (RBAC objects, arbitrary Helm repo install /
// upgrade) and integration configuration/testing (which can return stored
// credentials). Reads of GET /integrations stay allowed — the UI needs them.
func isOperatorForbidden(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/v1/rbac/"):
		return true
	case p == "/api/v1/helm/install" || p == "/api/v1/helm/upgrade":
		return true
	case strings.HasPrefix(p, "/api/v1/integrations/"):
		return true
	}
	return false
}

// restrictedSensitivePrefixes are cluster-wide or administrative surfaces a
// namespace-restricted identity may never touch, whatever the method.
var restrictedSensitivePrefixes = []string{
	"/api/v1/apply",
	"/api/v1/rbac",
	"/api/v1/audit",
	"/api/v1/users",
	"/api/v1/integrations",
	"/api/v1/gitops",
	"/api/v1/atlas",
	"/api/v1/packetwolf",
	"/api/v1/helm/install",
	"/api/v1/helm/upgrade",
	"/api/v1/helm/rollback",
	"/api/v1/helm/uninstall",
}

// restrictedDenied decides whether a namespace-restricted identity may make
// this request at all, before per-namespace checks. Fail-closed: besides the
// sensitive prefixes, any write to a route that is not a recognised
// namespaced resource is refused (e.g. DELETE /namespaces/{name}, node drain).
func restrictedDenied(r *http.Request) bool {
	p := r.URL.Path
	if isNodeShellPath(p) {
		return true // root on a node is never namespace-scoped
	}
	for _, pre := range restrictedSensitivePrefixes {
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	if isMutatingMethod(r.Method) {
		if _, scoped := matchNamespacedPrefix(p); !scoped {
			// logout and cluster activation are harmless session operations.
			if p == "/api/v1/auth/logout" {
				return false
			}
			return true
		}
	}
	return false
}

// pathNamespacePrefixes are the namespaced routes whose first path segment is
// the namespace (`/api/v1/pods/{namespace}/{name}`). For these the handler
// reads the path value, so the guard must authorize it even when a different
// ?namespace= is supplied.
var pathNamespacePrefixes = map[string]bool{
	"/api/v1/agent-runs":      true,
	"/api/v1/configmaps":      true,
	"/api/v1/cronjobs":        true,
	"/api/v1/helm/releases":   true,
	"/api/v1/hpas":            true,
	"/api/v1/ingresses":       true,
	"/api/v1/jobs":            true,
	"/api/v1/networkpolicies": true,
	"/api/v1/pods":            true,
	"/api/v1/resourcequotas":  true,
	"/api/v1/resources":       true,
	"/api/v1/secrets":         true,
	"/api/v1/services":        true,
	"/api/v1/vms":             true,
	"/api/v1/volumes":         true,
	"/api/v1/workloads":       true,
}

// namespacesOfRequest returns every namespace a request names and whether the
// route is namespace-scoped. For path-namespace routes it returns the path
// segment AND each ?namespace= value — all must be authorized, otherwise
// `GET /secrets/kube-system/x?namespace=team-a` would pass the guard while the
// handler serves kube-system. An empty string means "all namespaces".
func namespacesOfRequest(r *http.Request) ([]string, bool) {
	prefix, ok := matchNamespacedPrefix(r.URL.Path)
	if !ok {
		return nil, false
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, prefix), "/")
	vals, hasQuery := r.URL.Query()["namespace"]
	var out []string
	if pathNamespacePrefixes[prefix] {
		if rest != "" {
			seg, _, _ := strings.Cut(rest, "/")
			out = append(out, seg)
		}
		if hasQuery {
			out = append(out, vals...)
		}
	} else if hasQuery && len(vals) > 0 {
		out = append(out, vals...)
	} else if rest != "" {
		seg, _, _ := strings.Cut(rest, "/")
		out = append(out, seg)
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out, true
}
