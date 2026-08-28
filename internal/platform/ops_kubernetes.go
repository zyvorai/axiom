//go:build kubernetes

package platform

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/yaml"
)

var (
	volumeSnapshotGVR = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshots"}
	prometheusRuleGVR = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "prometheusrules"}
	argoAppGVRs       = []schema.GroupVersionResource{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
	}
	fluxKustomizationGVRs = []schema.GroupVersionResource{
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1beta2", Resource: "kustomizations"},
	}
	fluxHelmReleaseGVRs = []schema.GroupVersionResource{
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2beta2", Resource: "helmreleases"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2beta1", Resource: "helmreleases"},
	}
)

const (
	costCPURatePerCore  = 20.0
	costMemoryRatePerGi = 5.0
)

func isMissingCRD(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "the server could not find the requested resource") ||
		strings.Contains(s, "no matches for") ||
		strings.Contains(s, "failed to get API group resources") ||
		strings.Contains(s, "could not find the requested resource")
}

func (k *KubernetesProvider) ListSecrets(ctx context.Context, ns string) ([]SecretSummary, error) {
	list, err := k.client.CoreV1().Secrets(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]SecretSummary, 0, len(list.Items))
	for _, s := range list.Items {
		keys := make([]string, 0, len(s.Data))
		for key := range s.Data {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out = append(out, SecretSummary{
			Namespace: s.Namespace, Name: s.Name, Type: string(s.Type),
			Keys: keys, KeyCount: len(keys), Age: age(s.CreationTimestamp.Time),
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

func (k *KubernetesProvider) GetSecret(ctx context.Context, ns, name string, includeValues bool) (SecretDetail, error) {
	s, err := k.client.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return SecretDetail{}, err
	}
	keys := make([]SecretKeyInfo, 0, len(s.Data))
	values := map[string]string{}
	for key, raw := range s.Data {
		keys = append(keys, SecretKeyInfo{Key: key, Size: len(raw)})
		if includeValues {
			values[key] = string(raw)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Key < keys[j].Key })
	detail := SecretDetail{
		Namespace: s.Namespace, Name: s.Name, Type: string(s.Type),
		Keys: keys, Labels: s.Labels, Age: age(s.CreationTimestamp.Time),
	}
	if includeValues {
		detail.Values = values
	}
	return detail, nil
}

func (k *KubernetesProvider) CreateSecret(ctx context.Context, req CreateSecretRequest) (SecretDetail, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" {
		return SecretDetail{}, fmt.Errorf("namespace and name are required")
	}
	typ := corev1.SecretType(req.Type)
	if typ == "" {
		typ = corev1.SecretTypeOpaque
	}
	data := map[string][]byte{}
	for key, val := range req.Data {
		data[key] = []byte(val)
	}
	for key, val := range req.Keys {
		if _, exists := data[key]; !exists {
			data[key] = []byte(val)
		}
	}
	if len(data) == 0 {
		return SecretDetail{}, fmt.Errorf("at least one data/keys entry is required")
	}
	created, err := k.client.CoreV1().Secrets(req.Namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, Labels: req.Labels},
		Type:       typ,
		Data:       data,
	}, metav1.CreateOptions{})
	if err != nil {
		return SecretDetail{}, err
	}
	return k.GetSecret(ctx, created.Namespace, created.Name, false)
}

