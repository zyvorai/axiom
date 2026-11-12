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
	"sync"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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
	argoAppSetGVRs = []schema.GroupVersionResource{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applicationsets"},
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
	fluxGitRepositoryGVRs = []schema.GroupVersionResource{
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
		{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "gitrepositories"},
	}
	fluxHelmRepositoryGVRs = []schema.GroupVersionResource{
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories"},
		{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "helmrepositories"},
	}
	gatewayClassGVRs = []schema.GroupVersionResource{
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"},
		{Group: "gateway.networking.k8s.io", Version: "v1beta1", Resource: "gatewayclasses"},
	}
	gatewayGVRs = []schema.GroupVersionResource{
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"},
		{Group: "gateway.networking.k8s.io", Version: "v1beta1", Resource: "gateways"},
	}
	httpRouteGVRs = []schema.GroupVersionResource{
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"},
		{Group: "gateway.networking.k8s.io", Version: "v1beta1", Resource: "httproutes"},
	}
)

// costCPURatePerCore / costMemoryRatePerGi live in cost_rightsizing.go.

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
	// Prefer PartialObjectMetadata so list never downloads Secret.data (GAPS:
	// large clusters were slow because every Secret body was pulled).
	if out, ok := k.listSecretsMetadata(ctx, ns); ok {
		return out, nil
	}
	const page = int64(250)
	var out []SecretSummary
	var cont string
	for {
		list, err := k.client.CoreV1().Secrets(nsSelector(ns)).List(ctx, metav1.ListOptions{Limit: page, Continue: cont})
		if err != nil {
			return nil, err
		}
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
		cont = list.Continue
		if cont == "" {
			break
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
	sizeQty, err := resource.ParseQuantity(size) // user input: MustParse would panic the handler
	if err != nil {
		return Volume{}, fmt.Errorf("invalid size: %w", err)
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
					corev1.ResourceStorage: sizeQty,
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
	// Helm 3 labels every release Secret owner=helm. Selecting on that avoids
	// transferring every Secret in the cluster (with data) just to find them.
	list, err := k.client.CoreV1().Secrets(nsSelector(ns)).List(ctx, metav1.ListOptions{LabelSelector: "owner=helm"})
	if err != nil {
		return nil, err
	}
	// Pick the newest revision of each release from the labels alone, and only
	// then gunzip/decode that one payload — a release with 50 revisions would
	// otherwise be decoded 50 times per page load.
	type cand struct {
		s   corev1.Secret
		rev int
	}
	best := map[string]cand{}
	for _, s := range list.Items {
		if s.Type != "helm.sh/release.v1" && s.Labels["owner"] != "helm" {
			continue
		}
		name := s.Labels["name"]
		if name == "" {
			name = s.Annotations["meta.helm.sh/release-name"]
		}
		if name == "" {
			if parts := strings.Split(s.Name, "."); len(parts) >= 5 && parts[0] == "sh" && parts[1] == "helm" {
				name = parts[4]
			} else {
				name = s.Name
			}
		}
		relNS := s.Namespace
		if v := s.Annotations["meta.helm.sh/release-namespace"]; v != "" {
			relNS = v
		}
		rev, _ := strconv.Atoi(s.Labels["version"])
		key := relNS + "/" + name
		if prev, ok := best[key]; !ok || rev >= prev.rev {
			best[key] = cand{s: s, rev: rev}
		}
	}
	out := make([]HelmRelease, 0, len(best))
	for _, c := range best {
		out = append(out, helmReleaseFromSecret(c.s))
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
	out = append(out, k.listGitOpsGVR(ctx, argoAppSetGVRs, "ApplicationSet", gitOpsArgoStatus)...)
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

// TriggerGitOpsSync forces a re-sync, branching by kind (already known from
// ListGitOps's Kind field): Flux Kustomization/HelmRelease get their
// standard force-reconcile annotation bumped; Argo Application gets its
// top-level `operation.sync` field set, the same mechanism `argocd app
// sync` uses. Both are a JSON merge patch via the dynamic client, matching
// how Flux/Argo/Velero are already accessed everywhere else in this file —
// no SDK for either engine.
func (k *KubernetesProvider) TriggerGitOpsSync(ctx context.Context, kind, ns, name string) error {
	if k.dynamic == nil {
		return fmt.Errorf("dynamic client unavailable")
	}
	switch kind {
	case "Kustomization":
		return k.patchFirstGVR(ctx, fluxKustomizationGVRs, ns, name, fluxReconcilePatch())
	case "HelmRelease":
		return k.patchFirstGVR(ctx, fluxHelmReleaseGVRs, ns, name, fluxReconcilePatch())
	case "Application":
		return k.patchFirstGVR(ctx, argoAppGVRs, ns, name, argoSyncPatch())
	default:
		return fmt.Errorf("unsupported gitops kind %q", kind)
	}
}

func (k *KubernetesProvider) patchFirstGVR(ctx context.Context, gvrs []schema.GroupVersionResource, ns, name string, patch []byte) error {
	var lastErr error
	for _, gvr := range gvrs {
		_, err := k.dynamic.Resource(gvr).Namespace(ns).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
		if err == nil {
			return nil
		}
		lastErr = err
		if !isMissingCRD(err) {
			return err
		}
	}
	return lastErr
}

func fluxReconcilePatch() []byte {
	patch, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{
				"reconcile.fluxcd.io/requestedAt": time.Now().UTC().Format(time.RFC3339Nano),
			},
		},
	})
	return patch
}

func argoSyncPatch() []byte {
	patch, _ := json.Marshal(map[string]any{
		"operation": map[string]any{
			"sync": map[string]any{},
		},
	})
	return patch
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
	events, err := k.listEvents(ctx, ns, "type=Warning")
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

// CreateRole/CreateRoleBinding are deliberately namespaced-only — Axiom
// grants write RBAC for Role/RoleBinding but leaves ClusterRole/
// ClusterRoleBinding read-only, since a UI path to create cluster-scoped
// roles is a materially bigger privilege-escalation surface than a
// namespace-scoped one, and every other write feature in this app is
// namespace-scoped already (see deploy/k8s/rbac.yaml).
func (k *KubernetesProvider) CreateRole(ctx context.Context, req CreateRoleRequest) (RBACRoleSummary, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" {
		return RBACRoleSummary{}, fmt.Errorf("namespace and name are required")
	}
	if len(req.Rules) == 0 {
		return RBACRoleSummary{}, fmt.Errorf("at least one rule is required")
	}
	rules := make([]rbacv1.PolicyRule, 0, len(req.Rules))
	for _, r := range req.Rules {
		rules = append(rules, rbacv1.PolicyRule{APIGroups: r.APIGroups, Resources: r.Resources, Verbs: r.Verbs})
	}
	created, err := k.client.RbacV1().Roles(req.Namespace).Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace},
		Rules:      rules,
	}, metav1.CreateOptions{})
	if err != nil {
		return RBACRoleSummary{}, err
	}
	return RBACRoleSummary{Namespace: created.Namespace, Name: created.Name, Rules: len(created.Rules), Age: age(created.CreationTimestamp.Time)}, nil
}

