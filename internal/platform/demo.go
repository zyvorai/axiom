package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type DemoProvider struct {
	mu           sync.RWMutex
	workloads    map[string]Workload
	pods         map[string][]Pod
	lastReplicas map[string]int32
	sealed       map[string]bool
}

func NewDemoProvider() *DemoProvider {
	d := &DemoProvider{workloads: map[string]Workload{}, pods: map[string][]Pod{}, lastReplicas: map[string]int32{}, sealed: map[string]bool{}}
	seed := []Workload{
		{Namespace: "payments", Name: "payments-api", Kind: "Deployment", Image: "ghcr.io/zyvorai/payments:v2.8.4", Desired: 12, Ready: 12, Available: 12, Status: "Healthy", Age: "3d"},
		{Namespace: "commerce", Name: "checkout", Kind: "Deployment", Image: "ghcr.io/zyvorai/checkout:v1.14.2", Desired: 6, Ready: 6, Available: 6, Status: "Healthy", Age: "11d"},
		{Namespace: "data", Name: "redis", Kind: "StatefulSet", Image: "redis:7.4", Desired: 3, Ready: 3, Available: 3, Status: "Healthy", Age: "28d"},
		{Namespace: "observability", Name: "otel-collector", Kind: "Deployment", Image: "otel/opentelemetry-collector:0.120.0", Desired: 4, Ready: 3, Available: 3, Status: "Degraded", Age: "19d"},
	}
	for _, w := range seed {
		d.workloads[key(w.Namespace, w.Kind, w.Name)] = w
		d.rebuildPodsLocked(w)
	}
	return d
}

func key(ns, kind, name string) string { return ns + "/" + strings.ToLower(kind) + "/" + name }

func (d *DemoProvider) Overview(ctx context.Context) (Overview, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	pods := 0
	running := 0
	warnings := 0
	for _, ps := range d.pods {
		for _, p := range ps {
			pods++
			if p.Status == "Running" {
				running++
			}
		}
	}
	for _, w := range d.workloads {
		if w.Status != "Healthy" && w.Status != "Stopped" {
			warnings++
		}
	}
	vms, _ := d.ListVirtualMachines(ctx, "all")
	readyVMs := 0
	for _, vm := range vms {
		if vm.Ready {
			readyVMs++
		}
	}
	vols, _ := d.ListVolumes(ctx, "all")
	var used int64
	for _, v := range vols {
		used += parseMemoryBytes(v.Capacity)
	}
	capacity := int64(4) * 1024 * 1024 * 1024 * 1024
	util, _ := d.Utilization(ctx)
	regions := DefaultRegions("axiom-demo")
	return Overview{
		Cluster: "zyvor-demo", Health: "Healthy",
		Workloads: len(d.workloads), Pods: pods, RunningPods: running,
		Nodes: 4, ReadyNodes: 4, VMs: len(vms), ReadyVMs: readyVMs,
		CPUPercent: util.CPUPercent, MemoryPercent: util.MemoryPercent,
		StorageUsedBytes: used, StorageCapacityBytes: capacity,
		StorageUsedTB: bytesToTB(used), StorageCapacityTB: bytesToTB(capacity),
		Region: ActiveRegion(regions), Regions: regions,
		Utilization: util.Points, Warnings: warnings,
	}, nil
}

