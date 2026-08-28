package platform

import (
	"context"
	"fmt"
)

func (d *DemoProvider) GetRolloutStatus(ctx context.Context, ns, kind, name string) (RolloutStatus, error) {
	if kind != "Deployment" {
		return RolloutStatus{}, fmt.Errorf("rollout not supported for %s", kind)
	}
	return RolloutStatus{
		Status:    "Available",
		Replicas:  3,
		Ready:     3,
		Updated:   3,
		Available: 3,
		History: []RolloutRevision{
			{Revision: 2, Status: "active", Age: "2h"},
			{Revision: 1, Status: "inactive", Age: "1d"},
		},
	}, nil
}

func (d *DemoProvider) RolloutAction(ctx context.Context, ns, kind, name, action string) error {
	return nil
}
