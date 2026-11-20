package platform

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// In-memory VPA, ScaledObject and TriggerAuthentication stores for the demo
// provider, so the UI (and the e2e suite) can create an autoscaler, see it in
// the list and delete it again. Like demoHPAs they are process-wide and seeded
// from the deterministic demo data on first use.

var (
	demoAutoscalersMu     sync.Mutex
	demoAutoscalersSeeded bool
	demoVPAStore          map[string]VPASummary
	demoScaledObjectStore map[string]ScaledObjectSummary
)

func demoTriggerAuthentications() []TriggerAuthenticationSummary {
	return []TriggerAuthenticationSummary{
		{Namespace: "data", Name: "redis-auth", Kind: "TriggerAuthentication", Age: "6d"},
		{Namespace: "payments", Name: "kafka-auth", Kind: "TriggerAuthentication", Age: "3d"},
		{Namespace: "payments", Name: "prom-auth", Kind: "TriggerAuthentication", Age: "3d"},
		{Namespace: "payments", Name: "rabbitmq-auth", Kind: "TriggerAuthentication", Age: "3d"},
	}
}

func demoAutoscalersInitLocked() {
	if demoAutoscalersSeeded {
		return
	}
	demoAutoscalersSeeded = true
	demoVPAStore = map[string]VPASummary{}
	for _, v := range demoVPAs() {
		demoVPAStore[v.Namespace+"/"+v.Name] = v
	}
	demoScaledObjectStore = map[string]ScaledObjectSummary{}
	for _, s := range demoScaledObjects() {
		demoScaledObjectStore[s.Namespace+"/"+s.Name] = s
	}
	demoAutoscalerObjectsInitLocked() // full objects for editing (autoscaling_edit_demo.go)
}

func demoHPAsSnapshot(ns string) []HPASummary {
	demoHPAsMu.Lock()
	defer demoHPAsMu.Unlock()
	out := []HPASummary{}
	for _, h := range demoHPAs {
		if h.Namespace == ns {
			out = append(out, h)
		}
	}
	sortHPASummaries(out)
	return out
}

func (d *DemoProvider) CreateVPA(ctx context.Context, req VPACreateRequest) (AutoscalerCreateResult, error) {
	plan, err := buildVPA(req)
	if err != nil {
		return AutoscalerCreateResult{}, err
	}
	warnings := append(plan.Warnings, vpaHPAConflictWarnings(plan, demoHPAsSnapshot(req.Namespace))...)
	demoAutoscalersMu.Lock()
	defer demoAutoscalersMu.Unlock()
	demoAutoscalersInitLocked()
	key := req.Namespace + "/" + req.Name
	if _, exists := demoVPAStore[key]; exists {
		return AutoscalerCreateResult{}, NewStatusError(http.StatusConflict, fmt.Errorf("VerticalPodAutoscaler %s already exists", key))
	}
	demoVPAStore[key] = vpaSummaryFromPlan(req, plan)
	demoVPAObjects[key] = demoObjectFromPlan(plan)
	return AutoscalerCreateResult{OK: true, Created: []AutoscalerObjectRef{plan.Object.ref(plan.Object.Version)}, Warnings: nonNilStrings(warnings)}, nil
}

func (d *DemoProvider) DeleteVPA(ctx context.Context, ns, name string) error {
	demoAutoscalersMu.Lock()
	defer demoAutoscalersMu.Unlock()
	demoAutoscalersInitLocked()
	key := ns + "/" + name
	if _, ok := demoVPAStore[key]; !ok {
		return notFoundf("VerticalPodAutoscaler %s not found", key)
	}
	delete(demoVPAStore, key)
	delete(demoVPAObjects, key)
	return nil
}

func (d *DemoProvider) CreateScaledObject(ctx context.Context, req ScaledObjectCreateRequest) (AutoscalerCreateResult, error) {
	plan, err := buildScaledObject(req)
	if err != nil {
		return AutoscalerCreateResult{}, err
	}
	known := map[string]bool{}
	for _, ta := range demoTriggerAuthentications() {
		if ta.Namespace == req.Namespace {
			known[ta.Name] = true
		}
	}
	for _, ref := range plan.AuthRefs {
		if !known[ref] {
			return AutoscalerCreateResult{}, hpaBadRequest("TriggerAuthentication %s not found in namespace %s", ref, req.Namespace)
		}
	}
	warnings := append(plan.Warnings, kedaHPAConflictWarnings(plan, demoHPAsSnapshot(req.Namespace))...)
	demoAutoscalersMu.Lock()
	defer demoAutoscalersMu.Unlock()
	demoAutoscalersInitLocked()
	key := req.Namespace + "/" + req.Name
	if _, exists := demoScaledObjectStore[key]; exists {
		return AutoscalerCreateResult{}, NewStatusError(http.StatusConflict, fmt.Errorf("ScaledObject %s already exists", key))
	}
	demoScaledObjectStore[key] = scaledObjectSummaryFromPlan(req, plan)
	demoScaledObjectObjects[key] = demoObjectFromPlan(plan)
	return AutoscalerCreateResult{OK: true, Created: []AutoscalerObjectRef{plan.Object.ref(plan.Object.Version)}, Warnings: nonNilStrings(warnings)}, nil
}

func (d *DemoProvider) DeleteScaledObject(ctx context.Context, ns, name string) error {
	demoAutoscalersMu.Lock()
	defer demoAutoscalersMu.Unlock()
	demoAutoscalersInitLocked()
	key := ns + "/" + name
	if _, ok := demoScaledObjectStore[key]; !ok {
		return notFoundf("ScaledObject %s not found", key)
	}
	delete(demoScaledObjectStore, key)
	delete(demoScaledObjectObjects, key)
	return nil
}

func (d *DemoProvider) ListTriggerAuthentications(ctx context.Context, ns string) (TriggerAuthenticationList, error) {
	out := TriggerAuthenticationList{Installed: true, Items: []TriggerAuthenticationSummary{}}
	for _, ta := range demoTriggerAuthentications() {
		if demoNamespaceMatch(ns, ta.Namespace) {
			out.Items = append(out.Items, ta)
		}
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.Namespace == b.Namespace {
			return a.Name < b.Name
		}
		return a.Namespace < b.Namespace
	})
	return out, nil
}
