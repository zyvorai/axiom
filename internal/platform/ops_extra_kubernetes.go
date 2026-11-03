//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	externalSecretGVRs = []schema.GroupVersionResource{
		{Group: "external-secrets.io", Version: "v1", Resource: "externalsecrets"},
		{Group: "external-secrets.io", Version: "v1beta1", Resource: "externalsecrets"},
	}
	vmSnapshotGVRs = []schema.GroupVersionResource{
		{Group: "snapshot.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinesnapshots"},
		{Group: "snapshot.kubevirt.io", Version: "v1alpha1", Resource: "virtualmachinesnapshots"},
	}
	vmimGVR = schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachineinstancemigrations"}
)

func (k *KubernetesProvider) ListHelmHistory(ctx context.Context, ns, name string) ([]HelmRelease, error) {
	list, err := k.client.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("owner=helm,name=%s", name),
	})
	if err != nil {
		return nil, err
	}
	out := make([]HelmRelease, 0, len(list.Items))
	for _, s := range list.Items {
		if s.Type != "helm.sh/release.v1" && s.Labels["owner"] != "helm" {
			continue
		}
		out = append(out, helmReleaseFromSecret(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Revision > out[j].Revision })
	return out, nil
}

func (k *KubernetesProvider) HelmUninstall(ctx context.Context, ns, name string) (HelmActionResult, error) {
	return k.runHelmJob(ctx, ns, name, "uninstall", []string{"uninstall", name, "-n", ns, "--wait"})
}

func (k *KubernetesProvider) HelmRollback(ctx context.Context, ns, name string, revision int) (HelmActionResult, error) {
	args := []string{"rollback", name, "-n", ns, "--wait"}
	if revision > 0 {
		args = []string{"rollback", name, fmt.Sprintf("%d", revision), "-n", ns, "--wait"}
	}
	return k.runHelmJob(ctx, ns, name, "rollback", args)
}

func (k *KubernetesProvider) runHelmJob(ctx context.Context, ns, release, action string, args []string) (HelmActionResult, error) {
	if err := rejectFlagLike(ns, release); err != nil {
		return HelmActionResult{}, err
	}
	jobNS, sa, helmImage := helmJobConfig()
	jobName := helmJobName(action, release)
	backoff := int32(0)
	ttl := int32(600)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: jobNS,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "zyvor-axiom",
				"axiom.zyvor.dev/helm-action":  action,
				"axiom.zyvor.dev/release":      release,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: sa,
					Containers: []corev1.Container{{
						Name:    "helm",
						Image:   helmImage,
						Command: append([]string{"helm"}, args...),
					}},
				},
			},
		},
	}
	_, err := k.client.BatchV1().Jobs(jobNS).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return HelmActionResult{}, err
	}
	return HelmActionResult{
		OK: true, Action: action, JobName: jobNS + "/" + jobName,
		Message: fmt.Sprintf("Helm %s started for %s/%s", action, ns, release),
	}, nil
}