func (k *KubernetesProvider) DeleteSecret(ctx context.Context, ns, name string) error {
	return k.client.CoreV1().Secrets(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *KubernetesProvider) ListBackups(ctx context.Context, ns string) ([]BackupSnapshot, error) {
	if k.dynamic == nil {
		return []BackupSnapshot{}, nil
	}
	list, err := k.dynamic.Resource(volumeSnapshotGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isMissingCRD(err) {
			return []BackupSnapshot{}, nil
		}
		return nil, err
	}
	out := make([]BackupSnapshot, 0, len(list.Items))
	for _, item := range list.Items {
		pvc, _, _ := unstructured.NestedString(item.Object, "spec", "source", "persistentVolumeClaimName")
		ready, _, _ := unstructured.NestedBool(item.Object, "status", "readyToUse")
		status := "Pending"
		if ready {
			status = "Ready"
		}
		if msg, _, _ := unstructured.NestedString(item.Object, "status", "error", "message"); msg != "" {
			status = "Error"
		}
		vsc, _, _ := unstructured.NestedString(item.Object, "spec", "volumeSnapshotClassName")
		out = append(out, BackupSnapshot{
			Namespace: item.GetNamespace(), Name: item.GetName(), PVC: pvc,
			Ready: ready, Status: status, VolumeSnapshotClass: vsc,
			Age: age(item.GetCreationTimestamp().Time),
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

func (k *KubernetesProvider) CreateBackup(ctx context.Context, req CreateBackupRequest) (BackupSnapshot, error) {
	if k.dynamic == nil {
		return BackupSnapshot{}, fmt.Errorf("dynamic client unavailable")
	}
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.PVC) == "" {
		return BackupSnapshot{}, fmt.Errorf("namespace, name, and pvc are required")
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "snapshot.storage.k8s.io/v1",
		"kind":       "VolumeSnapshot",
		"metadata": map[string]any{
			"name":      req.Name,
			"namespace": req.Namespace,
		},
		"spec": map[string]any{
			"source": map[string]any{
				"persistentVolumeClaimName": req.PVC,
			},
		},
	}}
	if req.VolumeSnapshotClass != "" {
		_ = unstructured.SetNestedField(obj.Object, req.VolumeSnapshotClass, "spec", "volumeSnapshotClassName")
	}
	created, err := k.dynamic.Resource(volumeSnapshotGVR).Namespace(req.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return BackupSnapshot{}, err
	}
	return BackupSnapshot{
		Namespace: created.GetNamespace(), Name: created.GetName(), PVC: req.PVC,
		Ready: false, Status: "Pending", VolumeSnapshotClass: req.VolumeSnapshotClass,
		Age: age(created.GetCreationTimestamp().Time),
	}, nil
}

func (k *KubernetesProvider) RestoreBackup(ctx context.Context, req RestoreBackupRequest) (Volume, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.SnapshotName) == "" || strings.TrimSpace(req.PVCName) == "" {
		return Volume{}, fmt.Errorf("namespace, snapshotName, and pvcName are required")
	}
	size := req.Size
	if size == "" {
		size = "10Gi"
	}
	modes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	if len(req.AccessModes) > 0 {
		modes = modes[:0]
		for _, m := range req.AccessModes {
			modes = append(modes, corev1.PersistentVolumeAccessMode(m))
		}
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: req.PVCName, Namespace: req.Namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: modes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: strPtr("snapshot.storage.k8s.io"),
				Kind:     "VolumeSnapshot",
				Name:     req.SnapshotName,
			},
		},
	}
	if req.StorageClass != "" {
		pvc.Spec.StorageClassName = &req.StorageClass
	}
	created, err := k.client.CoreV1().PersistentVolumeClaims(req.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return Volume{}, err
	}
	sc := ""
	if created.Spec.StorageClassName != nil {
		sc = *created.Spec.StorageClassName
	}
	modeStrs := make([]string, 0, len(created.Spec.AccessModes))
	for _, m := range created.Spec.AccessModes {
		modeStrs = append(modeStrs, string(m))
	}
	return Volume{
		Namespace: created.Namespace, Name: created.Name, Status: string(created.Status.Phase),
		Capacity: size, StorageClass: sc, AccessModes: strings.Join(modeStrs, ","),
		Age: age(created.CreationTimestamp.Time),
	}, nil
}

func strPtr(s string) *string { return &s }

