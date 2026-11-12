package platform

import (
	"context"
	"fmt"
	"strings"
)

// StillExecute runs a Still ladder action after the operator reviewed BlastPreview.
// hush/seal → network isolate; sleep → scale to zero; end → delete workload.
func StillExecute(ctx context.Context, p Provider, ns, kind, name string, action StillAction) error {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	ns = strings.TrimSpace(ns)
	if ns == "" || kind == "" || name == "" {
		return fmt.Errorf("namespace, kind and name are required")
	}
	switch action {
	case StillHush, StillSeal:
		return p.SealWorkload(ctx, ns, kind, name)
	case StillSleep:
		return p.Stop(ctx, ns, kind, name)
	case StillEnd:
		return p.DeleteWorkload(ctx, ns, kind, name)
	default:
		return fmt.Errorf("unknown still action %q (use hush, sleep, seal or end)", action)
	}
}
