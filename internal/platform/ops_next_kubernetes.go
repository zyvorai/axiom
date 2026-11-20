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
)

var (
	vmCloneGVRs = []schema.GroupVersionResource{
		{Group: "clone.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclones"},
		{Group: "clone.kubevirt.io", Version: "v1alpha1", Resource: "virtualmachineclones"},
	}
	vmExportGVRs = []schema.GroupVersionResource{
		{Group: "export.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineexports"},
		{Group: "export.kubevirt.io", Version: "v1alpha1", Resource: "virtualmachineexports"},
	}
	vmRestoreGVRs = []schema.GroupVersionResource{
		{Group: "snapshot.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinerestores"},
		{Group: "snapshot.kubevirt.io", Version: "v1alpha1", Resource: "virtualmachinerestores"},
	}
	veleroBackupGVR = schema.GroupVersionResource{Group: "velero.io", Version: "v1", Resource: "backups"}
	veleroSchedGVR  = schema.GroupVersionResource{Group: "velero.io", Version: "v1", Resource: "schedules"}
	alertmanagerGVR = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "alertmanagers"}
)

func (k *KubernetesProvider) listFirstGVR(ctx context.Context, gvrs []schema.GroupVersionResource, ns string) (*unstructured.UnstructuredList, schema.GroupVersionResource, error) {
	var last error
	for _, gvr := range gvrs {
		list, err := k.dynamic.Resource(gvr).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
		if err == nil {
			return list, gvr, nil
		}
		last = err
		if !isMissingCRD(err) {
			return nil, gvr, err
		}
	}
	return nil, schema.GroupVersionResource{}, last
}

