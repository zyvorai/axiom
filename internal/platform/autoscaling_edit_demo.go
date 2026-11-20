package platform

import (
	"context"
)

// VPA / ScaledObject editing for the demo provider. Alongside the list
// summaries (autoscaling_create_demo.go) the demo keeps each object in full,
// with a fake resourceVersion that is bumped on every change so the stale
// guard behaves as on a cluster. Guarded by demoAutoscalersMu.

var (
	demoVPAObjects          map[string]map[string]any
	demoScaledObjectObjects map[string]map[string]any
)

// demoObjectFromPlan is a just-created object as the demo stores it.
func demoObjectFromPlan(plan autoscalerPlan) map[string]any {
	obj, _ := jsonNormalize(plan.Object.withVersion(plan.Object.Version)).(map[string]any)
	meta, _ := obj["metadata"].(map[string]any)
	meta["resourceVersion"] = "1"
	return obj
}

// demoSeedObject renders a seed object with the create builder, without the
// managed-by label (the seeds were not created by Axiom).
func demoSeedObject(plan autoscalerPlan, err error) map[string]any {
	if err != nil {
		panic("demo autoscaler seed: " + err.Error())
	}
	obj := demoObjectFromPlan(plan)
	meta, _ := obj["metadata"].(map[string]any)
	delete(meta, "labels")
	return obj
}

func demoAutoscalerObjectsInitLocked() {
	demoVPAObjects = map[string]map[string]any{}
	demoScaledObjectObjects = map[string]map[string]any{}
	vpaSeeds := map[string]VPACreateRequest{
		"commerce/checkout-vpa": {Namespace: "commerce", Name: "checkout-vpa", TargetKind: "Deployment", TargetName: "checkout", UpdateMode: "Auto",
			Containers: []VPAContainerPolicyRequest{{Name: "checkout", MinCPU: "100m", MaxCPU: "4", MinMemory: "128Mi", MaxMemory: "8Gi"}}},
		"data/redis-vpa": {Namespace: "data", Name: "redis-vpa", TargetKind: "StatefulSet", TargetName: "redis", UpdateMode: "Off"},
	}
	for key, req := range vpaSeeds {
		if _, ok := demoVPAStore[key]; ok {
			demoVPAObjects[key] = demoSeedObject(buildVPA(req))
		}
	}
	soSeeds := map[string]ScaledObjectCreateRequest{
		"data/redis-consumer": {Namespace: "data", Name: "redis-consumer", TargetKind: "StatefulSet", TargetName: "redis",
			MinReplicas: i32ptr(3), MaxReplicas: 6, Triggers: []ScaledObjectTriggerRequest{
				{Type: "redis", Settings: map[string]string{"address": "redis.data:6379", "listName": "jobs", "listLength": "10"}, AuthenticationRef: "redis-auth"},
				{Type: "cron", Settings: map[string]string{"timezone": "UTC", "start": "0 6 * * *", "end": "0 20 * * *", "desiredReplicas": "4"}},
			}},
		"payments/payments-api": {Namespace: "payments", Name: "payments-api", TargetKind: "Deployment", TargetName: "payments-api",
			MinReplicas: i32ptr(2), MaxReplicas: 20, Triggers: []ScaledObjectTriggerRequest{
				{Type: "kafka", Name: "orders-lag", Settings: map[string]string{"bootstrapServers": "kafka.data:9092", "consumerGroup": "payments", "topic": "orders", "lagThreshold": "50"}, AuthenticationRef: "kafka-auth"},
				{Type: "prometheus", Name: "http-requests", Settings: map[string]string{"serverAddress": "http://prometheus.monitoring:9090", "query": "sum(rate(http_requests_total{app=\"payments-api\"}[2m]))", "threshold": "100"}, AuthenticationRef: "prom-auth"},
			}},
	}
	for key, req := range soSeeds {
		if _, ok := demoScaledObjectStore[key]; ok {
			demoScaledObjectObjects[key] = demoSeedObject(buildScaledObject(req))
		}
	}
}

func i32ptr(v int32) *int32 { return &v }

// demoAutoscalerObject returns a copy of the stored object (caller holds the lock).
func demoAutoscalerObject(store map[string]map[string]any, kind, ns, name string) (map[string]any, error) {
	obj, ok := store[ns+"/"+name]
	if !ok {
		return nil, notFoundf("%s %s/%s not found", kind, ns, name)
	}
	cp, _ := jsonNormalize(obj).(map[string]any)
	return cp, nil
}

// demoStoreEdit saves plan.After with a bumped resourceVersion and returns it.
func demoStoreEdit(store map[string]map[string]any, key string, plan autoscalerEditPlan) string {
	if plan.Patch == nil {
		return plan.RV
	}
	rv := demoBumpResourceVersion(plan.RV)
	meta, _ := plan.After["metadata"].(map[string]any)
	meta["resourceVersion"] = rv
	store[key] = plan.After
	return rv
}

func (d *DemoProvider) GetVPAEditable(ctx context.Context, ns, name string) (VPAEditable, error) {
	demoAutoscalersMu.Lock()
	defer demoAutoscalersMu.Unlock()
	demoAutoscalersInitLocked()
	obj, err := demoAutoscalerObject(demoVPAObjects, "VerticalPodAutoscaler", ns, name)
	if err != nil {
		return VPAEditable{}, err
	}
	return readVPAEditable(obj), nil
}

