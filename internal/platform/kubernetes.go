//go:build kubernetes

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	lastReplicasAnnotation = "axiom.zyvor.dev/last-replicas"
	restartedAtAnnotation  = "axiom.zyvor.dev/restarted-at"
)

type KubernetesProvider struct {
	client  kubernetes.Interface
	dynamic dynamic.Interface
	config  *rest.Config
	// clusterKey distinguishes this provider's cached in-process state (the
	// pod/node metrics history ring in ops_kubernetes.go) from another
	// KubernetesProvider's when multiple are registered in the same process
	// — the multi-context kubeconfig case (see
	// factory_kubernetes.go:newKubernetesClusterRegistry). Left empty for
	// the single-cluster in-cluster/default construction paths, where only
	// one KubernetesProvider ever exists so there's nothing to collide with.
	clusterKey string
}

func NewKubernetesProvider(kubeconfig string) (*KubernetesProvider, error) {
	cfg, err := buildConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	applyClientDefaults(cfg)
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &KubernetesProvider{client: client, config: cfg, dynamic: dyn}, nil
}

func buildConfig(path string) (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".kube", "config")
		}
	}
	return clientcmd.BuildConfigFromFlags("", path)
}

func (k *KubernetesProvider) Overview(ctx context.Context) (Overview, error) {
	// Lightweight counts — avoid nested ListWorkloads+ListPods which amplify API pressure.
	deps, err := k.client.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Overview{}, err
	}
	sts, err := k.client.AppsV1().StatefulSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Overview{}, err
	}
	pods, err := k.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Overview{}, err
	}
	nodes, err := k.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Overview{}, err
	}
	running := 0
	warnings := 0
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			running++
		} else if p.Status.Phase != corev1.PodSucceeded {
			warnings++
		}
	}
	readyNodes := 0
	for _, n := range nodes.Items {
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				readyNodes++
				break
			}
		}
	}
	vms, _ := k.ListVirtualMachines(ctx, "all")
	readyVMs := 0
	for _, vm := range vms {
		if vm.Ready || strings.EqualFold(vm.Status, "Running") {
			readyVMs++
		}
	}
	vols, _ := k.ListVolumes(ctx, "all")
	var usedBytes int64
	for _, v := range vols {
		usedBytes += parseMemoryBytes(v.Capacity)
	}
	// Soft capacity target: 4 TB private cloud allocation shown in Emergent mockup,
	// raised automatically when real PVC usage exceeds it.
	capacityBytes := int64(4) * 1024 * 1024 * 1024 * 1024
	if usedBytes > capacityBytes {
		capacityBytes = usedBytes + (512 * 1024 * 1024 * 1024)
	}
	util, _ := k.Utilization(ctx)
	regions := DefaultRegions("zyvor")
	health := "Healthy"
	if readyNodes < len(nodes.Items) || warnings > 0 {
		health = "Attention"
	}
	return Overview{
		Cluster: "zyvor", Health: health,
		Workloads: len(deps.Items) + len(sts.Items),
		Pods:      len(pods.Items), RunningPods: running,
		Nodes: len(nodes.Items), ReadyNodes: readyNodes,
		VMs: len(vms), ReadyVMs: readyVMs,
		CPUPercent: util.CPUPercent, MemoryPercent: util.MemoryPercent,
		StorageUsedBytes: usedBytes, StorageCapacityBytes: capacityBytes,
		StorageUsedTB: bytesToTB(usedBytes), StorageCapacityTB: bytesToTB(capacityBytes),
		Region: ActiveRegion(regions), Regions: regions,
		Utilization: util.Points, Warnings: warnings,
	}, nil
}