func (k *KubernetesProvider) DeleteRole(ctx context.Context, ns, name string) error {
	return k.client.RbacV1().Roles(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *KubernetesProvider) CreateRoleBinding(ctx context.Context, req CreateRoleBindingRequest) (RBACBindingSummary, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.RoleName) == "" {
		return RBACBindingSummary{}, fmt.Errorf("namespace, name, and roleName are required")
	}
	if len(req.Subjects) == 0 {
		return RBACBindingSummary{}, fmt.Errorf("at least one subject is required")
	}
	subjects := make([]rbacv1.Subject, 0, len(req.Subjects))
	for _, s := range req.Subjects {
		subjNS := s.Namespace
		if subjNS == "" {
			subjNS = req.Namespace
		}
		subjects = append(subjects, rbacv1.Subject{Kind: s.Kind, Name: s.Name, Namespace: subjNS})
	}
	created, err := k.client.RbacV1().RoleBindings(req.Namespace).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: req.RoleName},
		Subjects:   subjects,
	}, metav1.CreateOptions{})
	if err != nil {
		return RBACBindingSummary{}, err
	}
	return RBACBindingSummary{
		Namespace: created.Namespace, Name: created.Name,
		RoleRef:  created.RoleRef.Kind + "/" + created.RoleRef.Name,
		Subjects: len(created.Subjects), Age: age(created.CreationTimestamp.Time),
	}, nil
}

func (k *KubernetesProvider) DeleteRoleBinding(ctx context.Context, ns, name string) error {
	return k.client.RbacV1().RoleBindings(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

// workloadOwnerForPod returns the display kind/name a pod's cost should roll
// up to. ReplicaSet owners have their pod-template-hash suffix stripped
// (Deployments always name their managed ReplicaSets "<name>-<hash>"), so
// pods group by "checkout" rather than the ReplicaSet's generated
// "checkout-7f7fc496b4".
func workloadOwnerForPod(p *corev1.Pod) (kind, name string) {
	for _, o := range p.OwnerReferences {
		if o.Controller == nil || !*o.Controller {
			continue
		}
		if o.Kind == "ReplicaSet" {
			if idx := strings.LastIndex(o.Name, "-"); idx > 0 {
				return "Deployment", o.Name[:idx]
			}
		}
		return o.Kind, o.Name
	}
	return "Pod", p.Name
}

func (k *KubernetesProvider) EstimateCost(ctx context.Context) (CostEstimate, error) {
	pods, err := k.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return CostEstimate{}, err
	}
	type acc struct{ cpu, mem float64 }
	type workloadAcc struct {
		acc
		namespace, kind string
	}
	byNS := map[string]*acc{}
	byWorkload := map[string]*workloadAcc{}
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
			continue
		}
		a := byNS[p.Namespace]
		if a == nil {
			a = &acc{}
			byNS[p.Namespace] = a
		}
		kind, name := workloadOwnerForPod(&p)
		wKey := p.Namespace + "/" + name
		w := byWorkload[wKey]
		if w == nil {
			w = &workloadAcc{namespace: p.Namespace, kind: kind}
			byWorkload[wKey] = w
		}
		for _, c := range p.Spec.Containers {
			if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				cpu := float64(q.MilliValue()) / 1000.0
				a.cpu += cpu
				w.cpu += cpu
			}
			if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				mem := float64(q.Value()) / (1024 * 1024 * 1024)
				a.mem += mem
				w.mem += mem
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
	wKeys := make([]string, 0, len(byWorkload))
	for k := range byWorkload {
		wKeys = append(wKeys, k)
	}
	sort.Strings(wKeys)
	workloadCosts := make([]WorkloadCost, 0, len(wKeys))
	for _, wKey := range wKeys {
		w := byWorkload[wKey]
		usd := w.cpu*costCPURatePerCore + w.mem*costMemoryRatePerGi
		workloadCosts = append(workloadCosts, WorkloadCost{
			Namespace: w.namespace, Workload: strings.TrimPrefix(wKey, w.namespace+"/"), Kind: w.kind,
			CPUCores: round1(w.cpu), MemoryGi: round1(w.mem), MonthlyUSD: round1(usd),
		})
	}
	return CostEstimate{
		TotalMonthlyUSD: round1(total),
		CPURatePerCore:  costCPURatePerCore,
		MemoryRatePerGi: costMemoryRatePerGi,
		Namespaces:      nsCosts,
		Workloads:       workloadCosts,
	}, nil
}

