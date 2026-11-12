//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	nvidiaClusterPolicyGVR = schema.GroupVersionResource{
		Group: "nvidia.com", Version: "v1", Resource: "clusterpolicies",
	}
	nvidiaGPUClusterGVR = schema.GroupVersionResource{
		Group: "nvidia.com", Version: "v1alpha1", Resource: "gpuclusters",
	}
	draDeviceClassGVR = schema.GroupVersionResource{
		Group: "resource.k8s.io", Version: "v1", Resource: "deviceclasses",
	}
	draResourceSliceGVR = schema.GroupVersionResource{
		Group: "resource.k8s.io", Version: "v1", Resource: "resourceslices",
	}
	draResourceClaimGVR = schema.GroupVersionResource{
		Group: "resource.k8s.io", Version: "v1", Resource: "resourceclaims",
	}
)

func resourceQuantityMap(list corev1.ResourceList) map[string]int64 {
	out := map[string]int64{}
	for name, q := range list {
		n := string(name)
		if n == nvidiaGPUResource || strings.HasPrefix(n, "nvidia.com/mig-") {
			out[n] = q.Value()
		}
	}
	return out
}

func containerGPURequests(p corev1.Pod) map[string]int64 {
	out := map[string]int64{}
	for _, c := range p.Spec.Containers {
		for name, q := range c.Resources.Requests {
			n := string(name)
			if n == nvidiaGPUResource || strings.HasPrefix(n, "nvidia.com/mig-") {
				out[n] += q.Value()
			}
		}
		if len(out) == 0 {
			for name, q := range c.Resources.Limits {
				n := string(name)
				if n == nvidiaGPUResource || strings.HasPrefix(n, "nvidia.com/mig-") {
					out[n] += q.Value()
				}
			}
		}
	}
	return out
}

func (k *KubernetesProvider) detectNVIDIAOperator(ctx context.Context) AcceleratorsOperator {
	op := AcceleratorsOperator{}
	dss, err := k.client.AppsV1().DaemonSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ds := range dss.Items {
			name := strings.ToLower(ds.Name)
			ns := strings.ToLower(ds.Namespace)
			blob := name + " " + ns
			for _, v := range ds.Labels {
				blob += " " + strings.ToLower(v)
			}
			if strings.Contains(blob, "nvidia-device-plugin") || (strings.Contains(blob, "device-plugin") && strings.Contains(blob, "nvidia")) {
				op.DevicePlugin = true
			}
			if strings.Contains(blob, "dcgm-exporter") || (strings.Contains(blob, "dcgm") && strings.Contains(blob, "nvidia")) {
				op.DCGMExporter = true
			}
			if strings.Contains(blob, "gpu-operator") || strings.Contains(blob, "nvidia-operator") {
				op.GPUOperator = true
			}
		}
	}
	if k.dynamic != nil {
		if list, err := k.dynamic.Resource(nvidiaClusterPolicyGVR).List(ctx, metav1.ListOptions{}); err == nil && len(list.Items) > 0 {
			op.GPUOperator = true
			op.ClusterPolicy = list.Items[0].GetName()
			if st, ok := list.Items[0].Object["status"].(map[string]any); ok {
				if s, _ := st["state"].(string); s != "" {
					op.State = s
				}
			}
		}
		if list, err := k.dynamic.Resource(nvidiaGPUClusterGVR).List(ctx, metav1.ListOptions{}); err == nil && len(list.Items) > 0 {
			op.GPUOperator = true
			op.GPUCluster = list.Items[0].GetName()
			if st, ok := list.Items[0].Object["status"].(map[string]any); ok {
				if s, _ := st["state"].(string); s != "" && op.State == "" {
					op.State = s
				}
			}
		}
	}
	if !op.GPUOperator && !op.DevicePlugin && !op.DCGMExporter {
		op.Note = "Install NVIDIA GPU Operator for drivers, device plugin, and DCGM"
	}
	return op
}

func (k *KubernetesProvider) detectDRA(ctx context.Context) *AcceleratorsDRA {
	if k.dynamic == nil {
		return nil
	}
	dra := &AcceleratorsDRA{}
	count := func(gvr schema.GroupVersionResource) int {
		list, err := k.dynamic.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 500})
		if err != nil {
			// cluster-scoped resources
			list, err = k.dynamic.Resource(gvr).List(ctx, metav1.ListOptions{Limit: 500})
			if err != nil {
				return -1
			}
		}
		return len(list.Items)
	}
	dc := count(draDeviceClassGVR)
	if dc < 0 {
		dra.Note = "resource.k8s.io DRA CRDs not available"
		return dra
	}
	dra.Available = true
	dra.DeviceClasses = dc
	if n := count(draResourceSliceGVR); n >= 0 {
		dra.ResourceSlices = n
	}
	if n := count(draResourceClaimGVR); n >= 0 {
		dra.ResourceClaims = n
	}
	return dra
}

