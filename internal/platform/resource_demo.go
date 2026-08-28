package platform

import (
	"context"
	"fmt"
)

func (d *DemoProvider) GetResource(ctx context.Context, ns, kind, name string) (ResourceDetail, error) {
	return ResourceDetail{
		Namespace: ns,
		Kind:      kind,
		Name:      name,
		Labels:    map[string]string{"app": name},
		Manifest: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       kind,
			"metadata":   map[string]any{"name": name, "namespace": ns},
		},
	}, nil
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