func (d *DemoProvider) ListWorkloads(ctx context.Context, ns string) ([]Workload, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Workload, 0, len(d.workloads))
	for _, w := range d.workloads {
		if ns == "" || ns == "all" || w.Namespace == ns {
			out = append(out, w)
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

func (d *DemoProvider) Launch(ctx context.Context, req LaunchRequest) (Workload, error) {
	if req.Name == "" || req.Image == "" {
		return Workload{}, errors.New("name and image are required")
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Replicas < 1 {
		req.Replicas = 1
	}
	w := Workload{Namespace: req.Namespace, Name: req.Name, Kind: "Deployment", Image: req.Image, Desired: req.Replicas, Ready: req.Replicas, Available: req.Replicas, Status: "Healthy", Age: "now", Labels: map[string]string{"app.kubernetes.io/name": req.Name, "app.kubernetes.io/managed-by": "zyvor-axiom"}}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.workloads[key(w.Namespace, w.Kind, w.Name)] = w
	d.rebuildPodsLocked(w)
	return w, nil
}

func (d *DemoProvider) Scale(ctx context.Context, ns, kind, name string, replicas int32) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	w, ok := d.workloads[key(ns, kind, name)]
	if !ok {
		return errors.New("workload not found")
	}
	if replicas < 0 {
		return errors.New("replicas must be >= 0")
	}
	w.Desired = replicas
	w.Ready = replicas
	w.Available = replicas
	if replicas == 0 {
		w.Status = "Stopped"
	} else {
		w.Status = "Healthy"
	}
	d.workloads[key(ns, kind, name)] = w
	d.rebuildPodsLocked(w)
	return nil
}
func (d *DemoProvider) Start(ctx context.Context, ns, kind, name string) error {
	k := key(ns, kind, name)
	d.mu.RLock()
	w, ok := d.workloads[k]
	last := d.lastReplicas[k]
	d.mu.RUnlock()
	if !ok {
		return errors.New("workload not found")
	}
	n := w.Desired
	if n == 0 {
		n = last
		if n <= 0 {
			n = 3
		}
	}
	return d.Scale(ctx, ns, kind, name, n)
}
func (d *DemoProvider) Stop(ctx context.Context, ns, kind, name string) error {
	k := key(ns, kind, name)
	d.mu.Lock()
	w, ok := d.workloads[k]
	if !ok {
		d.mu.Unlock()
		return errors.New("workload not found")
	}
	if w.Desired > 0 {
		d.lastReplicas[k] = w.Desired
	}
	d.mu.Unlock()
	return d.Scale(ctx, ns, kind, name, 0)
}
func (d *DemoProvider) Restart(ctx context.Context, ns, kind, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	w, ok := d.workloads[key(ns, kind, name)]
	if !ok {
		return errors.New("workload not found")
	}
	d.rebuildPodsLocked(w)
	return nil
}
func (d *DemoProvider) DeleteWorkload(ctx context.Context, ns, kind, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.workloads, key(ns, kind, name))
	delete(d.pods, ns+"/"+name)
	delete(d.sealed, key(ns, kind, name))
	return nil
}

func (d *DemoProvider) SealWorkload(ctx context.Context, ns, kind, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sealed[key(ns, kind, name)] = true
	if w, ok := d.workloads[key(ns, kind, name)]; ok {
		d.rebuildPodsLocked(w)
	}
	return nil
}

func (d *DemoProvider) UnsealWorkload(ctx context.Context, ns, kind, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.sealed, key(ns, kind, name))
	if w, ok := d.workloads[key(ns, kind, name)]; ok {
		d.rebuildPodsLocked(w)
	}
	return nil
}

func (d *DemoProvider) WorkloadVolumes(ctx context.Context, ns, workload string) ([]string, error) {
	return []string{}, nil
}

func (d *DemoProvider) PodProxyTarget(ctx context.Context, ns, podName string, port int32) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, ps := range d.pods {
		for _, p := range ps {
			if p.Namespace == ns && p.Name == podName {
				pnum := port
				if len(p.Ports) > 0 {
					pnum = p.Ports[0].ContainerPort
				}
				return fmt.Sprintf("http://%s:%d", p.IP, pnum), nil
			}
		}
	}
	return "", fmt.Errorf("pod not found")
}

