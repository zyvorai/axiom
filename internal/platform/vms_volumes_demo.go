package platform

import (
	"context"
	"fmt"
	"sort"
	"time"
)

func demoBaseVMs() []VirtualMachine {
	return []VirtualMachine{
		{Namespace: "default", Name: "edge-api-vm", Status: "Running", Ready: true, Running: true, CPU: "4", Memory: "8Gi", Node: "worker-a", OS: "ubuntu 22.04", Age: "12d"},
		{Namespace: "platform", Name: "observability-vm", Status: "Running", Ready: true, Running: true, CPU: "2", Memory: "4Gi", Node: "worker-b", OS: "ubuntu 22.04", Age: "4d"},
		{Namespace: "labs", Name: "chrome-qa", Status: "Stopped", Ready: false, Running: false, CPU: "2", Memory: "4Gi", OS: "ubuntu 22.04", Age: "23d"},
	}
}

func demoVMExists(ns, name string) bool {
	for _, vm := range demoBaseVMs() {
		if vm.Namespace == ns && vm.Name == name {
			return true
		}
	}
	return false
}

func (d *DemoProvider) vmExists(ns, name string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.vmExistsLocked(ns, name)
}

func (d *DemoProvider) vmExistsLocked(ns, name string) bool {
	if demoVMExists(ns, name) {
		return true
	}
	for _, vm := range d.extraVMs {
		if vm.Namespace == ns && vm.Name == name {
			return true
		}
	}
	return false
}

func (d *DemoProvider) ListVirtualMachines(ctx context.Context, ns string) ([]VirtualMachine, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	all := demoBaseVMs()
	all = append(all, d.extraVMs...)
	for i := range all {
		vk := all[i].Namespace + "/" + all[i].Name
		if running, ok := d.vmRunning[vk]; ok {
			all[i].Running = running
			all[i].Ready = running
			if running {
				all[i].Status = "Running"
			} else {
				all[i].Status = "Stopped"
			}
		}
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
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.vmExistsLocked(ns, name) {
		return fmt.Errorf("vm %s/%s not found", ns, name)
	}
	d.vmRunning[ns+"/"+name] = true
	return nil
}

func (d *DemoProvider) StopVirtualMachine(ctx context.Context, ns, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.vmExistsLocked(ns, name) {
		return fmt.Errorf("vm %s/%s not found", ns, name)
	}
	d.vmRunning[ns+"/"+name] = false
	return nil
}

func (d *DemoProvider) ListVolumes(ctx context.Context, ns string) ([]Volume, error) {
	all := []Volume{
		{Namespace: "default", Name: "edge-api-data", Status: "Bound", Capacity: "100Gi", StorageClass: "zyvor-rbd-prod", AccessModes: "ReadWriteOnce", VolumeName: "pvc-edge-1", Age: "12d"},
		{Namespace: "platform", Name: "obs-wal", Status: "Bound", Capacity: "50Gi", StorageClass: "zyvor-rbd-prod", AccessModes: "ReadWriteOnce", VolumeName: "pvc-obs-1", Age: "4d"},
		{Namespace: "data", Name: "postgres-data", Status: "Bound", Capacity: "200Gi", StorageClass: "zyvor-cephfs-shared", AccessModes: "ReadWriteMany", VolumeName: "pvc-pg-1", Age: "28d"},
		{Namespace: "labs", Name: "qa-scratch", Status: "Pending", Capacity: "20Gi", StorageClass: "local-path", AccessModes: "ReadWriteOnce", Age: "2h"},
	}
	d.mu.RLock()
	all = append(all, d.extraVolumes...)
	d.mu.RUnlock()
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

func (d *DemoProvider) ListStorageClasses(ctx context.Context) ([]StorageClassSummary, error) {
	return []StorageClassSummary{
		{Name: "zyvor-rbd-prod", Provisioner: "rbd.csi.ceph.com", ReclaimPolicy: "Delete", VolumeBindingMode: "Immediate", IsDefault: true, Age: "60d"},
		{Name: "zyvor-cephfs-shared", Provisioner: "cephfs.csi.ceph.com", ReclaimPolicy: "Retain", VolumeBindingMode: "Immediate", Age: "60d"},
		{Name: "local-path", Provisioner: "rancher.io/local-path", ReclaimPolicy: "Delete", VolumeBindingMode: "WaitForFirstConsumer", Age: "60d"},
	}, nil
}

func (d *DemoProvider) ListPriorityClasses(ctx context.Context) ([]PriorityClassSummary, error) {
	return []PriorityClassSummary{
		{Name: "system-cluster-critical", Value: 2000000000, GlobalDefault: false, Description: "Used for system critical pods that must run in the cluster, but can be moved to another node if necessary.", Age: "60d"},
		{Name: "system-node-critical", Value: 2000001000, GlobalDefault: false, Description: "Used for system critical pods that must not be moved from their current node.", Age: "60d"},
		{Name: "high-priority", Value: 1000000, GlobalDefault: false, Description: "Payments and checkout workloads.", Age: "20d"},
		{Name: "default-priority", Value: 0, GlobalDefault: true, Description: "Default priority for everything else.", Age: "60d"},
	}, nil
}

func (d *DemoProvider) ListPersistentVolumes(ctx context.Context) ([]PersistentVolumeSummary, error) {
	return []PersistentVolumeSummary{
		{Name: "pvc-edge-1", Capacity: "100Gi", AccessModes: "ReadWriteOnce", ReclaimPolicy: "Delete", Status: "Bound", Claim: "default/edge-api-data", StorageClass: "zyvor-rbd-prod", Age: "12d"},
		{Name: "pvc-obs-1", Capacity: "50Gi", AccessModes: "ReadWriteOnce", ReclaimPolicy: "Delete", Status: "Bound", Claim: "platform/obs-wal", StorageClass: "zyvor-rbd-prod", Age: "4d"},
		{Name: "pvc-pg-1", Capacity: "200Gi", AccessModes: "ReadWriteMany", ReclaimPolicy: "Retain", Status: "Bound", Claim: "data/postgres-data", StorageClass: "zyvor-cephfs-shared", Age: "28d"},
	}, nil
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
