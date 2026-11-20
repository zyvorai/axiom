package platform

import (
	"context"
	"fmt"
	"net/http"
)

// GitOpsSuspendRequest toggles Flux suspend or Argo automated sync.
type GitOpsSuspendRequest struct {
	Suspend         bool   `json:"suspend"`
	ResourceVersion string `json:"resourceVersion"`
}

// GitOpsSuspendResult is the response for POST .../suspend.
type GitOpsSuspendResult struct {
	OK      bool `json:"ok"`
	Changed bool `json:"changed"`
}

// SuspendGitOpsApp suspends or resumes a GitOps application when the provider supports it.
func SuspendGitOpsApp(ctx context.Context, p Provider, kind, ns, name string, req GitOpsSuspendRequest) (GitOpsSuspendResult, error) {
	if sp, ok := p.(*SwitchableProvider); ok {
		return sp.SuspendGitOpsApp(ctx, kind, ns, name, req)
	}
	if s, ok := p.(interface {
		SuspendGitOpsApp(context.Context, string, string, string, GitOpsSuspendRequest) (GitOpsSuspendResult, error)
	}); ok {
		return s.SuspendGitOpsApp(ctx, kind, ns, name, req)
	}
	return GitOpsSuspendResult{}, NewStatusError(http.StatusNotImplemented, fmt.Errorf("gitops suspend is not available on this provider"))
}