func (k *KubernetesProvider) ListHelmReleases(ctx context.Context, ns string) ([]HelmRelease, error) {
	list, err := k.client.CoreV1().Secrets(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	seen := map[string]HelmRelease{}
	for _, s := range list.Items {
		isHelm := s.Type == "helm.sh/release.v1" || s.Labels["owner"] == "helm"
		if !isHelm {
			continue
		}
		rel := helmReleaseFromSecret(s)
		key := rel.Namespace + "/" + rel.Name
		if prev, ok := seen[key]; ok {
			if rel.Revision >= prev.Revision {
				seen[key] = rel
			}
			continue
		}
		seen[key] = rel
	}
	out := make([]HelmRelease, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func helmReleaseFromSecret(s corev1.Secret) HelmRelease {
	name := s.Labels["name"]
	if name == "" {
		name = s.Annotations["meta.helm.sh/release-name"]
	}
	if name == "" {
		// sh.helm.release.v1.<name>.v<rev>
		parts := strings.Split(s.Name, ".")
		if len(parts) >= 5 && parts[0] == "sh" && parts[1] == "helm" {
			name = parts[4]
		} else {
			name = s.Name
		}
	}
	ns := s.Namespace
	if v := s.Annotations["meta.helm.sh/release-namespace"]; v != "" {
		ns = v
	}
	status := s.Labels["status"]
	if status == "" {
		status = "unknown"
	}
	revision := 0
	if v := s.Labels["version"]; v != "" {
		revision, _ = strconv.Atoi(v)
	}
	chart := s.Labels["chart"]
	if raw, ok := s.Data["release"]; ok {
		if decoded, err := decodeHelmReleasePayload(raw); err == nil {
			if decoded.Name != "" {
				name = decoded.Name
			}
			if decoded.Namespace != "" {
				ns = decoded.Namespace
			}
			if decoded.Version > 0 {
				revision = decoded.Version
			}
			if decoded.Info.Status != "" {
				status = decoded.Info.Status
			}
			if decoded.Chart.Metadata.Name != "" {
				chart = decoded.Chart.Metadata.Name
				if decoded.Chart.Metadata.Version != "" {
					chart = chart + "-" + decoded.Chart.Metadata.Version
				}
			}
		}
	}
	return HelmRelease{
		Name: name, Namespace: ns, Status: status, Chart: chart,
		Revision: revision, Age: age(s.CreationTimestamp.Time),
	}
}

type helmReleasePayload struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int    `json:"version"`
	Info      struct {
		Status string `json:"status"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"metadata"`
	} `json:"chart"`
}

func decodeHelmReleasePayload(data []byte) (helmReleasePayload, error) {
	var out helmReleasePayload
	raw := data
	if decoded, err := base64.StdEncoding.DecodeString(string(data)); err == nil {
		raw = decoded
	}
	r, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return out, err
	}
	defer r.Close()
	unzipped, err := io.ReadAll(r)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(unzipped, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (k *KubernetesProvider) ListGitOps(ctx context.Context) ([]GitOpsItem, error) {
	if k.dynamic == nil {
		return []GitOpsItem{}, nil
	}
	var out []GitOpsItem
	out = append(out, k.listGitOpsGVR(ctx, argoAppGVRs, "Application", gitOpsArgoStatus)...)
	out = append(out, k.listGitOpsGVR(ctx, fluxKustomizationGVRs, "Kustomization", gitOpsFluxStatus)...)
	out = append(out, k.listGitOpsGVR(ctx, fluxHelmReleaseGVRs, "HelmRelease", gitOpsFluxStatus)...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			if out[i].Namespace == out[j].Namespace {
				return out[i].Name < out[j].Name
			}
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Kind < out[j].Kind
	})
	if out == nil {
		out = []GitOpsItem{}
	}
	return out, nil
}

type gitOpsStatusFn func(unstructured.Unstructured) (status, source string)

func (k *KubernetesProvider) listGitOpsGVR(ctx context.Context, gvrs []schema.GroupVersionResource, kind string, statusFn gitOpsStatusFn) []GitOpsItem {
	for _, gvr := range gvrs {
		list, err := k.dynamic.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			if isMissingCRD(err) {
				continue
			}
			continue
		}
		out := make([]GitOpsItem, 0, len(list.Items))
		for _, item := range list.Items {
			status, source := statusFn(item)
			out = append(out, GitOpsItem{
				Kind: kind, Namespace: item.GetNamespace(), Name: item.GetName(),
				Status: status, Source: source, Age: age(item.GetCreationTimestamp().Time),
			})
		}
		return out
	}
	return nil
}

func gitOpsArgoStatus(item unstructured.Unstructured) (string, string) {
	status, _, _ := unstructured.NestedString(item.Object, "status", "sync", "status")
	if status == "" {
		status, _, _ = unstructured.NestedString(item.Object, "status", "health", "status")
	}
	if status == "" {
		status = "Unknown"
	}
	repo, _, _ := unstructured.NestedString(item.Object, "spec", "source", "repoURL")
	path, _, _ := unstructured.NestedString(item.Object, "spec", "source", "path")
	source := repo
	if path != "" && repo != "" {
		source = repo + ":" + path
	}
	return status, source
}

func gitOpsFluxStatus(item unstructured.Unstructured) (string, string) {
	conds, _, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	status := "Unknown"
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if fmt.Sprint(m["type"]) == "Ready" {
			if fmt.Sprint(m["status"]) == "True" {
				status = "Ready"
			} else {
				status = fmt.Sprint(m["reason"])
				if status == "" {
					status = "NotReady"
				}
			}
			break
		}
	}
	source, _, _ := unstructured.NestedString(item.Object, "spec", "sourceRef", "name")
	kind, _, _ := unstructured.NestedString(item.Object, "spec", "sourceRef", "kind")
	if kind != "" && source != "" {
		source = kind + "/" + source
	}
	if chart, _, _ := unstructured.NestedString(item.Object, "spec", "chart", "spec", "chart"); chart != "" {
		source = chart
	}
	return status, source
}

