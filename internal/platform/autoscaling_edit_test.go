package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// liveFromPlan is a created object as a cluster would return it, plus the
// unmanaged extras every edit must preserve.
func liveVPA(t *testing.T) map[string]any {
	t.Helper()
	plan, err := buildVPA(baseVPA())
	return liveFromPlan(t, plan, err)
}

func liveSO(t *testing.T) map[string]any {
	t.Helper()
	plan, err := buildScaledObject(baseSO())
	return liveFromPlan(t, plan, err)
}

func liveFromPlan(t *testing.T, plan autoscalerPlan, err error) map[string]any {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := jsonNormalize(plan.Object.withVersion(plan.Object.Version)).(map[string]any)
	meta := obj["metadata"].(map[string]any)
	meta["resourceVersion"] = "42"
	meta["annotations"] = map[string]any{"owner": "team-a", "kubectl.kubernetes.io/last-applied-configuration": `{"leaked":"old-spec"}`}
	meta["finalizers"] = []any{"example.com/keep"}
	obj["spec"].(map[string]any)["unknownField"] = "kept"
	obj["status"] = map[string]any{"phase": "Ready"}
	return obj
}

func assertExtrasKept(t *testing.T, after map[string]any) {
	t.Helper()
	if nestedString(after, "metadata", "annotations", "owner") != "team-a" || len(nestedStrings(after, "metadata", "finalizers")) != 1 ||
		nestedString(after, "spec", "unknownField") != "kept" || nestedString(after, "status", "phase") != "Ready" {
		t.Fatalf("unmanaged fields lost: %+v", after)
	}
}

func autoscalerPatchOps(t *testing.T, p []byte) []map[string]any {
	t.Helper()
	var ops []map[string]any
	if err := json.Unmarshal(p, &ops); err != nil {
		t.Fatal(err)
	}
	if len(ops) < 2 || ops[0]["op"] != "test" || ops[0]["path"] != "/metadata/resourceVersion" || ops[0]["value"] != "42" {
		t.Fatalf("patch must start with the resourceVersion test: %s", p)
	}
	for _, op := range ops[1:] {
		path, _ := op["path"].(string)
		if !strings.HasPrefix(path, "/spec/") {
			t.Fatalf("patch touches an unmanaged path: %s", p)
		}
	}
	return ops
}

func vpaEditReq(ed VPAEditable, mut func(*VPAEditRequest)) VPAEditRequest {
	r := VPAEditRequest{VPACreateRequest: VPACreateRequest{
		Namespace: ed.Namespace, Name: ed.Name, TargetKind: ed.TargetKind, TargetName: ed.TargetName,
		UpdateMode: ed.UpdateMode, ControlledResources: ed.ControlledResources, Containers: ed.Containers,
	}, ResourceVersion: ed.ResourceVersion}
	if mut != nil {
		mut(&r)
	}
	return r
}

func soEditReq(ed ScaledObjectEditable, mut func(*ScaledObjectEditRequest)) ScaledObjectEditRequest {
	minR := ed.MinReplicas
	r := ScaledObjectEditRequest{ScaledObjectCreateRequest: ScaledObjectCreateRequest{
		Namespace: ed.Namespace, Name: ed.Name, TargetKind: ed.TargetKind, TargetName: ed.TargetName,
		MinReplicas: &minR, MaxReplicas: ed.MaxReplicas, PollingInterval: ed.PollingInterval, CooldownPeriod: ed.CooldownPeriod,
		Triggers: append([]ScaledObjectTriggerRequest{}, ed.Triggers...),
	}, ResourceVersion: ed.ResourceVersion}
	if mut != nil {
		mut(&r)
	}
	return r
}