// InstallHelmChart launches the same "Job running alpine/helm" mechanism as
// HelmUninstall/HelmRollback, rather than adding a Helm Go SDK dependency.
// Uses `helm install --repo <url>` (fetches directly from the repo's index)
// instead of a separate `helm repo add && helm install` shell pipeline —
// this keeps every argument a literal element of the Job's Command slice,
// with no shell interpolation of user input anywhere, so there's no command
// injection surface regardless of what characters are in the release/chart
// name or repo URL.
func (k *KubernetesProvider) InstallHelmChart(ctx context.Context, req InstallHelmRequest) (HelmActionResult, error) {
	oci := req.OCIRef != ""
	if oci {
		if err := ValidateHelmOCIInstall(req); err != nil {
			return HelmActionResult{}, err
		}
	} else {
		if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.ReleaseName) == "" ||
			strings.TrimSpace(req.RepoURL) == "" || strings.TrimSpace(req.ChartName) == "" {
			return HelmActionResult{}, fmt.Errorf("namespace, releaseName, repoUrl, and chartName are required")
		}

		if err := rejectFlagLike(req.Namespace, req.ReleaseName, req.RepoURL, req.ChartName, req.ChartVersion); err != nil {
			return HelmActionResult{}, err
		}
	}
	jobNS, helmSA, helmImage := helmJobConfig()
	jobName := helmJobName("install", req.ReleaseName)

	// No --wait here (unlike uninstall/rollback): install is already framed
	// to the caller as "started", not a synchronous result, and --wait would
	// require RBAC read on apps/replicasets that Axiom's ServiceAccount
	// doesn't have — confirmed against the real cluster, where a chart's
	// resources were created successfully but the Job still reported
	// failure because `helm install --wait` couldn't poll rollout status.
	args := []string{"install", req.ReleaseName, req.ChartName, "--repo", req.RepoURL, "-n", req.Namespace, "--create-namespace"}
	if oci {
		// helm pulls oci:// references directly; there is no --repo.
		args = []string{"install", req.ReleaseName, req.OCIRef, "-n", req.Namespace, "--create-namespace"}
	}
	if req.ChartVersion != "" {
		args = append(args, "--version", req.ChartVersion)
	}

	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount
	valuesSecret := ""
	if strings.TrimSpace(req.Values) != "" {
		vol, mnt, name, err := k.createHelmValuesSecret(ctx, jobNS, jobName, req.Values)
		if err != nil {
			return HelmActionResult{}, err
		}
		valuesSecret = name
		volumes = append(volumes, vol)
		mounts = append(mounts, mnt)
		args = append(args, "-f", "/values/values.yaml")
	}

	backoff := int32(0)
	ttl := int32(600)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: jobNS,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "zyvor-axiom",
				"axiom.zyvor.dev/helm-action":  "install",
				"axiom.zyvor.dev/release":      req.ReleaseName,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: helmSA,
					Volumes:            volumes,
					Containers: []corev1.Container{{
						Name:         "helm",
						Image:        helmImage,
						Command:      append([]string{"helm"}, args...),
						VolumeMounts: mounts,
					}},
				},
			},
		},
	}
	created, err := k.client.BatchV1().Jobs(jobNS).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return HelmActionResult{}, err
	}
	k.adoptHelmValuesSecret(ctx, jobNS, valuesSecret, created)
	return HelmActionResult{
		OK: true, Action: "install", JobName: jobNS + "/" + jobName,
		Message: fmt.Sprintf("Helm install started for %s/%s", req.Namespace, req.ReleaseName),
	}, nil
}

func sanitizeDNS(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := b.String()
	if len(out) > 40 {
		out = out[:40]
	}
	return strings.Trim(out, "-")
}