func (k *KubernetesProvider) ListAlerts(ctx context.Context, ns string) (AlertsResponse, error) {
	events, err := k.ListEvents(ctx, ns)
	if err != nil {
		return AlertsResponse{}, err
	}
	warnings := make([]Event, 0)
	for _, e := range events {
		if strings.EqualFold(e.Type, "Warning") {
			warnings = append(warnings, e)
		}
	}
	rules := []PrometheusRuleSummary{}
	if k.dynamic != nil {
		list, err := k.dynamic.Resource(prometheusRuleGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, item := range list.Items {
				groups, _, _ := unstructured.NestedSlice(item.Object, "spec", "groups")
				rules = append(rules, PrometheusRuleSummary{
					Namespace: item.GetNamespace(), Name: item.GetName(),
					Groups: len(groups), Age: age(item.GetCreationTimestamp().Time),
				})
			}
			sort.Slice(rules, func(i, j int) bool {
				if rules[i].Namespace == rules[j].Namespace {
					return rules[i].Name < rules[j].Name
				}
				return rules[i].Namespace < rules[j].Namespace
			})
		} else if !isMissingCRD(err) {
			return AlertsResponse{Warnings: warnings, Rules: rules}, err
		}
	}
	return AlertsResponse{Warnings: warnings, Rules: rules, Alertmanagers: k.listAlertmanagers(ctx, ns)}, nil
}

func (k *KubernetesProvider) ListRBAC(ctx context.Context, ns string) (RBACSummary, error) {
	summary := RBACSummary{
		ClusterRoles:        []RBACRoleSummary{},
		ClusterRoleBindings: []RBACBindingSummary{},
		Roles:               []RBACRoleSummary{},
		RoleBindings:        []RBACBindingSummary{},
	}
	crs, err := k.client.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		return summary, err
	}
	for _, cr := range crs.Items {
		summary.ClusterRoles = append(summary.ClusterRoles, RBACRoleSummary{
			Name: cr.Name, Rules: len(cr.Rules), Age: age(cr.CreationTimestamp.Time),
		})
	}
	crbs, err := k.client.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return summary, err
	}
	for _, b := range crbs.Items {
		summary.ClusterRoleBindings = append(summary.ClusterRoleBindings, RBACBindingSummary{
			Name: b.Name, RoleRef: b.RoleRef.Kind + "/" + b.RoleRef.Name,
			Subjects: len(b.Subjects), Age: age(b.CreationTimestamp.Time),
		})
	}
	roles, err := k.client.RbacV1().Roles(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return summary, err
	}
	for _, r := range roles.Items {
		summary.Roles = append(summary.Roles, RBACRoleSummary{
			Namespace: r.Namespace, Name: r.Name, Rules: len(r.Rules), Age: age(r.CreationTimestamp.Time),
		})
	}
	rbs, err := k.client.RbacV1().RoleBindings(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return summary, err
	}
	for _, b := range rbs.Items {
		summary.RoleBindings = append(summary.RoleBindings, RBACBindingSummary{
			Namespace: b.Namespace, Name: b.Name, RoleRef: b.RoleRef.Kind + "/" + b.RoleRef.Name,
			Subjects: len(b.Subjects), Age: age(b.CreationTimestamp.Time),
		})
	}
	_ = rbacv1.SchemeGroupVersion
	return summary, nil
}

