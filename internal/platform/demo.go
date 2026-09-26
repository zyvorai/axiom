package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
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
	namespaces   map[string]*Namespace
	cordoned     map[string]bool
	nodeTaints   map[string][]TaintSummary
	rollouts     map[string]*demoRolloutState
	vmRunning    map[string]bool
	extraVolumes []Volume
	extraVMs     []VirtualMachine
}

func NewDemoProvider() *DemoProvider {
	d := &DemoProvider{
		workloads: map[string]Workload{}, pods: map[string][]Pod{}, lastReplicas: map[string]int32{}, sealed: map[string]bool{},
		namespaces: map[string]*Namespace{}, cordoned: map[string]bool{}, nodeTaints: map[string][]TaintSummary{},
		rollouts: map[string]*demoRolloutState{}, vmRunning: map[string]bool{},
	}
	seed := []Workload{
		{Namespace: "payments", Name: "payments-api", Kind: "Deployment", Image: "ghcr.io/zyvorai/payments:v2.8.4", Desired: 12, Ready: 12, Available: 12, Status: "Healthy", Age: "3d"},
		{Namespace: "commerce", Name: "checkout", Kind: "Deployment", Image: "ghcr.io/zyvorai/checkout:v1.14.2", Desired: 6, Ready: 6, Available: 6, Status: "Healthy", Age: "11d"},
		{Namespace: "data", Name: "redis", Kind: "StatefulSet", Image: "redis:7.4", Desired: 3, Ready: 3, Available: 3, Status: "Healthy", Age: "28d"},
		{Namespace: "observability", Name: "otel-collector", Kind: "Deployment", Image: "otel/opentelemetry-collector:0.120.0", Desired: 4, Ready: 3, Available: 3, Status: "Degraded", Age: "19d"},
		{Namespace: "observability", Name: "log-shipper", Kind: "DaemonSet", Image: "fluent/fluent-bit:3.1", Desired: 4, Ready: 4, Available: 4, Status: "Healthy", Age: "45d"},
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

// maxDemoReplicas bounds how many synthetic Pod objects rebuildPodsLocked
// will ever materialize for one workload. Without this, a huge replica
// count (e.g. from a malformed request, or just a typo with an extra
// digit) makes make([]Pod, 0, desired) attempt a multi-gigabyte
// allocation — a Go out-of-memory condition is a fatal, unrecoverable
// runtime error that kills the whole process, not something a deferred
// recover() in the HTTP handler can catch. A real cluster has no
// equivalent failure mode here since scaling a Deployment doesn't
// synchronously materialize pod objects in the caller's process.
const maxDemoReplicas = 1000

// dnsLabelRE is what the API server enforces for most object names (RFC 1123 label).
var dnsLabelRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

func (d *DemoProvider) Launch(ctx context.Context, req LaunchRequest) (Workload, error) {
	if req.Name == "" || req.Image == "" {
		return Workload{}, errors.New("name and image are required")
	}
	if !dnsLabelRE.MatchString(req.Name) {
		return Workload{}, fmt.Errorf("invalid name %q: use lowercase letters, numbers and '-' (max 63)", req.Name)
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Replicas < 1 {
		req.Replicas = 1
	}
	if req.Replicas > maxDemoReplicas {
		return Workload{}, fmt.Errorf("replicas must be <= %d in demo mode", maxDemoReplicas)
	}
	w := Workload{Namespace: req.Namespace, Name: req.Name, Kind: "Deployment", Image: req.Image, Desired: req.Replicas, Ready: req.Replicas, Available: req.Replicas, Status: "Healthy", Age: "now", Labels: map[string]string{"app.kubernetes.io/name": req.Name, "app.kubernetes.io/managed-by": "zyvor-axiom"}}
	d.mu.Lock()
	defer d.mu.Unlock()
	// A real cluster's API server rejects a Create for a name that already
	// exists in the namespace with a 409 AlreadyExists — mirror that here
	// so a duplicate submission (e.g. a double-click before the Launch
	// button disables) reports a clear conflict instead of silently
	// overwriting the existing workload and reporting success either way.
	if _, exists := d.workloads[key(w.Namespace, w.Kind, w.Name)]; exists {
		return Workload{}, fmt.Errorf("%s %s/%s already exists", strings.ToLower(w.Kind), w.Namespace, w.Name)
	}
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
	if replicas > maxDemoReplicas {
		return fmt.Errorf("replicas must be <= %d in demo mode", maxDemoReplicas)
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
	k := key(ns, kind, name)
	if _, ok := d.workloads[k]; !ok {
		return fmt.Errorf("%s %s/%s not found", strings.ToLower(kind), ns, name)
	}
	delete(d.workloads, k)
	delete(d.pods, ns+"/"+name)
	delete(d.sealed, k)
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
func (d *DemoProvider) DiagnosePod(ctx context.Context, ns, name string) (PodDiagnosis, error) {
	pods, err := d.ListPods(ctx, ns)
	if err != nil {
		return PodDiagnosis{}, err
	}
	for _, p := range pods {
		if p.Name != name {
			continue
		}
		diag := PodDiagnosis{Namespace: ns, Name: name, Phase: p.Status}
		switch p.Status {
		case "CrashLoopBackOff":
			diag.Scheduled = true
			diag.Diagnosis = "Container is crash-looping — check pod logs for the startup error."
			diag.Events = []string{"BackOff: Back-off restarting failed container"}
		case "Sealed":
			diag.Scheduled = true
			diag.Diagnosis = "Pod is running but network-isolated by an Axiom seal (deny-all NetworkPolicy)."
		default:
			diag.Scheduled = true
			diag.Diagnosis = "Pod is scheduled and running normally."
		}
		return diag, nil
	}
	return PodDiagnosis{}, errors.New("pod not found")
}
func demoBaseNodes() []Node {
	return []Node{
		{Name: "axiom-01", Status: "Ready", Role: "server", Version: "v1.32.4+k3s1", InternalIP: "10.20.0.11", Pods: 18, CPU: "16", Memory: "64Gi", AllocatableCPU: "16", AllocatableMemory: "64Gi", AllocatablePods: "110", OSImage: "Ubuntu 22.04", KernelVersion: "5.15.0", ContainerRuntime: "containerd://1.7", Age: "41d", Conditions: []NodeCondition{{Type: "Ready", Status: "True"}}},
		{Name: "axiom-02", Status: "Ready", Role: "server", Version: "v1.32.4+k3s1", InternalIP: "10.20.0.12", Pods: 21, CPU: "16", Memory: "64Gi", AllocatableCPU: "16", AllocatableMemory: "64Gi", AllocatablePods: "110", OSImage: "Ubuntu 22.04", KernelVersion: "5.15.0", ContainerRuntime: "containerd://1.7", Age: "41d", Conditions: []NodeCondition{{Type: "Ready", Status: "True"}}},
		{Name: "axiom-03", Status: "Ready", Role: "server", Version: "v1.32.4+k3s1", InternalIP: "10.20.0.13", Pods: 17, CPU: "16", Memory: "64Gi", AllocatableCPU: "16", AllocatableMemory: "64Gi", AllocatablePods: "110", OSImage: "Ubuntu 22.04", KernelVersion: "5.15.0", ContainerRuntime: "containerd://1.7", Age: "41d", Conditions: []NodeCondition{{Type: "Ready", Status: "True"}}},
		{Name: "gpu-01", Status: "Ready", Role: "agent", Version: "v1.32.4+k3s1", InternalIP: "10.20.0.21", Pods: 11, CPU: "32", Memory: "256Gi", AllocatableCPU: "32", AllocatableMemory: "256Gi", AllocatablePods: "110", OSImage: "Ubuntu 22.04", KernelVersion: "5.15.0", ContainerRuntime: "containerd://1.7", Age: "17d", Conditions: []NodeCondition{{Type: "Ready", Status: "True"}}, Taints: []TaintSummary{{Key: "nvidia.com/gpu", Value: "true", Effect: "NoSchedule"}}},
	}
}

func demoNodeExists(name string) bool {
	for _, n := range demoBaseNodes() {
		if n.Name == name {
			return true
		}
	}
	return false
}

func (d *DemoProvider) ListNodes(ctx context.Context) ([]Node, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	nodes := demoBaseNodes()
	for i := range nodes {
		n := &nodes[i]
		if d.cordoned[n.Name] {
			n.Unschedulable = true
			n.Status = "SchedulingDisabled"
		}
		if extra := d.nodeTaints[n.Name]; len(extra) > 0 {
			n.Taints = append(append([]TaintSummary{}, n.Taints...), extra...)
		}
	}
	return nodes, nil
}

func (d *DemoProvider) Cordon(ctx context.Context, name string, cordon bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !demoNodeExists(name) {
		return fmt.Errorf("node %s not found", name)
	}
	d.cordoned[name] = cordon
	return nil
}

func (d *DemoProvider) Drain(ctx context.Context, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !demoNodeExists(name) {
		return fmt.Errorf("node %s not found", name)
	}
	d.cordoned[name] = true
	for wk, ps := range d.pods {
		ns, wname, ok := strings.Cut(wk, "/")
		if !ok {
			continue
		}
		kind := ""
		for _, w := range d.workloads {
			if w.Namespace == ns && w.Name == wname {
				kind = w.Kind
				break
			}
		}
		if kind == "DaemonSet" {
			continue
		}
		kept := ps[:0]
		for _, p := range ps {
			if p.Node == name {
				continue
			}
			kept = append(kept, p)
		}
		d.pods[wk] = kept
	}
	return nil
}

func (d *DemoProvider) AddTaint(ctx context.Context, name, key, value, effect string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !demoNodeExists(name) {
		return fmt.Errorf("node %s not found", name)
	}
	for _, t := range d.nodeTaints[name] {
		if t.Key == key && t.Effect == effect {
			return nil
		}
	}
	d.nodeTaints[name] = append(d.nodeTaints[name], TaintSummary{Key: key, Value: value, Effect: effect})
	return nil
}

func (d *DemoProvider) RemoveTaint(ctx context.Context, name, key, effect string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	kept := d.nodeTaints[name][:0]
	for _, t := range d.nodeTaints[name] {
		if t.Key == key && t.Effect == effect {
			continue
		}
		kept = append(kept, t)
	}
	d.nodeTaints[name] = kept
	return nil
}
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
	// Defense in depth: every caller is expected to have already rejected
	// w.Desired > maxDemoReplicas, but clamp here too so a future call
	// site can never reintroduce the huge-allocation crash by skipping
	// that check.
	desired := w.Desired
	if desired > maxDemoReplicas {
		desired = maxDemoReplicas
	}
	ps := make([]Pod, 0, desired)
	sealed := d.sealed[key(w.Namespace, w.Kind, w.Name)]
	for i := int32(0); i < desired; i++ {
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
			Ports:      []PodPort{{Container: w.Name, ContainerPort: 8080, Protocol: "TCP"}},
			Labels:     map[string]string{"app": w.Name, "app.kubernetes.io/name": w.Name},
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
	for name, n := range d.namespaces {
		cp := *n
		nsMap[name] = &cp
	}
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
	if existing, ok := d.namespaces[name]; ok {
		return *existing, nil
	}
	ns := Namespace{Name: name, Status: "Active", Age: "now"}
	d.namespaces[name] = &ns
	return ns, nil
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
	delete(d.namespaces, name)
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
			Ports:    []ServicePort{{Name: "http", Port: 80, TargetPort: "8080", Protocol: "TCP"}},
			Selector: map[string]string{"app": w.Name}, Endpoints: eps, Age: w.Age,
		})
	}
	demoServicesExtraMu.Lock()
	for _, svc := range demoServicesExtra {
		if ns == "" || ns == "all" || svc.Namespace == ns {
			out = append(out, svc)
		}
	}
	demoServicesExtraMu.Unlock()
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