func (k *KubernetesProvider) ListWorkloads(ctx context.Context, ns string) ([]Workload, error) {
	selectorNS := ns
	if ns == "" || ns == "all" {
		selectorNS = metav1.NamespaceAll
	}
	deps, err := k.client.AppsV1().Deployments(selectorNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	sts, err := k.client.AppsV1().StatefulSets(selectorNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	dss, err := k.client.AppsV1().DaemonSets(selectorNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Workload, 0, len(deps.Items)+len(sts.Items)+len(dss.Items))
	for _, d := range deps.Items {
		out = append(out, deploymentToWorkload(d))
	}
	for _, s := range sts.Items {
		out = append(out, statefulSetToWorkload(s))
	}
	for _, ds := range dss.Items {
		out = append(out, daemonSetToWorkload(ds))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func deploymentToWorkload(d appsv1.Deployment) Workload {
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	image := ""
	if len(d.Spec.Template.Spec.Containers) > 0 {
		image = d.Spec.Template.Spec.Containers[0].Image
	}
	status := "Healthy"
	if desired == 0 {
		status = "Stopped"
	} else if d.Status.AvailableReplicas < desired {
		status = "Degraded"
	}
	return Workload{Namespace: d.Namespace, Name: d.Name, Kind: "Deployment", Image: image, Desired: desired, Ready: d.Status.ReadyReplicas, Available: d.Status.AvailableReplicas, Status: status, Age: age(d.CreationTimestamp.Time), Labels: d.Labels}
}
func statefulSetToWorkload(s appsv1.StatefulSet) Workload {
	desired := int32(1)
	if s.Spec.Replicas != nil {
		desired = *s.Spec.Replicas
	}
	image := ""
	if len(s.Spec.Template.Spec.Containers) > 0 {
		image = s.Spec.Template.Spec.Containers[0].Image
	}
	status := "Healthy"
	if desired == 0 {
		status = "Stopped"
	} else if s.Status.ReadyReplicas < desired {
		status = "Degraded"
	}
	return Workload{Namespace: s.Namespace, Name: s.Name, Kind: "StatefulSet", Image: image, Desired: desired, Ready: s.Status.ReadyReplicas, Available: s.Status.CurrentReplicas, Status: status, Age: age(s.CreationTimestamp.Time), Labels: s.Labels}
}

// daemonSetToWorkload has no replica count to report — a DaemonSet runs
// exactly one Pod per matching node, so "Desired" here means "how many
// nodes it should be scheduled on," not a scalable target. Scale/Stop/
// Start correctly don't apply and keep returning their existing
// "unsupported workload kind" error for it.
func daemonSetToWorkload(d appsv1.DaemonSet) Workload {
	image := ""
	if len(d.Spec.Template.Spec.Containers) > 0 {
		image = d.Spec.Template.Spec.Containers[0].Image
	}
	desired := d.Status.DesiredNumberScheduled
	status := "Healthy"
	if d.Status.NumberReady < desired {
		status = "Degraded"
	}
	return Workload{Namespace: d.Namespace, Name: d.Name, Kind: "DaemonSet", Image: image, Desired: desired, Ready: d.Status.NumberReady, Available: d.Status.NumberAvailable, Status: status, Age: age(d.CreationTimestamp.Time), Labels: d.Labels}
}

func (k *KubernetesProvider) Launch(ctx context.Context, req LaunchRequest) (Workload, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Image) == "" {
		return Workload{}, errors.New("name and image are required")
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Replicas < 1 {
		req.Replicas = 1
	}
	labels := map[string]string{"app.kubernetes.io/name": req.Name, "app.kubernetes.io/managed-by": "zyvor-axiom"}
	container := corev1.Container{Name: req.Name, Image: req.Image, ImagePullPolicy: corev1.PullIfNotPresent}
	if req.Port > 0 {
		container.Ports = []corev1.ContainerPort{{Name: "http", ContainerPort: req.Port}}
	}
	for key, val := range req.Environment {
		container.Env = append(container.Env, corev1.EnvVar{Name: key, Value: val})
	}
	if req.CPU != "" || req.Memory != "" {
		container.Resources = corev1.ResourceRequirements{Requests: corev1.ResourceList{}, Limits: corev1.ResourceList{}}
		if req.CPU != "" {
			q, err := resource.ParseQuantity(req.CPU)
			if err != nil {
				return Workload{}, fmt.Errorf("invalid cpu: %w", err)
			}
			container.Resources.Requests[corev1.ResourceCPU] = q
			container.Resources.Limits[corev1.ResourceCPU] = q
		}
		if req.Memory != "" {
			q, err := resource.ParseQuantity(req.Memory)
			if err != nil {
				return Workload{}, fmt.Errorf("invalid memory: %w", err)
			}
			container.Resources.Requests[corev1.ResourceMemory] = q
			container.Resources.Limits[corev1.ResourceMemory] = q
		}
	}
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, Labels: labels, Annotations: map[string]string{}}, Spec: appsv1.DeploymentSpec{Replicas: &req.Replicas, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{Containers: []corev1.Container{container}}}}}
	if req.Intent != "" {
		dep.Annotations[IntentAnnotation] = req.Intent
	}
	created, err := k.client.AppsV1().Deployments(req.Namespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil {
		return Workload{}, err
	}
	if req.Expose && req.Port > 0 {
		svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, Labels: labels}, Spec: corev1.ServiceSpec{Selector: labels, Ports: []corev1.ServicePort{{Name: "http", Port: req.Port, TargetPort: intstr.FromInt32(req.Port)}}}}
		if _, err := k.client.CoreV1().Services(req.Namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
			return deploymentToWorkload(*created), fmt.Errorf("deployment created but service failed: %w", err)
		}
	}
	return deploymentToWorkload(*created), nil
}

