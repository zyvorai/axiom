package platform

import (
	"context"
	"fmt"
	"net/http"
)

func (sp *SwitchableProvider) SuspendGitOpsApp(ctx context.Context, kind, ns, name string, req GitOpsSuspendRequest) (GitOpsSuspendResult, error) {
	type suspender interface {
		SuspendGitOpsApp(context.Context, string, string, string, GitOpsSuspendRequest) (GitOpsSuspendResult, error)
	}
	if s, ok := sp.current(ctx).(suspender); ok {
		return s.SuspendGitOpsApp(ctx, kind, ns, name, req)
	}
	return GitOpsSuspendResult{}, NewStatusError(http.StatusNotImplemented, fmt.Errorf("gitops suspend is not available on this provider"))
}

func (sp *SwitchableProvider) ListDRA(ctx context.Context) (DRAInventory, error) {
	type lister interface {
		ListDRA(context.Context) (DRAInventory, error)
	}
	if l, ok := sp.current(ctx).(lister); ok {
		return l.ListDRA(ctx)
	}
	return DRAInventory{Available: false, Note: "DRA browser not available on this provider", Objects: []DRAObject{}, Counts: map[string]int{}}, nil
}

func (sp *SwitchableProvider) ListKarpenter(ctx context.Context) (KarpenterInventory, error) {
	type lister interface {
		ListKarpenter(context.Context) (KarpenterInventory, error)
	}
	if l, ok := sp.current(ctx).(lister); ok {
		return l.ListKarpenter(ctx)
	}
	return emptyKarpenter("Karpenter view not available on this provider"), nil
}
