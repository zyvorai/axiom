//go:build kubernetes

package platform

import (
	"context"
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

func (k *KubernetesProvider) ListAgentRuns(ctx context.Context, ns string) ([]AgentRun, error) {
	if k.dynamic == nil {
		return []AgentRun{}, nil
	}
	list, err := k.dynamic.Resource(agentRunGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isMissingCRD(err) {
			return []AgentRun{}, nil
		}
		return nil, err
	}
	out := make([]AgentRun, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, agentRunFromUnstructured(&list.Items[i]))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func (k *KubernetesProvider) GetAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	if k.dynamic == nil {
		return AgentRun{}, fmt.Errorf("dynamic client unavailable")
	}
	obj, err := k.dynamic.Resource(agentRunGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return AgentRun{}, err
	}
	return agentRunFromUnstructured(obj), nil
}

func (k *KubernetesProvider) CreateAgentRun(ctx context.Context, req CreateAgentRunRequest) (AgentRun, error) {
	if k.dynamic == nil {
		return AgentRun{}, fmt.Errorf("dynamic client unavailable")
	}
	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = "default"
	}
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		return AgentRun{}, fmt.Errorf("goal is required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("run-%d", time.Now().Unix()%1000000)
	}
	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "platform.example.com/v1",
		"kind":       "AgentRun",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]any{
			"goal":     goal,
			"autonomy": "ProposeOnly",
			"dryRun":   dryRun,
		},
	}}
	if req.ExtraContext != "" {
		_ = unstructured.SetNestedField(obj.Object, req.ExtraContext, "spec", "extraContext")
	}
	if req.Target != nil && req.Target.Name != "" && req.Target.Kind != "" {
		target := map[string]any{
			"kind": req.Target.Kind,
			"name": req.Target.Name,
		}
		if req.Target.APIVersion != "" {
			target["apiVersion"] = req.Target.APIVersion
		}
		tns := req.Target.Namespace
		if tns == "" {
			tns = ns
		}
		target["namespace"] = tns
		_ = unstructured.SetNestedMap(obj.Object, target, "spec", "target")
	}
	created, err := k.dynamic.Resource(agentRunGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return AgentRun{}, err
	}
	return agentRunFromUnstructured(created), nil
}

func (k *KubernetesProvider) ApproveAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	return k.annotateAgentRun(ctx, ns, name, agentAnnotationApproved, "true")
}

func (k *KubernetesProvider) DenyAgentRun(ctx context.Context, ns, name string) (AgentRun, error) {
	return k.annotateAgentRun(ctx, ns, name, agentAnnotationDenied, "true")
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
	return agentRunFromUnstructured(obj), nil
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

	canAct := strings.EqualFold(phase, "Proposed") &&
		(strings.EqualFold(approval, "Pending") || approval == "")
	anns := obj.GetAnnotations()
	if anns[agentAnnotationApproved] == "true" || anns[agentAnnotationDenied] == "true" {
		canAct = false
	}

	return AgentRun{
		Namespace: obj.GetNamespace(), Name: obj.GetName(),
		Goal: goal, Autonomy: autonomy, DryRun: dryRun, Target: target,
		Phase: phase, Approval: approval, Message: message, Risk: risk,
		Plan: plan, Executed: executed,
		Age: age(obj.GetCreationTimestamp().Time),
		CanApprove: canAct, CanDeny: canAct,
	}
}
