//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	kubevirtVMGVR  = schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"}
	kubevirtVMIGVR = schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachineinstances"}
)

type utilSample struct {
	at  time.Time
	cpu float64
	mem float64
}

var (
	utilMu sync.Mutex
	// utilHistory is keyed by cluster, like the pod/node metric rings — with a
	// single global slice, switching clusters interleaved their samples.
	utilHistory = map[string][]utilSample{}
)

func (k *KubernetesProvider) ListVirtualMachines(ctx context.Context, ns string) ([]VirtualMachine, error) {
	if k.dynamic == nil {
		return []VirtualMachine{}, nil
	}
	selectorNS := ns
	if ns == "" || ns == "all" {
		selectorNS = metav1.NamespaceAll
	}
	list, err := k.dynamic.Resource(kubevirtVMGVR).Namespace(selectorNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		// KubeVirt may be absent — surface empty rather than failing the console.
		if strings.Contains(err.Error(), "the server could not find the requested resource") ||
			strings.Contains(err.Error(), "no matches for") {
			return []VirtualMachine{}, nil
		}
		return nil, err
	}
	vmiNode := map[string]string{}
	if vmis, err := k.dynamic.Resource(kubevirtVMIGVR).Namespace(selectorNS).List(ctx, metav1.ListOptions{}); err == nil {
		for _, item := range vmis.Items {
			key := item.GetNamespace() + "/" + item.GetName()
			node, _, _ := unstructured.NestedString(item.Object, "status", "nodeName")
			vmiNode[key] = node
		}
	}
	out := make([]VirtualMachine, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, unstructuredToVM(item, vmiNode[item.GetNamespace()+"/"+item.GetName()]))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func unstructuredToVM(item unstructured.Unstructured, node string) VirtualMachine {
	status, _, _ := unstructured.NestedString(item.Object, "status", "printableStatus")
	if status == "" {
		status = "Unknown"
	}
	ready, _, _ := unstructured.NestedBool(item.Object, "status", "ready")
	running, found, _ := unstructured.NestedBool(item.Object, "spec", "running")
	if !found {
		runStrategy, _, _ := unstructured.NestedString(item.Object, "spec", "runStrategy")
		running = strings.EqualFold(runStrategy, "Always") || strings.EqualFold(runStrategy, "RerunOnFailure")
	}
	cores, _, _ := unstructured.NestedInt64(item.Object, "spec", "template", "spec", "domain", "cpu", "cores")
	sockets, _, _ := unstructured.NestedInt64(item.Object, "spec", "template", "spec", "domain", "cpu", "sockets")
	threads, _, _ := unstructured.NestedInt64(item.Object, "spec", "template", "spec", "domain", "cpu", "threads")
	cpu := cores
	if sockets > 0 {
		if cpu == 0 {
			cpu = 1
		}
		cpu *= sockets
	}
	if threads > 0 {
		if cpu == 0 {
			cpu = 1
		}
		cpu *= threads
	}
	mem, _, _ := unstructured.NestedString(item.Object, "spec", "template", "spec", "domain", "memory", "guest")
	if mem == "" {
		mem, _, _ = unstructured.NestedString(item.Object, "spec", "template", "spec", "domain", "resources", "requests", "memory")
	}
	osName := item.GetLabels()["os"]
	if ver := item.GetLabels()["os.version"]; ver != "" {
		if osName != "" {
			osName += " " + ver
		} else {
			osName = ver
		}
	}
	cpuLabel := "—"
	if cpu > 0 {
		cpuLabel = fmt.Sprintf("%d", cpu)
	}
	if mem == "" {
		mem = "—"
	}
	return VirtualMachine{
		Namespace: item.GetNamespace(),
		Name:      item.GetName(),
		Status:    status,
		Ready:     ready,
		Running:   running,
		CPU:       cpuLabel,
		Memory:    mem,
		Node:      node,
		OS:        osName,
		Age:       age(item.GetCreationTimestamp().Time),
	}
}

func (k *KubernetesProvider) StartVirtualMachine(ctx context.Context, ns, name string) error {
	return k.setVMRunStrategy(ctx, ns, name, "Always")
}

func (k *KubernetesProvider) StopVirtualMachine(ctx context.Context, ns, name string) error {
	return k.setVMRunStrategy(ctx, ns, name, "Halted")
}

func (k *KubernetesProvider) setVMRunStrategy(ctx context.Context, ns, name, strategy string) error {
	if k.dynamic == nil {
		return fmt.Errorf("kubevirt client unavailable")
	}
	obj, err := k.dynamic.Resource(kubevirtVMGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	unstructured.RemoveNestedField(obj.Object, "spec", "running")
	if err := unstructured.SetNestedField(obj.Object, strategy, "spec", "runStrategy"); err != nil {
		return err
	}
	_, err = k.dynamic.Resource(kubevirtVMGVR).Namespace(ns).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func (k *KubernetesProvider) ListVolumes(ctx context.Context, ns string) ([]Volume, error) {
	selectorNS := ns
	if ns == "" || ns == "all" {
		selectorNS = metav1.NamespaceAll
	}
	list, err := k.client.CoreV1().PersistentVolumeClaims(selectorNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Volume, 0, len(list.Items))
	for _, pvc := range list.Items {
		cap := "—"
		if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			cap = q.String()
		} else if pvc.Spec.Resources.Requests != nil {
			if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
				cap = q.String()
			}
		}
		sc := ""
		if pvc.Spec.StorageClassName != nil {
			sc = *pvc.Spec.StorageClassName
		}
		modes := make([]string, 0, len(pvc.Spec.AccessModes))
		for _, m := range pvc.Spec.AccessModes {
			modes = append(modes, string(m))
		}
		out = append(out, Volume{
			Namespace:    pvc.Namespace,
			Name:         pvc.Name,
			Status:       string(pvc.Status.Phase),
			Capacity:     cap,
			StorageClass: sc,
			AccessModes:  strings.Join(modes, ","),
			VolumeName:   pvc.Spec.VolumeName,
			Age:          age(pvc.CreationTimestamp.Time),
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

func (k *KubernetesProvider) ListStorageClasses(ctx context.Context) ([]StorageClassSummary, error) {
	list, err := k.client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]StorageClassSummary, 0, len(list.Items))
	for _, sc := range list.Items {
		reclaim := ""
		if sc.ReclaimPolicy != nil {
			reclaim = string(*sc.ReclaimPolicy)
		}
		binding := ""
		if sc.VolumeBindingMode != nil {
			binding = string(*sc.VolumeBindingMode)
		}
		out = append(out, StorageClassSummary{
			Name:              sc.Name,
			Provisioner:       sc.Provisioner,
			ReclaimPolicy:     reclaim,
			VolumeBindingMode: binding,
			IsDefault:         sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true",
			Age:               age(sc.CreationTimestamp.Time),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (k *KubernetesProvider) ListPriorityClasses(ctx context.Context) ([]PriorityClassSummary, error) {
	list, err := k.client.SchedulingV1().PriorityClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]PriorityClassSummary, 0, len(list.Items))
	for _, pc := range list.Items {
		out = append(out, PriorityClassSummary{
			Name:          pc.Name,
			Value:         pc.Value,
			GlobalDefault: pc.GlobalDefault,
			Description:   pc.Description,
			Age:           age(pc.CreationTimestamp.Time),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	return out, nil
}

func (k *KubernetesProvider) ListPersistentVolumes(ctx context.Context) ([]PersistentVolumeSummary, error) {
	list, err := k.client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]PersistentVolumeSummary, 0, len(list.Items))
	for _, pv := range list.Items {
		cap := "—"
		if q, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
			cap = q.String()
		}
		modes := make([]string, 0, len(pv.Spec.AccessModes))
		for _, m := range pv.Spec.AccessModes {
			modes = append(modes, string(m))
		}
		claim := ""
		if pv.Spec.ClaimRef != nil {
			claim = pv.Spec.ClaimRef.Namespace + "/" + pv.Spec.ClaimRef.Name
		}
		out = append(out, PersistentVolumeSummary{
			Name:          pv.Name,
			Capacity:      cap,
			AccessModes:   strings.Join(modes, ","),
			ReclaimPolicy: string(pv.Spec.PersistentVolumeReclaimPolicy),
			Status:        string(pv.Status.Phase),
			Claim:         claim,
			StorageClass:  pv.Spec.StorageClassName,
			Age:           age(pv.CreationTimestamp.Time),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (k *KubernetesProvider) Utilization(ctx context.Context) (UtilizationSeries, error) {
	cpu, mem, err := k.readNodeMetrics(ctx)
	source := "metrics-server"
	if err != nil {
		// No metrics-server (or it failed): say so. Previously this invented
		// 42.8% / 60% and recorded them as real samples, so the dashboard
		// showed plausible utilization on a cluster that reported none.
		return UtilizationSeries{Points: []UtilizationPoint{}, Source: "unavailable"}, nil
	}
	recordUtilization(k.clusterKey, cpu, mem)
	points := utilizationSeries(k.clusterKey)
	peakCPU, peakMem := cpu, mem
	for _, p := range points {
		if p.CPU > peakCPU {
			peakCPU = p.CPU
		}
		if p.Memory > peakMem {
			peakMem = p.Memory
		}
	}
	return UtilizationSeries{
		CPUPercent:    round1(cpu),
		MemoryPercent: round1(mem),
		PeakCPU:       round1(peakCPU),
		PeakMemory:    round1(peakMem),
		Points:        points,
		Source:        source,
	}, nil
}

type metricsList struct {
	Items []struct {
		Usage struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"usage"`
	} `json:"items"`
}

func (k *KubernetesProvider) readNodeMetrics(ctx context.Context) (float64, float64, error) {
	nodes, err := k.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, 0, err
	}
	var allocCPU, allocMem int64
	for _, n := range nodes.Items {
		allocCPU += n.Status.Allocatable.Cpu().MilliValue()
		allocMem += n.Status.Allocatable.Memory().Value()
	}
	if allocCPU == 0 || allocMem == 0 {
		return 0, 0, fmt.Errorf("no allocatable capacity")
	}
	data, err := k.client.CoreV1().RESTClient().Get().
		AbsPath("/apis/metrics.k8s.io/v1beta1/nodes").
		Do(ctx).Raw()
	if err != nil {
		return 0, 0, err
	}
	var ml metricsList
	if err := json.Unmarshal(data, &ml); err != nil {
		return 0, 0, err
	}
	var usedCPU, usedMem int64
	for _, item := range ml.Items {
		usedCPU += parseCPUMillis(item.Usage.CPU)
		usedMem += parseMemoryBytes(item.Usage.Memory)
	}
	cpuPct := (float64(usedCPU) / float64(allocCPU)) * 100
	memPct := (float64(usedMem) / float64(allocMem)) * 100
	return clampPct(cpuPct), clampPct(memPct), nil
}

func parseCPUMillis(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "n") {
		var n int64
		fmt.Sscanf(s, "%dn", &n)
		return n / 1_000_000
	}
	if strings.HasSuffix(s, "u") {
		var n int64
		fmt.Sscanf(s, "%du", &n)
		return n / 1000
	}
	if strings.HasSuffix(s, "m") {
		var n int64
		fmt.Sscanf(s, "%dm", &n)
		return n
	}
	var cores float64
	fmt.Sscanf(s, "%f", &cores)
	return int64(cores * 1000)
}

func recordUtilization(cluster string, cpu, mem float64) {
	utilMu.Lock()
	defer utilMu.Unlock()
	now := time.Now().UTC()
	h := utilHistory[cluster]
	if len(h) > 0 && now.Sub(h[len(h)-1].at) < 20*time.Second {
		h[len(h)-1] = utilSample{at: now, cpu: cpu, mem: mem}
		return
	}
	h = append(h, utilSample{at: now, cpu: cpu, mem: mem})
	if len(h) > 48 {
		h = h[len(h)-48:]
	}
	utilHistory[cluster] = h
}

// utilizationSeries returns only samples that were actually measured. It used
// to pad short histories with a decorative sine-wave "24h" curve, which
// presented invented data as history.
func utilizationSeries(cluster string) []UtilizationPoint {
	utilMu.Lock()
	hist := append([]utilSample(nil), utilHistory[cluster]...)
	utilMu.Unlock()
	out := make([]UtilizationPoint, 0, len(hist))
	for _, s := range hist {
		out = append(out, UtilizationPoint{At: s.at.Format("15:04"), CPU: round1(s.cpu), Memory: round1(s.mem)})
	}
	return out
}
