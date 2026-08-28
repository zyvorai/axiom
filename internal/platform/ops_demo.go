package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	demoSecretsMu sync.Mutex
	demoSecrets   = map[string]SecretDetail{
		"default/app-config": {
			Namespace: "default", Name: "app-config", Type: "Opaque", Age: "5d",
			Keys:   []SecretKeyInfo{{Key: "API_KEY", Size: 32}, {Key: "DB_URL", Size: 48}},
			Values: map[string]string{"API_KEY": "demo-api-key-xxxxxxxx", "DB_URL": "postgres://demo:demo@db:5432/app"},
			Labels: map[string]string{"app": "edge"},
		},
		"platform/tls-ingress": {
			Namespace: "platform", Name: "tls-ingress", Type: "kubernetes.io/tls", Age: "12d",
			Keys:   []SecretKeyInfo{{Key: "tls.crt", Size: 1200}, {Key: "tls.key", Size: 1700}},
			Values: map[string]string{"tls.crt": "-----BEGIN CERT-----", "tls.key": "-----BEGIN KEY-----"},
		},
	}
	demoBackupsMu sync.Mutex
	demoBackups   = []BackupSnapshot{
		{Namespace: "default", Name: "edge-api-data-snap", PVC: "edge-api-data", Ready: true, Status: "Ready", VolumeSnapshotClass: "csi-rbd", Age: "2d"},
		{Namespace: "data", Name: "postgres-nightly", PVC: "postgres-data", Ready: true, Status: "Ready", VolumeSnapshotClass: "csi-rbd", Age: "18h"},
		{Namespace: "platform", Name: "obs-wal-snap", PVC: "obs-wal", Ready: false, Status: "Pending", VolumeSnapshotClass: "csi-rbd", Age: "10m"},
	}
)

func secretKey(ns, name string) string { return ns + "/" + name }