func TestVPAEditRoundTrip(t *testing.T) {
	obj := liveVPA(t)
	ed := readVPAEditable(obj)
	if !ed.Editable || ed.ResourceVersion != "42" || ed.UpdateMode != "Off" || len(ed.Containers) != 1 ||
		ed.Containers[0] != (VPAContainerPolicyRequest{Name: "app", MinCPU: "50m", MaxCPU: "2", MinMemory: "64Mi", MaxMemory: "2Gi"}) ||
		strings.Join(ed.ReadOnly, ",") != "namespace,name,targetKind,targetName" || strings.Join(ed.ControlledResources, ",") != "cpu,memory" {
		t.Fatalf("editable: %+v", ed)
	}

	// Unchanged body: noop, no patch.
	plan, err := planVPAEdit(obj, vpaEditReq(ed, nil))
	if err != nil || !plan.Preview.Noop || plan.Patch != nil || len(plan.Preview.Changes) != 0 {
		t.Fatalf("noop: %v %+v", err, plan.Preview)
	}
	// Equal quantities written differently are not a change.
	plan, err = planVPAEdit(obj, vpaEditReq(ed, func(r *VPAEditRequest) {
		r.Containers = []VPAContainerPolicyRequest{{Name: "app", MinCPU: "0.05", MaxCPU: "2000m", MinMemory: "64Mi", MaxMemory: "2Gi"}}
	}))
	if err != nil || !plan.Preview.Noop {
		t.Fatalf("equal quantities: %v %+v", err, plan.Preview.Changes)
	}

	// Mode change + bound change + added container.
	plan, err = planVPAEdit(obj, vpaEditReq(ed, func(r *VPAEditRequest) {
		r.UpdateMode = "Auto"
		r.Containers = []VPAContainerPolicyRequest{{Name: "app", MinCPU: "50m", MaxCPU: "4", MinMemory: "64Mi", MaxMemory: "2Gi"}, {Name: "sidecar", MaxMemory: "256Mi"}}
	}))
	if err != nil || plan.Preview.Noop {
		t.Fatalf("edit: %v", err)
	}
	want := []GitOpsEditChange{
		{Object: "VerticalPodAutoscaler/commerce/checkout-vpa", Field: "spec.updatePolicy.updateMode", From: "Off", To: "Auto"},
		{Object: "VerticalPodAutoscaler/commerce/checkout-vpa", Field: "spec.resourcePolicy.containerPolicies[app]",
			From: "controlledResources=cpu,memory minAllowed.cpu=50m minAllowed.memory=64Mi maxAllowed.cpu=2 maxAllowed.memory=2Gi",
			To:   "controlledResources=cpu,memory minAllowed.cpu=50m minAllowed.memory=64Mi maxAllowed.cpu=4 maxAllowed.memory=2Gi"},
		{Object: "VerticalPodAutoscaler/commerce/checkout-vpa", Field: "spec.resourcePolicy.containerPolicies[sidecar]",
			From: "", To: "controlledResources=cpu,memory maxAllowed.memory=256Mi"},
	}
	if len(plan.Preview.Changes) != len(want) {
		t.Fatalf("changes: %+v", plan.Preview.Changes)
	}
	for i := range want {
		if plan.Preview.Changes[i] != want[i] {
			t.Fatalf("change %d: %+v, want %+v", i, plan.Preview.Changes[i], want[i])
		}
	}
	if strings.Join(plan.Fields, ",") != "spec.updatePolicy.updateMode,spec.resourcePolicy.containerPolicies[app],spec.resourcePolicy.containerPolicies[sidecar]" {
		t.Fatalf("fields: %v", plan.Fields)
	}
	if len(plan.Preview.Warnings) != 1 || !strings.Contains(plan.Preview.Warnings[0], "pods may be restarted") {
		t.Fatalf("Auto warning: %v", plan.Preview.Warnings)
	}
	ops := autoscalerPatchOps(t, plan.Patch)
	if len(ops) != 3 || ops[1]["path"] != "/spec/updatePolicy/updateMode" || ops[2]["path"] != "/spec/resourcePolicy/containerPolicies" {
		t.Fatalf("ops: %s", plan.Patch)
	}
	assertExtrasKept(t, plan.After)
	if !strings.Contains(plan.Preview.YAML, "updateMode: Auto") || !strings.Contains(plan.Preview.YAML, "containerName: sidecar") ||
		!strings.Contains(plan.Preview.YAML, "unknownField: kept") || strings.Contains(plan.Preview.YAML, "old-spec") {
		t.Fatalf("yaml: %s", plan.Preview.YAML)
	}

	// Removing every container policy removes resourcePolicy.
	plan, err = planVPAEdit(obj, vpaEditReq(ed, func(r *VPAEditRequest) { r.Containers = []VPAContainerPolicyRequest{} }))
	if err != nil || len(plan.Preview.Changes) != 1 || plan.Preview.Changes[0].Field != "spec.resourcePolicy.containerPolicies[app]" || plan.Preview.Changes[0].To != "" {
		t.Fatalf("remove: %v %+v", err, plan.Preview.Changes)
	}
	if ops := autoscalerPatchOps(t, plan.Patch); ops[1]["op"] != "remove" || ops[1]["path"] != "/spec/resourcePolicy" {
		t.Fatalf("remove ops: %s", plan.Patch)
	}
	// Omitted containers (null) keep the current ones.
	plan, err = planVPAEdit(obj, vpaEditReq(ed, func(r *VPAEditRequest) { r.Containers = nil; r.UpdateMode = "" }))
	if err != nil || !plan.Preview.Noop {
		t.Fatalf("omitted keeps: %v %+v", err, plan.Preview.Changes)
	}
	// Restricting to cpu without containers adds the "*" carrier policy.
	plan, err = planVPAEdit(obj, vpaEditReq(ed, func(r *VPAEditRequest) {
		r.ControlledResources = []string{"cpu"}
		r.Containers = []VPAContainerPolicyRequest{}
	}))
	if err != nil || len(plan.Preview.Changes) != 2 || plan.Preview.Changes[0].Field != "spec.resourcePolicy.containerPolicies[*]" {
		t.Fatalf("carrier: %v %+v", err, plan.Preview.Changes)
	}
	carrierObj := plan.After
	carrierObj["metadata"].(map[string]any)["resourceVersion"] = "43"
	if ced := readVPAEditable(carrierObj); !ced.Editable || len(ced.Containers) != 0 || strings.Join(ced.ControlledResources, ",") != "cpu" {
		t.Fatalf("carrier read back: %+v", ced)
	}

	// Validation as create.
	if _, err := planVPAEdit(obj, vpaEditReq(ed, func(r *VPAEditRequest) { r.UpdateMode = "Sometimes" })); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("bad mode: %v", err)
	}
}