func (d *DemoProvider) PreviewVPAEdit(ctx context.Context, ns, name string, req VPAEditRequest) (AutoscalerEditPreview, error) {
	demoAutoscalersMu.Lock()
	defer demoAutoscalersMu.Unlock()
	demoAutoscalersInitLocked()
	obj, err := demoAutoscalerObject(demoVPAObjects, "VerticalPodAutoscaler", ns, name)
	if err != nil {
		return AutoscalerEditPreview{}, err
	}
	plan, err := planVPAEdit(obj, req)
	return plan.Preview, err
}

func (d *DemoProvider) UpdateVPA(ctx context.Context, ns, name string, req VPAEditRequest) (AutoscalerEditResult, error) {
	demoAutoscalersMu.Lock()
	demoAutoscalersInitLocked()
	obj, err := demoAutoscalerObject(demoVPAObjects, "VerticalPodAutoscaler", ns, name)
	if err != nil {
		demoAutoscalersMu.Unlock()
		return AutoscalerEditResult{}, err
	}
	plan, err := planVPAEdit(obj, req)
	if err != nil {
		demoAutoscalersMu.Unlock()
		return AutoscalerEditResult{}, err
	}
	key := ns + "/" + name
	rv := demoStoreEdit(demoVPAObjects, key, plan)
	if s, ok := demoVPAStore[key]; ok {
		s.UpdateMode = nestedString(plan.After, "spec", "updatePolicy", "updateMode")
		demoVPAStore[key] = s
	}
	demoAutoscalersMu.Unlock()
	res := plan.result(rv)
	res.Warnings = append(res.Warnings, vpaHPAConflictWarnings(plan.Build, demoHPAsSnapshot(ns))...)
	return res, nil
}

func (d *DemoProvider) GetScaledObjectEditable(ctx context.Context, ns, name string) (ScaledObjectEditable, error) {
	demoAutoscalersMu.Lock()
	defer demoAutoscalersMu.Unlock()
	demoAutoscalersInitLocked()
	obj, err := demoAutoscalerObject(demoScaledObjectObjects, "ScaledObject", ns, name)
	if err != nil {
		return ScaledObjectEditable{}, err
	}
	return readScaledObjectEditable(obj), nil
}

func (d *DemoProvider) PreviewScaledObjectEdit(ctx context.Context, ns, name string, req ScaledObjectEditRequest) (AutoscalerEditPreview, error) {
	demoAutoscalersMu.Lock()
	defer demoAutoscalersMu.Unlock()
	demoAutoscalersInitLocked()
	obj, err := demoAutoscalerObject(demoScaledObjectObjects, "ScaledObject", ns, name)
	if err != nil {
		return AutoscalerEditPreview{}, err
	}
	plan, err := planScaledObjectEdit(obj, req)
	return plan.Preview, err
}

func (d *DemoProvider) UpdateScaledObject(ctx context.Context, ns, name string, req ScaledObjectEditRequest) (AutoscalerEditResult, error) {
	demoAutoscalersMu.Lock()
	demoAutoscalersInitLocked()
	obj, err := demoAutoscalerObject(demoScaledObjectObjects, "ScaledObject", ns, name)
	if err != nil {
		demoAutoscalersMu.Unlock()
		return AutoscalerEditResult{}, err
	}
	plan, err := planScaledObjectEdit(obj, req)
	if err != nil {
		demoAutoscalersMu.Unlock()
		return AutoscalerEditResult{}, err
	}
	known := map[string]bool{}
	for _, ta := range demoTriggerAuthentications() {
		if ta.Namespace == ns {
			known[ta.Name] = true
		}
	}
	for _, ref := range plan.Build.AuthRefs {
		if plan.Patch != nil && !known[ref] {
			demoAutoscalersMu.Unlock()
			return AutoscalerEditResult{}, hpaBadRequest("TriggerAuthentication %s not found in namespace %s", ref, ns)
		}
	}
	key := ns + "/" + name
	rv := demoStoreEdit(demoScaledObjectObjects, key, plan)
	if s, ok := demoScaledObjectStore[key]; ok && plan.Patch != nil {
		spec, _ := plan.After["spec"].(map[string]any)
		s.MinReplicas, _ = editInt(spec["minReplicaCount"], 0)
		s.MaxReplicas, _ = editInt(spec["maxReplicaCount"], 100)
		s.Triggers = []ScaledObjectTrigger{}
		triggers, _ := spec["triggers"].([]any)
		for _, t := range triggers {
			m, _ := t.(map[string]any)
			typ, _ := m["type"].(string)
			tn, _ := m["name"].(string)
			md, _ := m["metadata"].(map[string]any)
			s.Triggers = append(s.Triggers, ScaledObjectTrigger{Type: typ, Name: kedaTriggerName(tn, md)})
		}
		demoScaledObjectStore[key] = s
	}
	demoAutoscalersMu.Unlock()
	res := plan.result(rv)
	res.Warnings = append(res.Warnings, kedaHPAConflictWarnings(plan.Build, demoHPAsSnapshot(ns))...)
	return res, nil
}
