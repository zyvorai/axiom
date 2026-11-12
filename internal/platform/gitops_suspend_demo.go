package platform

import (
	"context"
	"fmt"
	"net/http"
)

func (d *DemoProvider) SuspendGitOpsApp(ctx context.Context, kind, ns, name string, req GitOpsSuspendRequest) (GitOpsSuspendResult, error) {
	if req.ResourceVersion == "" {
		return GitOpsSuspendResult{}, NewStatusError(http.StatusBadRequest, fmt.Errorf("resourceVersion is required"))
	}
	if kind == "ApplicationSet" {
		return GitOpsSuspendResult{}, NewStatusError(http.StatusUnprocessableEntity, fmt.Errorf("ApplicationSet cannot be suspended here; edit the ApplicationSet instead"))
	}
	demoGitOpsMu.Lock()
	defer demoGitOpsMu.Unlock()
	key := ns + "/" + name
	g, ok := demoGitOps[key]
	if !ok || g.Kind != kind {
		return GitOpsSuspendResult{}, notFoundf("%s %s/%s not found", kind, ns, name)
	}
	if g.Suspended == req.Suspend {
		return GitOpsSuspendResult{OK: true, Changed: false}, nil
	}
	g.Suspended = req.Suspend
	if req.Suspend {
		if kind == "Application" {
			g.Status = "Suspended"
		} else {
			g.Status = "Suspended"
		}
	} else if kind == "Application" {
		g.Status = "Synced"
	} else {
		g.Status = "Ready"
	}
	demoGitOps[key] = g
	return GitOpsSuspendResult{OK: true, Changed: true}, nil
}
