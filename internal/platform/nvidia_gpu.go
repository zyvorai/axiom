package platform

import (
	"context"
	"fmt"
	"strings"
)

const (
	nvidiaGPUResource     = "nvidia.com/gpu"
	nvidiaGPUProduct      = "nvidia.com/gpu.product"
	nvidiaCUDADriver      = "nvidia.com/cuda.driver-version.full"
	nvidiaGPUDriver       = "nvidia.com/gpu.driver"
	nvidiaSharingStrategy = "nvidia.com/gpu.sharing-strategy"
	nvidiaGPUReplicas     = "nvidia.com/gpu.replicas"
	nvidiaMIGConfig       = "nvidia.com/mig.config"
	nvidiaMIGConfigState  = "nvidia.com/mig.config.state"
)

// applyNodeGPUFields fills NVIDIA capacity/label fields on a Node summary.
func applyNodeGPUFields(n *Node, capacity, allocatable map[string]int64, labels map[string]string) {
	if n == nil {
		return
	}
	if v, ok := capacity[nvidiaGPUResource]; ok && v > 0 {
		n.GPU = fmt.Sprintf("%d", v)
	}
	if v, ok := allocatable[nvidiaGPUResource]; ok && v > 0 {
		n.AllocatableGPU = fmt.Sprintf("%d", v)
	}
	if labels != nil {
		n.GPUProduct = firstNonEmpty(labels[nvidiaGPUProduct], labels["nvidia.com/gpu.product"])
		n.GPUDriver = firstNonEmpty(labels[nvidiaCUDADriver], labels[nvidiaGPUDriver])
		n.SharingStrategy = labels[nvidiaSharingStrategy]
		n.GPUReplicas = labels[nvidiaGPUReplicas]
		n.MIGConfig = labels[nvidiaMIGConfig]
		n.MIGConfigState = labels[nvidiaMIGConfigState]
	}
	mig := map[string]string{}
	for k, v := range capacity {
		if strings.HasPrefix(k, "nvidia.com/mig-") && v > 0 {
			mig[k] = fmt.Sprintf("%d", v)
		}
	}
	if len(mig) > 0 {
		n.MIG = mig
	}
}

func podGPURequest(requests map[string]int64) string {
	if v, ok := requests[nvidiaGPUResource]; ok && v > 0 {
		return fmt.Sprintf("%d", v)
	}
	for k, v := range requests {
		if strings.HasPrefix(k, "nvidia.com/mig-") && v > 0 {
			return fmt.Sprintf("%s=%d", strings.TrimPrefix(k, "nvidia.com/"), v)
		}
	}
	return ""
}

func isGPUQuotaKey(k string) bool {
	k = strings.ToLower(k)
	return k == "requests.nvidia.com/gpu" || k == "nvidia.com/gpu" ||
		strings.HasPrefix(k, "requests.nvidia.com/mig-") || strings.HasPrefix(k, "nvidia.com/mig-")
}

func filterGPUQuotas(quotas []ResourceQuotaSummary) []AcceleratorQuota {
	var out []AcceleratorQuota
	for _, q := range quotas {
		hard, used := map[string]string{}, map[string]string{}
		for k, v := range q.Hard {
			if isGPUQuotaKey(k) {
				hard[k] = v
			}
		}
		for k, v := range q.Used {
			if isGPUQuotaKey(k) {
				used[k] = v
			}
		}
		if len(hard) == 0 && len(used) == 0 {
			continue
		}
		out = append(out, AcceleratorQuota{Namespace: q.Namespace, Name: q.Name, Hard: hard, Used: used})
	}
	return out
}

