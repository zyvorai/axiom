//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var (
	agentRunGVR = schema.GroupVersionResource{
		Group: "platform.example.com", Version: "v1", Resource: "agentruns",
	}
	platformIntentGVR = schema.GroupVersionResource{
		Group: "platform.example.com", Version: "v1", Resource: "intents",
	}
)

const (
	agentAnnotationApproved = "platform.example.com/approved"
	agentAnnotationDenied   = "platform.example.com/denied"
)

func (k *KubernetesProvider) agentCM() *K8sAgentRunStore {
	if k == nil || k.client == nil {
		return nil
	}
	return NewK8sAgentRunStore(k.client, "")
}

func (k *KubernetesProvider) ListAgentRuns(ctx context.Context, ns string) ([]AgentRun, error) {
	crd, err := k.listAgentRunsCRD(ctx, ns)
	if err != nil && !isMissingCRD(err) {
		return nil, err
	}
	var cm []AgentRun
	if store := k.agentCM(); store != nil {
		var cmErr error
		cm, cmErr = store.List(ctx, ns)
		if cmErr != nil {
			if len(crd) == 0 {
				return nil, cmErr
			}
			cm = nil
		}
	}
	out := append(crd, cm...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func (k *KubernetesProvider) listAgentRunsCRD(ctx context.Context, ns string) ([]AgentRun, error) {
	if k.dynamic == nil {
		return []AgentRun{}, nil
	}
	list, err := k.dynamic.Resource(agentRunGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]AgentRun, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, agentRunFromUnstructured(&list.Items[i]))
	}
	return out, nil
}

func (k *KubernetesProvider) GetAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	if k.dynamic != nil {
		obj, err := k.dynamic.Resource(agentRunGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			return agentRunFromUnstructured(obj), nil
		}
		if !isMissingCRD(err) && !isNotFound(err) {
			return AgentRun{}, err
		}
	}
	if store := k.agentCM(); store != nil {
		return store.Get(ctx, ns, name)
	}
	return AgentRun{}, fmt.Errorf("agent run %s/%s not found", ns, name)
}

func (k *KubernetesProvider) CreateAgentRun(ctx context.Context, req CreateAgentRunRequest) (AgentRun, error) {
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		return AgentRun{}, fmt.Errorf("goal is required")
	}
	run := newStoredAgentRun(req)

	if k.dynamic != nil {
		obj := agentRunToUnstructured(run, req.ExtraContext)
		created, err := k.dynamic.Resource(agentRunGVR).Namespace(run.Namespace).Create(ctx, obj, metav1.CreateOptions{})
		if err == nil {
			out := agentRunFromUnstructured(created)
			out.OwnedByAxiom = true
			out.Source = run.Source
			return out, nil
		}
		if !isMissingCRD(err) {
			return AgentRun{}, err
		}
	}
	if store := k.agentCM(); store != nil {
		return store.Create(ctx, run)
	}
	return AgentRun{}, fmt.Errorf("dynamic client unavailable")
}

func (k *KubernetesProvider) UpdateAgentRun(ctx context.Context, run AgentRun) (AgentRun, error) {
	if k.dynamic != nil {
		obj, err := k.dynamic.Resource(agentRunGVR).Namespace(run.Namespace).Get(ctx, run.Name, metav1.GetOptions{})
		if err == nil {
			return k.patchAgentRunCRD(ctx, obj, run)
		}
		if !isMissingCRD(err) && !isNotFound(err) {
			return AgentRun{}, err
		}
	}
	if store := k.agentCM(); store != nil {
		return store.Update(ctx, run)
	}
	return AgentRun{}, fmt.Errorf("agent run %s/%s not found", run.Namespace, run.Name)
}