func TestVPAEditUnsetUpdateModeIsAuto(t *testing.T) {
	obj := liveVPA(t)
	delete(obj["spec"].(map[string]any), "updatePolicy")
	ed := readVPAEditable(obj)
	if ed.UpdateMode != "Auto" {
		t.Fatalf("unset mode: %q", ed.UpdateMode)
	}
	if plan, err := planVPAEdit(obj, vpaEditReq(ed, nil)); err != nil || !plan.Preview.Noop {
		t.Fatalf("unset mode noop: %v %+v", err, plan.Preview.Changes)
	}
	plan, err := planVPAEdit(obj, vpaEditReq(ed, func(r *VPAEditRequest) { r.UpdateMode = "Off" }))
	if err != nil || len(plan.Preview.Changes) != 1 || plan.Preview.Changes[0].From != "" || plan.Preview.Changes[0].To != "Off" {
		t.Fatalf("set mode: %v %+v", err, plan.Preview.Changes)
	}
	if ops := autoscalerPatchOps(t, plan.Patch); ops[1]["path"] != "/spec/updatePolicy" {
		t.Fatalf("ops: %s", plan.Patch)
	}
}

func TestAutoscalerEditGuards(t *testing.T) {
	vpa := liveVPA(t)
	ved := readVPAEditable(vpa)
	so := liveSO(t)
	sed := readScaledObjectEditable(so)
	for name, err := range map[string]error{
		"vpa name":        second(planVPAEdit(vpa, vpaEditReq(ved, func(r *VPAEditRequest) { r.Name = "other" }))),
		"vpa namespace":   second(planVPAEdit(vpa, vpaEditReq(ved, func(r *VPAEditRequest) { r.Namespace = "other" }))),
		"vpa targetKind":  second(planVPAEdit(vpa, vpaEditReq(ved, func(r *VPAEditRequest) { r.TargetKind = "StatefulSet" }))),
		"vpa targetName":  second(planVPAEdit(vpa, vpaEditReq(ved, func(r *VPAEditRequest) { r.TargetName = "other" }))),
		"so targetName":   second(planScaledObjectEdit(so, soEditReq(sed, func(r *ScaledObjectEditRequest) { r.TargetName = "other" }))),
		"so targetKind":   second(planScaledObjectEdit(so, soEditReq(sed, func(r *ScaledObjectEditRequest) { r.TargetKind = "StatefulSet" }))),
		"vpa missing rv":  second(planVPAEdit(vpa, vpaEditReq(ved, func(r *VPAEditRequest) { r.ResourceVersion = "" }))),
		"so bad trigger":  second(planScaledObjectEdit(so, soEditReq(sed, func(r *ScaledObjectEditRequest) { r.Triggers[0].Type = "mysql" }))),
		"so max too high": second(planScaledObjectEdit(so, soEditReq(sed, func(r *ScaledObjectEditRequest) { r.MaxReplicas = 1 << 20 }))),
	} {
		if HTTPStatus(err) != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %v", name, err)
		}
	}
	_, err := planVPAEdit(vpa, vpaEditReq(ved, func(r *VPAEditRequest) { r.TargetName = "other" }))
	if err == nil || !strings.Contains(err.Error(), "targetName cannot be changed") {
		t.Fatalf("immutable message: %v", err)
	}
	for name, err := range map[string]error{
		"vpa": second(planVPAEdit(vpa, vpaEditReq(ved, func(r *VPAEditRequest) { r.ResourceVersion = "41" }))),
		"so":  second(planScaledObjectEdit(so, soEditReq(sed, func(r *ScaledObjectEditRequest) { r.ResourceVersion = "41" }))),
	} {
		if HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "the object changed since you opened it") {
			t.Errorf("%s stale: %v", name, err)
		}
	}
}