func (k *KubernetesProvider) Scale(ctx context.Context, ns, kind, name string, replicas int32) error {
	if replicas < 0 {
		return errors.New("replicas must be >= 0")
	}
	switch strings.ToLower(kind) {
	case "deployment":
		obj, err := k.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.Spec.Replicas = &replicas
		_, err = k.client.AppsV1().Deployments(ns).Update(ctx, obj, metav1.UpdateOptions{})
		return err
	case "statefulset":
		obj, err := k.client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.Spec.Replicas = &replicas
		_, err = k.client.AppsV1().StatefulSets(ns).Update(ctx, obj, metav1.UpdateOptions{})
		return err
	default:
		return fmt.Errorf("unsupported workload kind %q", kind)
	}
}
func (k *KubernetesProvider) Stop(ctx context.Context, ns, kind, name string) error {
	return k.storeAndScale(ctx, ns, kind, name, 0, true)
}
func (k *KubernetesProvider) Start(ctx context.Context, ns, kind, name string) error {
	return k.storeAndScale(ctx, ns, kind, name, -1, false)
}
func (k *KubernetesProvider) storeAndScale(ctx context.Context, ns, kind, name string, target int32, stopping bool) error {
	switch strings.ToLower(kind) {
	case "deployment":
		obj, err := k.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if obj.Annotations == nil {
			obj.Annotations = map[string]string{}
		}
		current := int32(1)
		if obj.Spec.Replicas != nil {
			current = *obj.Spec.Replicas
		}
		if stopping && current > 0 {
			obj.Annotations[lastReplicasAnnotation] = strconv.Itoa(int(current))
			target = 0
		} else if !stopping && current > 0 {
			target = current // already running: Start must not change the replica count
		} else if !stopping {
			target = 3
			if saved, err := strconv.Atoi(obj.Annotations[lastReplicasAnnotation]); err == nil && saved > 0 {
				target = int32(saved)
			}
		}
		obj.Spec.Replicas = &target
		_, err = k.client.AppsV1().Deployments(ns).Update(ctx, obj, metav1.UpdateOptions{})
		return err
	case "statefulset":
		obj, err := k.client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if obj.Annotations == nil {
			obj.Annotations = map[string]string{}
		}
		current := int32(1)
		if obj.Spec.Replicas != nil {
			current = *obj.Spec.Replicas
		}
		if stopping && current > 0 {
			obj.Annotations[lastReplicasAnnotation] = strconv.Itoa(int(current))
			target = 0
		} else if !stopping && current > 0 {
			target = current // already running: Start must not change the replica count
		} else if !stopping {
			target = 3
			if saved, err := strconv.Atoi(obj.Annotations[lastReplicasAnnotation]); err == nil && saved > 0 {
				target = int32(saved)
			}
		}
		obj.Spec.Replicas = &target
		_, err = k.client.AppsV1().StatefulSets(ns).Update(ctx, obj, metav1.UpdateOptions{})
		return err
	default:
		return fmt.Errorf("unsupported workload kind %q", kind)
	}
}
func (k *KubernetesProvider) Restart(ctx context.Context, ns, kind, name string) error {
	stamp := time.Now().UTC().Format(time.RFC3339)
	patch := map[string]any{"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{"annotations": map[string]string{restartedAtAnnotation: stamp}}}}}
	raw, _ := json.Marshal(patch)
	switch strings.ToLower(kind) {
	case "deployment":
		_, err := k.client.AppsV1().Deployments(ns).Patch(ctx, name, types.MergePatchType, raw, metav1.PatchOptions{})
		return err
	case "statefulset":
		_, err := k.client.AppsV1().StatefulSets(ns).Patch(ctx, name, types.MergePatchType, raw, metav1.PatchOptions{})
		return err
	case "daemonset":
		_, err := k.client.AppsV1().DaemonSets(ns).Patch(ctx, name, types.MergePatchType, raw, metav1.PatchOptions{})
		return err
	default:
		return fmt.Errorf("unsupported workload kind %q", kind)
	}
}
func (k *KubernetesProvider) DeleteWorkload(ctx context.Context, ns, kind, name string) error {
	prop := metav1.DeletePropagationForeground
	var err error
	switch strings.ToLower(kind) {
	case "deployment":
		err = k.client.AppsV1().Deployments(ns).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &prop})
	case "statefulset":
		err = k.client.AppsV1().StatefulSets(ns).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &prop})
	case "daemonset":
		err = k.client.AppsV1().DaemonSets(ns).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &prop})
	default:
		return fmt.Errorf("unsupported workload kind %q", kind)
	}
	if err != nil {
		return err
	}
	// Clean up the Service created by Axiom Launch, but only when the
	// managed-by label proves Axiom owns it.
	if svc, getErr := k.client.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{}); getErr == nil {
		if svc.Labels["app.kubernetes.io/managed-by"] == "zyvor-axiom" {
			_ = k.client.CoreV1().Services(ns).Delete(ctx, name, metav1.DeleteOptions{})
		}
	}
	return nil
}