func (d *DemoProvider) ListSecrets(ctx context.Context, ns string) ([]SecretSummary, error) {
	demoSecretsMu.Lock()
	defer demoSecretsMu.Unlock()
	out := make([]SecretSummary, 0, len(demoSecrets))
	for _, s := range demoSecrets {
		if ns != "" && ns != "all" && s.Namespace != ns {
			continue
		}
		keys := make([]string, 0, len(s.Keys))
		for _, k := range s.Keys {
			keys = append(keys, k.Key)
		}
		out = append(out, SecretSummary{
			Namespace: s.Namespace, Name: s.Name, Type: s.Type,
			Keys: keys, KeyCount: len(keys), Age: s.Age,
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

func (d *DemoProvider) GetSecret(ctx context.Context, ns, name string, includeValues bool) (SecretDetail, error) {
	demoSecretsMu.Lock()
	defer demoSecretsMu.Unlock()
	s, ok := demoSecrets[secretKey(ns, name)]
	if !ok {
		return SecretDetail{}, fmt.Errorf("secret %s/%s not found", ns, name)
	}
	out := SecretDetail{
		Namespace: s.Namespace, Name: s.Name, Type: s.Type,
		Keys: append([]SecretKeyInfo(nil), s.Keys...), Labels: s.Labels, Age: s.Age,
	}
	if includeValues {
		out.Values = map[string]string{}
		for k, v := range s.Values {
			out.Values[k] = v
		}
	}
	return out, nil
}

func (d *DemoProvider) CreateSecret(ctx context.Context, req CreateSecretRequest) (SecretDetail, error) {
	if req.Namespace == "" || req.Name == "" {
		return SecretDetail{}, fmt.Errorf("namespace and name are required")
	}
	typ := req.Type
	if typ == "" {
		typ = "Opaque"
	}
	keys := make([]SecretKeyInfo, 0, len(req.Data))
	values := map[string]string{}
	for k, v := range req.Data {
		keys = append(keys, SecretKeyInfo{Key: k, Size: len(v)})
		values[k] = v
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Key < keys[j].Key })
	detail := SecretDetail{
		Namespace: req.Namespace, Name: req.Name, Type: typ,
		Keys: keys, Values: values, Labels: req.Labels, Age: "now",
	}
	demoSecretsMu.Lock()
	demoSecrets[secretKey(req.Namespace, req.Name)] = detail
	demoSecretsMu.Unlock()
	return detail, nil
}

func (d *DemoProvider) DeleteSecret(ctx context.Context, ns, name string) error {
	demoSecretsMu.Lock()
	defer demoSecretsMu.Unlock()
	key := secretKey(ns, name)
	if _, ok := demoSecrets[key]; !ok {
		return fmt.Errorf("secret %s/%s not found", ns, name)
	}
	delete(demoSecrets, key)
	return nil
}

func (d *DemoProvider) ListBackups(ctx context.Context, ns string) ([]BackupSnapshot, error) {
	demoBackupsMu.Lock()
	defer demoBackupsMu.Unlock()
	out := make([]BackupSnapshot, 0, len(demoBackups))
	for _, b := range demoBackups {
		if ns == "" || ns == "all" || b.Namespace == ns {
			out = append(out, b)
		}
	}
	return out, nil
}

func (d *DemoProvider) CreateBackup(ctx context.Context, req CreateBackupRequest) (BackupSnapshot, error) {
	if req.Namespace == "" || req.Name == "" || req.PVC == "" {
		return BackupSnapshot{}, fmt.Errorf("namespace, name, and pvc are required")
	}
	snap := BackupSnapshot{
		Namespace: req.Namespace, Name: req.Name, PVC: req.PVC,
		Ready: true, Status: "Ready",
		VolumeSnapshotClass: req.VolumeSnapshotClass,
		Age:                 "now",
	}
	if snap.VolumeSnapshotClass == "" {
		snap.VolumeSnapshotClass = "csi-rbd"
	}
	demoBackupsMu.Lock()
	demoBackups = append(demoBackups, snap)
	demoBackupsMu.Unlock()
	return snap, nil
}

func (d *DemoProvider) RestoreBackup(ctx context.Context, req RestoreBackupRequest) (Volume, error) {
	if req.Namespace == "" || req.SnapshotName == "" || req.PVCName == "" {
		return Volume{}, fmt.Errorf("namespace, snapshotName, and pvcName are required")
	}
	size := req.Size
	if size == "" {
		size = "10Gi"
	}
	sc := req.StorageClass
	if sc == "" {
		sc = "zyvor-rbd-prod"
	}
	modes := "ReadWriteOnce"
	if len(req.AccessModes) > 0 {
		modes = strings.Join(req.AccessModes, ",")
	}
	return Volume{
		Namespace: req.Namespace, Name: req.PVCName, Status: "Bound",
		Capacity: size, StorageClass: sc, AccessModes: modes, Age: "now",
	}, nil
}

func (d *DemoProvider) ListHelmReleases(ctx context.Context, ns string) ([]HelmRelease, error) {
	all := []HelmRelease{
		{Name: "ingress-nginx", Namespace: "ingress-nginx", Status: "deployed", Chart: "ingress-nginx-4.11.0", Revision: 3, Age: "45d"},
		{Name: "prometheus", Namespace: "monitoring", Status: "deployed", Chart: "kube-prometheus-stack-61.2.0", Revision: 7, Age: "22d"},
		{Name: "cert-manager", Namespace: "cert-manager", Status: "deployed", Chart: "cert-manager-v1.15.1", Revision: 2, Age: "60d"},
	}
	out := make([]HelmRelease, 0, len(all))
	for _, r := range all {
		if ns == "" || ns == "all" || r.Namespace == ns {
			out = append(out, r)
		}
	}
	return out, nil
}

func (d *DemoProvider) ListGitOps(ctx context.Context) ([]GitOpsItem, error) {
	return []GitOpsItem{
		{Kind: "Application", Namespace: "argocd", Name: "platform-apps", Status: "Synced", Source: "https://github.com/zyvorai/platform.git", Age: "14d"},
		{Kind: "Kustomization", Namespace: "flux-system", Name: "apps", Status: "Ready", Source: "GitRepository/flux-system", Age: "9d"},
		{Kind: "HelmRelease", Namespace: "monitoring", Name: "prometheus", Status: "Ready", Source: "HelmRepository/prometheus-community", Age: "22d"},
	}, nil
}

func (d *DemoProvider) ListAlerts(ctx context.Context, ns string) (AlertsResponse, error) {
	events, _ := d.ListEvents(ctx, ns)
	warnings := make([]Event, 0)
	for _, e := range events {
		if strings.EqualFold(e.Type, "Warning") {
			warnings = append(warnings, e)
		}
	}
	rules := []PrometheusRuleSummary{
		{Namespace: "monitoring", Name: "kubernetes-apps", Groups: 4, Age: "22d"},
		{Namespace: "monitoring", Name: "node-exporter", Groups: 2, Age: "22d"},
	}
	if ns != "" && ns != "all" {
		filtered := rules[:0]
		for _, r := range rules {
			if r.Namespace == ns {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}
	return AlertsResponse{Warnings: warnings, Rules: rules, Alertmanagers: []AlertmanagerSummary{
		{Namespace: "monitoring", Name: "kube-prometheus-alertmanager", Version: "v0.27.0", Replicas: 1, Ready: true, Age: "22d"},
	}}, nil
}

func (d *DemoProvider) ListRBAC(ctx context.Context, ns string) (RBACSummary, error) {
	summary := RBACSummary{
		ClusterRoles: []RBACRoleSummary{
			{Name: "cluster-admin", Rules: 1, Age: "120d"},
			{Name: "zyvor-axiom", Rules: 8, Age: "30d"},
			{Name: "view", Rules: 12, Age: "120d"},
		},
		ClusterRoleBindings: []RBACBindingSummary{
			{Name: "zyvor-axiom", RoleRef: "ClusterRole/zyvor-axiom", Subjects: 1, Age: "30d"},
			{Name: "cluster-admin", RoleRef: "ClusterRole/cluster-admin", Subjects: 2, Age: "120d"},
		},
		Roles: []RBACRoleSummary{
			{Namespace: "default", Name: "app-editor", Rules: 5, Age: "15d"},
			{Namespace: "platform", Name: "ops-reader", Rules: 3, Age: "20d"},
		},
		RoleBindings: []RBACBindingSummary{
			{Namespace: "default", Name: "app-editor-binding", RoleRef: "Role/app-editor", Subjects: 2, Age: "15d"},
			{Namespace: "platform", Name: "ops-reader-binding", RoleRef: "Role/ops-reader", Subjects: 1, Age: "20d"},
		},
	}
	if ns != "" && ns != "all" {
		roles := summary.Roles[:0]
		for _, r := range summary.Roles {
			if r.Namespace == ns {
				roles = append(roles, r)
			}
		}
		summary.Roles = roles
		bindings := summary.RoleBindings[:0]
		for _, b := range summary.RoleBindings {
			if b.Namespace == ns {
				bindings = append(bindings, b)
			}
		}
		summary.RoleBindings = bindings
	}
	return summary, nil
}

func (d *DemoProvider) EstimateCost(ctx context.Context) (CostEstimate, error) {
	const cpuRate, memRate = 20.0, 5.0
	nsCosts := []NamespaceCost{
		{Namespace: "payments", CPUCores: 6.0, MemoryGi: 12.0, MonthlyUSD: 6*cpuRate + 12*memRate},
		{Namespace: "commerce", CPUCores: 3.0, MemoryGi: 6.0, MonthlyUSD: 3*cpuRate + 6*memRate},
		{Namespace: "data", CPUCores: 4.5, MemoryGi: 16.0, MonthlyUSD: 4.5*cpuRate + 16*memRate},
		{Namespace: "observability", CPUCores: 2.0, MemoryGi: 8.0, MonthlyUSD: 2*cpuRate + 8*memRate},
	}
	var total float64
	for _, n := range nsCosts {
		total += n.MonthlyUSD
	}
	return CostEstimate{
		TotalMonthlyUSD: round1(total), CPURatePerCore: cpuRate, MemoryRatePerGi: memRate,
		Namespaces: nsCosts,
	}, nil
}

func (d *DemoProvider) ValidateManifest(ctx context.Context, manifest string) (ValidateResult, error) {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return ValidateResult{Valid: false, Errors: []string{"empty manifest"}}, nil
	}
	kind, apiVersion, name, ns := parseManifestFields(manifest)
	if kind == "" || apiVersion == "" {
		return ValidateResult{Valid: false, Kind: kind, APIVersion: apiVersion, Errors: []string{"kind and apiVersion are required"}}, nil
	}
	if name == "" {
		return ValidateResult{Valid: false, Kind: kind, APIVersion: apiVersion, Errors: []string{"metadata.name is required"}}, nil
	}
	return ValidateResult{
		Valid: true, Kind: kind, APIVersion: apiVersion, Namespace: ns, Name: name,
		Message: "manifest structure looks valid (demo dry-run)",
	}, nil
}

func parseManifestFields(manifest string) (kind, apiVersion, name, ns string) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(manifest), &obj); err == nil {
		kind, _ = obj["kind"].(string)
		apiVersion, _ = obj["apiVersion"].(string)
		if meta, ok := obj["metadata"].(map[string]any); ok {
			name, _ = meta["name"].(string)
			ns, _ = meta["namespace"].(string)
		}
		return kind, apiVersion, name, ns
	}
	// Lightweight YAML field scrape for demo builds (no yaml dependency).
	for _, line := range strings.Split(manifest, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "kind:") {
			kind = strings.TrimSpace(strings.TrimPrefix(line, "kind:"))
			kind = strings.Trim(kind, `"'`)
		} else if strings.HasPrefix(line, "apiVersion:") {
			apiVersion = strings.TrimSpace(strings.TrimPrefix(line, "apiVersion:"))
			apiVersion = strings.Trim(apiVersion, `"'`)
		} else if strings.HasPrefix(line, "name:") && name == "" {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.Trim(name, `"'`)
		} else if strings.HasPrefix(line, "namespace:") && ns == "" {
			ns = strings.TrimSpace(strings.TrimPrefix(line, "namespace:"))
			ns = strings.Trim(ns, `"'`)
		}
	}
	return kind, apiVersion, name, ns
}

func (d *DemoProvider) VMConsole(ctx context.Context, ns, name string) (VMConsoleInfo, error) {
	vms, err := d.ListVirtualMachines(ctx, ns)
	if err != nil {
		return VMConsoleInfo{}, err
	}
	for _, vm := range vms {
		if vm.Name == name {
			return VMConsoleInfo{
				Namespace: ns, Name: name, Ready: vm.Ready,
				VNCPath: fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/vnc", ns, name),
				Virtctl: fmt.Sprintf("virtctl vnc %s -n %s", name, ns),
			}, nil
		}
	}
	return VMConsoleInfo{}, fmt.Errorf("vm %s/%s not found", ns, name)
}

var (
	demoConfigMapsMu sync.Mutex
	demoConfigMaps   = map[string]ConfigMapDetail{
		"default/app-config": {
			Namespace: "default", Name: "app-config", Age: "5d",
			Data:   map[string]string{"LOG_LEVEL": "info", "FEATURE_FLAGS": "beta-checkout=true"},
			Labels: map[string]string{"app": "edge"},
		},
		"observability/otel-collector-config": {
			Namespace: "observability", Name: "otel-collector-config", Age: "19d",
			Data: map[string]string{"config.yaml": "receivers:\n  otlp:\n    protocols:\n      grpc:"},
		},
	}
)

func configMapKey(ns, name string) string { return ns + "/" + name }

func (d *DemoProvider) ListConfigMaps(ctx context.Context, ns string) ([]ConfigMapSummary, error) {
	demoConfigMapsMu.Lock()
	defer demoConfigMapsMu.Unlock()
	out := make([]ConfigMapSummary, 0, len(demoConfigMaps))
	for _, c := range demoConfigMaps {
		if ns != "" && ns != "all" && c.Namespace != ns {
			continue
		}
		keys := make([]string, 0, len(c.Data))
		for k := range c.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out = append(out, ConfigMapSummary{Namespace: c.Namespace, Name: c.Name, Keys: keys, KeyCount: len(keys), Age: c.Age})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func (d *DemoProvider) GetConfigMap(ctx context.Context, ns, name string) (ConfigMapDetail, error) {
	demoConfigMapsMu.Lock()
	defer demoConfigMapsMu.Unlock()
	c, ok := demoConfigMaps[configMapKey(ns, name)]
	if !ok {
		return ConfigMapDetail{}, fmt.Errorf("configmap %s/%s not found", ns, name)
	}
	return c, nil
}

func (d *DemoProvider) CreateConfigMap(ctx context.Context, req CreateConfigMapRequest) (ConfigMapDetail, error) {
	if req.Namespace == "" || req.Name == "" {
		return ConfigMapDetail{}, fmt.Errorf("namespace and name are required")
	}
	if len(req.Data) == 0 {
		return ConfigMapDetail{}, fmt.Errorf("at least one data entry is required")
	}
	detail := ConfigMapDetail{Namespace: req.Namespace, Name: req.Name, Data: req.Data, Labels: req.Labels, Age: "now"}
	demoConfigMapsMu.Lock()
	demoConfigMaps[configMapKey(req.Namespace, req.Name)] = detail
	demoConfigMapsMu.Unlock()
	return detail, nil
}

func (d *DemoProvider) DeleteConfigMap(ctx context.Context, ns, name string) error {
	demoConfigMapsMu.Lock()
	defer demoConfigMapsMu.Unlock()
	key := configMapKey(ns, name)
	if _, ok := demoConfigMaps[key]; !ok {
		return fmt.Errorf("configmap %s/%s not found", ns, name)
	}
	delete(demoConfigMaps, key)
	return nil
}

func (d *DemoProvider) ListResourceQuotas(ctx context.Context, ns string) ([]ResourceQuotaSummary, error) {
	all := []ResourceQuotaSummary{
		{Namespace: "tenant-a", Name: "tenant-a-quota", Age: "30d",
			Hard: map[string]string{"requests.cpu": "20", "requests.memory": "40Gi", "pods": "100"},
			Used: map[string]string{"requests.cpu": "6", "requests.memory": "12Gi", "pods": "23"}},
		{Namespace: "tenant-b", Name: "tenant-b-quota", Age: "30d",
			Hard: map[string]string{"requests.cpu": "10", "requests.memory": "20Gi", "pods": "50"},
			Used: map[string]string{"requests.cpu": "3", "requests.memory": "6Gi", "pods": "11"}},
	}
	out := make([]ResourceQuotaSummary, 0, len(all))
	for _, q := range all {
		if ns == "" || ns == "all" || q.Namespace == ns {
			out = append(out, q)
		}
	}
	return out, nil
}

func (d *DemoProvider) ListLimitRanges(ctx context.Context, ns string) ([]LimitRangeSummary, error) {
	all := []LimitRangeSummary{
		{Namespace: "tenant-a", Name: "tenant-a-limits", Age: "30d", Limits: []LimitRangeItem{
			{Type: "Container", Default: "cpu=500m mem=512Mi", DefaultRequest: "cpu=100m mem=128Mi", Max: "cpu=2 mem=2Gi"},
		}},
	}
	out := make([]LimitRangeSummary, 0, len(all))
	for _, l := range all {
		if ns == "" || ns == "all" || l.Namespace == ns {
			out = append(out, l)
		}
	}
	return out, nil
}

func (d *DemoProvider) ListIngresses(ctx context.Context, ns string) ([]IngressSummary, error) {
	all := []IngressSummary{
		{Namespace: "commerce", Name: "checkout", Class: "nginx", Hosts: []string{"checkout.zyvor.dev"}, Address: "203.0.113.10", TLS: true, Age: "20d"},
		{Namespace: "payments", Name: "payments-api", Class: "nginx", Hosts: []string{"api.payments.zyvor.dev"}, Address: "203.0.113.11", TLS: true, Age: "9d"},
	}
	out := make([]IngressSummary, 0, len(all))
	for _, i := range all {
		if ns == "" || ns == "all" || i.Namespace == ns {
			out = append(out, i)
		}
	}
	return out, nil
}