func (k *KubernetesProvider) ListExternalSecrets(ctx context.Context, ns string) ([]ExternalSecretSummary, error) {
	if k.dynamic == nil {
		return []ExternalSecretSummary{}, nil
	}
	var list *unstructured.UnstructuredList
	var err error
	for _, gvr := range externalSecretGVRs {
		list, err = k.dynamic.Resource(gvr).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
		if err == nil {
			break
		}
		if !isMissingCRD(err) {
			return nil, err
		}
	}
	if err != nil {
		return []ExternalSecretSummary{}, nil
	}
	out := make([]ExternalSecretSummary, 0, len(list.Items))
	for _, item := range list.Items {
		store, _, _ := unstructured.NestedString(item.Object, "spec", "secretStoreRef", "name")
		refresh, _, _ := unstructured.NestedString(item.Object, "spec", "refreshInterval")
		ready := false
		status := "Unknown"
		conds, _, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
		for _, c := range conds {
			m, _ := c.(map[string]any)
			if fmt.Sprint(m["type"]) == "Ready" {
				status = fmt.Sprint(m["reason"])
				ready = fmt.Sprint(m["status"]) == "True"
				if msg := fmt.Sprint(m["message"]); msg != "" && msg != "<nil>" {
					status = msg
					if len(status) > 80 {
						status = status[:80] + "…"
					}
				}
			}
		}
		out = append(out, ExternalSecretSummary{
			Namespace: item.GetNamespace(), Name: item.GetName(), Store: store,
			Refresh: refresh, Status: status, Ready: ready, Age: age(item.GetCreationTimestamp().Time),
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

func (k *KubernetesProvider) ListVMSnapshots(ctx context.Context, ns string) ([]VMSnapshot, error) {
	if k.dynamic == nil {
		return []VMSnapshot{}, nil
	}
	var list *unstructured.UnstructuredList
	var err error
	for _, gvr := range vmSnapshotGVRs {
		list, err = k.dynamic.Resource(gvr).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{})
		if err == nil {
			break
		}
		if !isMissingCRD(err) {
			return nil, err
		}
	}
	if err != nil {
		return []VMSnapshot{}, nil
	}
	out := make([]VMSnapshot, 0, len(list.Items))
	for _, item := range list.Items {
		vm, _, _ := unstructured.NestedString(item.Object, "spec", "source", "name")
		ready, _, _ := unstructured.NestedBool(item.Object, "status", "readyToUse")
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		if phase == "" {
			if ready {
				phase = "Succeeded"
			} else {
				phase = "InProgress"
			}
		}
		out = append(out, VMSnapshot{
			Namespace: item.GetNamespace(), Name: item.GetName(), VM: vm,
			Ready: ready, Status: phase, Age: age(item.GetCreationTimestamp().Time),
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

func (k *KubernetesProvider) CreateVMSnapshot(ctx context.Context, req CreateVMSnapshotRequest) (VMSnapshot, error) {
	if k.dynamic == nil {
		return VMSnapshot{}, fmt.Errorf("dynamic client unavailable")
	}
	if req.Namespace == "" || req.VM == "" {
		return VMSnapshot{}, fmt.Errorf("namespace and vm are required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("%s-snap-%d", req.VM, time.Now().Unix()%100000)
	}
	var lastErr error
	for _, gvr := range vmSnapshotGVRs {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": gvr.Group + "/" + gvr.Version,
			"kind":       "VirtualMachineSnapshot",
			"metadata": map[string]any{
				"name":      name,
				"namespace": req.Namespace,
			},
			"spec": map[string]any{
				"source": map[string]any{
					"apiGroup": "kubevirt.io",
					"kind":     "VirtualMachine",
					"name":     req.VM,
				},
			},
		}}
		created, err := k.dynamic.Resource(gvr).Namespace(req.Namespace).Create(ctx, obj, metav1.CreateOptions{})
		if err == nil {
			return VMSnapshot{
				Namespace: created.GetNamespace(), Name: created.GetName(), VM: req.VM,
				Status: "InProgress", Age: age(created.GetCreationTimestamp().Time),
			}, nil
		}
		lastErr = err
		if !isMissingCRD(err) {
			return VMSnapshot{}, err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("VirtualMachineSnapshot CRD not available")
	}
	return VMSnapshot{}, lastErr
}

func (k *KubernetesProvider) LiveMigrateVM(ctx context.Context, ns, name string) (HelmActionResult, error) {
	if k.dynamic == nil {
		return HelmActionResult{}, fmt.Errorf("dynamic client unavailable")
	}
	migName := fmt.Sprintf("%s-migrate-%d", name, time.Now().Unix()%100000)
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachineInstanceMigration",
		"metadata": map[string]any{
			"name":      migName,
			"namespace": ns,
		},
		"spec": map[string]any{
			"vmiName": name,
		},
	}}
	_, err := k.dynamic.Resource(vmimGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return HelmActionResult{}, err
	}
	return HelmActionResult{
		OK: true, Action: "migrate", JobName: ns + "/" + migName,
		Message: fmt.Sprintf("Live migration requested for %s/%s", ns, name),
	}, nil
}
