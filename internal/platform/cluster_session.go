package platform

import "context"

// ClusterSessionHeader carries a per-browser-tab (or per-script) identifier
// so SwitchableProvider can track which cluster is "active" separately per
// caller, instead of one process-wide switch that different tabs/users would
// otherwise silently stomp on. Set by internal/api's request middleware.
const ClusterSessionHeader = "X-Axiom-Session"

type clusterSessionCtxKey struct{}

// WithClusterSession attaches the caller's cluster-session key to ctx.
func WithClusterSession(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, clusterSessionCtxKey{}, key)
}

func clusterSessionFromContext(ctx context.Context) string {
	v, _ := ctx.Value(clusterSessionCtxKey{}).(string)
	return v
}
