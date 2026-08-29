package platform

import (
	"context"
	"fmt"
	"sync"
	"time"
)

var (
	demoVMClonesMu sync.Mutex
	demoVMClones   = map[string]VMClone{
		"default/edge-clone-1": {Namespace: "default", Name: "edge-clone-1", Source: "edge-api-vm", SourceKind: "VirtualMachine", Target: "edge-api-vm-copy", Phase: "Succeeded", Age: "2d"},
	}
)

func (d *DemoProvider) ListVMClones(ctx context.Context, ns string) ([]VMClone, error) {
	demoVMClonesMu.Lock()
	defer demoVMClonesMu.Unlock()
	out := make([]VMClone, 0, len(demoVMClones))
	for _, c := range demoVMClones {
		if ns == "" || ns == "all" || c.Namespace == ns {
			out = append(out, c)
		}
	}
	return out, nil
}

func (d *DemoProvider) CreateVMClone(ctx context.Context, req CreateVMCloneRequest) (VMClone, error) {
	if req.Namespace == "" || req.SourceVM == "" || req.TargetVM == "" {
		return VMClone{}, fmt.Errorf("namespace, sourceVM, and targetVM are required")
	}
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s-clone", req.TargetVM)
	}
	kind := req.SourceKind
	if kind == "" {
		kind = "VirtualMachine"
	}
	clone := VMClone{Namespace: req.Namespace, Name: name, Source: req.SourceVM, SourceKind: kind, Target: req.TargetVM, Phase: "Succeeded", Age: "now"}
	demoVMClonesMu.Lock()
	demoVMClones[req.Namespace+"/"+name] = clone
	demoVMClonesMu.Unlock()
	return clone, nil
}

var (
	demoVMExportsMu sync.Mutex
	demoVMExports   = map[string]VMExport{
		"default/edge-export": {Namespace: "default", Name: "edge-export", Source: "edge-api-vm", Phase: "Ready", Links: "https://export.example/vmi", Age: "1d"},
	}
)

func (d *DemoProvider) ListVMExports(ctx context.Context, ns string) ([]VMExport, error) {
	demoVMExportsMu.Lock()
	defer demoVMExportsMu.Unlock()
	out := make([]VMExport, 0, len(demoVMExports))
	for _, e := range demoVMExports {
		if ns == "" || ns == "all" || e.Namespace == ns {
			out = append(out, e)
		}
	}
	return out, nil
}

func (d *DemoProvider) CreateVMExport(ctx context.Context, req CreateVMExportRequest) (VMExport, error) {
	if req.Namespace == "" || req.VM == "" {
		return VMExport{}, fmt.Errorf("namespace and vm are required")
	}
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s-export", req.VM)
	}
	exp := VMExport{Namespace: req.Namespace, Name: name, Source: req.VM, Phase: "Ready", Links: fmt.Sprintf("https://export.example/%s", name), Age: "now"}
	demoVMExportsMu.Lock()
	demoVMExports[req.Namespace+"/"+name] = exp
	demoVMExportsMu.Unlock()
	return exp, nil
}

var (
	demoVMRestoresMu sync.Mutex
	demoVMRestores   = map[string]VMRestore{
		"default/edge-restore": {Namespace: "default", Name: "edge-restore", Snapshot: "edge-api-vm-snap", Target: "edge-api-vm", Complete: true, Phase: "Complete", Age: "5d"},
	}
)

func (d *DemoProvider) ListVMRestores(ctx context.Context, ns string) ([]VMRestore, error) {
	demoVMRestoresMu.Lock()
	defer demoVMRestoresMu.Unlock()
	out := make([]VMRestore, 0, len(demoVMRestores))
	for _, r := range demoVMRestores {
		if ns == "" || ns == "all" || r.Namespace == ns {
			out = append(out, r)
		}
	}
	return out, nil
}

func (d *DemoProvider) CreateVMRestore(ctx context.Context, req CreateVMRestoreRequest) (VMRestore, error) {
	if req.Namespace == "" || req.Snapshot == "" || req.TargetVM == "" {
		return VMRestore{}, fmt.Errorf("namespace, snapshot, and targetVM are required")
	}
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s-restore", req.TargetVM)
	}
	restore := VMRestore{Namespace: req.Namespace, Name: name, Snapshot: req.Snapshot, Target: req.TargetVM, Complete: true, Phase: "Complete", Age: "now"}
	demoVMRestoresMu.Lock()
	demoVMRestores[req.Namespace+"/"+name] = restore
	demoVMRestoresMu.Unlock()
	return restore, nil
}

func (d *DemoProvider) ListVeleroBackups(ctx context.Context, ns string) ([]VeleroBackup, error) {
	all := []VeleroBackup{
		{Namespace: "velero", Name: "forge-backup-demo", Phase: "Completed", Namespaces: "forge,forge-system", Age: "3d"},
	}
	return filterNS(ns, all, func(b VeleroBackup) string { return b.Namespace }), nil
}

var (
	demoVeleroSchedulesMu sync.Mutex
	demoVeleroSchedules   = map[string]VeleroSchedule{
		"velero/forge-daily": {Namespace: "velero", Name: "forge-daily", Schedule: "0 2 * * *", LastBackup: time.Now().Add(-24 * time.Hour).Format(time.RFC3339), Age: "30d"},
	}
)

func (d *DemoProvider) ListVeleroSchedules(ctx context.Context, ns string) ([]VeleroSchedule, error) {
	demoVeleroSchedulesMu.Lock()
	defer demoVeleroSchedulesMu.Unlock()
	out := make([]VeleroSchedule, 0, len(demoVeleroSchedules))
	for _, s := range demoVeleroSchedules {
		if ns == "" || ns == "all" || s.Namespace == ns {
			out = append(out, s)
		}
	}
	return out, nil
}

func (d *DemoProvider) CreateVeleroSchedule(ctx context.Context, req CreateVeleroScheduleRequest) (VeleroSchedule, error) {
	if req.Namespace == "" || req.Name == "" || req.Schedule == "" {
		return VeleroSchedule{}, fmt.Errorf("namespace, name, and schedule are required")
	}
	sched := VeleroSchedule{Namespace: req.Namespace, Name: req.Name, Schedule: req.Schedule, Age: "now"}
	demoVeleroSchedulesMu.Lock()
	demoVeleroSchedules[req.Namespace+"/"+req.Name] = sched
	demoVeleroSchedulesMu.Unlock()
	return sched, nil
}

func filterNS[T any](ns string, all []T, nsOf func(T) string) []T {
	out := make([]T, 0, len(all))
	for _, item := range all {
		n := nsOf(item)
		if ns == "" || ns == "all" || n == ns {
			out = append(out, item)
		}
	}
	return out
}