func (k *KubernetesProvider) ListPods(ctx context.Context, ns string) ([]Pod, error) {
	selectorNS := ns
	if ns == "" || ns == "all" {
		selectorNS = metav1.NamespaceAll
	}
	items, err := k.client.CoreV1().Pods(selectorNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	desiredByWL := map[string]int32{}
	sealed := map[string]bool{}
	intentByWL := map[string]string{}
	if deps, err := k.client.AppsV1().Deployments(selectorNS).List(ctx, metav1.ListOptions{}); err == nil {
		for _, d := range deps.Items {
			key := d.Namespace + "/" + d.Name
			if d.Spec.Replicas != nil {
				desiredByWL[key] = *d.Spec.Replicas
			}
			if IsSealedAnnotation(d.Annotations) {
				sealed[key] = true
			}
			if d.Annotations != nil {
				if v := d.Annotations[IntentAnnotation]; v != "" {
					intentByWL[key] = v
				}
			}
		}
	}
	if sts, err := k.client.AppsV1().StatefulSets(selectorNS).List(ctx, metav1.ListOptions{}); err == nil {
		for _, s := range sts.Items {
			key := s.Namespace + "/" + s.Name
			if s.Spec.Replicas != nil {
				desiredByWL[key] = *s.Spec.Replicas
			}
			if IsSealedAnnotation(s.Annotations) {
				sealed[key] = true
			}
			if s.Annotations != nil {
				if v := s.Annotations[IntentAnnotation]; v != "" {
					intentByWL[key] = v
				}
			}
		}
	}
	out := make([]Pod, 0, len(items.Items))
	for _, p := range items.Items {
		ready := 0
		restarts := int32(0)
		for _, cs := range p.Status.ContainerStatuses {
			if cs.Ready {
				ready++
			}
			restarts += cs.RestartCount
		}
		for _, cs := range p.Status.InitContainerStatuses {
			restarts += cs.RestartCount
		}
		phase := string(p.Status.Phase)
		for _, cs := range p.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				phase = "CrashLoopBackOff"
				break
			}
		}
		if phase == string(corev1.PodPending) {
			for _, cs := range p.Status.InitContainerStatuses {
				if cs.State.Waiting != nil || (cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0) {
					phase = "CrashLoop"
					break
				}
			}
		}
		wl := workloadNameFromPod(p)
		wlKey := p.Namespace + "/" + wl
		isSealed := sealed[wlKey]
		if isSealed {
			phase = "Sealed"
		}
		cpuLim, memLim := containerLimits(p)
		image := ""
		if len(p.Spec.Containers) > 0 {
			image = p.Spec.Containers[0].Image
		}
		replicas := desiredByWL[wlKey]
		replica := int32(1)
		if idx := strings.LastIndex(p.Name, "-"); idx > 0 {
			if n, err := strconv.ParseInt(p.Name[idx+1:], 10, 32); err == nil {
				replica = int32(n)
			}
		}
		cinfo := podContainerInfo(p)
		containers := make([]string, 0, len(p.Spec.Containers))
		for _, c := range p.Spec.Containers {
			containers = append(containers, c.Name)
		}
		ports := make([]PodPort, 0)
		for _, c := range p.Spec.Containers {
			for _, cp := range c.Ports {
				ports = append(ports, PodPort{Name: cp.Name, Container: c.Name, ContainerPort: cp.ContainerPort, Protocol: string(cp.Protocol)})
			}
		}
		out = append(out, Pod{
			Namespace: p.Namespace, Name: p.Name, Node: p.Spec.NodeName, Status: phase,
			Ready: fmt.Sprintf("%d/%d", ready, len(p.Spec.Containers)), Restarts: restarts,
			IP: p.Status.PodIP, Age: age(p.CreationTimestamp.Time), Containers: containers, ContainerInfo: cinfo,
			Ports: ports, Labels: p.Labels, WorkloadID: wl, Replica: replica, Replicas: replicas,
			Floor: string(FloorForNamespace(p.Namespace)), CPULimit: cpuLim, MemLimit: memLim,
			QOS: podQOS(p), Image: image, Intent: intentByWL[wlKey], Sealed: isSealed,
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
func (k *KubernetesProvider) DeletePod(ctx context.Context, ns, name string) error {
	return k.client.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{})
}
func (k *KubernetesProvider) EvictPod(ctx context.Context, ns, name string) error {
	return k.client.PolicyV1().Evictions(ns).Evict(ctx, &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, DeleteOptions: &metav1.DeleteOptions{}})
}

// DiagnosePod aggregates the pod's PodScheduled condition, matching
// FailedScheduling/warning events, and container waiting reasons into a
// single plain-English diagnosis — the "why is this pod not running"
// answer real operators reach for, without a new k8s API dependency
// beyond what ListPods/ListEvents already use.
func (k *KubernetesProvider) DiagnosePod(ctx context.Context, ns, name string) (PodDiagnosis, error) {
	pod, err := k.client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return PodDiagnosis{}, err
	}
	diag := PodDiagnosis{Namespace: ns, Name: name, Phase: string(pod.Status.Phase)}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodScheduled {
			diag.Scheduled = c.Status == corev1.ConditionTrue
			diag.Reason = c.Reason
			diag.Message = c.Message
			break
		}
	}
	events, err := k.client.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: fields.AndSelectors(
			fields.OneTermEqualSelector("involvedObject.name", name),
			fields.OneTermEqualSelector("involvedObject.kind", "Pod"),
		).String(),
	})
	var evStrs []string
	if err == nil {
		for _, e := range events.Items {
			if e.Type == corev1.EventTypeWarning {
				evStrs = append(evStrs, fmt.Sprintf("%s: %s", e.Reason, e.Message))
			}
		}
	}
	diag.Events = evStrs

	switch {
	case !diag.Scheduled:
		switch {
		case diag.Message != "":
			diag.Diagnosis = diag.Message
		case len(evStrs) > 0:
			diag.Diagnosis = evStrs[0]
		default:
			diag.Diagnosis = "Pod has not been scheduled to a node yet."
		}
	case pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded:
		diag.Diagnosis = "Pod is scheduled and running normally."
	default:
		var waiting []string
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				waiting = append(waiting, fmt.Sprintf("%s: %s (%s)", cs.Name, cs.State.Waiting.Reason, cs.State.Waiting.Message))
			}
		}
		switch {
		case len(waiting) > 0:
			diag.Diagnosis = strings.Join(waiting, "; ")
		case len(evStrs) > 0:
			diag.Diagnosis = evStrs[0]
		default:
			diag.Diagnosis = fmt.Sprintf("Pod is scheduled to %s and starting.", pod.Spec.NodeName)
		}
	}
	return diag, nil
}

