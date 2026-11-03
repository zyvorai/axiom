package platform

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// demoRightsizingLimits are the limits the demo containers declare (the demo
// recommendation rows only carry requests). redis has a memory limit equal to
// its request so applying its "under" recommendation shows a raised limit.
var demoRightsizingLimits = map[string]rightsizingCurrentResources{
	"payments/Deployment/payments-api/api": {CPULimitMilli: 2000, MemLimitBytes: 4096 * mib},
	"data/StatefulSet/redis/redis":         {MemLimitBytes: 5461 * mib},
}

// demoRightsizingApplied holds, per DemoProvider, the resources of containers
// a rightsizing apply has written, so the next report reflects them. It lives
// beside the provider (not in it) to keep the apply self-contained.
var demoRightsizingApplied = struct {
	sync.Mutex
	m map[*DemoProvider]map[string]rightsizingCurrentResources
}{m: map[*DemoProvider]map[string]rightsizingCurrentResources{}}

// demoRightsizingResources is the container's current requests and limits:
// what an apply wrote, else the seed values. The caller holds the lock.
func (d *DemoProvider) demoRightsizingResourcesLocked(c rightsizingContainer) rightsizingCurrentResources {
	if cur, ok := demoRightsizingApplied.m[d][c.key()]; ok {
		return cur
	}
	cur := demoRightsizingLimits[c.key()]
	cur.CPURequestMilli, cur.MemRequestBytes = c.RequestCPUMilli, c.RequestMemBytes
	return cur
}

// demoRightsizingContainer returns c with the requests an apply wrote.
func (d *DemoProvider) demoRightsizingContainer(c rightsizingContainer) rightsizingContainer {
	demoRightsizingApplied.Lock()
	defer demoRightsizingApplied.Unlock()
	cur := d.demoRightsizingResourcesLocked(c)
	c.RequestCPUMilli, c.RequestMemBytes = cur.CPURequestMilli, cur.MemRequestBytes
	return c
}

// ApplyRightsizing applies one demo recommendation in memory: after a real
// apply the next ListRightsizing reports the container at the recommendation.
func (d *DemoProvider) ApplyRightsizing(ctx context.Context, req RightsizingApplyRequest) (RightsizingApplyResult, error) {
	req, err := NormalizeRightsizingApplyRequest(req)
	if err != nil {
		return RightsizingApplyResult{}, err
	}
	key := req.containerKey()
	var (
		row          *rightsizingContainer
		usage        containerUsage
		workloadSeen bool
	)
	for i := range demoRightsizing {
		c := demoRightsizing[i].c
		if c.Namespace == req.Namespace && c.Kind == req.Kind && c.Workload == req.Workload {
			workloadSeen = true
			if c.Container == req.Container {
				row, usage = &demoRightsizing[i].c, demoRightsizing[i].u
			}
		}
	}
	if row == nil {
		if workloadSeen {
			return RightsizingApplyResult{}, NewStatusError(http.StatusNotFound, fmt.Errorf("container %q not found in %s", req.Container, req.workloadRef()))
		}
		return RightsizingApplyResult{}, NewStatusError(http.StatusNotFound, fmt.Errorf("%s not found", req.workloadRef()))
	}

	demoRightsizingApplied.Lock()
	defer demoRightsizingApplied.Unlock()
	cur := d.demoRightsizingResourcesLocked(*row)
	c := *row
	c.RequestCPUMilli, c.RequestMemBytes = cur.CPURequestMilli, cur.MemRequestBytes
	src := snapshotUsageSource{}
	item, ok := recommendContainer(c, usage, src.basis())
	if !ok {
		return RightsizingApplyResult{}, rightsizingNoUsage(req)
	}
	plan, err := prepareRightsizingApply(req, item, cur)
	if err != nil {
		return RightsizingApplyResult{}, err
	}
	if !req.DryRun {
		if demoRightsizingApplied.m[d] == nil {
			demoRightsizingApplied.m[d] = map[string]rightsizingCurrentResources{}
		}
		demoRightsizingApplied.m[d][key] = plan.applyTo(cur)
	}
	return RightsizingApplyResult{
		OK: true, DryRun: req.DryRun, Changes: plan.Changes, Rollout: rightsizingRolloutNote(req.Kind, "", false),
		Verdict: item.Verdict, Source: src.name(), Confidence: src.confidence(req.Window),
	}, nil
}