func (k *KubernetesProvider) patchAgentRunCRD(ctx context.Context, obj *unstructured.Unstructured, run AgentRun) (AgentRun, error) {
	status := map[string]any{
		"phase":    run.Phase,
		"approval": run.Approval,
		"message":  run.Message,
	}
	if run.Plan != nil {
		plan := map[string]any{
			"summary":          run.Plan.Summary,
			"rootCause":        run.Plan.RootCause,
			"risk":             run.Plan.Risk,
			"requiresApproval": true,
		}
		actions := make([]any, 0, len(run.Plan.Actions))
		for _, a := range run.Plan.Actions {
			am := map[string]any{
				"id": a.ID, "type": a.Type, "risk": a.Risk, "description": a.Description,
			}
			if a.Target != nil {
				am["target"] = map[string]any{
					"apiVersion": a.Target.APIVersion, "kind": a.Target.Kind,
					"namespace": a.Target.Namespace, "name": a.Target.Name,
				}
			}
			if len(a.Params) > 0 {
				am["params"] = a.Params
			}
			actions = append(actions, am)
		}
		plan["actions"] = actions
		status["plan"] = plan
	}
	if len(run.Executed) > 0 {
		ex := make([]any, 0, len(run.Executed))
		for _, e := range run.Executed {
			ex = append(ex, map[string]any{"id": e.ID, "type": e.Type, "ok": e.OK, "message": e.Message})
		}
		status["executed"] = ex
	}
	anns := obj.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	if run.OwnedByAxiom {
		anns[AgentOwnedAnnotation] = "true"
	}
	if run.Source != "" {
		anns["axiom.zyvor.dev/source"] = run.Source
	}
	patch := map[string]any{
		"metadata": map[string]any{"annotations": anns},
		"status":   status,
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return AgentRun{}, err
	}
	updated, err := k.dynamic.Resource(agentRunGVR).Namespace(run.Namespace).Patch(
		ctx, run.Name, types.MergePatchType, raw, metav1.PatchOptions{}, "status",
	)
	if err != nil {
		// Some CRDs lack a status subresource — fall back to full object merge.
		updated, err = k.dynamic.Resource(agentRunGVR).Namespace(run.Namespace).Patch(
			ctx, run.Name, types.MergePatchType, raw, metav1.PatchOptions{},
		)
		if err != nil {
			return AgentRun{}, err
		}
	}
	out := agentRunFromUnstructured(updated)
	out.OwnedByAxiom = run.OwnedByAxiom
	out.Source = run.Source
	return out, nil
}

func (k *KubernetesProvider) ApproveAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	run, err := k.GetAgentRun(ctx, ns, name)
	if err != nil {
		return AgentRun{}, err
	}
	if !agentRunCanDecide(run) {
		return AgentRun{}, errAgentRunNotPending()
	}
	// Axiom-owned runs (ConfigMap or annotated CR): execute allowlisted actions.
	if run.OwnedByAxiom {
		executed, err := ExecuteAgentRun(ctx, k, run)
		if err != nil {
			return AgentRun{}, err
		}
		return k.UpdateAgentRun(ctx, executed)
	}
	// External Hermes controller path: annotate only.
	if k.dynamic != nil {
		if _, err := k.dynamic.Resource(agentRunGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{}); err == nil {
			return k.annotateAgentRun(ctx, ns, name, agentAnnotationApproved, "true")
		}
	}
	if store := k.agentCM(); store != nil {
		run.Approval = "Approved"
		run.Phase = "Succeeded"
		agentRunDecideFlags(&run)
		return store.Update(ctx, run)
	}
	return AgentRun{}, fmt.Errorf("agent run %s/%s not found", ns, name)
}

func (k *KubernetesProvider) DenyAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	run, err := k.GetAgentRun(ctx, ns, name)
	if err != nil {
		return AgentRun{}, err
	}
	if !agentRunCanDecide(run) {
		return AgentRun{}, errAgentRunNotPending()
	}
	if k.dynamic != nil {
		if obj, err := k.dynamic.Resource(agentRunGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{}); err == nil {
			_ = obj
			return k.annotateAgentRun(ctx, ns, name, agentAnnotationDenied, "true")
		} else if !isMissingCRD(err) && !isNotFound(err) {
			return AgentRun{}, err
		}
	}
	run.Approval = "Denied"
	run.Phase = "Denied"
	run.Message = "denied by operator"
	agentRunDecideFlags(&run)
	return k.UpdateAgentRun(ctx, run)
}