func (k *KubernetesProvider) ListNodes(ctx context.Context) ([]Node, error) {
	nodes, err := k.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	pods, err := k.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, p := range pods.Items {
		if p.Spec.NodeName != "" {
			counts[p.Spec.NodeName]++
		}
	}
	out := make([]Node, 0, len(nodes.Items))
	for _, n := range nodes.Items {
		status := "NotReady"
		conditions := make([]NodeCondition, 0, len(n.Status.Conditions))
		for _, c := range n.Status.Conditions {
			conditions = append(conditions, NodeCondition{
				Type:    string(c.Type),
				Status:  string(c.Status),
				Reason:  c.Reason,
				Message: c.Message,
			})
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				status = "Ready"
			}
		}
		if n.Spec.Unschedulable {
			status = "SchedulingDisabled"
		}
		role := "agent"
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
			role = "server"
		}
		if _, ok := n.Labels["node-role.kubernetes.io/master"]; ok {
			role = "server"
		}
		ip := ""
		for _, a := range n.Status.Addresses {
			if a.Type == corev1.NodeInternalIP {
				ip = a.Address
			}
		}
		taints := make([]TaintSummary, 0, len(n.Spec.Taints))
		for _, t := range n.Spec.Taints {
			taints = append(taints, TaintSummary{Key: t.Key, Value: t.Value, Effect: string(t.Effect)})
		}
		out = append(out, Node{
			Name:              n.Name,
			Status:            status,
			Role:              role,
			Version:           n.Status.NodeInfo.KubeletVersion,
			InternalIP:        ip,
			Pods:              counts[n.Name],
			CPU:               n.Status.Capacity.Cpu().String(),
			Memory:            n.Status.Capacity.Memory().String(),
			AllocatableCPU:    n.Status.Allocatable.Cpu().String(),
			AllocatableMemory: n.Status.Allocatable.Memory().String(),
			AllocatablePods:   n.Status.Allocatable.Pods().String(),
			OSImage:           n.Status.NodeInfo.OSImage,
			KernelVersion:     n.Status.NodeInfo.KernelVersion,
			ContainerRuntime:  n.Status.NodeInfo.ContainerRuntimeVersion,
			Conditions:        conditions,
			Taints:            taints,
			Unschedulable:     n.Spec.Unschedulable,
			Age:               age(n.CreationTimestamp.Time),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (k *KubernetesProvider) Cordon(ctx context.Context, name string, cordon bool) error {
	node, err := k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	node.Spec.Unschedulable = cordon
	_, err = k.client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	return err
}

func (k *KubernetesProvider) AddTaint(ctx context.Context, name, key, value, effect string) error {
	node, err := k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	taint := corev1.Taint{Key: key, Value: value, Effect: corev1.TaintEffect(effect)}
	for _, t := range node.Spec.Taints {
		if t.Key == taint.Key && t.Effect == taint.Effect {
			return nil
		}
	}
	node.Spec.Taints = append(node.Spec.Taints, taint)
	_, err = k.client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	return err
}

func (k *KubernetesProvider) RemoveTaint(ctx context.Context, name, key, effect string) error {
	node, err := k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	kept := node.Spec.Taints[:0]
	for _, t := range node.Spec.Taints {
		if t.Key == key && string(t.Effect) == effect {
			continue
		}
		kept = append(kept, t)
	}
	node.Spec.Taints = kept
	_, err = k.client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	return err
}
func (k *KubernetesProvider) Drain(ctx context.Context, name string) error {
	if err := k.Cordon(ctx, name, true); err != nil {
		return fmt.Errorf("cordon node: %w", err)
	}
	pods, err := k.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("spec.nodeName", name).String()})
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range pods.Items {
		if isDaemonSetPod(&p) || isMirrorPod(&p) || p.Namespace == "kube-system" {
			continue
		}
		if err := k.EvictPod(ctx, p.Namespace, p.Name); err != nil {
			errs = append(errs, fmt.Errorf("%s/%s: %w", p.Namespace, p.Name, err)) // %w keeps the apierror, so HTTPStatus still sees 429/404
		}
	}
	return errors.Join(errs...)
}
func isDaemonSetPod(p *corev1.Pod) bool {
	for _, o := range p.OwnerReferences {
		if o.Kind == "DaemonSet" && o.Controller != nil && *o.Controller {
			return true
		}
	}
	return false
}