func buildAcceleratorsReport(nodes []Node, pods []Pod, op AcceleratorsOperator) AcceleratorsReport {
	rep := AcceleratorsReport{
		Provider: "none",
		Status:   "missing",
		Operator: op,
		Nodes:    []AcceleratorNode{},
		Pods:     []AcceleratorPod{},
		Counts:   map[string]int{},
	}
	gpuNodes := 0
	gpuCount := 0
	for _, n := range nodes {
		if n.GPU == "" && n.AllocatableGPU == "" && len(n.MIG) == 0 {
			continue
		}
		gpuNodes++
		if n.GPU != "" {
			var c int
			fmt.Sscanf(n.GPU, "%d", &c)
			gpuCount += c
		}
		rep.Nodes = append(rep.Nodes, AcceleratorNode{
			Name: n.Name, Status: n.Status, GPU: n.GPU, AllocatableGPU: n.AllocatableGPU,
			Product: n.GPUProduct, Driver: n.GPUDriver, MIG: n.MIG, Unschedulable: n.Unschedulable,
			SharingStrategy: n.SharingStrategy, Replicas: n.GPUReplicas,
			MIGConfig: n.MIGConfig, MIGConfigState: n.MIGConfigState,
		})
	}
	for _, p := range pods {
		if p.GPU == "" {
			continue
		}
		rep.Pods = append(rep.Pods, AcceleratorPod{
			Namespace: p.Namespace, Name: p.Name, Node: p.Node, GPU: p.GPU, Status: p.Status, Image: p.Image,
			SchedulingIssue: p.Message,
		})
	}
	rep.Counts["nodes"] = gpuNodes
	rep.Counts["gpus"] = gpuCount
	rep.Counts["pods"] = len(rep.Pods)

	hasStack := op.GPUOperator || op.DevicePlugin || op.DCGMExporter || op.ClusterPolicy != "" || op.GPUCluster != ""
	switch {
	case gpuNodes > 0 && hasStack:
		rep.Provider, rep.Status = "nvidia", "ready"
		rep.Summary = fmt.Sprintf("%d GPU node(s), %d GPU(s), %d GPU pod(s); NVIDIA stack detected", gpuNodes, gpuCount, len(rep.Pods))
	case gpuNodes > 0:
		rep.Provider, rep.Status = "nvidia", "degraded"
		rep.Summary = fmt.Sprintf("%d GPU node(s) with capacity, but GPU Operator / device plugin not detected", gpuNodes)
		if rep.Operator.Note == "" {
			rep.Operator.Note = "Install the NVIDIA GPU Operator for drivers, device plugin, and DCGM"
		}
	case hasStack:
		rep.Provider, rep.Status = "nvidia", "degraded"
		rep.Summary = "NVIDIA components present, but no nvidia.com/gpu capacity on nodes yet"
	default:
		rep.Summary = "No NVIDIA GPUs or GPU Operator detected"
		rep.Operator.Note = "Install NVIDIA GPU Operator, then label/taint GPU nodes as usual"
	}
	return rep
}

// ListAccelerators for DemoProvider.
func (d *DemoProvider) ListAccelerators(ctx context.Context) (AcceleratorsReport, error) {
	nodes, err := d.ListNodes(ctx)
	if err != nil {
		return AcceleratorsReport{}, err
	}
	for i := range nodes {
		if nodes[i].Name == "gpu-01" {
			nodes[i].SharingStrategy = "none"
			nodes[i].GPUReplicas = "8"
			nodes[i].MIGConfig = "all-disabled"
			nodes[i].MIGConfigState = "success"
		}
	}
	pods, err := d.ListPods(ctx, "all")
	if err != nil {
		return AcceleratorsReport{}, err
	}
	// Demo pending GPU pod with scheduling pressure.
	pods = append(pods, Pod{
		Namespace: "ml", Name: "train-job-0", Node: "", Status: "Pending", Ready: "0/1",
		GPU: "4", Image: "nvcr.io/nvidia/pytorch:24.08-py3",
		Message: "0/4 nodes are available: 3 Insufficient nvidia.com/gpu, 1 node(s) had taint {nvidia.com/gpu}",
	})
	op := AcceleratorsOperator{
		GPUOperator: true, DevicePlugin: true, DCGMExporter: true,
		ClusterPolicy: "cluster-policy", State: "ready",
		Note: "Demo cluster simulates NVIDIA GPU Operator",
	}
	rep := buildAcceleratorsReport(nodes, pods, op)
	quotas, _ := d.ListResourceQuotas(ctx, "all")
	// Inject a demo GPU quota on ml.
	rep.Quotas = append(filterGPUQuotas(quotas), AcceleratorQuota{
		Namespace: "ml", Name: "ml-gpu-quota",
		Hard: map[string]string{"requests.nvidia.com/gpu": "8"},
		Used: map[string]string{"requests.nvidia.com/gpu": "2"},
	})
	rep.DRA = &AcceleratorsDRA{Available: false, Note: "resource.k8s.io DRA not present in demo"}
	rep.Metrics = &AcceleratorsMetrics{
		Available: true, GPUUtil: 42,
		IdleHint: "2 of 8 GPUs requested; demo util ~42% — not idle",
		Note:     "Demo DCGM snapshot",
	}
	return rep, nil
}