func (k *KubernetesProvider) ValidateManifest(ctx context.Context, manifest string) (ValidateResult, error) {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return ValidateResult{Valid: false, Errors: []string{"empty manifest"}}, nil
	}
	if hasMultipleDocuments(manifest) {
		return ValidateResult{Valid: false, Errors: []string{errMultiDocument}}, nil
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
		return VMConsoleInfo{}, notFoundf("vm %s/%s not found", ns, name)
	}
	return VMConsoleInfo{
		Namespace: found.Namespace, Name: found.Name, Ready: found.Ready,
		VNCPath: fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/vnc", found.Namespace, found.Name),
		Virtctl: fmt.Sprintf("virtctl vnc %s -n %s", found.Name, found.Namespace),
		WsPath:  fmt.Sprintf("/api/v1/vms/%s/%s/vnc", found.Namespace, found.Name),
	}, nil
}

func (k *KubernetesProvider) ListConfigMaps(ctx context.Context, ns string) ([]ConfigMapSummary, error) {
	list, err := k.client.CoreV1().ConfigMaps(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]ConfigMapSummary, 0, len(list.Items))
	for _, c := range list.Items {
		if c.Namespace == "kube-system" || c.Namespace == "kube-public" || c.Namespace == "kube-node-lease" {
			continue
		}
		keys := make([]string, 0, len(c.Data))
		for key := range c.Data {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out = append(out, ConfigMapSummary{
			Namespace: c.Namespace, Name: c.Name,
			Keys: keys, KeyCount: len(keys), Age: age(c.CreationTimestamp.Time),
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

func (k *KubernetesProvider) GetConfigMap(ctx context.Context, ns, name string) (ConfigMapDetail, error) {
	c, err := k.client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ConfigMapDetail{}, err
	}
	return ConfigMapDetail{
		Namespace: c.Namespace, Name: c.Name, Data: c.Data, Labels: c.Labels,
		Age: age(c.CreationTimestamp.Time),
	}, nil
}

func (k *KubernetesProvider) CreateConfigMap(ctx context.Context, req CreateConfigMapRequest) (ConfigMapDetail, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" {
		return ConfigMapDetail{}, fmt.Errorf("namespace and name are required")
	}
	if len(req.Data) == 0 {
		return ConfigMapDetail{}, fmt.Errorf("at least one data entry is required")
	}
	created, err := k.client.CoreV1().ConfigMaps(req.Namespace).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, Labels: req.Labels},
		Data:       req.Data,
	}, metav1.CreateOptions{})
	if err != nil {
		return ConfigMapDetail{}, err
	}
	return k.GetConfigMap(ctx, created.Namespace, created.Name)
}