func isMirrorPod(p *corev1.Pod) bool {
	_, ok := p.Annotations[corev1.MirrorPodAnnotationKey]
	return ok
}

func (k *KubernetesProvider) ListEvents(ctx context.Context, ns string) ([]Event, error) {
	return k.listEvents(ctx, ns, "")
}

// listEvents lists events, newest first, capped at 200. A non-empty
// fieldSelector (e.g. "type=Warning") is applied by the API server BEFORE the
// cap — filtering client-side after the cap let a burst of Normal events push
// real Warnings out of the Alerts page and Slack dispatch.
func (k *KubernetesProvider) listEvents(ctx context.Context, ns, fieldSelector string) ([]Event, error) {
	selectorNS := ns
	if ns == "" || ns == "all" {
		selectorNS = metav1.NamespaceAll
	}
	events, err := k.client.CoreV1().Events(selectorNS).List(ctx, metav1.ListOptions{FieldSelector: fieldSelector})
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(events.Items))
	for _, e := range events.Items {
		t := e.LastTimestamp.Time
		if t.IsZero() {
			t = e.EventTime.Time
		}
		if t.IsZero() {
			t = e.CreationTimestamp.Time
		}
		out = append(out, Event{Namespace: e.Namespace, Type: e.Type, Reason: e.Reason, Object: e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name, Message: e.Message, Count: e.Count, Time: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if len(out) > 200 {
		out = out[:200]
	}
	return out, nil
}

func (k *KubernetesProvider) PodLogs(ctx context.Context, ns, pod, container string, follow bool, tail int64, previous bool) (io.ReadCloser, error) {
	opts := &corev1.PodLogOptions{Container: container, Follow: follow, Timestamps: true, Previous: previous}
	if tail > 0 {
		opts.TailLines = &tail
	}
	cs := k.client
	if follow {
		cs = k.streamingClient() // no body-read timeout: a followed log must not be cut at the API timeout
	}
	req := cs.CoreV1().Pods(ns).GetLogs(pod, opts)
	return req.Stream(ctx)
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		return err == nil && u.Host == r.Host
	},
}

type lockedWSWriter struct {
	mu   *sync.Mutex
	conn *websocket.Conn
}

func (w lockedWSWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

type terminalSizeQueue struct {
	ch   chan remotecommand.TerminalSize
	done <-chan struct{}
}

// Next blocks for the next resize. The channel is never closed (the reader
// goroutine sends on it, and closing a channel with a live sender panics and
// takes the whole process down); the session ending is signalled via done.
func (q *terminalSizeQueue) Next() *remotecommand.TerminalSize {
	select {
	case s := <-q.ch:
		return &s
	case <-q.done:
		return nil
	}
}

func (k *KubernetesProvider) ExecCommand(ctx context.Context, ns, pod, container string, command []string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("command is required")
	}
	// Bound hang risk when the runtime never starts the process (common with
	// stuck CRI / network-policy edge cases). Callers can still pass a shorter
	// deadline via ctx.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}
	if container == "" {
		p, err := k.client.CoreV1().Pods(ns).Get(ctx, pod, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if len(p.Spec.Containers) == 0 {
			return "", errors.New("pod has no containers")
		}
		container = p.Spec.Containers[0].Name
	}
	req := k.client.CoreV1().RESTClient().Post().Resource("pods").Name(pod).Namespace(ns).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: container, Command: command, Stdout: true, Stderr: true, TTY: false}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(k.config, http.MethodPost, req.URL())
	if err != nil {
		return "", err
	}
	stdout, stderr := &limitedBuffer{max: maxExecOutputBytes}, &limitedBuffer{max: maxExecOutputBytes}
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: stdout, Stderr: stderr, Tty: false}); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stderr.Len() > 0 {
		return stdout.String() + stderr.String(), nil
	}
	return stdout.String(), nil
}