func (d *DemoProvider) ListPods(ctx context.Context, ns string) ([]Pod, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var out []Pod
	for k, ps := range d.pods {
		if ns == "" || ns == "all" || strings.HasPrefix(k, ns+"/") {
			out = append(out, ps...)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (d *DemoProvider) DeletePod(ctx context.Context, ns, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, ps := range d.pods {
		if !strings.HasPrefix(k, ns+"/") {
			continue
		}
		for i, p := range ps {
			if p.Name == name {
				ps = append(ps[:i], ps[i+1:]...)
				d.pods[k] = ps
				return nil
			}
		}
	}
	return errors.New("pod not found")
}
func (d *DemoProvider) EvictPod(ctx context.Context, ns, name string) error {
	return d.DeletePod(ctx, ns, name)
}
func (d *DemoProvider) ListNodes(ctx context.Context) ([]Node, error) {
	return []Node{
		{Name: "axiom-01", Status: "Ready", Role: "server", Version: "v1.32.4+k3s1", InternalIP: "10.20.0.11", Pods: 18, CPU: "16", Memory: "64Gi", AllocatableCPU: "16", AllocatableMemory: "64Gi", AllocatablePods: "110", OSImage: "Ubuntu 22.04", KernelVersion: "5.15.0", ContainerRuntime: "containerd://1.7", Age: "41d", Conditions: []NodeCondition{{Type: "Ready", Status: "True"}}},
		{Name: "axiom-02", Status: "Ready", Role: "server", Version: "v1.32.4+k3s1", InternalIP: "10.20.0.12", Pods: 21, CPU: "16", Memory: "64Gi", AllocatableCPU: "16", AllocatableMemory: "64Gi", AllocatablePods: "110", OSImage: "Ubuntu 22.04", KernelVersion: "5.15.0", ContainerRuntime: "containerd://1.7", Age: "41d", Conditions: []NodeCondition{{Type: "Ready", Status: "True"}}},
		{Name: "axiom-03", Status: "Ready", Role: "server", Version: "v1.32.4+k3s1", InternalIP: "10.20.0.13", Pods: 17, CPU: "16", Memory: "64Gi", AllocatableCPU: "16", AllocatableMemory: "64Gi", AllocatablePods: "110", OSImage: "Ubuntu 22.04", KernelVersion: "5.15.0", ContainerRuntime: "containerd://1.7", Age: "41d", Conditions: []NodeCondition{{Type: "Ready", Status: "True"}}},
		{Name: "gpu-01", Status: "Ready", Role: "agent", Version: "v1.32.4+k3s1", InternalIP: "10.20.0.21", Pods: 11, CPU: "32", Memory: "256Gi", AllocatableCPU: "32", AllocatableMemory: "256Gi", AllocatablePods: "110", OSImage: "Ubuntu 22.04", KernelVersion: "5.15.0", ContainerRuntime: "containerd://1.7", Age: "17d", Conditions: []NodeCondition{{Type: "Ready", Status: "True"}}},
	}, nil
}
func (d *DemoProvider) Cordon(ctx context.Context, name string, cordon bool) error { return nil }
func (d *DemoProvider) Drain(ctx context.Context, name string) error               { return nil }
func (d *DemoProvider) ListEvents(ctx context.Context, ns string) ([]Event, error) {
	now := time.Now()
	return []Event{{Namespace: "payments", Type: "Normal", Reason: "ScalingReplicaSet", Object: "Deployment/payments-api", Message: "Scaled up replica set payments-api to 12", Count: 1, Time: now.Add(-8 * time.Minute)}, {Namespace: "observability", Type: "Warning", Reason: "BackOff", Object: "Pod/otel-collector-69d8", Message: "Back-off restarting failed container collector", Count: 4, Time: now.Add(-3 * time.Minute)}}, nil
}
func (d *DemoProvider) PodLogs(ctx context.Context, ns, pod, container string, follow bool, tail int64, previous bool) (io.ReadCloser, error) {
	lines := []string{"2026-08-27T02:03:14Z INFO request accepted trace=aa71", "2026-08-27T02:03:14Z INFO cache hit key=customer:1842", "2026-08-27T02:03:15Z WARN upstream latency service=postgres duration=74ms", "2026-08-27T02:03:16Z INFO request complete status=200 duration=18ms"}
	return io.NopCloser(strings.NewReader(strings.Join(lines, "\n") + "\n")), nil
}
func (d *DemoProvider) ExecCommand(ctx context.Context, ns, pod, container string, command []string) (string, error) {
	cmd := strings.TrimSpace(strings.Join(command, " "))
	if len(command) >= 3 && (command[0] == "/bin/sh" || command[0] == "/bin/bash") && command[1] == "-lc" {
		cmd = strings.TrimSpace(strings.Join(command[2:], " "))
	}
	switch cmd {
	case "", "sh", "/bin/sh", "bash", "/bin/bash":
		return "axiom@" + pod + ":/app$", nil
	case "whoami":
		return "axiom", nil
	case "pwd":
		return "/app", nil
	case "hostname":
		return pod, nil
	case "ls", "ls -la":
		return "drwxr-xr-x  1 axiom axiom 4096 .\n-rwxr-xr-x  1 axiom axiom 8421376 server\n-rw-r--r--  1 axiom axiom    1248 config.yaml", nil
	case "env":
		return "AXIOM_MODE=demo\nAPP_ENV=production\nPORT=8080\nHOSTNAME=" + pod, nil
	case "curl localhost:8080/health", "curl http://localhost:8080/health":
		return "{\"status\":\"ok\",\"latency_ms\":18}", nil
	default:
		return fmt.Sprintf("demo: executed %q in %s/%s", cmd, ns, pod), nil
	}
}

func (d *DemoProvider) rebuildPodsLocked(w Workload) {
	ps := make([]Pod, 0, w.Desired)
	sealed := d.sealed[key(w.Namespace, w.Kind, w.Name)]
	for i := int32(0); i < w.Desired; i++ {
		name := fmt.Sprintf("%s-%02d", w.Name, i+1)
		status := "Running"
		ready := "1/1"
		if sealed {
			status = "Sealed"
		}
		if w.Status == "Degraded" && i == w.Desired-1 && !sealed {
			status = "CrashLoopBackOff"
			ready = "0/1"
		}
		if w.Desired == 0 {
			continue
		}
		cinfo := []PodContainerStatus{{Name: w.Name, Ready: status == "Running", State: status}}
		if status == "CrashLoopBackOff" {
			cinfo = []PodContainerStatus{
				{Name: "init-config", Init: true, Ready: false, State: "CrashLoopBackOff"},
				{Name: w.Name, Ready: false, State: "Waiting"},
			}
		}
		ps = append(ps, Pod{
			Namespace: w.Namespace, Name: name, Node: fmt.Sprintf("axiom-%02d", (i%3)+1),
			Status: status, Ready: ready,
			Restarts: func() int32 {
				if status == "CrashLoopBackOff" {
					return 4
				}
				return 0
			}(),
			IP: fmt.Sprintf("10.42.%d.%d", (i%4)+1, 20+i), Age: w.Age,
			Containers: []string{w.Name}, ContainerInfo: cinfo,
			Ports: []PodPort{{Container: w.Name, ContainerPort: 8080, Protocol: "TCP"}},
			Labels: map[string]string{"app": w.Name, "app.kubernetes.io/name": w.Name},
			WorkloadID: w.Name, Replica: i + 1, Replicas: w.Desired, Floor: string(FloorForNamespace(w.Namespace)),
			CPULimit: "500m", MemLimit: "512Mi", QOS: "Burstable", Image: w.Image, Sealed: sealed,
		})
	}
	d.pods[w.Namespace+"/"+w.Name] = ps
}

func (d *DemoProvider) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	nsMap := map[string]*Namespace{}
	for _, w := range d.workloads {
		if nsMap[w.Namespace] == nil {
			nsMap[w.Namespace] = &Namespace{Name: w.Namespace, Status: "Active", Age: "30d"}
		}
	}
	for _, ps := range d.pods {
		for _, p := range ps {
			if nsMap[p.Namespace] == nil {
				nsMap[p.Namespace] = &Namespace{Name: p.Namespace, Status: "Active", Age: "30d"}
			}
			nsMap[p.Namespace].Pods++
		}
	}
	for _, w := range d.workloads {
		nsMap[w.Namespace].Services++
	}
	out := make([]Namespace, 0, len(nsMap))
	for _, n := range nsMap {
		out = append(out, *n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (d *DemoProvider) CreateNamespace(ctx context.Context, name string) (Namespace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Namespace{}, errors.New("namespace name is required")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, w := range d.workloads {
		if w.Namespace == name {
			return Namespace{Name: name, Status: "Active", Age: "now"}, nil
		}
	}
	return Namespace{Name: name, Status: "Active", Age: "now"}, nil
}

func (d *DemoProvider) DeleteNamespace(ctx context.Context, name string) error {
	if name == "default" {
		return errors.New("cannot delete default namespace in demo")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, w := range d.workloads {
		if w.Namespace == name {
			delete(d.workloads, k)
			delete(d.pods, w.Namespace+"/"+w.Name)
		}
	}
	return nil
}

func (d *DemoProvider) ListServices(ctx context.Context, ns string) ([]Service, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Service, 0)
	for _, w := range d.workloads {
		if ns != "" && ns != "all" && w.Namespace != ns {
			continue
		}
		ps := d.pods[w.Namespace+"/"+w.Name]
		eps := make([]string, 0, len(ps))
		for _, p := range ps {
			eps = append(eps, p.Name)
		}
		out = append(out, Service{
			Namespace: w.Namespace, Name: w.Name, Type: "ClusterIP", ClusterIP: fmt.Sprintf("10.43.%d.%d", len(out)+1, 10),
			Ports: []ServicePort{{Name: "http", Port: 80, TargetPort: "8080", Protocol: "TCP"}},
			Selector: map[string]string{"app": w.Name}, Endpoints: eps, Age: w.Age,
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

func (d *DemoProvider) NetworkMap(ctx context.Context, ns string) (NetworkMap, error) {
	services, err := d.ListServices(ctx, ns)
	if err != nil {
		return NetworkMap{}, err
	}
	pods, err := d.ListPods(ctx, ns)
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
		portLabels := []string{"80→8080"}
		addNode(NetworkNode{ID: sid, Kind: "service", Namespace: s.Namespace, Name: s.Name, Label: s.Name, IP: s.ClusterIP, Ports: portLabels})
		for _, ep := range s.Endpoints {
			pid := fmt.Sprintf("pod:%s/%s", s.Namespace, ep)
			var pod Pod
			for _, p := range pods {
				if p.Name == ep && p.Namespace == s.Namespace {
					pod = p
					break
				}
			}
			addNode(NetworkNode{ID: pid, Kind: "pod", Namespace: s.Namespace, Name: ep, Label: ep, IP: pod.IP, Ports: []string{"8080/TCP"}})
			edges = append(edges, NetworkEdge{From: sid, To: pid, Port: 80, Protocol: "TCP", Label: "TCP:80"})
		}
	}
	return NetworkMap{Namespace: ns, Nodes: nodes, Edges: edges}, nil
}
