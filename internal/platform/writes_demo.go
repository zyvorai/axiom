package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Demo implementations for the write-path expansion (update ConfigMap/Secret,
// create VM/PVC/Service/Ingress/Quota, Velero backup, Helm upgrade).

func (d *DemoProvider) UpdateConfigMap(ctx context.Context, req CreateConfigMapRequest) (ConfigMapDetail, error) {
	if req.Namespace == "" || req.Name == "" {
		return ConfigMapDetail{}, fmt.Errorf("namespace and name are required")
	}
	demoConfigMapsMu.Lock()
	defer demoConfigMapsMu.Unlock()
	key := configMapKey(req.Namespace, req.Name)
	cur, ok := demoConfigMaps[key]
	if !ok {
		return ConfigMapDetail{}, fmt.Errorf("configmap %s/%s not found", req.Namespace, req.Name)
	}
	if req.Data != nil {
		cur.Data = req.Data
	}
	if req.Labels != nil {
		cur.Labels = req.Labels
	}
	demoConfigMaps[key] = cur
	return cur, nil
}

func (d *DemoProvider) UpdateSecret(ctx context.Context, req CreateSecretRequest) (SecretDetail, error) {
	if req.Namespace == "" || req.Name == "" {
		return SecretDetail{}, fmt.Errorf("namespace and name are required")
	}
	data := req.Data
	if len(data) == 0 {
		data = req.Keys
	}
	demoSecretsMu.Lock()
	defer demoSecretsMu.Unlock()
	key := req.Namespace + "/" + req.Name
	cur, ok := demoSecrets[key]
	if !ok {
		return SecretDetail{}, fmt.Errorf("secret %s/%s not found", req.Namespace, req.Name)
	}
	keys := make([]SecretKeyInfo, 0, len(data))
	values := map[string]string{}
	for k, v := range data {
		keys = append(keys, SecretKeyInfo{Key: k, Size: len(v)})
		values[k] = v
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Key < keys[j].Key })
	cur.Keys = keys
	cur.Values = values
	if req.Type != "" {
		cur.Type = req.Type
	}
	if req.Labels != nil {
		cur.Labels = req.Labels
	}
	demoSecrets[key] = cur
	out := cur
	out.Values = nil
	return out, nil
}

var (
	demoServicesExtraMu   sync.Mutex
	demoServicesExtra     = map[string]Service{}
	demoIngressExtraMu    sync.Mutex
	demoIngressExtra      = map[string]IngressSummary{}
	demoQuotaExtraMu      sync.Mutex
	demoQuotaExtra        = map[string]ResourceQuotaSummary{}
	demoVeleroBackupMu    sync.Mutex
	demoVeleroBackupExtra = map[string]VeleroBackup{}
)

func (d *DemoProvider) CreateService(ctx context.Context, req CreateServiceRequest) (Service, error) {
	if req.Namespace == "" || req.Name == "" {
		return Service{}, fmt.Errorf("namespace and name are required")
	}
	typ := req.Type
	if typ == "" {
		typ = "ClusterIP"
	}
	port := req.Port
	if port == 0 {
		port = 80
	}
	tp := req.TargetPort
	if tp == 0 {
		tp = port
	}
	svc := Service{
		Namespace: req.Namespace, Name: req.Name, Type: typ, ClusterIP: "10.96.0.50",
		Ports:     []ServicePort{{Port: port, TargetPort: fmt.Sprintf("%d", tp), Protocol: "TCP"}},
		Selector:  req.Selector, Endpoints: []string{}, Age: "now",
	}
	demoServicesExtraMu.Lock()
	defer demoServicesExtraMu.Unlock()
	key := req.Namespace + "/" + req.Name
	if _, exists := demoServicesExtra[key]; exists {
		return Service{}, fmt.Errorf("service %s/%s already exists", req.Namespace, req.Name)
	}
	demoServicesExtra[key] = svc
	return svc, nil
}

func (d *DemoProvider) DeleteService(ctx context.Context, ns, name string) error {
	demoServicesExtraMu.Lock()
	defer demoServicesExtraMu.Unlock()
	key := ns + "/" + name
	if _, ok := demoServicesExtra[key]; !ok {
		return fmt.Errorf("service %s/%s not found", ns, name)
	}
	delete(demoServicesExtra, key)
	return nil
}

func (d *DemoProvider) CreateVolume(ctx context.Context, req CreateVolumeRequest) (Volume, error) {
	if req.Namespace == "" || req.Name == "" || req.Size == "" {
		return Volume{}, fmt.Errorf("namespace, name, and size are required")
	}
	mode := req.AccessMode
	if mode == "" {
		mode = "ReadWriteOnce"
	}
	v := Volume{Namespace: req.Namespace, Name: req.Name, Status: "Bound", Capacity: req.Size, StorageClass: req.StorageClass, AccessModes: mode, Age: "now"}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, existing := range d.extraVolumes {
		if existing.Namespace == req.Namespace && existing.Name == req.Name {
			return Volume{}, fmt.Errorf("pvc %s/%s already exists", req.Namespace, req.Name)
		}
	}
	d.extraVolumes = append(d.extraVolumes, v)
	return v, nil
}

func (d *DemoProvider) DeleteVolume(ctx context.Context, ns, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.extraVolumes[:0]
	found := false
	for _, v := range d.extraVolumes {
		if v.Namespace == ns && v.Name == name {
			found = true
			continue
		}
		out = append(out, v)
	}
	if !found {
		return fmt.Errorf("pvc %s/%s not found", ns, name)
	}
	d.extraVolumes = out
	return nil
}