func (k *KubernetesProvider) DeleteConfigMap(ctx context.Context, ns, name string) error {
	return k.client.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *KubernetesProvider) ListResourceQuotas(ctx context.Context, ns string) ([]ResourceQuotaSummary, error) {
	list, err := k.client.CoreV1().ResourceQuotas(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]ResourceQuotaSummary, 0, len(list.Items))
	for _, q := range list.Items {
		hard := map[string]string{}
		for k, v := range q.Status.Hard {
			hard[string(k)] = v.String()
		}
		used := map[string]string{}
		for k, v := range q.Status.Used {
			used[string(k)] = v.String()
		}
		out = append(out, ResourceQuotaSummary{
			Namespace: q.Namespace, Name: q.Name, Hard: hard, Used: used,
			Age: age(q.CreationTimestamp.Time),
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

func (k *KubernetesProvider) ListLimitRanges(ctx context.Context, ns string) ([]LimitRangeSummary, error) {
	list, err := k.client.CoreV1().LimitRanges(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	fmtRes := func(rl corev1.ResourceList) string {
		cpu, hasCPU := rl[corev1.ResourceCPU]
		mem, hasMem := rl[corev1.ResourceMemory]
		var parts []string
		if hasCPU {
			parts = append(parts, "cpu="+cpu.String())
		}
		if hasMem {
			parts = append(parts, "mem="+mem.String())
		}
		return strings.Join(parts, " ")
	}
	out := make([]LimitRangeSummary, 0, len(list.Items))
	for _, lr := range list.Items {
		items := make([]LimitRangeItem, 0, len(lr.Spec.Limits))
		for _, l := range lr.Spec.Limits {
			items = append(items, LimitRangeItem{
				Type:           string(l.Type),
				Max:            fmtRes(l.Max),
				Min:            fmtRes(l.Min),
				Default:        fmtRes(l.Default),
				DefaultRequest: fmtRes(l.DefaultRequest),
			})
		}
		out = append(out, LimitRangeSummary{
			Namespace: lr.Namespace, Name: lr.Name, Limits: items,
			Age: age(lr.CreationTimestamp.Time),
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

func (k *KubernetesProvider) ListIngresses(ctx context.Context, ns string) ([]IngressSummary, error) {
	list, err := k.client.NetworkingV1().Ingresses(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]IngressSummary, 0, len(list.Items))
	for _, ing := range list.Items {
		hosts := make([]string, 0, len(ing.Spec.Rules))
		for _, r := range ing.Spec.Rules {
			if r.Host != "" {
				hosts = append(hosts, r.Host)
			}
		}
		class := ""
		if ing.Spec.IngressClassName != nil {
			class = *ing.Spec.IngressClassName
		}
		address := ""
		if len(ing.Status.LoadBalancer.Ingress) > 0 {
			lb := ing.Status.LoadBalancer.Ingress[0]
			if lb.IP != "" {
				address = lb.IP
			} else {
				address = lb.Hostname
			}
		}
		out = append(out, IngressSummary{
			Namespace: ing.Namespace, Name: ing.Name, Class: class,
			Hosts: hosts, Address: address, TLS: len(ing.Spec.TLS) > 0,
			Age: age(ing.CreationTimestamp.Time),
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

// ListGateways lists Gateway API objects (GatewayClass, Gateway, HTTPRoute)
// via the dynamic client, since gateway.networking.k8s.io isn't in
// client-go's typed clientset. Tolerates the CRDs being absent, same as
// ListGitOps.
func (k *KubernetesProvider) ListGateways(ctx context.Context, ns string) ([]GatewaySummary, error) {
	if k.dynamic == nil {
		return []GatewaySummary{}, nil
	}
	var out []GatewaySummary
	out = append(out, k.listGatewayGVR(ctx, gatewayClassGVRs, "GatewayClass", "", gatewayClassStatus)...)
	out = append(out, k.listGatewayGVR(ctx, gatewayGVRs, "Gateway", ns, gatewayStatus)...)
	out = append(out, k.listGatewayGVR(ctx, httpRouteGVRs, "HTTPRoute", ns, httpRouteStatus)...)
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
		out = []GatewaySummary{}
	}
	return out, nil
}

type gatewayStatusFn func(unstructured.Unstructured) (class, controller, status string, hosts, addresses []string)

func (k *KubernetesProvider) listGatewayGVR(ctx context.Context, gvrs []schema.GroupVersionResource, kind, ns string, statusFn gatewayStatusFn) []GatewaySummary {
	for _, gvr := range gvrs {
		var list *unstructured.UnstructuredList
		var err error
		if ns != "" && ns != "all" {
			list, err = k.dynamic.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
		} else {
			list, err = k.dynamic.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		}
		if err != nil {
			if isMissingCRD(err) {
				continue
			}
			continue
		}
		out := make([]GatewaySummary, 0, len(list.Items))
		for _, item := range list.Items {
			class, controller, status, hosts, addresses := statusFn(item)
			out = append(out, GatewaySummary{
				Kind: kind, Namespace: item.GetNamespace(), Name: item.GetName(),
				Class: class, Controller: controller, Provider: gatewayProvider(controller, class),
				Hosts: hosts, Addresses: addresses, Status: status, Age: age(item.GetCreationTimestamp().Time),
			})
		}
		return out
	}
	return nil
}

func gatewayClassStatus(item unstructured.Unstructured) (string, string, string, []string, []string) {
	controller, _, _ := unstructured.NestedString(item.Object, "spec", "controllerName")
	conditions, _, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	return "", controller, conditionStatus(conditions, "Accepted"), nil, nil
}

func gatewayStatus(item unstructured.Unstructured) (string, string, string, []string, []string) {
	className, _, _ := unstructured.NestedString(item.Object, "spec", "gatewayClassName")
	listeners, _, _ := unstructured.NestedSlice(item.Object, "spec", "listeners")
	hosts := make([]string, 0, len(listeners))
	for _, l := range listeners {
		if lm, ok := l.(map[string]any); ok {
			if h, ok := lm["hostname"].(string); ok && h != "" {
				hosts = append(hosts, h)
			}
		}
	}
	addresses := gatewayAddresses(item)
	conditions, _, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	return className, "", conditionStatus(conditions, "Programmed"), hosts, addresses
}

func httpRouteStatus(item unstructured.Unstructured) (string, string, string, []string, []string) {
	hostnames, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "hostnames")
	parents, _, _ := unstructured.NestedSlice(item.Object, "status", "parents")
	status := "Unknown"
	if len(parents) > 0 {
		if pm, ok := parents[0].(map[string]any); ok {
			if conditions, ok := pm["conditions"].([]any); ok {
				status = conditionStatus(conditions, "Accepted")
			}
		}
	}
	return "", "", status, hostnames, nil
}

func gatewayAddresses(item unstructured.Unstructured) []string {
	raw, _, _ := unstructured.NestedSlice(item.Object, "status", "addresses")
	out := make([]string, 0, len(raw))
	for _, a := range raw {
		am, ok := a.(map[string]any)
		if !ok {
			continue
		}
		v, _ := am["value"].(string)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func conditionStatus(conditions []any, wantType string) string {
	for _, c := range conditions {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := cm["type"].(string); t == wantType {
			if s, _ := cm["status"].(string); s == "True" {
				return wantType
			}
			if reason, ok := cm["reason"].(string); ok && reason != "" {
				return reason
			}
			return "False"
		}
	}
	return "Unknown"
}

func jobStatus(j batchv1.Job) string {
	for _, c := range j.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return "Failed"
		}
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return "Complete"
		}
	}
	if j.Status.Active > 0 {
		return "Running"
	}
	return "Pending"
}

func jobDuration(j batchv1.Job) string {
	if j.Status.StartTime == nil {
		return ""
	}
	end := j.Status.CompletionTime
	if end == nil {
		return age(j.Status.StartTime.Time)
	}
	return end.Sub(j.Status.StartTime.Time).Round(time.Second).String()
}

func jobToSummary(j batchv1.Job) JobSummary {
	owner := ""
	for _, ref := range j.OwnerReferences {
		if ref.Kind == "CronJob" {
			owner = ref.Name
		}
	}
	completions := "1"
	if j.Spec.Completions != nil {
		completions = strconv.Itoa(int(*j.Spec.Completions))
	}
	return JobSummary{
		Namespace:    j.Namespace,
		Name:         j.Name,
		OwnerCronJob: owner,
		Status:       jobStatus(j),
		Completions:  fmt.Sprintf("%d/%s", j.Status.Succeeded, completions),
		Duration:     jobDuration(j),
		Age:          age(j.CreationTimestamp.Time),
	}
}

func (k *KubernetesProvider) ListJobs(ctx context.Context, ns string) ([]JobSummary, error) {
	list, err := k.client.BatchV1().Jobs(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]JobSummary, 0, len(list.Items))
	for _, j := range list.Items {
		out = append(out, jobToSummary(j))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

func (k *KubernetesProvider) DeleteJob(ctx context.Context, ns, name string) error {
	prop := metav1.DeletePropagationForeground
	return k.client.BatchV1().Jobs(ns).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &prop})
}

func (k *KubernetesProvider) ListCronJobs(ctx context.Context, ns string) ([]CronJobSummary, error) {
	list, err := k.client.BatchV1().CronJobs(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]CronJobSummary, 0, len(list.Items))
	for _, c := range list.Items {
		lastSchedule := ""
		if c.Status.LastScheduleTime != nil {
			lastSchedule = age(c.Status.LastScheduleTime.Time) + " ago"
		}
		out = append(out, CronJobSummary{
			Namespace:    c.Namespace,
			Name:         c.Name,
			Schedule:     c.Spec.Schedule,
			Suspend:      c.Spec.Suspend != nil && *c.Spec.Suspend,
			Active:       len(c.Status.Active),
			LastSchedule: lastSchedule,
			Age:          age(c.CreationTimestamp.Time),
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

func (k *KubernetesProvider) TriggerCronJob(ctx context.Context, ns, name string) (JobSummary, error) {
	cj, err := k.client.BatchV1().CronJobs(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return JobSummary{}, err
	}
	labels := map[string]string{}
	for key, v := range cj.Spec.JobTemplate.Labels {
		labels[key] = v
	}
	annotations := map[string]string{"cronjob.kubernetes.io/instantiate": "manual"}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name + "-manual-",
			Namespace:    ns,
			Labels:       labels,
			Annotations:  annotations,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1",
				Kind:       "CronJob",
				Name:       cj.Name,
				UID:        cj.UID,
			}},
		},
		Spec: cj.Spec.JobTemplate.Spec,
	}
	created, err := k.client.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return JobSummary{}, err
	}
	return jobToSummary(*created), nil
}

func (k *KubernetesProvider) SetCronJobSuspend(ctx context.Context, ns, name string, suspend bool) error {
	cj, err := k.client.BatchV1().CronJobs(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	cj.Spec.Suspend = &suspend
	_, err = k.client.BatchV1().CronJobs(ns).Update(ctx, cj, metav1.UpdateOptions{})
	return err
}

func (k *KubernetesProvider) DeleteCronJob(ctx context.Context, ns, name string) error {
	prop := metav1.DeletePropagationForeground
	return k.client.BatchV1().CronJobs(ns).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &prop})
}

// --- HPA ---

// ListHPAs and CreateHPA live in autoscaling_kubernetes.go.

func (k *KubernetesProvider) DeleteHPA(ctx context.Context, ns, name string) error {
	return k.client.AutoscalingV2().HorizontalPodAutoscalers(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

// --- PodDisruptionBudgets ---

func (k *KubernetesProvider) ListPodDisruptionBudgets(ctx context.Context, ns string) ([]PDBSummary, error) {
	list, err := k.client.PolicyV1().PodDisruptionBudgets(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]PDBSummary, 0, len(list.Items))
	for _, p := range list.Items {
		minAvail, maxUnavail := "", ""
		if p.Spec.MinAvailable != nil {
			minAvail = p.Spec.MinAvailable.String()
		}
		if p.Spec.MaxUnavailable != nil {
			maxUnavail = p.Spec.MaxUnavailable.String()
		}
		out = append(out, PDBSummary{
			Namespace: p.Namespace, Name: p.Name,
			MinAvailable: minAvail, MaxUnavailable: maxUnavail,
			CurrentHealthy: p.Status.CurrentHealthy, DesiredHealthy: p.Status.DesiredHealthy,
			ExpectedPods: p.Status.ExpectedPods,
			Age:          age(p.CreationTimestamp.Time),
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

// --- NetworkPolicies ---

func (k *KubernetesProvider) ListNetworkPolicies(ctx context.Context, ns string) ([]NetworkPolicySummary, error) {
	list, err := k.client.NetworkingV1().NetworkPolicies(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]NetworkPolicySummary, 0, len(list.Items))
	for _, np := range list.Items {
		types := make([]string, 0, len(np.Spec.PolicyTypes))
		for _, t := range np.Spec.PolicyTypes {
			types = append(types, string(t))
		}
		selStr := "(all pods)"
		if len(np.Spec.PodSelector.MatchLabels) > 0 {
			parts := make([]string, 0, len(np.Spec.PodSelector.MatchLabels))
			for key, val := range np.Spec.PodSelector.MatchLabels {
				parts = append(parts, key+"="+val)
			}
			sort.Strings(parts)
			selStr = strings.Join(parts, ",")
		}
		out = append(out, NetworkPolicySummary{
			Namespace: np.Namespace, Name: np.Name,
			PodSelector: selStr, PolicyTypes: types,
			Age: age(np.CreationTimestamp.Time),
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

func (k *KubernetesProvider) DeleteNetworkPolicy(ctx context.Context, ns, name string) error {
	return k.client.NetworkingV1().NetworkPolicies(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

// --- Generic apply ---

// ApplyManifest mirrors ValidateManifest's parse/discover logic, but performs
// a real server-side-apply Patch instead of a dry-run Create.
func (k *KubernetesProvider) ApplyManifest(ctx context.Context, manifest string) (ApplyResult, error) {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return ApplyResult{Valid: false, Errors: []string{"empty manifest"}}, nil
	}
	if hasMultipleDocuments(manifest) {
		return ApplyResult{Valid: false, Errors: []string{errMultiDocument}}, nil
	}
	var obj unstructured.Unstructured
	if err := json.Unmarshal([]byte(manifest), &obj.Object); err != nil {
		if err := yaml.Unmarshal([]byte(manifest), &obj.Object); err != nil {
			return ApplyResult{Valid: false, Errors: []string{err.Error()}}, nil
		}
	}
	gvk := obj.GroupVersionKind()
	if gvk.Kind == "" || gvk.Version == "" {
		return ApplyResult{Valid: false, Errors: []string{"kind and apiVersion are required"}}, nil
	}
	// see DiffManifest: managedFields copied from a live object must not be sent in an apply
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	name := obj.GetName()
	ns := obj.GetNamespace()
	if name == "" {
		return ApplyResult{Valid: false, Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String(), Errors: []string{"metadata.name is required"}}, nil
	}
	if k.dynamic == nil || k.client == nil {
		return ApplyResult{Valid: false, Errors: []string{"dynamic client unavailable"}}, nil
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(k.client.Discovery()))
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return ApplyResult{
			Valid: false, Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String(),
			Namespace: ns, Name: name, Errors: []string{err.Error()},
		}, nil
	}
	raw, err := json.Marshal(obj.Object)
	if err != nil {
		return ApplyResult{Valid: false, Errors: []string{err.Error()}}, nil
	}
	force := true
	patchOpts := metav1.PatchOptions{FieldManager: "zyvor-axiom", Force: &force}
	var applyErr error
	if mapping.Scope.Name() == "namespace" {
		if ns == "" {
			ns = "default"
		}
		_, applyErr = k.dynamic.Resource(mapping.Resource).Namespace(ns).Patch(ctx, name, types.ApplyPatchType, raw, patchOpts)
	} else {
		_, applyErr = k.dynamic.Resource(mapping.Resource).Patch(ctx, name, types.ApplyPatchType, raw, patchOpts)
	}
	if applyErr != nil {
		return ApplyResult{
			Valid: false, Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String(),
			Namespace: ns, Name: name, Message: "apply failed", Errors: []string{applyErr.Error()},
		}, nil
	}
	return ApplyResult{
		Valid: true, Kind: gvk.Kind, APIVersion: gvk.GroupVersion().String(),
		Namespace: ns, Name: name, Message: "applied",
	}, nil
}

// --- Per-pod metrics (live rolling window, not historical Prometheus data) ---

type podMetricSample struct {
	at  time.Time
	cpu int64
	mem int64
}

var (
	podMetricsMu   sync.Mutex
	podMetricsHist = map[string][]podMetricSample{}
)

func recordPodMetric(key string, cpu, mem int64) {
	podMetricsMu.Lock()
	defer podMetricsMu.Unlock()
	now := time.Now().UTC()
	hist := podMetricsHist[key]
	if len(hist) > 0 && now.Sub(hist[len(hist)-1].at) < 20*time.Second {
		hist[len(hist)-1] = podMetricSample{at: now, cpu: cpu, mem: mem}
	} else {
		hist = append(hist, podMetricSample{at: now, cpu: cpu, mem: mem})
		if len(hist) > 48 {
			hist = hist[len(hist)-48:]
		}
	}
	podMetricsHist[key] = hist
	if len(podMetricsHist) > 500 {
		cutoff := now.Add(-30 * time.Minute)
		for k, h := range podMetricsHist {
			if len(h) == 0 || h[len(h)-1].at.Before(cutoff) {
				delete(podMetricsHist, k)
			}
		}
	}
}

func podMetricPoints(key string) []PodMetricPoint {
	podMetricsMu.Lock()
	hist := append([]podMetricSample(nil), podMetricsHist[key]...)
	podMetricsMu.Unlock()
	out := make([]PodMetricPoint, 0, len(hist))
	for _, s := range hist {
		out = append(out, PodMetricPoint{At: s.at.Format("15:04:05"), CPU: s.cpu, Memory: s.mem})
	}
	return out
}

type podMetricsAPIResponse struct {
	Containers []struct {
		Name  string `json:"name"`
		Usage struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"usage"`
	} `json:"containers"`
}

func (k *KubernetesProvider) GetPodMetrics(ctx context.Context, ns, name string) (PodMetricSeries, error) {
	data, err := k.client.CoreV1().RESTClient().Get().
		AbsPath(fmt.Sprintf("/apis/metrics.k8s.io/v1beta1/namespaces/%s/pods/%s", ns, name)).
		Do(ctx).Raw()
	if err != nil {
		return PodMetricSeries{}, err
	}
	var resp podMetricsAPIResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return PodMetricSeries{}, err
	}
	var totalCPU, totalMem int64
	containers := make([]ContainerMetric, 0, len(resp.Containers))
	for _, c := range resp.Containers {
		cpu := parseCPUMillis(c.Usage.CPU)
		mem := parseMemoryBytes(c.Usage.Memory)
		containers = append(containers, ContainerMetric{Name: c.Name, CPUMillicores: cpu, MemoryBytes: mem})
		totalCPU += cpu
		totalMem += mem
	}
	key := k.metricsKey(ns + "/" + name)
	recordPodMetric(key, totalCPU, totalMem)
	return PodMetricSeries{
		Namespace: ns, Name: name, CPUMillicores: totalCPU, MemoryBytes: totalMem,
		Containers: containers, Points: podMetricPoints(key), Source: "metrics-server",
	}, nil
}

type nodeMetricsAPIResponse struct {
	Usage struct {
		CPU    string `json:"cpu"`
		Memory string `json:"memory"`
	} `json:"usage"`
}

// metricsKey scopes a pod/node-metrics ring-buffer key to this provider's
// cluster. podMetricsHist (ops_kubernetes.go) is a package-level map shared
// by every KubernetesProvider in the process — in the multi-context
// kubeconfig case (factory_kubernetes.go), that's more than one real
// cluster. Without this, two clusters both happening to have e.g. a
// "default/nginx" Pod would silently share (and corrupt) each other's
// metric history when a user switches between them.
func (k *KubernetesProvider) metricsKey(suffix string) string {
	if k.clusterKey == "" {
		return suffix
	}
	return k.clusterKey + "|" + suffix
}

// GetNodeMetrics reuses the pod-metrics ring buffer (recordPodMetric/podMetricPoints)
// keyed by "node:<name>" — node names never contain a "/", so this can't collide
// with a pod key ("namespace/name").
func (k *KubernetesProvider) GetNodeMetrics(ctx context.Context, name string) (NodeMetricSeries, error) {
	data, err := k.client.CoreV1().RESTClient().Get().
		AbsPath(fmt.Sprintf("/apis/metrics.k8s.io/v1beta1/nodes/%s", name)).
		Do(ctx).Raw()
	if err != nil {
		return NodeMetricSeries{}, err
	}
	var resp nodeMetricsAPIResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return NodeMetricSeries{}, err
	}
	cpu := parseCPUMillis(resp.Usage.CPU)
	mem := parseMemoryBytes(resp.Usage.Memory)
	key := k.metricsKey("node:" + name)
	recordPodMetric(key, cpu, mem)
	podPoints := podMetricPoints(key)
	points := make([]NodeMetricPoint, len(podPoints))
	for i, p := range podPoints {
		points[i] = NodeMetricPoint{At: p.At, CPU: p.CPU, Memory: p.Memory}
	}
	return NodeMetricSeries{Name: name, CPUMillicores: cpu, MemoryBytes: mem, Points: points, Source: "metrics-server"}, nil
}

// --- Generic CRD browser ---

var crdGVR = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

func (k *KubernetesProvider) ListCRDs(ctx context.Context) ([]CRDSummary, error) {
	if k.dynamic == nil {
		return nil, fmt.Errorf("dynamic client unavailable")
	}
	list, err := k.dynamic.Resource(crdGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]CRDSummary, 0, len(list.Items))
	for _, item := range list.Items {
		group, _, _ := unstructured.NestedString(item.Object, "spec", "group")
		kind, _, _ := unstructured.NestedString(item.Object, "spec", "names", "kind")
		plural, _, _ := unstructured.NestedString(item.Object, "spec", "names", "plural")
		scope, _, _ := unstructured.NestedString(item.Object, "spec", "scope")
		versions, _, _ := unstructured.NestedSlice(item.Object, "spec", "versions")
		version := ""
		for _, v := range versions {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			served, _ := vm["served"].(bool)
			vname, _ := vm["name"].(string)
			if served && vname != "" {
				version = vname
				break
			}
		}
		if version == "" && len(versions) > 0 {
			if vm, ok := versions[0].(map[string]any); ok {
				version, _ = vm["name"].(string)
			}
		}
		out = append(out, CRDSummary{Group: group, Version: version, Kind: kind, Plural: plural, Scope: scope, Age: age(item.GetCreationTimestamp().Time)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group == out[j].Group {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Group < out[j].Group
	})
	return out, nil
}

// ListCRDInstances lists instances of an arbitrary CRD. ns == "" means
// cluster-scoped (or "all namespaces" for a namespaced CRD).
func (k *KubernetesProvider) ListCRDInstances(ctx context.Context, group, version, plural, ns string) ([]map[string]any, error) {
	if k.dynamic == nil {
		return nil, fmt.Errorf("dynamic client unavailable")
	}
	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: plural}
	var list *unstructured.UnstructuredList
	var err error
	if ns != "" {
		list, err = k.dynamic.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
	} else {
		list, err = k.dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(list.Items))
	for _, item := range list.Items {
		row := map[string]any{
			"name": item.GetName(),
			"age":  age(item.GetCreationTimestamp().Time),
		}
		if item.GetNamespace() != "" {
			row["namespace"] = item.GetNamespace()
		}
		if labels := item.GetLabels(); len(labels) > 0 {
			row["labels"] = labels
		}
		out = append(out, row)
	}
	return out, nil
}

// GetCRDInstance returns the raw object for a single CRD instance. ns == ""
// means cluster-scoped.
func (k *KubernetesProvider) GetCRDInstance(ctx context.Context, group, version, plural, ns, name string) (map[string]any, error) {
	if k.dynamic == nil {
		return nil, fmt.Errorf("dynamic client unavailable")
	}
	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: plural}
	var obj *unstructured.Unstructured
	var err error
	if ns != "" {
		obj, err = k.dynamic.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	} else {
		obj, err = k.dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, err
	}
	return obj.Object, nil
}

// DeleteCRDInstance makes the generic CRD browser read-write: any instance
// of any CRD the ServiceAccount has delete RBAC for can be removed. This
// is also the one action Tier 1 plugins (see ListPlugins) are allowed to
// invoke.
func (k *KubernetesProvider) DeleteCRDInstance(ctx context.Context, group, version, plural, ns, name string) error {
	if k.dynamic == nil {
		return fmt.Errorf("dynamic client unavailable")
	}
	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: plural}
	if ns != "" {
		return k.dynamic.Resource(gvr).Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{})
	}
	return k.dynamic.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
}

// --- Plugin discovery (Tier 1: declarative CRD-view plugins) ---
//
// An AxiomPlugin is pure data — it tells the frontend how to render an
// existing CRD (which columns, which of a small whitelisted action set)
// using Axiom's own already-trusted rendering code. It never carries or
// executes third-party code, and it cannot grant itself RBAC beyond what
// its target CRD's group already has in deploy/k8s/rbac.yaml. See
// docs/ARCHITECTURE.md's plugin architecture section.
var axiomPluginGVR = schema.GroupVersionResource{Group: "axiom.zyvor.dev", Version: "v1alpha1", Resource: "axiomplugins"}

func (k *KubernetesProvider) ListPlugins(ctx context.Context) ([]PluginManifest, error) {
	if k.dynamic == nil {
		return []PluginManifest{}, nil
	}
	list, err := k.dynamic.Resource(axiomPluginGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isMissingCRD(err) {
			return []PluginManifest{}, nil
		}
		return nil, err
	}
	out := make([]PluginManifest, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, pluginManifestFromUnstructured(item))
	}
	return out, nil
}

func pluginManifestFromUnstructured(item unstructured.Unstructured) PluginManifest {
	displayName, _, _ := unstructured.NestedString(item.Object, "spec", "displayName")
	icon, _, _ := unstructured.NestedString(item.Object, "spec", "icon")
	targetGroup, _, _ := unstructured.NestedString(item.Object, "spec", "target", "group")
	targetVersion, _, _ := unstructured.NestedString(item.Object, "spec", "target", "version")
	targetKind, _, _ := unstructured.NestedString(item.Object, "spec", "target", "kind")
	targetPlural, _, _ := unstructured.NestedString(item.Object, "spec", "target", "plural")
	targetScope, _, _ := unstructured.NestedString(item.Object, "spec", "target", "scope")

	var columns []PluginColumn
	if rawColumns, _, _ := unstructured.NestedSlice(item.Object, "spec", "columns"); rawColumns != nil {
		for _, c := range rawColumns {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			path, _ := cm["path"].(string)
			label, _ := cm["label"].(string)
			typ, _ := cm["type"].(string)
			columns = append(columns, PluginColumn{Path: path, Label: label, Type: typ})
		}
	}

	var actions []PluginAction
	if rawActions, _, _ := unstructured.NestedSlice(item.Object, "spec", "actions"); rawActions != nil {
		for _, a := range rawActions {
			am, ok := a.(map[string]any)
			if !ok {
				continue
			}
			name, _ := am["name"].(string)
			typ, _ := am["type"].(string)
			actions = append(actions, PluginAction{Name: name, Type: typ})
		}
	}

	return PluginManifest{
		Name:        item.GetName(),
		DisplayName: displayName,
		Icon:        icon,
		Target: PluginTarget{
			Group: targetGroup, Version: targetVersion, Kind: targetKind,
			Plural: targetPlural, Scope: targetScope,
		},
		Columns: columns,
		Actions: actions,
	}
}

func admissionWebhookRules(rules []admissionregistrationv1.RuleWithOperations) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		groups := strings.Join(r.APIGroups, ",")
		if groups == "" {
			groups = "*"
		}
		versions := strings.Join(r.APIVersions, ",")
		resources := strings.Join(r.Resources, ",")
		out = append(out, fmt.Sprintf("%s/%s/%s", groups, versions, resources))
	}
	return out
}

func admissionFailurePolicy(p *admissionregistrationv1.FailurePolicyType) string {
	if p == nil {
		return "Fail"
	}
	return string(*p)
}

// ListAdmissionWebhooks lists every ValidatingWebhookConfiguration and
// MutatingWebhookConfiguration, flattened to one row per webhook entry
// (a single configuration can register multiple webhooks).
func (k *KubernetesProvider) ListAdmissionWebhooks(ctx context.Context) ([]AdmissionWebhookSummary, error) {
	out := []AdmissionWebhookSummary{}
	validating, err := k.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, cfg := range validating.Items {
		age := age(cfg.CreationTimestamp.Time)
		for _, wh := range cfg.Webhooks {
			out = append(out, AdmissionWebhookSummary{
				ConfigName: cfg.Name, WebhookName: wh.Name, Type: "Validating",
				Rules: admissionWebhookRules(wh.Rules), FailurePolicy: admissionFailurePolicy(wh.FailurePolicy), Age: age,
			})
		}
	}
	mutating, err := k.client.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, cfg := range mutating.Items {
		age := age(cfg.CreationTimestamp.Time)
		for _, wh := range cfg.Webhooks {
			out = append(out, AdmissionWebhookSummary{
				ConfigName: cfg.Name, WebhookName: wh.Name, Type: "Mutating",
				Rules: admissionWebhookRules(wh.Rules), FailurePolicy: admissionFailurePolicy(wh.FailurePolicy), Age: age,
			})
		}
	}
	return out, nil
}
