//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var veleroRestoreGVR = schema.GroupVersionResource{Group: "velero.io", Version: "v1", Resource: "restores"}

func (k *KubernetesProvider) ListVeleroRestores(ctx context.Context, ns string) ([]VeleroRestore, error) {
	if k.dynamic == nil {
		return []VeleroRestore{}, nil
	}
	list, err := k.dynamic.Resource(veleroRestoreGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isMissingCRD(err) {
			return []VeleroRestore{}, nil
		}
		return nil, err
	}
	out := make([]VeleroRestore, 0, len(list.Items))
	for _, item := range list.Items {
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		if phase == "" {
			phase = "New"
		}
		backup, _, _ := unstructured.NestedString(item.Object, "spec", "backupName")
		included, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "includedNamespaces")
		policy, _, _ := unstructured.NestedString(item.Object, "spec", "existingResourcePolicy")
		errs, _, _ := unstructured.NestedInt64(item.Object, "status", "errors")
		warns, _, _ := unstructured.NestedInt64(item.Object, "status", "warnings")
		out = append(out, VeleroRestore{
			Namespace: item.GetNamespace(), Name: item.GetName(), Backup: backup, Phase: phase,
			Namespaces: strings.Join(included, ","), ExistingResourcePolicy: policy,
			Errors: int(errs), Warnings: int(warns), Age: age(item.GetCreationTimestamp().Time),
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

// planVeleroRestore reads the Backup and the live namespaces, then delegates
// to the shared pure planner.
func (k *KubernetesProvider) planVeleroRestore(ctx context.Context, req VeleroRestoreRequest) (VeleroRestorePreview, VeleroRestoreRequest, error) {
	if k.dynamic == nil {
		return VeleroRestorePreview{}, req, fmt.Errorf("dynamic client unavailable")
	}
	nr, err := NormalizeVeleroRestoreRequest(req)
	if err != nil {
		return VeleroRestorePreview{}, req, err
	}
	nr.Confirm = req.Confirm
	obj, err := k.dynamic.Resource(veleroBackupGVR).Namespace(nr.Namespace).Get(ctx, nr.BackupName, metav1.GetOptions{})
	if err != nil {
		if isMissingCRD(err) {
			return VeleroRestorePreview{}, nr, notFoundf("velero backup %s/%s not found", nr.Namespace, nr.BackupName)
		}
		return VeleroRestorePreview{}, nr, err
	}
	info := veleroBackupInfoFromObject(obj.Object)
	info.Namespace, info.Name = obj.GetNamespace(), obj.GetName()
	nss, err := k.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return VeleroRestorePreview{}, nr, err
	}
	existing := make(map[string]bool, len(nss.Items))
	for _, n := range nss.Items {
		existing[n.Name] = true
	}
	p, err := PlanVeleroRestore(nr, info, existing)
	return p, nr, err
}

func veleroBackupInfoFromObject(o map[string]any) VeleroBackupInfo {
	phase, _, _ := unstructured.NestedString(o, "status", "phase")
	inc, _, _ := unstructured.NestedStringSlice(o, "spec", "includedNamespaces")
	exc, _, _ := unstructured.NestedStringSlice(o, "spec", "excludedNamespaces")
	ir, _, _ := unstructured.NestedStringSlice(o, "spec", "includedResources")
	er, _, _ := unstructured.NestedStringSlice(o, "spec", "excludedResources")
	errs, _, _ := unstructured.NestedInt64(o, "status", "errors")
	warns, _, _ := unstructured.NestedInt64(o, "status", "warnings")
	done, _, _ := unstructured.NestedInt64(o, "status", "progress", "itemsBackedUp")
	total, _, _ := unstructured.NestedInt64(o, "status", "progress", "totalItems")
	return VeleroBackupInfo{
		Phase: phase, IncludedNamespaces: inc, ExcludedNamespaces: exc, IncludedResources: ir, ExcludedResources: er,
		Errors: int(errs), Warnings: int(warns), ItemsBackedUp: int(done), TotalItems: int(total),
	}
}

func (k *KubernetesProvider) PreviewVeleroRestore(ctx context.Context, req VeleroRestoreRequest) (VeleroRestorePreview, error) {
	p, _, err := k.planVeleroRestore(ctx, req)
	return p, err
}

func (k *KubernetesProvider) CreateVeleroRestore(ctx context.Context, req VeleroRestoreRequest) (VeleroRestore, error) {
	p, nr, err := k.planVeleroRestore(ctx, req)
	if err != nil {
		return VeleroRestore{}, err
	}
	if err := CheckVeleroRestoreConfirm(p, nr.Confirm); err != nil {
		return VeleroRestore{}, err
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "velero.io/v1",
		"kind":       "Restore",
		"metadata":   map[string]any{"name": nr.Name, "namespace": nr.Namespace},
		"spec":       BuildVeleroRestoreSpec(nr),
	}}
	created, err := k.dynamic.Resource(veleroRestoreGVR).Namespace(nr.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return VeleroRestore{}, err
	}
	return VeleroRestore{
		Namespace: created.GetNamespace(), Name: created.GetName(), Backup: nr.BackupName, Phase: "New",
		Namespaces: strings.Join(nr.IncludedNamespaces, ","), ExistingResourcePolicy: nr.ExistingResourcePolicy,
		Age: age(created.GetCreationTimestamp().Time),
	}, nil
}
