package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// attachGPUWaste adds over-requested GPU hints onto an accelerators report.
func attachGPUWaste(rep *AcceleratorsReport, util float64) {
	if rep == nil {
		return
	}
	var waste []AcceleratorWaste
	for _, p := range rep.Pods {
		req, err := strconv.Atoi(strings.TrimSpace(p.GPU))
		if err != nil || req < 2 {
			continue // only suggest lowering whole-GPU counts ≥2
		}
		// Without per-pod DCGM, use cluster avg: if util < 15% and request ≥2, recommend 1.
		if util > 0 && util < 15 {
			waste = append(waste, AcceleratorWaste{
				Namespace: p.Namespace, Name: p.Name, Kind: "Pod",
				GPURequest: p.GPU, GPURecommend: "1", Util: util,
				Verdict: "over-requested",
				Note:    fmt.Sprintf("Cluster GPU util ~%.0f%% with request %s — consider lowering to 1 if the workload is idle", util, p.GPU),
			})
		}
	}
	// Demo-friendly: always surface one waste row when we have ml inference pods and low/demo util.
	if len(waste) == 0 && util > 0 && util < 50 {
		for _, p := range rep.Pods {
			if p.Namespace == "ml" && p.GPU != "" {
				waste = append(waste, AcceleratorWaste{
					Namespace: p.Namespace, Name: p.Name, Kind: "Deployment", Container: "inference",
					GPURequest: p.GPU, GPURecommend: p.GPU, Util: util,
					Verdict: "ok",
					Note:    "Util moderate — keep GPU request; watch DCGM before lowering",
				})
				break
			}
		}
	}
	rep.Waste = waste
}

// ApplyGPURightsizing lowers nvidia.com/gpu requests on a workload (demo path).
func (d *DemoProvider) ApplyGPURightsizing(ctx context.Context, req GPURightsizingApplyRequest) (GPURightsizingApplyResult, error) {
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Name = strings.TrimSpace(req.Name)
	if req.Namespace == "" || req.Name == "" {
		return GPURightsizingApplyResult{}, rightsizingBadRequest("namespace and name are required")
	}
	if req.Kind == "" {
		req.Kind = "Deployment"
	}
	from, to := "2", "1"
	if req.DryRun {
		return GPURightsizingApplyResult{OK: true, DryRun: true, From: from, To: to, Note: "demo dry-run: would lower nvidia.com/gpu request", Rollout: "RollingUpdate"}, nil
	}
	return GPURightsizingApplyResult{OK: true, DryRun: false, From: from, To: to, Note: "demo: recorded GPU rightsizing (no live patch)", Rollout: "RollingUpdate"}, nil
}
