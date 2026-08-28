package platform

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func (d *DemoProvider) ListVMClones(ctx context.Context, ns string) ([]VMClone, error) {
	all := []VMClone{
		{Namespace: "default", Name: "edge-clone-1", Source: "edge-api-vm", SourceKind: "VirtualMachine", Target: "edge-api-vm-copy", Phase: "Succeeded", Age: "2d"},
	}
	return filterNS(ns, all, func(c VMClone) string { return c.Namespace }), nil
}

func (d *DemoProvider) CreateVMClone(ctx context.Context, req CreateVMCloneRequest) (VMClone, error) {
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s-clone", req.TargetVM)
	}
	kind := req.SourceKind
	if kind == "" {
		kind = "VirtualMachine"
	}
	return VMClone{Namespace: req.Namespace, Name: name, Source: req.SourceVM, SourceKind: kind, Target: req.TargetVM, Phase: "InProgress", Age: "now"}, nil
}

func (d *DemoProvider) ListVMExports(ctx context.Context, ns string) ([]VMExport, error) {
	all := []VMExport{
		{Namespace: "default", Name: "edge-export", Source: "edge-api-vm", Phase: "Ready", Links: "https://export.example/vmi", Age: "1d"},
	}
	return filterNS(ns, all, func(e VMExport) string { return e.Namespace }), nil
}

func (d *DemoProvider) CreateVMExport(ctx context.Context, req CreateVMExportRequest) (VMExport, error) {
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s-export", req.VM)
	}
	return VMExport{Namespace: req.Namespace, Name: name, Source: req.VM, Phase: "Pending", Age: "now"}, nil
}

func (d *DemoProvider) ListVMRestores(ctx context.Context, ns string) ([]VMRestore, error) {
	all := []VMRestore{
		{Namespace: "default", Name: "edge-restore", Snapshot: "edge-api-vm-snap", Target: "edge-api-vm", Complete: true, Phase: "Complete", Age: "5d"},
	}
	return filterNS(ns, all, func(r VMRestore) string { return r.Namespace }), nil
}

func (d *DemoProvider) CreateVMRestore(ctx context.Context, req CreateVMRestoreRequest) (VMRestore, error) {
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s-restore", req.TargetVM)
	}
	return VMRestore{Namespace: req.Namespace, Name: name, Snapshot: req.Snapshot, Target: req.TargetVM, Phase: "InProgress", Age: "now"}, nil
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