func (k *KubernetesProvider) EstimateCost(ctx context.Context) (CostEstimate, error) {
	pods, err := k.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return CostEstimate{}, err
	}
	type acc struct{ cpu, mem float64 }
	byNS := map[string]*acc{}
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
			continue
		}
		a := byNS[p.Namespace]
		if a == nil {
			a = &acc{}
			byNS[p.Namespace] = a
		}
		for _, c := range p.Spec.Containers {
			if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				a.cpu += float64(q.MilliValue()) / 1000.0
			}
			if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				a.mem += float64(q.Value()) / (1024 * 1024 * 1024)
			}
		}
	}
	names := make([]string, 0, len(byNS))
	for ns := range byNS {
		names = append(names, ns)
	}
	sort.Strings(names)
	nsCosts := make([]NamespaceCost, 0, len(names))
	var total float64
	for _, ns := range names {
		a := byNS[ns]
		usd := a.cpu*costCPURatePerCore + a.mem*costMemoryRatePerGi
		total += usd
		nsCosts = append(nsCosts, NamespaceCost{
			Namespace: ns, CPUCores: round1(a.cpu), MemoryGi: round1(a.mem), MonthlyUSD: round1(usd),
		})
	}
	return CostEstimate{
		TotalMonthlyUSD: round1(total),
		CPURatePerCore:  costCPURatePerCore,
		MemoryRatePerGi: costMemoryRatePerGi,
		Namespaces:      nsCosts,
	}, nil
}

func (k *KubernetesProvider) ValidateManifest(ctx context.Context, manifest string) (ValidateResult, error) {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return ValidateResult{Valid: false, Errors: []string{"empty manifest"}}, nil
	}
	var obj unstructured.Unstructured
	if err := json.Unmarshal([]byte(manifest), &obj.Object); err != nil {
		if err := yaml.Unmarshal([]byte(manifest), &obj.Object); err != nil {
			return ValidateResult{Valid: false, Errors: []string{err.Error()}}, nil
		}
	}
	gvk := obj.GroupVersionKind()
	if gvk.Kind == "" || gvk.Version == "" {
		return ValidateResult{Valid: false, Errors: []string{"kind and apiVersion are required"}}, nil
	}
	name := obj.GetName()
	ns := obj.GetNamespace()
	if name == "" {
		return ValidateResult{Valid: false, Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String(), Errors: []string{"metadata.name is required"}}, nil
	}
	if k.dynamic == nil || k.client == nil {
		return ValidateResult{Valid: true, Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String(), Namespace: ns, Name: name, Message: "structure valid (no dry-run client)"}, nil
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(k.client.Discovery()))
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return ValidateResult{
			Valid: false, Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String(),
			Namespace: ns, Name: name, Errors: []string{err.Error()},
		}, nil
	}
	opts := metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}
	var createErr error
	if mapping.Scope.Name() == "namespace" {
		if ns == "" {
			ns = "default"
			obj.SetNamespace(ns)
		}
		_, createErr = k.dynamic.Resource(mapping.Resource).Namespace(ns).Create(ctx, &obj, opts)
	} else {
		_, createErr = k.dynamic.Resource(mapping.Resource).Create(ctx, &obj, opts)
	}
	if createErr != nil {
		return ValidateResult{
			Valid: false, Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String(),
			Namespace: ns, Name: name, Message: "dry-run failed", Errors: []string{createErr.Error()},
		}, nil
	}
	return ValidateResult{
		Valid: true, Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String(),
		Namespace: ns, Name: name, Message: "dry-run create succeeded",
	}, nil
}

func (k *KubernetesProvider) VMConsole(ctx context.Context, ns, name string) (VMConsoleInfo, error) {
	vms, err := k.ListVirtualMachines(ctx, ns)
	if err != nil {
		return VMConsoleInfo{}, err
	}
	var found *VirtualMachine
	for i := range vms {
		if vms[i].Name == name && (ns == "" || vms[i].Namespace == ns) {
			found = &vms[i]
			break
		}
	}
	if found == nil {
		// Still return console metadata if VMI might exist.
		ready := false
		if k.dynamic != nil {
			if obj, err := k.dynamic.Resource(kubevirtVMGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{}); err == nil {
				ready, _, _ = unstructured.NestedBool(obj.Object, "status", "ready")
				found = &VirtualMachine{Namespace: ns, Name: name, Ready: ready}
			}
		}
	}
	if found == nil {
		return VMConsoleInfo{}, fmt.Errorf("vm %s/%s not found", ns, name)
	}
	return VMConsoleInfo{
		Namespace: found.Namespace, Name: found.Name, Ready: found.Ready,
		VNCPath: fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/vnc", found.Namespace, found.Name),
		Virtctl: fmt.Sprintf("virtctl vnc %s -n %s", found.Name, found.Namespace),
	}, nil
}