func (d *DemoProvider) CreateVirtualMachine(ctx context.Context, req CreateVMRequest) (VirtualMachine, error) {
	if req.Namespace == "" || req.Name == "" {
		return VirtualMachine{}, fmt.Errorf("namespace and name are required")
	}
	cpu, mem := req.CPU, req.Memory
	if cpu == "" {
		cpu = "1"
	}
	if mem == "" {
		mem = "2Gi"
	}
	status := "Stopped"
	if req.Running {
		status = "Running"
	}
	vm := VirtualMachine{Namespace: req.Namespace, Name: req.Name, Status: status, Ready: req.Running, Running: req.Running, CPU: cpu, Memory: mem, OS: "linux", Age: "now"}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, existing := range append(demoBaseVMs(), d.extraVMs...) {
		if existing.Namespace == req.Namespace && existing.Name == req.Name {
			return VirtualMachine{}, fmt.Errorf("vm %s/%s already exists", req.Namespace, req.Name)
		}
	}
	d.extraVMs = append(d.extraVMs, vm)
	d.vmRunning[req.Namespace+"/"+req.Name] = req.Running
	return vm, nil
}

func (d *DemoProvider) DeleteVirtualMachine(ctx context.Context, ns, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.extraVMs[:0]
	found := false
	for _, vm := range d.extraVMs {
		if vm.Namespace == ns && vm.Name == name {
			found = true
			continue
		}
		out = append(out, vm)
	}
	if !found {
		// Allow deleting only extra VMs; seeded ones stay for demos.
		return fmt.Errorf("vm %s/%s not found or is a built-in demo VM", ns, name)
	}
	d.extraVMs = out
	delete(d.vmRunning, ns+"/"+name)
	return nil
}

func (d *DemoProvider) CreateIngress(ctx context.Context, req CreateIngressRequest) (IngressSummary, error) {
	if req.Namespace == "" || req.Name == "" || req.Host == "" || req.Service == "" {
		return IngressSummary{}, fmt.Errorf("namespace, name, host, and service are required")
	}
	ing := IngressSummary{Namespace: req.Namespace, Name: req.Name, Class: req.Class, Hosts: []string{req.Host}, TLS: req.TLSSecret != "", Age: "now"}
	demoIngressExtraMu.Lock()
	defer demoIngressExtraMu.Unlock()
	key := req.Namespace + "/" + req.Name
	if _, exists := demoIngressExtra[key]; exists {
		return IngressSummary{}, fmt.Errorf("ingress %s/%s already exists", req.Namespace, req.Name)
	}
	demoIngressExtra[key] = ing
	return ing, nil
}

func (d *DemoProvider) DeleteIngress(ctx context.Context, ns, name string) error {
	demoIngressExtraMu.Lock()
	defer demoIngressExtraMu.Unlock()
	key := ns + "/" + name
	if _, ok := demoIngressExtra[key]; !ok {
		return fmt.Errorf("ingress %s/%s not found", ns, name)
	}
	delete(demoIngressExtra, key)
	return nil
}

func (d *DemoProvider) CreateResourceQuota(ctx context.Context, req CreateResourceQuotaRequest) (ResourceQuotaSummary, error) {
	if req.Namespace == "" || req.Name == "" || len(req.Hard) == 0 {
		return ResourceQuotaSummary{}, fmt.Errorf("namespace, name, and hard limits are required")
	}
	q := ResourceQuotaSummary{Namespace: req.Namespace, Name: req.Name, Hard: req.Hard, Used: map[string]string{}, Age: "now"}
	demoQuotaExtraMu.Lock()
	defer demoQuotaExtraMu.Unlock()
	key := req.Namespace + "/" + req.Name
	if _, exists := demoQuotaExtra[key]; exists {
		return ResourceQuotaSummary{}, fmt.Errorf("resourcequota %s/%s already exists", req.Namespace, req.Name)
	}
	demoQuotaExtra[key] = q
	return q, nil
}

func (d *DemoProvider) DeleteResourceQuota(ctx context.Context, ns, name string) error {
	demoQuotaExtraMu.Lock()
	defer demoQuotaExtraMu.Unlock()
	key := ns + "/" + name
	if _, ok := demoQuotaExtra[key]; !ok {
		return fmt.Errorf("resourcequota %s/%s not found", ns, name)
	}
	delete(demoQuotaExtra, key)
	return nil
}

func (d *DemoProvider) CreateVeleroBackup(ctx context.Context, req CreateVeleroBackupRequest) (VeleroBackup, error) {
	if req.Namespace == "" || req.Name == "" {
		return VeleroBackup{}, fmt.Errorf("namespace and name are required")
	}
	nsList := strings.Join(req.IncludedNamespaces, ",")
	if nsList == "" {
		nsList = "*"
	}
	b := VeleroBackup{Namespace: req.Namespace, Name: req.Name, Phase: "New", Namespaces: nsList, Age: "now"}
	demoVeleroBackupMu.Lock()
	defer demoVeleroBackupMu.Unlock()
	key := req.Namespace + "/" + req.Name
	if _, exists := demoVeleroBackupExtra[key]; exists {
		return VeleroBackup{}, fmt.Errorf("velero backup %s/%s already exists", req.Namespace, req.Name)
	}
	demoVeleroBackupExtra[key] = b
	return b, nil
}

func (d *DemoProvider) UpgradeHelmChart(ctx context.Context, req InstallHelmRequest) (HelmActionResult, error) {
	if req.Namespace == "" || req.ReleaseName == "" {
		return HelmActionResult{}, fmt.Errorf("namespace and releaseName are required")
	}
	return HelmActionResult{OK: true, Action: "upgrade", Message: fmt.Sprintf("demo helm upgrade %s/%s", req.Namespace, req.ReleaseName)}, nil
}