func (k *KubernetesProvider) ListVMClones(ctx context.Context, ns string) ([]VMClone, error) {
	if k.dynamic == nil {
		return []VMClone{}, nil
	}
	list, _, err := k.listFirstGVR(ctx, vmCloneGVRs, ns)
	if err != nil {
		if isMissingCRD(err) {
			return []VMClone{}, nil
		}
		return nil, err
	}
	out := make([]VMClone, 0, len(list.Items))
	for _, item := range list.Items {
		srcKind, _, _ := unstructured.NestedString(item.Object, "spec", "source", "kind")
		srcName, _, _ := unstructured.NestedString(item.Object, "spec", "source", "name")
		tgt, _, _ := unstructured.NestedString(item.Object, "spec", "target", "name")
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		if phase == "" {
			phase = "Unknown"
		}
		out = append(out, VMClone{
			Namespace: item.GetNamespace(), Name: item.GetName(),
			Source: srcName, SourceKind: srcKind, Target: tgt, Phase: phase,
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

func (k *KubernetesProvider) CreateVMClone(ctx context.Context, req CreateVMCloneRequest) (VMClone, error) {
	if k.dynamic == nil {
		return VMClone{}, fmt.Errorf("dynamic client unavailable")
	}
	if req.Namespace == "" || req.SourceVM == "" || req.TargetVM == "" {
		return VMClone{}, fmt.Errorf("namespace, sourceVM, and targetVM are required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("%s-clone-%d", req.TargetVM, time.Now().Unix()%100000)
	}
	kind := req.SourceKind
	apiGroup := "kubevirt.io"
	if kind == "" || kind == "VirtualMachine" {
		kind = "VirtualMachine"
		apiGroup = "kubevirt.io"
	} else if kind == "VirtualMachineSnapshot" {
		apiGroup = "snapshot.kubevirt.io"
	}
	var lastErr error
	for _, gvr := range vmCloneGVRs {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": gvr.Group + "/" + gvr.Version,
			"kind":       "VirtualMachineClone",
			"metadata":   map[string]any{"name": name, "namespace": req.Namespace},
			"spec": map[string]any{
				"source": map[string]any{"apiGroup": apiGroup, "kind": kind, "name": req.SourceVM},
				"target": map[string]any{"apiGroup": "kubevirt.io", "kind": "VirtualMachine", "name": req.TargetVM},
			},
		}}
		created, err := k.dynamic.Resource(gvr).Namespace(req.Namespace).Create(ctx, obj, metav1.CreateOptions{})
		if err == nil {
			return VMClone{
				Namespace: created.GetNamespace(), Name: created.GetName(),
				Source: req.SourceVM, SourceKind: kind, Target: req.TargetVM, Phase: "InProgress",
				Age: age(created.GetCreationTimestamp().Time),
			}, nil
		}
		lastErr = err
		if !isMissingCRD(err) {
			return VMClone{}, err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("VirtualMachineClone CRD not available")
	}
	return VMClone{}, lastErr
}

func (k *KubernetesProvider) ListVMExports(ctx context.Context, ns string) ([]VMExport, error) {
	if k.dynamic == nil {
		return []VMExport{}, nil
	}
	list, _, err := k.listFirstGVR(ctx, vmExportGVRs, ns)
	if err != nil {
		if isMissingCRD(err) {
			return []VMExport{}, nil
		}
		return nil, err
	}
	out := make([]VMExport, 0, len(list.Items))
	for _, item := range list.Items {
		src, _, _ := unstructured.NestedString(item.Object, "spec", "source", "name")
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		if phase == "" {
			phase = "Unknown"
		}
		links := ""
		if ext, ok, _ := unstructured.NestedMap(item.Object, "status", "links", "external"); ok {
			if u, _, _ := unstructured.NestedString(ext, "virtvnc"); u != "" {
				links = u
			} else if u, _, _ := unstructured.NestedString(ext, "url"); u != "" {
				links = u
			}
		}
		out = append(out, VMExport{
			Namespace: item.GetNamespace(), Name: item.GetName(), Source: src,
			Phase: phase, Links: links, Age: age(item.GetCreationTimestamp().Time),
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

func (k *KubernetesProvider) CreateVMExport(ctx context.Context, req CreateVMExportRequest) (VMExport, error) {
	if k.dynamic == nil {
		return VMExport{}, fmt.Errorf("dynamic client unavailable")
	}
	if req.Namespace == "" || req.VM == "" {
		return VMExport{}, fmt.Errorf("namespace and vm are required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("%s-export-%d", req.VM, time.Now().Unix()%100000)
	}
	var lastErr error
	for _, gvr := range vmExportGVRs {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": gvr.Group + "/" + gvr.Version,
			"kind":       "VirtualMachineExport",
			"metadata":   map[string]any{"name": name, "namespace": req.Namespace},
			"spec": map[string]any{
				"source": map[string]any{
					"apiGroup": "kubevirt.io",
					"kind":     "VirtualMachine",
					"name":     req.VM,
				},
			},
		}}
		created, err := k.dynamic.Resource(gvr).Namespace(req.Namespace).Create(ctx, obj, metav1.CreateOptions{})
		if err == nil {
			return VMExport{
				Namespace: created.GetNamespace(), Name: created.GetName(), Source: req.VM,
				Phase: "Pending", Age: age(created.GetCreationTimestamp().Time),
			}, nil
		}
		lastErr = err
		if !isMissingCRD(err) {
			return VMExport{}, err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("VirtualMachineExport CRD not available")
	}
	return VMExport{}, lastErr
}

func (k *KubernetesProvider) ListVMRestores(ctx context.Context, ns string) ([]VMRestore, error) {
	if k.dynamic == nil {
		return []VMRestore{}, nil
	}
	list, _, err := k.listFirstGVR(ctx, vmRestoreGVRs, ns)
	if err != nil {
		if isMissingCRD(err) {
			return []VMRestore{}, nil
		}
		return nil, err
	}
	out := make([]VMRestore, 0, len(list.Items))
	for _, item := range list.Items {
		snap, _, _ := unstructured.NestedString(item.Object, "spec", "virtualMachineSnapshotName")
		tgt, _, _ := unstructured.NestedString(item.Object, "spec", "target", "name")
		complete, _, _ := unstructured.NestedBool(item.Object, "status", "complete")
		phase := "InProgress"
		if complete {
			phase = "Complete"
		}
		out = append(out, VMRestore{
			Namespace: item.GetNamespace(), Name: item.GetName(), Snapshot: snap,
			Target: tgt, Complete: complete, Phase: phase, Age: age(item.GetCreationTimestamp().Time),
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

func (k *KubernetesProvider) CreateVMRestore(ctx context.Context, req CreateVMRestoreRequest) (VMRestore, error) {
	if k.dynamic == nil {
		return VMRestore{}, fmt.Errorf("dynamic client unavailable")
	}
	if req.Namespace == "" || req.Snapshot == "" || req.TargetVM == "" {
		return VMRestore{}, fmt.Errorf("namespace, snapshot, and targetVM are required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("%s-restore-%d", req.TargetVM, time.Now().Unix()%100000)
	}
	var lastErr error
	for _, gvr := range vmRestoreGVRs {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": gvr.Group + "/" + gvr.Version,
			"kind":       "VirtualMachineRestore",
			"metadata":   map[string]any{"name": name, "namespace": req.Namespace},
			"spec": map[string]any{
				"virtualMachineSnapshotName": req.Snapshot,
				"target": map[string]any{
					"apiGroup": "kubevirt.io",
					"kind":     "VirtualMachine",
					"name":     req.TargetVM,
				},
			},
		}}
		created, err := k.dynamic.Resource(gvr).Namespace(req.Namespace).Create(ctx, obj, metav1.CreateOptions{})
		if err == nil {
			return VMRestore{
				Namespace: created.GetNamespace(), Name: created.GetName(),
				Snapshot: req.Snapshot, Target: req.TargetVM, Phase: "InProgress",
				Age: age(created.GetCreationTimestamp().Time),
			}, nil
		}
		lastErr = err
		if !isMissingCRD(err) {
			return VMRestore{}, err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("VirtualMachineRestore CRD not available")
	}
	return VMRestore{}, lastErr
}

func (k *KubernetesProvider) ListVeleroBackups(ctx context.Context, ns string) ([]VeleroBackup, error) {
	if k.dynamic == nil {
		return []VeleroBackup{}, nil
	}
	list, err := k.dynamic.Resource(veleroBackupGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isMissingCRD(err) {
			return []VeleroBackup{}, nil
		}
		return nil, err
	}
	out := make([]VeleroBackup, 0, len(list.Items))
	for _, item := range list.Items {
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		if phase == "" {
			phase = "New"
		}
		included, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "includedNamespaces")
		errs, _, _ := unstructured.NestedInt64(item.Object, "status", "errors")
		out = append(out, VeleroBackup{
			Namespace: item.GetNamespace(), Name: item.GetName(), Phase: phase,
			Namespaces: strings.Join(included, ","), Errors: int(errs),
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

func (k *KubernetesProvider) ListVeleroSchedules(ctx context.Context, ns string) ([]VeleroSchedule, error) {
	if k.dynamic == nil {
		return []VeleroSchedule{}, nil
	}
	list, err := k.dynamic.Resource(veleroSchedGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isMissingCRD(err) {
			return []VeleroSchedule{}, nil
		}
		return nil, err
	}
	out := make([]VeleroSchedule, 0, len(list.Items))
	for _, item := range list.Items {
		sched, _, _ := unstructured.NestedString(item.Object, "spec", "schedule")
		paused, _, _ := unstructured.NestedBool(item.Object, "spec", "paused")
		last, _, _ := unstructured.NestedString(item.Object, "status", "lastBackup")
		out = append(out, VeleroSchedule{
			Namespace: item.GetNamespace(), Name: item.GetName(), Schedule: sched,
			Paused: paused, LastBackup: last, Age: age(item.GetCreationTimestamp().Time),
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

func (k *KubernetesProvider) CreateVeleroSchedule(ctx context.Context, req CreateVeleroScheduleRequest) (VeleroSchedule, error) {
	if k.dynamic == nil {
		return VeleroSchedule{}, fmt.Errorf("dynamic client unavailable")
	}
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Schedule) == "" {
		return VeleroSchedule{}, fmt.Errorf("namespace, name, and schedule are required")
	}
	template := map[string]any{
		"includedNamespaces": []any{"*"},
	}
	if req.TTL != "" {
		template["ttl"] = req.TTL
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "velero.io/v1",
		"kind":       "Schedule",
		"metadata": map[string]any{
			"name":      req.Name,
			"namespace": req.Namespace,
		},
		"spec": map[string]any{
			"schedule": req.Schedule,
			"template": template,
		},
	}}
	created, err := k.dynamic.Resource(veleroSchedGVR).Namespace(req.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return VeleroSchedule{}, err
	}
	return VeleroSchedule{
		Namespace: created.GetNamespace(), Name: created.GetName(), Schedule: req.Schedule,
		Age: age(created.GetCreationTimestamp().Time),
	}, nil
}

func (k *KubernetesProvider) listAlertmanagers(ctx context.Context, ns string) []AlertmanagerSummary {
	if k.dynamic == nil {
		return []AlertmanagerSummary{}
	}
	list, err := k.dynamic.Resource(alertmanagerGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return []AlertmanagerSummary{}
	}
	out := make([]AlertmanagerSummary, 0, len(list.Items))
	for _, item := range list.Items {
		replicas, _, _ := unstructured.NestedInt64(item.Object, "spec", "replicas")
		version, _, _ := unstructured.NestedString(item.Object, "status", "version")
		if version == "" {
			version, _, _ = unstructured.NestedString(item.Object, "spec", "version")
		}
		avail, _, _ := unstructured.NestedInt64(item.Object, "status", "availableReplicas")
		ready := avail > 0 || (replicas == 0)
		conds, _, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
		for _, c := range conds {
			m, _ := c.(map[string]any)
			if fmt.Sprint(m["type"]) == "Available" && fmt.Sprint(m["status"]) == "True" {
				ready = true
			}
		}
		out = append(out, AlertmanagerSummary{
			Namespace: item.GetNamespace(), Name: item.GetName(), Version: version,
			Replicas: replicas, Ready: ready, Age: age(item.GetCreationTimestamp().Time),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out
}