func (k *KubernetesProvider) annotateAgentRun(ctx context.Context, ns, name, key, value string) (AgentRun, error) {
	if k.dynamic == nil {
		return AgentRun{}, fmt.Errorf("dynamic client unavailable")
	}
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, key, value)
	obj, err := k.dynamic.Resource(agentRunGVR).Namespace(ns).Patch(
		ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{},
	)
	if err != nil {
		return AgentRun{}, err
	}
	out := agentRunFromUnstructured(obj)
	if key == agentAnnotationApproved {
		out.Approval = "Approved"
	}
	if key == agentAnnotationDenied {
		out.Approval = "Denied"
		out.Phase = "Denied"
	}
	agentRunDecideFlags(&out)
	return out, nil
}

func (k *KubernetesProvider) ListPlatformIntents(ctx context.Context, ns string) ([]PlatformIntent, error) {
	if k.dynamic == nil {
		return []PlatformIntent{}, nil
	}
	list, err := k.dynamic.Resource(platformIntentGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isMissingCRD(err) {
			return []PlatformIntent{}, nil
		}
		return nil, err
	}
	out := make([]PlatformIntent, 0, len(list.Items))
	for _, item := range list.Items {
		wk, _, _ := unstructured.NestedString(item.Object, "spec", "workloadKind")
		img, _, _ := unstructured.NestedString(item.Object, "spec", "image")
		cpu, _, _ := unstructured.NestedString(item.Object, "spec", "cpu")
		mem, _, _ := unstructured.NestedString(item.Object, "spec", "memory")
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		msg, _, _ := unstructured.NestedString(item.Object, "status", "message")
		applied, _, _ := unstructured.NestedString(item.Object, "status", "appliedTo")
		var replicas int64
		if r, ok, _ := unstructured.NestedInt64(item.Object, "spec", "replicas"); ok {
			replicas = r
		}
		specName, _, _ := unstructured.NestedString(item.Object, "spec", "name")
		if specName == "" {
			specName = item.GetName()
		}
		if phase == "" {
			phase = "Pending"
		}
		out = append(out, PlatformIntent{
			Namespace: item.GetNamespace(), Name: item.GetName(),
			WorkloadKind: wk, Replicas: int32(replicas), Image: img, CPU: cpu, Memory: mem,
			Phase: phase, Message: msg, AppliedTo: applied,
			Age: age(item.GetCreationTimestamp().Time),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func agentRunToUnstructured(run AgentRun, extra string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "platform.example.com/v1",
		"kind":       "AgentRun",
		"metadata": map[string]any{
			"name":      run.Name,
			"namespace": run.Namespace,
			"annotations": map[string]any{
				AgentOwnedAnnotation:     "true",
				"axiom.zyvor.dev/source": run.Source,
			},
		},
		"spec": map[string]any{
			"goal":     run.Goal,
			"autonomy": "ProposeOnly",
			"dryRun":   run.DryRun,
		},
	}}
	if extra != "" {
		_ = unstructured.SetNestedField(obj.Object, extra, "spec", "extraContext")
	}
	if run.Target != nil && run.Target.Name != "" && run.Target.Kind != "" {
		target := map[string]any{
			"kind": run.Target.Kind,
			"name": run.Target.Name,
		}
		if run.Target.APIVersion != "" {
			target["apiVersion"] = run.Target.APIVersion
		}
		tns := run.Target.Namespace
		if tns == "" {
			tns = run.Namespace
		}
		target["namespace"] = tns
		_ = unstructured.SetNestedMap(obj.Object, target, "spec", "target")
	}
	return obj
}

func agentRunFromUnstructured(obj *unstructured.Unstructured) AgentRun {
	goal, _, _ := unstructured.NestedString(obj.Object, "spec", "goal")
	autonomy, _, _ := unstructured.NestedString(obj.Object, "spec", "autonomy")
	if autonomy == "" {
		autonomy = "ProposeOnly"
	}
	dryRun, _, _ := unstructured.NestedBool(obj.Object, "spec", "dryRun")
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	if phase == "" {
		phase = "Pending"
	}
	approval, _, _ := unstructured.NestedString(obj.Object, "status", "approval")
	if approval == "" {
		approval = "Pending"
	}
	message, _, _ := unstructured.NestedString(obj.Object, "status", "message")
	risk, _, _ := unstructured.NestedString(obj.Object, "status", "plan", "risk")

	var target *AgentObjectRef
	if tm, ok, _ := unstructured.NestedMap(obj.Object, "spec", "target"); ok && tm != nil {
		t := &AgentObjectRef{}
		if v, _ := tm["apiVersion"].(string); v != "" {
			t.APIVersion = v
		}
		if v, _ := tm["kind"].(string); v != "" {
			t.Kind = v
		}
		if v, _ := tm["namespace"].(string); v != "" {
			t.Namespace = v
		}
		if v, _ := tm["name"].(string); v != "" {
			t.Name = v
		}
		if t.Name != "" {
			target = t
		}
	}

	var plan *AgentPlan
	if pm, ok, _ := unstructured.NestedMap(obj.Object, "status", "plan"); ok && pm != nil {
		p := &AgentPlan{}
		if v, _ := pm["summary"].(string); v != "" {
			p.Summary = v
		}
		if v, _ := pm["rootCause"].(string); v != "" {
			p.RootCause = v
		}
		if v, _ := pm["risk"].(string); v != "" {
			p.Risk = v
			risk = v
		}
		if v, ok := pm["requiresApproval"].(bool); ok {
			p.RequiresApproval = v
		}
		if raw, ok := pm["actions"].([]any); ok {
			for _, a := range raw {
				am, ok := a.(map[string]any)
				if !ok {
					continue
				}
				act := AgentAction{}
				if v, _ := am["id"].(string); v != "" {
					act.ID = v
				}
				if v, _ := am["type"].(string); v != "" {
					act.Type = v
				}
				if v, _ := am["risk"].(string); v != "" {
					act.Risk = v
				}
				if v, _ := am["description"].(string); v != "" {
					act.Description = v
				}
				if tm, ok := am["target"].(map[string]any); ok {
					t := &AgentObjectRef{}
					if v, _ := tm["apiVersion"].(string); v != "" {
						t.APIVersion = v
					}
					if v, _ := tm["kind"].(string); v != "" {
						t.Kind = v
					}
					if v, _ := tm["namespace"].(string); v != "" {
						t.Namespace = v
					}
					if v, _ := tm["name"].(string); v != "" {
						t.Name = v
					}
					act.Target = t
				}
				if pm, ok := am["params"].(map[string]any); ok {
					act.Params = map[string]string{}
					for k, v := range pm {
						if s, ok := v.(string); ok {
							act.Params[k] = s
						}
					}
				}
				p.Actions = append(p.Actions, act)
			}
		}
		plan = p
	}

	var executed []AgentExecuted
	if raw, ok, _ := unstructured.NestedSlice(obj.Object, "status", "executed"); ok {
		for _, a := range raw {
			am, ok := a.(map[string]any)
			if !ok {
				continue
			}
			ex := AgentExecuted{}
			if v, _ := am["id"].(string); v != "" {
				ex.ID = v
			}
			if v, _ := am["type"].(string); v != "" {
				ex.Type = v
			}
			if v, ok := am["ok"].(bool); ok {
				ex.OK = v
			}
			if v, _ := am["message"].(string); v != "" {
				ex.Message = v
			}
			executed = append(executed, ex)
		}
	}

	anns := obj.GetAnnotations()
	owned := anns[AgentOwnedAnnotation] == "true"
	src := anns["axiom.zyvor.dev/source"]
	if anns[agentAnnotationApproved] == "true" {
		approval = "Approved"
	}
	if anns[agentAnnotationDenied] == "true" {
		approval = "Denied"
		if phase == "Pending" || phase == "Proposed" {
			phase = "Denied"
		}
	}

	run := AgentRun{
		Namespace: obj.GetNamespace(), Name: obj.GetName(),
		Goal: goal, Autonomy: autonomy, DryRun: dryRun, Target: target,
		Phase: phase, Approval: approval, Message: message, Risk: risk,
		Plan: plan, Executed: executed,
		Age: age(obj.GetCreationTimestamp().Time),
		OwnedByAxiom: owned, Source: src,
		CreatedAt: obj.GetCreationTimestamp().UTC().Format(time.RFC3339),
	}
	agentRunDecideFlags(&run)
	return run
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not found")
}
