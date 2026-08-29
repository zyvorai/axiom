package platform

import (
	"context"
	"fmt"
	"strings"
)

func (d *DemoProvider) GetResource(ctx context.Context, ns, kind, name string) (ResourceDetail, error) {
	detail := ResourceDetail{
		Namespace: ns,
		Kind:      kind,
		Name:      name,
		Labels:    map[string]string{"app": name},
		Manifest: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       kind,
			"metadata":   map[string]any{"name": name, "namespace": ns},
		},
	}
	if strings.ToLower(kind) != "pod" {
		d.mu.RLock()
		if ps, ok := d.pods[ns+"/"+name]; ok {
			detail.Pods = append([]Pod(nil), ps...)
		}
		d.mu.RUnlock()
	}
	events, err := d.ListEvents(ctx, ns)
	if err == nil {
		for _, e := range events {
			if strings.Contains(e.Object, name) {
				detail.RelatedEvents = append(detail.RelatedEvents, e)
				if len(detail.RelatedEvents) >= 20 {
					break
				}
			}
		}
	}
	return detail, nil
}

func (d *DemoProvider) GetNode(ctx context.Context, name string) (Node, error) {
	nodes, err := d.ListNodes(ctx)
	if err != nil {
		return Node{}, err
	}
	for _, n := range nodes {
		if n.Name == name {
			return n, nil
		}
	}
	return Node{}, fmt.Errorf("node %s not found", name)
}

func (d *DemoProvider) WorkloadNetworkGuard(ctx context.Context, ns, kind, name string) (NetworkGuardSummary, error) {
	return NetworkGuardSummary{Configured: false, Message: "PacketWolf not configured in demo mode"}, nil
}