func (k *KubernetesProvider) gpuSchedulingIssues(ctx context.Context, pods []Pod) map[string]string {
	out := map[string]string{}
	evs, err := k.client.CoreV1().Events(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: "reason=FailedScheduling",
	})
	if err != nil {
		return out
	}
	want := map[string]bool{}
	for _, p := range pods {
		if p.GPU != "" && (p.Status == "Pending" || p.Node == "") {
			want[p.Namespace+"/"+p.Name] = true
		}
	}
	for _, e := range evs.Items {
		key := e.InvolvedObject.Namespace + "/" + e.InvolvedObject.Name
		if !want[key] {
			continue
		}
		msg := e.Message
		if !strings.Contains(strings.ToLower(msg), "nvidia.com/gpu") && !strings.Contains(strings.ToLower(msg), "mig-") {
			if !strings.Contains(strings.ToLower(msg), "insufficient") {
				continue
			}
		}
		out[key] = msg
	}
	return out
}

func (k *KubernetesProvider) acceleratorsMetrics(ctx context.Context, rep *AcceleratorsReport) {
	client := PrometheusFromProvider(k)
	if client == nil {
		return
	}
	m := &AcceleratorsMetrics{Available: true}
	m.GPUUtil = firstPromValue(ctx, client, []string{
		`avg(DCGM_FI_DEV_GPU_UTIL)`,
		`avg(DCGM_FI_DEV_GPU_UTIL_RATIO)*100`,
		`avg(DCGM_FI_PROF_GR_ENGINE_ACTIVE)*100`,
	})
	m.FBUsedBytes = firstPromValue(ctx, client, []string{
		`avg(DCGM_FI_DEV_FB_USED)*1024*1024`,
		`avg(DCGM_FI_DEV_FB_USED_BYTES)`,
	})
	m.FBFreeBytes = firstPromValue(ctx, client, []string{
		`avg(DCGM_FI_DEV_FB_FREE)*1024*1024`,
		`avg(DCGM_FI_DEV_FB_FREE_BYTES)`,
	})
	m.TempC = firstPromValue(ctx, client, []string{
		`avg(DCGM_FI_DEV_GPU_TEMP)`,
		`avg(DCGM_FI_DEV_GPU_TEMP_CELSIUS)`,
	})
	m.PowerW = firstPromValue(ctx, client, []string{
		`avg(DCGM_FI_DEV_POWER_USAGE)`,
		`avg(DCGM_FI_DEV_POWER_USAGE_WATTS)`,
	})
	m.XID = firstPromValue(ctx, client, []string{
		`sum(DCGM_FI_DEV_XID_ERRORS)`,
		`sum(DCGM_FI_DEV_XID_ERRORS_TOTAL)`,
	})
	if m.GPUUtil == 0 && rep.Counts["gpus"] > 0 && rep.Counts["pods"] > 0 {
		m.IdleHint = "GPU pods are scheduled but DCGM util is ~0 — possible idle / time-sliced attribution gap"
	} else if m.GPUUtil > 0 && m.GPUUtil < 10 && rep.Counts["pods"] > 0 {
		m.IdleHint = fmt.Sprintf("Average GPU util ~%.0f%% with %d GPU pod(s) — silicon may be underused", m.GPUUtil, rep.Counts["pods"])
	}
	if m.GPUUtil == 0 && m.FBUsedBytes == 0 && m.TempC == 0 && m.PowerW == 0 && m.IdleHint == "" {
		m.Note = "DCGM series not found in Prometheus (install GPU Operator DCGM Exporter and scrape :9400)"
	}
	rep.Metrics = m
}

// firstPromValue returns the first non-empty instant sample across queries.
func firstPromValue(ctx context.Context, client *PrometheusClient, queries []string) float64 {
	if client == nil {
		return 0
	}
	for _, q := range queries {
		samples, err := client.Query(ctx, q)
		if err != nil || len(samples) == 0 {
			continue
		}
		return samples[0].Value
	}
	return 0
}

func (k *KubernetesProvider) ListAccelerators(ctx context.Context) (AcceleratorsReport, error) {
	nodes, err := k.ListNodes(ctx)
	if err != nil {
		return AcceleratorsReport{}, err
	}
	pods, err := k.ListPods(ctx, "all")
	if err != nil {
		return AcceleratorsReport{}, err
	}
	issues := k.gpuSchedulingIssues(ctx, pods)
	for i := range pods {
		key := pods[i].Namespace + "/" + pods[i].Name
		if msg, ok := issues[key]; ok {
			pods[i].Message = msg
		}
	}
	rep := buildAcceleratorsReport(nodes, pods, k.detectNVIDIAOperator(ctx))
	if quotas, err := k.ListResourceQuotas(ctx, "all"); err == nil {
		rep.Quotas = filterGPUQuotas(quotas)
	}
	rep.DRA = k.detectDRA(ctx)
	k.acceleratorsMetrics(ctx, &rep)
	util := 0.0
	if rep.Metrics != nil {
		util = rep.Metrics.GPUUtil
	}
	attachGPUWaste(&rep, util)
	return rep, nil
}
