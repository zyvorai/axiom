package platform

import (
	"context"
	"sort"
	"time"
)

func (d *DemoProvider) ListVirtualMachines(ctx context.Context, ns string) ([]VirtualMachine, error) {
	all := []VirtualMachine{
		{Namespace: "default", Name: "edge-api-vm", Status: "Running", Ready: true, Running: true, CPU: "4", Memory: "8Gi", Node: "worker-a", OS: "ubuntu 22.04", Age: "12d"},
		{Namespace: "platform", Name: "observability-vm", Status: "Running", Ready: true, Running: true, CPU: "2", Memory: "4Gi", Node: "worker-b", OS: "ubuntu 22.04", Age: "4d"},
		{Namespace: "labs", Name: "chrome-qa", Status: "Stopped", Ready: false, Running: false, CPU: "2", Memory: "4Gi", OS: "ubuntu 22.04", Age: "23d"},
	}
	out := make([]VirtualMachine, 0, len(all))
	for _, vm := range all {
		if ns == "" || ns == "all" || vm.Namespace == ns {
			out = append(out, vm)
		}
	}
	return out, nil
}

func (d *DemoProvider) StartVirtualMachine(ctx context.Context, ns, name string) error {
	return nil
}

func (d *DemoProvider) StopVirtualMachine(ctx context.Context, ns, name string) error {
	return nil
}

func (d *DemoProvider) ListVolumes(ctx context.Context, ns string) ([]Volume, error) {
	all := []Volume{
		{Namespace: "default", Name: "edge-api-data", Status: "Bound", Capacity: "100Gi", StorageClass: "zyvor-rbd-prod", AccessModes: "ReadWriteOnce", VolumeName: "pvc-edge-1", Age: "12d"},
		{Namespace: "platform", Name: "obs-wal", Status: "Bound", Capacity: "50Gi", StorageClass: "zyvor-rbd-prod", AccessModes: "ReadWriteOnce", VolumeName: "pvc-obs-1", Age: "4d"},
		{Namespace: "data", Name: "postgres-data", Status: "Bound", Capacity: "200Gi", StorageClass: "zyvor-cephfs-shared", AccessModes: "ReadWriteMany", VolumeName: "pvc-pg-1", Age: "28d"},
		{Namespace: "labs", Name: "qa-scratch", Status: "Pending", Capacity: "20Gi", StorageClass: "local-path", AccessModes: "ReadWriteOnce", Age: "2h"},
	}
	out := make([]Volume, 0, len(all))
	for _, v := range all {
		if ns == "" || ns == "all" || v.Namespace == ns {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func (d *DemoProvider) Utilization(ctx context.Context) (UtilizationSeries, error) {
	cpu, mem := 42.8, 61.2
	now := time.Now()
	points := make([]UtilizationPoint, 0, 25)
	for i := 0; i < 25; i++ {
		t := now.Add(time.Duration(i-24) * time.Hour)
		wave := float64((i%7)-3) * 2.4
		at := t.Format("15:04")
		if i == 24 {
			at = "Now"
		}
		points = append(points, UtilizationPoint{At: at, CPU: round1(clampPct(cpu + wave)), Memory: round1(clampPct(mem + wave*0.6))})
	}
	return UtilizationSeries{CPUPercent: cpu, MemoryPercent: mem, PeakCPU: 68.4, PeakMemory: 74.1, Points: points, Source: "demo"}, nil
}