func second[T any](_ T, err error) error { return err }

func TestScaledObjectEditRoundTrip(t *testing.T) {
	obj := liveSO(t)
	ed := readScaledObjectEditable(obj)
	if !ed.Editable || ed.MinReplicas != 0 || ed.MaxReplicas != 20 || ed.PollingInterval != 30 || ed.CooldownPeriod != 300 ||
		len(ed.Triggers) != 2 || ed.Triggers[1].AuthenticationRef != "prom-auth" || ed.Triggers[0].Settings["timezone"] != "Europe/Berlin" {
		t.Fatalf("editable: %+v", ed)
	}
	if plan, err := planScaledObjectEdit(obj, soEditReq(ed, nil)); err != nil || !plan.Preview.Noop || plan.Patch != nil {
		t.Fatalf("noop: %v %+v", err, plan.Preview.Changes)
	}

	// Numbers.
	plan, err := planScaledObjectEdit(obj, soEditReq(ed, func(r *ScaledObjectEditRequest) {
		r.MaxReplicas, r.PollingInterval, r.CooldownPeriod = 30, 15, 600
		one := int32(1)
		r.MinReplicas = &one
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Preview.Changes) != 4 || plan.Preview.Changes[1] != (GitOpsEditChange{Object: "ScaledObject/payments/payments-api-scaler", Field: "spec.maxReplicaCount", From: "20", To: "30"}) ||
		plan.Preview.Changes[0].Field != "spec.minReplicaCount" || plan.Preview.Changes[2].Field != "spec.pollingInterval" || plan.Preview.Changes[3].To != "600" {
		t.Fatalf("number changes: %+v", plan.Preview.Changes)
	}
	autoscalerPatchOps(t, plan.Patch)
	assertExtrasKept(t, plan.After)

	// Trigger setting change.
	plan, _ = planScaledObjectEdit(obj, soEditReq(ed, func(r *ScaledObjectEditRequest) {
		r.Triggers[1] = ScaledObjectTriggerRequest{Type: "prometheus", AuthenticationRef: "prom-auth",
			Settings: map[string]string{"serverAddress": "http://prometheus.monitoring:9090", "query": "sum(rate(http_requests_total[2m]))", "threshold": "200"}}
	}))
	if len(plan.Preview.Changes) != 1 || plan.Preview.Changes[0].Field != "spec.triggers[1]" ||
		!strings.Contains(plan.Preview.Changes[0].From, "threshold=100") || !strings.Contains(plan.Preview.Changes[0].To, "threshold=200") {
		t.Fatalf("trigger change: %+v", plan.Preview.Changes)
	}
	if ops := autoscalerPatchOps(t, plan.Patch); len(ops) != 2 || ops[1]["path"] != "/spec/triggers" {
		t.Fatalf("trigger ops: %s", plan.Patch)
	}

	// Removal.
	plan, _ = planScaledObjectEdit(obj, soEditReq(ed, func(r *ScaledObjectEditRequest) { r.Triggers = r.Triggers[:1] }))
	if len(plan.Preview.Changes) != 1 || plan.Preview.Changes[0].Field != "spec.triggers[1]" || plan.Preview.Changes[0].To != "" {
		t.Fatalf("removal: %+v", plan.Preview.Changes)
	}
	// Addition.
	plan, err = planScaledObjectEdit(obj, soEditReq(ed, func(r *ScaledObjectEditRequest) {
		r.Triggers = append(r.Triggers, ScaledObjectTriggerRequest{Type: "cpu", Settings: map[string]string{"type": "Utilization", "value": "70"}})
	}))
	if err != nil || len(plan.Preview.Changes) != 1 || plan.Preview.Changes[0].Field != "spec.triggers[2]" || plan.Preview.Changes[0].From != "" ||
		plan.Preview.Changes[0].To != "type=cpu metricType=Utilization value=70" {
		t.Fatalf("addition: %v %+v", err, plan.Preview.Changes)
	}
	// Reorder: both positions change.
	plan, _ = planScaledObjectEdit(obj, soEditReq(ed, func(r *ScaledObjectEditRequest) { r.Triggers[0], r.Triggers[1] = r.Triggers[1], r.Triggers[0] }))
	if len(plan.Preview.Changes) != 2 || plan.Preview.Changes[0].Field != "spec.triggers[0]" || !strings.HasPrefix(plan.Preview.Changes[0].To, "type=prometheus") {
		t.Fatalf("reorder: %+v", plan.Preview.Changes)
	}
	// Scale-to-zero rule as on create.
	if _, err := planScaledObjectEdit(obj, soEditReq(ed, func(r *ScaledObjectEditRequest) {
		r.Triggers = []ScaledObjectTriggerRequest{{Type: "cpu", Settings: map[string]string{"type": "Utilization", "value": "70"}}}
	})); HTTPStatus(err) != http.StatusBadRequest || !strings.Contains(err.Error(), "scale to zero") {
		t.Fatalf("min 0 cpu only: %v", err)
	}
	// An explicit authenticationRef kind TriggerAuthentication is not a diff.
	tr := obj["spec"].(map[string]any)["triggers"].([]any)[1].(map[string]any)
	tr["authenticationRef"].(map[string]any)["kind"] = "TriggerAuthentication"
	if plan, err := planScaledObjectEdit(obj, soEditReq(readScaledObjectEditable(obj), nil)); err != nil || !plan.Preview.Noop {
		t.Fatalf("explicit kind: %v %+v", err, plan.Preview.Changes)
	}
}

func TestScaledObjectEditDefaultsUnset(t *testing.T) {
	obj := liveSO(t)
	spec := obj["spec"].(map[string]any)
	delete(spec, "pollingInterval")
	delete(spec, "cooldownPeriod")
	delete(spec, "minReplicaCount")
	delete(spec, "maxReplicaCount")
	ed := readScaledObjectEditable(obj)
	if ed.MinReplicas != 0 || ed.MaxReplicas != 100 || ed.PollingInterval != 30 || ed.CooldownPeriod != 300 {
		t.Fatalf("defaults: %+v", ed)
	}
	if plan, err := planScaledObjectEdit(obj, soEditReq(ed, nil)); err != nil || !plan.Preview.Noop {
		t.Fatalf("defaults noop: %v %+v", err, plan.Preview.Changes)
	}
}

func TestAutoscalerEditNonEditable(t *testing.T) {
	vpaCases := map[string]func(o map[string]any){
		"container mode":   func(o map[string]any) { vpaPolicy(o)["mode"] = "Off" },
		"controlledValues": func(o map[string]any) { vpaPolicy(o)["controlledValues"] = "RequestsOnly" },
		"bound beyond cpu": func(o map[string]any) { vpaPolicy(o)["minAllowed"].(map[string]any)["nvidia.com/gpu"] = "1" },
		"recommenders":     func(o map[string]any) { spec(o)["recommenders"] = []any{map[string]any{"name": "custom"}} },
		"evictionRequirement": func(o map[string]any) {
			spec(o)["updatePolicy"].(map[string]any)["evictionRequirements"] = []any{map[string]any{"changeRequirement": "TargetHigherThanRequests"}}
		},
		"odd apiVersion": func(o map[string]any) { spec(o)["targetRef"].(map[string]any)["apiVersion"] = "argoproj.io/v1alpha1" },
		"odd kind":       func(o map[string]any) { spec(o)["targetRef"].(map[string]any)["kind"] = "Rollout" },
		"different controlled": func(o map[string]any) {
			rp := spec(o)["resourcePolicy"].(map[string]any)
			rp["containerPolicies"] = append(rp["containerPolicies"].([]any), map[string]any{"containerName": "b", "controlledResources": []any{"cpu"}})
		},
		"other resourcePolicy key": func(o map[string]any) { spec(o)["resourcePolicy"].(map[string]any)["extra"] = true },
	}
	for name, mut := range vpaCases {
		obj := liveVPA(t)
		mut(obj)
		ed := readVPAEditable(obj)
		if ed.Editable || len(ed.Reasons) == 0 {
			t.Errorf("vpa %s: should not be editable: %+v", name, ed)
			continue
		}
		if _, err := planVPAEdit(obj, vpaEditReq(ed, nil)); HTTPStatus(err) != http.StatusBadRequest || !strings.Contains(err.Error(), "cannot be edited here") {
			t.Errorf("vpa %s: PUT must be 400: %v", name, err)
		}
	}

	soCases := map[string]func(o map[string]any){
		"advanced":           func(o map[string]any) { spec(o)["advanced"] = map[string]any{"restoreToOriginalReplicaCount": true} },
		"fallback":           func(o map[string]any) { spec(o)["fallback"] = map[string]any{"failureThreshold": 3, "replicas": 6} },
		"envSourceContainer": func(o map[string]any) { spec(o)["scaleTargetRef"].(map[string]any)["envSourceContainerName"] = "app" },
		"odd apiVersion": func(o map[string]any) {
			spec(o)["scaleTargetRef"].(map[string]any)["apiVersion"] = "argoproj.io/v1alpha1"
		},
		"unknown type":    func(o map[string]any) { soTrigger(o, 0)["type"] = "mysql" },
		"unknown setting": func(o map[string]any) { soTrigger(o, 1)["metadata"].(map[string]any)["ignoreNullValues"] = "true" },
		"cluster auth": func(o map[string]any) {
			soTrigger(o, 1)["authenticationRef"].(map[string]any)["kind"] = "ClusterTriggerAuthentication"
		},
		"useCachedMetrics":    func(o map[string]any) { soTrigger(o, 1)["useCachedMetrics"] = true },
		"metricType on event": func(o map[string]any) { soTrigger(o, 1)["metricType"] = "Value" },
		"unknown trigger key": func(o map[string]any) { soTrigger(o, 1)["extra"] = "x" },
	}
	for name, mut := range soCases {
		obj := liveSO(t)
		mut(obj)
		ed := readScaledObjectEditable(obj)
		if ed.Editable || len(ed.Reasons) == 0 {
			t.Errorf("so %s: should not be editable: %+v", name, ed)
			continue
		}
		if _, err := planScaledObjectEdit(obj, soEditReq(ed, nil)); HTTPStatus(err) != http.StatusBadRequest {
			t.Errorf("so %s: PUT must be 400: %v", name, err)
		}
	}
	// Unknown spec keys alone do not block editing (they are preserved).
	obj := liveSO(t)
	spec(obj)["initialCooldownPeriod"] = int64(60)
	if ed := readScaledObjectEditable(obj); !ed.Editable {
		t.Fatalf("unknown spec key blocks: %+v", ed.Reasons)
	}
}

func spec(o map[string]any) map[string]any { return o["spec"].(map[string]any) }

func vpaPolicy(o map[string]any) map[string]any {
	return spec(o)["resourcePolicy"].(map[string]any)["containerPolicies"].([]any)[0].(map[string]any)
}

func soTrigger(o map[string]any, i int) map[string]any {
	return spec(o)["triggers"].([]any)[i].(map[string]any)
}

func TestScaledObjectEditNeverReturnsCredentials(t *testing.T) {
	const secret = "s3cr3t-hunter2"
	cases := map[string]func(o map[string]any){
		"userinfo URL": func(o map[string]any) {
			soTrigger(o, 1)["metadata"].(map[string]any)["serverAddress"] = "http://admin:" + secret + "@prom:9090"
		},
		"password key":     func(o map[string]any) { soTrigger(o, 1)["metadata"].(map[string]any)["password"] = secret },
		"bearer token key": func(o map[string]any) { soTrigger(o, 1)["metadata"].(map[string]any)["bearerToken"] = secret },
		"redis host userinf": func(o map[string]any) {
			spec(o)["triggers"] = append(spec(o)["triggers"].([]any), map[string]any{"type": "redis", "metadata": map[string]any{"address": "user:" + secret + "@redis:6379", "listName": "q", "listLength": "5"}})
		},
		"rabbitmq host": func(o map[string]any) {
			spec(o)["triggers"] = append(spec(o)["triggers"].([]any), map[string]any{"type": "rabbitmq", "metadata": map[string]any{"host": "amqp://u:" + secret + "@rabbit:5672", "queueName": "q", "mode": "QueueLength", "value": "5"}})
		},
	}
	for name, mut := range cases {
		obj := liveSO(t)
		mut(obj)
		ed := readScaledObjectEditable(obj)
		b, _ := json.Marshal(ed)
		if ed.Editable || strings.Contains(string(b), secret) {
			t.Errorf("%s: editable=%v, body %s", name, ed.Editable, b)
		}
		_, err := planScaledObjectEdit(obj, soEditReq(ed, nil))
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("%s: PUT error %v", name, err)
		}
	}
}

func TestDemoAutoscalerEditParity(t *testing.T) {
	ctx := context.Background()
	d := NewDemoProvider()

	// Seeded objects are editable.
	sed, err := d.GetScaledObjectEditable(ctx, "payments", "payments-api")
	if err != nil || !sed.Editable || sed.MinReplicas != 2 || len(sed.Triggers) != 2 || sed.Triggers[0].Name != "orders-lag" {
		t.Fatalf("seed so: %v %+v", err, sed)
	}
	ved, err := d.GetVPAEditable(ctx, "commerce", "checkout-vpa")
	if err != nil || !ved.Editable || ved.UpdateMode != "Auto" || len(ved.Containers) != 1 {
		t.Fatalf("seed vpa: %v %+v", err, ved)
	}
	if _, err := d.GetVPAEditable(ctx, "commerce", "nope"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("404: %v", err)
	}

	// Create → edit → stale.
	req := baseSO()
	req.Name = "parity-scaler"
	if _, err := d.CreateScaledObject(ctx, req); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteScaledObject(ctx, "payments", "parity-scaler") })
	ed, err := d.GetScaledObjectEditable(ctx, "payments", "parity-scaler")
	if err != nil || !ed.Editable || ed.ResourceVersion != "1" {
		t.Fatalf("get: %v %+v", err, ed)
	}
	p, err := d.PreviewScaledObjectEdit(ctx, "payments", "parity-scaler", soEditReq(ed, func(r *ScaledObjectEditRequest) { r.MaxReplicas = 30 }))
	if err != nil || len(p.Changes) != 1 || p.Changes[0].Field != "spec.maxReplicaCount" {
		t.Fatalf("preview: %v %+v", err, p)
	}
	res, err := d.UpdateScaledObject(ctx, "payments", "parity-scaler", soEditReq(ed, func(r *ScaledObjectEditRequest) { r.MaxReplicas = 30 }))
	if err != nil || !res.OK || res.ResourceVersion != "2" || strings.Join(res.Changed, ",") != "spec.maxReplicaCount" {
		t.Fatalf("update: %v %+v", err, res)
	}
	list, _ := d.ListScaledObjects(ctx, "payments")
	for _, s := range list.Items {
		if s.Name == "parity-scaler" && s.MaxReplicas != 30 {
			t.Fatalf("list not updated: %+v", s)
		}
	}
	if _, err := d.UpdateScaledObject(ctx, "payments", "parity-scaler", soEditReq(ed, func(r *ScaledObjectEditRequest) { r.MaxReplicas = 40 })); HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("stale: %v", err)
	}
	ed2, _ := d.GetScaledObjectEditable(ctx, "payments", "parity-scaler")
	if ed2.MaxReplicas != 30 || ed2.ResourceVersion != "2" {
		t.Fatalf("after: %+v", ed2)
	}
	// Unknown TriggerAuthentication: 400 as on create.
	if _, err := d.UpdateScaledObject(ctx, "payments", "parity-scaler", soEditReq(ed2, func(r *ScaledObjectEditRequest) { r.Triggers[1].AuthenticationRef = "nope-auth" })); HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("unknown auth: %v", err)
	}

	vreq := baseVPA()
	vreq.Name = "parity-vpa"
	if _, err := d.CreateVPA(ctx, vreq); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteVPA(ctx, "commerce", "parity-vpa") })
	ved, _ = d.GetVPAEditable(ctx, "commerce", "parity-vpa")
	vres, err := d.UpdateVPA(ctx, "commerce", "parity-vpa", vpaEditReq(ved, func(r *VPAEditRequest) { r.UpdateMode = "Initial" }))
	if err != nil || vres.ResourceVersion != "2" || len(vres.Warnings) < 2 {
		t.Fatalf("vpa update (mode + HPA conflict warnings): %v %+v", err, vres)
	}
	vlist, _ := d.ListVPAs(ctx, "commerce")
	for _, v := range vlist.Items {
		if v.Name == "parity-vpa" && v.UpdateMode != "Initial" {
			t.Fatalf("vpa list not updated: %+v", v)
		}
	}
	// Noop PUT keeps the version.
	ved, _ = d.GetVPAEditable(ctx, "commerce", "parity-vpa")
	if vres, err := d.UpdateVPA(ctx, "commerce", "parity-vpa", vpaEditReq(ved, nil)); err != nil || !vres.Noop || vres.ResourceVersion != "2" {
		t.Fatalf("noop: %v %+v", err, vres)
	}
}