func (k *KubernetesProvider) ExecWS(ctx context.Context, w http.ResponseWriter, r *http.Request, ns, pod, container, shell string) error {
	release, ok := acquireExecWSSession()
	if !ok {
		http.Error(w, "too many open terminal sessions; close one and retry", http.StatusTooManyRequests)
		return errors.New("terminal session limit reached")
	}
	defer release()
	if shell == "" {
		shell = "/bin/sh"
	}
	if container == "" {
		p, err := k.client.CoreV1().Pods(ns).Get(ctx, pod, metav1.GetOptions{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return err
		}
		if len(p.Spec.Containers) == 0 {
			err := errors.New("pod has no containers")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return err
		}
		container = p.Spec.Containers[0].Name
	}
	req := k.client.CoreV1().RESTClient().Post().Resource("pods").Name(pod).Namespace(ns).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: container, Command: []string{shell}, Stdin: true, Stdout: true, Stderr: true, TTY: true}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(k.config, http.MethodPost, req.URL())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return err
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadLimit(maxExecWSInput) // a client must not be able to push arbitrarily large frames
	IncPresence(ns, pod)
	defer DecPresence(ns, pod)
	stdinR, stdinW := io.Pipe()
	defer stdinR.Close()
	defer stdinW.Close()
	var writeMu sync.Mutex
	out := lockedWSWriter{mu: &writeMu, conn: conn}
	// Detach from the HTTP request context: some middleware cancel it when
	// headers flush / the handler's surrounding stack thinks the request is
	// done, which would kill an otherwise healthy PTY mid-session.
	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	resize := &terminalSizeQueue{ch: make(chan remotecommand.TerminalSize, 4), done: streamCtx.Done()}
	readErr := make(chan error, 1)
	go func() {
		for {
			typ, msg, err := conn.ReadMessage()
			if err != nil {
				readErr <- err
				streamCancel()
				return
			}
			if typ != websocket.TextMessage && typ != websocket.BinaryMessage {
				continue
			}
			if bytes.HasPrefix(msg, []byte("__resize__:")) {
				parts := strings.Split(strings.TrimPrefix(string(msg), "__resize__:"), "x")
				if len(parts) == 2 {
					w64, _ := strconv.ParseUint(parts[0], 10, 16)
					h64, _ := strconv.ParseUint(parts[1], 10, 16)
					select {
					case resize.ch <- remotecommand.TerminalSize{Width: uint16(w64), Height: uint16(h64)}:
					default:
					}
				}
				continue
			}
			if _, err := stdinW.Write(msg); err != nil {
				readErr <- err
				streamCancel()
				return
			}
		}
	}()
	execErr := make(chan error, 1)
	go func() {
		execErr <- executor.StreamWithContext(streamCtx, remotecommand.StreamOptions{Stdin: stdinR, Stdout: out, Stderr: out, Tty: true, TerminalSizeQueue: resize})
	}()
	select {
	case err := <-execErr:
		if err != nil {
			msg := fmt.Sprintf("\r\n\x1b[31mexec failed:\x1b[0m %s\r\n", err.Error())
			if strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "executable file not found") {
				msg += "\x1b[33mThis container has no shell (distroless/scratch). Try a different pod or container, or use one-shot exec with a binary that exists.\x1b[0m\r\n"
			}
			writeMu.Lock()
			_ = conn.WriteMessage(websocket.TextMessage, []byte(msg))
			writeMu.Unlock()
		}
		return err
	case <-readErr:
		return nil
	}
}

func age(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func nsSelector(ns string) string {
	if ns == "" || ns == "all" {
		return metav1.NamespaceAll
	}
	return ns
}

func labelsMatch(podLabels, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if podLabels[k] != v {
			return false
		}
	}
	return true
}

