package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// ExecuteAgentRun runs allowlisted plan actions for an Axiom-owned run.
// DryRun=true records success without mutating the cluster (except noop).
func ExecuteAgentRun(ctx context.Context, p Provider, run AgentRun) (AgentRun, error) {
	if run.Plan == nil || len(run.Plan.Actions) == 0 {
		run.Phase = "Succeeded"
		run.Approval = "Approved"
		run.Message = "no actions to execute"
		agentRunDecideFlags(&run)
		return run, nil
	}
	executed := make([]AgentExecuted, 0, len(run.Plan.Actions))
	allOK := true
	for _, a := range run.Plan.Actions {
		ex := AgentExecuted{ID: a.ID, Type: a.Type, OK: true}
		if run.DryRun {
			ex.Message = "dry-run: skipped mutation"
			executed = append(executed, ex)
			continue
		}
		if err := executeAgentAction(ctx, p, run, a); err != nil {
			ex.OK = false
			ex.Message = err.Error()
			allOK = false
		} else {
			ex.Message = "ok"
		}
		executed = append(executed, ex)
		if !ex.OK {
			break
		}
	}
	run.Executed = executed
	run.Approval = "Approved"
	if allOK {
		run.Phase = "Succeeded"
		run.Message = "actions executed"
	} else {
		run.Phase = "Failed"
		run.Message = "one or more actions failed; halted"
	}
	agentRunDecideFlags(&run)
	return run, nil
}

func executeAgentAction(ctx context.Context, p Provider, run AgentRun, a AgentAction) error {
	if !IsAllowlistedAgentAction(a.Type) {
		return fmt.Errorf("action type %q is not allowlisted", a.Type)
	}
	ns, kind, name := resolveAgentTarget(run, a)
	switch a.Type {
	case AgentActionNoop:
		return nil
	case AgentActionRestartPod, AgentActionDeletePod:
		if name == "" {
			return fmt.Errorf("pod name required")
		}
		return p.DeletePod(ctx, ns, name)
	case AgentActionRolloutRestart:
		if name == "" {
			return fmt.Errorf("workload name required")
		}
		if kind == "" {
			kind = "Deployment"
		}
		return p.Restart(ctx, ns, kind, name)
	case AgentActionScaleWorkload:
		if name == "" {
			return fmt.Errorf("workload name required")
		}
		if kind == "" {
			kind = "Deployment"
		}
		replicas := int32(1)
		if a.Params != nil {
			if raw := strings.TrimSpace(a.Params["replicas"]); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil || n < 0 || n > 1000 {
					return fmt.Errorf("invalid replicas %q", raw)
				}
				replicas = int32(n)
			}
		}
		return p.Scale(ctx, ns, kind, name, replicas)
	case AgentActionCordonNode:
		node := name
		if node == "" && a.Target != nil {
			node = a.Target.Name
		}
		if node == "" {
			return fmt.Errorf("node name required")
		}
		return p.Cordon(ctx, node, true)
	case AgentActionUncordonNode:
		node := name
		if node == "" && a.Target != nil {
			node = a.Target.Name
		}
		if node == "" {
			return fmt.Errorf("node name required")
		}
		return p.Cordon(ctx, node, false)
	case AgentActionApplyRightsizing:
		req := RightsizingApplyRequest{
			Kind:      paramOr(a, "kind", kind),
			Namespace: paramOr(a, "namespace", ns),
			Workload:  paramOr(a, "workload", name),
			Container: paramOr(a, "container", ""),
			DryRun:    false,
		}
		if req.Workload == "" || req.Container == "" {
			return fmt.Errorf("apply_rightsizing needs workload and container params")
		}
		if raw := paramOr(a, "resources", "cpu,memory"); raw != "" {
			for _, part := range strings.Split(raw, ",") {
				part = strings.TrimSpace(strings.ToLower(part))
				if part == "cpu" || part == "memory" {
					req.Resources = append(req.Resources, part)
				}
			}
		}
		if len(req.Resources) == 0 {
			req.Resources = []string{"cpu", "memory"}
		}
		_, err := p.ApplyRightsizing(ctx, req)
		return err
	case AgentActionSealWorkload:
		if name == "" {
			return fmt.Errorf("workload name required")
		}
		if kind == "" {
			kind = "Deployment"
		}
		return p.SealWorkload(ctx, ns, kind, name)
	case AgentActionClonePVC:
		src := paramOr(a, "sourcePVC", name)
		dst := paramOr(a, "name", "")
		if dst == "" {
			dst = paramOr(a, "targetPVC", "")
		}
		size := paramOr(a, "size", "10Gi")
		if ns == "" || src == "" || dst == "" {
			return fmt.Errorf("clone_pvc needs namespace, sourcePVC, and name/targetPVC")
		}
		_, err := p.CreateVolume(ctx, CreateVolumeRequest{
			Namespace: ns, Name: dst, Size: size,
			StorageClass: paramOr(a, "storageClass", ""),
			SourcePVC:    src,
		})
		return err
	default:
		return fmt.Errorf("unsupported action %q", a.Type)
	}
}

func paramOr(a AgentAction, key, fallback string) string {
	if a.Params != nil {
		if v := strings.TrimSpace(a.Params[key]); v != "" {
			return v
		}
	}
	return fallback
}

func resolveAgentTarget(run AgentRun, a AgentAction) (ns, kind, name string) {
	ns = run.Namespace
	if a.Target != nil {
		if a.Target.Namespace != "" {
			ns = a.Target.Namespace
		}
		kind = a.Target.Kind
		name = a.Target.Name
	}
	if name == "" && run.Target != nil {
		if run.Target.Namespace != "" {
			ns = run.Target.Namespace
		}
		if kind == "" {
			kind = run.Target.Kind
		}
		name = run.Target.Name
	}
	return ns, kind, name
}
