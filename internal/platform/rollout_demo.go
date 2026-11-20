package platform

import (
	"context"
	"fmt"
	"net/http"
)

type demoRolloutState struct {
	paused   bool
	revision int
	history  []RolloutRevision // newest first
}

func (d *DemoProvider) getRolloutStateLocked(k, age string) *demoRolloutState {
	rs, ok := d.rollouts[k]
	if !ok {
		rs = &demoRolloutState{revision: 1, history: []RolloutRevision{{Revision: 1, Status: "active", Age: age}}}
		d.rollouts[k] = rs
	}
	return rs
}

func (d *DemoProvider) GetRolloutStatus(ctx context.Context, ns, kind, name string) (RolloutStatus, error) {
	if kind != "Deployment" {
		return RolloutStatus{}, fmt.Errorf("rollout not supported for %s", kind)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	k := key(ns, kind, name)
	w, ok := d.workloads[k]
	if !ok {
		return RolloutStatus{}, fmt.Errorf("workload %s/%s not found", ns, name)
	}
	rs := d.getRolloutStateLocked(k, w.Age)
	status := "Available"
	if rs.paused {
		status = "Paused"
	} else if w.Ready < w.Desired {
		status = "Progressing"
	}
	return RolloutStatus{
		Status: status, Paused: rs.paused,
		Replicas: w.Desired, Ready: w.Ready, Updated: w.Desired, Available: w.Available,
		History: append([]RolloutRevision(nil), rs.history...),
	}, nil
}

func (d *DemoProvider) RolloutAction(ctx context.Context, ns, kind, name, action string) error {
	if kind != "Deployment" {
		return fmt.Errorf("rollout not supported for %s", kind)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	k := key(ns, kind, name)
	w, ok := d.workloads[k]
	if !ok {
		return fmt.Errorf("workload %s/%s not found", ns, name)
	}
	rs := d.getRolloutStateLocked(k, w.Age)
	switch action {
	case "pause":
		rs.paused = true
	case "resume":
		rs.paused = false
	case "restart":
		rs.revision++
		for i := range rs.history {
			rs.history[i].Status = "inactive"
		}
		rs.history = append([]RolloutRevision{{Revision: rs.revision, Status: "active", Age: "now"}}, rs.history...)
		d.rebuildPodsLocked(w)
	case "undo":
		if len(rs.history) < 2 {
			return NewStatusError(http.StatusConflict, fmt.Errorf("no previous revision to undo to"))
		}
		rs.history = rs.history[1:]
		rs.history[0].Status = "active"
		rs.revision = rs.history[0].Revision
		d.rebuildPodsLocked(w)
	default:
		return fmt.Errorf("unsupported rollout action %q", action)
	}
	return nil
}