func (k *KubernetesProvider) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	items, err := k.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	pods, _ := k.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	services, _ := k.client.CoreV1().Services(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	podCounts := map[string]int{}
	svcCounts := map[string]int{}
	for _, p := range pods.Items {
		podCounts[p.Namespace]++
	}
	for _, s := range services.Items {
		svcCounts[s.Namespace]++
	}
	out := make([]Namespace, 0, len(items.Items))
	for _, n := range items.Items {
		status := "Active"
		if n.Status.Phase == corev1.NamespaceTerminating {
			status = "Terminating"
		}
		out = append(out, Namespace{Name: n.Name, Status: status, Pods: podCounts[n.Name], Services: svcCounts[n.Name], Age: age(n.CreationTimestamp.Time)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (k *KubernetesProvider) CreateNamespace(ctx context.Context, name string) (Namespace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Namespace{}, errors.New("namespace name is required")
	}
	n, err := k.client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}, metav1.CreateOptions{})
	if err != nil {
		return Namespace{}, err
	}
	return Namespace{Name: n.Name, Status: "Active", Age: age(n.CreationTimestamp.Time)}, nil
}

func (k *KubernetesProvider) DeleteNamespace(ctx context.Context, name string) error {
	if protectedNamespace(name) {
		return NewStatusError(http.StatusForbidden, fmt.Errorf("namespace %q is protected and cannot be deleted from Axiom", name))
	}
	return k.client.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *KubernetesProvider) ListServices(ctx context.Context, ns string) ([]Service, error) {
	items, err := k.client.CoreV1().Services(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	pods, err := k.client.CoreV1().Pods(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Service, 0, len(items.Items))
	for _, s := range items.Items {
		ports := make([]ServicePort, 0, len(s.Spec.Ports))
		for _, p := range s.Spec.Ports {
			ports = append(ports, ServicePort{Name: p.Name, Port: p.Port, TargetPort: p.TargetPort.String(), Protocol: string(p.Protocol), NodePort: p.NodePort})
		}
		endpoints := make([]string, 0)
		for _, p := range pods.Items {
			if p.Namespace == s.Namespace && labelsMatch(p.Labels, s.Spec.Selector) {
				endpoints = append(endpoints, p.Name)
			}
		}
		sort.Strings(endpoints)
		out = append(out, Service{Namespace: s.Namespace, Name: s.Name, Type: string(s.Spec.Type), ClusterIP: s.Spec.ClusterIP, Ports: ports, Selector: s.Spec.Selector, Endpoints: endpoints, Age: age(s.CreationTimestamp.Time)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func (k *KubernetesProvider) NetworkMap(ctx context.Context, ns string) (NetworkMap, error) {
	services, err := k.ListServices(ctx, ns)
	if err != nil {
		return NetworkMap{}, err
	}
	pods, err := k.ListPods(ctx, ns)
	if err != nil {
		return NetworkMap{}, err
	}
	nodes := make([]NetworkNode, 0)
	edges := make([]NetworkEdge, 0)
	seen := map[string]struct{}{}
	addNode := func(n NetworkNode) {
		if _, ok := seen[n.ID]; ok {
			return
		}
		seen[n.ID] = struct{}{}
		nodes = append(nodes, n)
	}
	for _, s := range services {
		sid := fmt.Sprintf("service:%s/%s", s.Namespace, s.Name)
		portLabels := make([]string, 0, len(s.Ports))
		for _, p := range s.Ports {
			portLabels = append(portLabels, fmt.Sprintf("%d→%s", p.Port, p.TargetPort))
		}
		addNode(NetworkNode{ID: sid, Kind: "service", Namespace: s.Namespace, Name: s.Name, Label: s.Name, IP: s.ClusterIP, Ports: portLabels})
		for _, ep := range s.Endpoints {
			pid := fmt.Sprintf("pod:%s/%s", s.Namespace, ep)
			var pod Pod
			for _, p := range pods {
				if p.Namespace == s.Namespace && p.Name == ep {
					pod = p
					break
				}
			}
			podPorts := make([]string, 0, len(pod.Ports))
			for _, p := range pod.Ports {
				podPorts = append(podPorts, fmt.Sprintf("%d/%s", p.ContainerPort, p.Protocol))
			}
			addNode(NetworkNode{ID: pid, Kind: "pod", Namespace: s.Namespace, Name: ep, Label: ep, IP: pod.IP, Ports: podPorts})
			for _, p := range s.Ports {
				edges = append(edges, NetworkEdge{From: sid, To: pid, Port: p.Port, Protocol: p.Protocol, Label: fmt.Sprintf("%s:%d", p.Protocol, p.Port)})
			}
		}
	}
	// Pods without a service (orphan exposure via container ports)
	for _, p := range pods {
		pid := fmt.Sprintf("pod:%s/%s", p.Namespace, p.Name)
		if _, ok := seen[pid]; ok {
			continue
		}
		if len(p.Ports) == 0 {
			continue
		}
		podPorts := make([]string, 0, len(p.Ports))
		for _, port := range p.Ports {
			podPorts = append(podPorts, fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol))
		}
		addNode(NetworkNode{ID: pid, Kind: "pod", Namespace: p.Namespace, Name: p.Name, Label: p.Name, IP: p.IP, Ports: podPorts})
	}
	return NetworkMap{Namespace: ns, Nodes: nodes, Edges: edges}, nil
}
