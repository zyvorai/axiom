//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (k *KubernetesProvider) UpdateConfigMap(ctx context.Context, req CreateConfigMapRequest) (ConfigMapDetail, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" {
		return ConfigMapDetail{}, fmt.Errorf("namespace and name are required")
	}
	cur, err := k.client.CoreV1().ConfigMaps(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		return ConfigMapDetail{}, err
	}
	if req.Data != nil {
		cur.Data = req.Data
	}
	if req.Labels != nil {
		cur.Labels = req.Labels
	}
	updated, err := k.client.CoreV1().ConfigMaps(req.Namespace).Update(ctx, cur, metav1.UpdateOptions{})
	if err != nil {
		return ConfigMapDetail{}, err
	}
	return k.GetConfigMap(ctx, updated.Namespace, updated.Name)
}

func (k *KubernetesProvider) UpdateSecret(ctx context.Context, req CreateSecretRequest) (SecretDetail, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" {
		return SecretDetail{}, fmt.Errorf("namespace and name are required")
	}
	cur, err := k.client.CoreV1().Secrets(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		return SecretDetail{}, err
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
	cur.Data = data
	if req.Type != "" {
		cur.Type = corev1.SecretType(req.Type)
	}
	if req.Labels != nil {
		cur.Labels = req.Labels
	}
	updated, err := k.client.CoreV1().Secrets(req.Namespace).Update(ctx, cur, metav1.UpdateOptions{})
	if err != nil {
		return SecretDetail{}, err
	}
	return k.GetSecret(ctx, updated.Namespace, updated.Name, false)
}

func (k *KubernetesProvider) CreateService(ctx context.Context, req CreateServiceRequest) (Service, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" {
		return Service{}, fmt.Errorf("namespace and name are required")
	}
	port := req.Port
	if port == 0 {
		port = 80
	}
	tp := req.TargetPort
	if tp == 0 {
		tp = port
	}
	typ := corev1.ServiceTypeClusterIP
	switch strings.ToLower(req.Type) {
	case "nodeport":
		typ = corev1.ServiceTypeNodePort
	case "loadbalancer":
		typ = corev1.ServiceTypeLoadBalancer
	}
	created, err := k.client.CoreV1().Services(req.Namespace).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace},
		Spec: corev1.ServiceSpec{
			Type:     typ,
			Selector: req.Selector,
			Ports:    []corev1.ServicePort{{Port: port, TargetPort: intstr.FromInt32(tp), Protocol: corev1.ProtocolTCP}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return Service{}, err
	}
	ports := make([]ServicePort, 0, len(created.Spec.Ports))
	for _, p := range created.Spec.Ports {
		ports = append(ports, ServicePort{Name: p.Name, Port: p.Port, TargetPort: p.TargetPort.String(), Protocol: string(p.Protocol), NodePort: p.NodePort})
	}
	return Service{
		Namespace: created.Namespace, Name: created.Name, Type: string(created.Spec.Type),
		ClusterIP: created.Spec.ClusterIP, Ports: ports, Selector: created.Spec.Selector,
		Endpoints: []string{}, Age: age(created.CreationTimestamp.Time),
	}, nil
}

func (k *KubernetesProvider) DeleteService(ctx context.Context, ns, name string) error {
	return k.client.CoreV1().Services(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *KubernetesProvider) CreateVolume(ctx context.Context, req CreateVolumeRequest) (Volume, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Size) == "" {
		return Volume{}, fmt.Errorf("namespace, name, and size are required")
	}
	qty, err := resource.ParseQuantity(req.Size)
	if err != nil {
		return Volume{}, fmt.Errorf("invalid size: %w", err)
	}
	mode := corev1.ReadWriteOnce
	switch strings.ToLower(req.AccessMode) {
	case "readwritemany", "rwx":
		mode = corev1.ReadWriteMany
	case "readonlymany", "rox":
		mode = corev1.ReadOnlyMany
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{mode},
			Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: qty}},
		},
	}
	if req.StorageClass != "" {
		sc := req.StorageClass
		pvc.Spec.StorageClassName = &sc
	}
	created, err := k.client.CoreV1().PersistentVolumeClaims(req.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return Volume{}, err
	}
	return volumeFromPVC(*created), nil
}

func (k *KubernetesProvider) DeleteVolume(ctx context.Context, ns, name string) error {
	return k.client.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *KubernetesProvider) CreateIngress(ctx context.Context, req CreateIngressRequest) (IngressSummary, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" || req.Host == "" || req.Service == "" {
		return IngressSummary{}, fmt.Errorf("namespace, name, host, and service are required")
	}
	port := req.Port
	if port == 0 {
		port = 80
	}
	pathType := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: req.Host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: req.Service,
									Port: networkingv1.ServiceBackendPort{Number: port},
								},
							},
						}},
					},
				},
			}},
		},
	}
	if req.Class != "" {
		ing.Spec.IngressClassName = &req.Class
	}
	if req.TLSSecret != "" {
		ing.Spec.TLS = []networkingv1.IngressTLS{{Hosts: []string{req.Host}, SecretName: req.TLSSecret}}
	}
	created, err := k.client.NetworkingV1().Ingresses(req.Namespace).Create(ctx, ing, metav1.CreateOptions{})
	if err != nil {
		return IngressSummary{}, err
	}
	return ingressFromK8s(*created), nil
}

func (k *KubernetesProvider) DeleteIngress(ctx context.Context, ns, name string) error {
	return k.client.NetworkingV1().Ingresses(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *KubernetesProvider) CreateResourceQuota(ctx context.Context, req CreateResourceQuotaRequest) (ResourceQuotaSummary, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" || len(req.Hard) == 0 {
		return ResourceQuotaSummary{}, fmt.Errorf("namespace, name, and hard limits are required")
	}
	hard := corev1.ResourceList{}
	for k, v := range req.Hard {
		qty, err := resource.ParseQuantity(v)
		if err != nil {
			return ResourceQuotaSummary{}, fmt.Errorf("invalid hard[%s]: %w", k, err)
		}
		hard[corev1.ResourceName(k)] = qty
	}
	created, err := k.client.CoreV1().ResourceQuotas(req.Namespace).Create(ctx, &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace},
		Spec:       corev1.ResourceQuotaSpec{Hard: hard},
	}, metav1.CreateOptions{})
	if err != nil {
		return ResourceQuotaSummary{}, err
	}
	outHard := map[string]string{}
	for rk, qty := range created.Spec.Hard {
		outHard[string(rk)] = qty.String()
	}
	return ResourceQuotaSummary{Namespace: created.Namespace, Name: created.Name, Hard: outHard, Used: map[string]string{}, Age: age(created.CreationTimestamp.Time)}, nil
}

func (k *KubernetesProvider) DeleteResourceQuota(ctx context.Context, ns, name string) error {
	return k.client.CoreV1().ResourceQuotas(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *KubernetesProvider) CreateVirtualMachine(ctx context.Context, req CreateVMRequest) (VirtualMachine, error) {
	if k.dynamic == nil {
		return VirtualMachine{}, fmt.Errorf("dynamic client unavailable")
	}
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" {
		return VirtualMachine{}, fmt.Errorf("namespace and name are required")
	}
	cpu, mem, image := req.CPU, req.Memory, req.Image
	if cpu == "" {
		cpu = "1"
	}
	if mem == "" {
		mem = "2Gi"
	}
	if image == "" {
		image = "quay.io/containerdisks/ubuntu:22.04"
	}
	runStrategy := "Halted"
	if req.Running {
		runStrategy = "Always"
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata": map[string]any{
			"name":      req.Name,
			"namespace": req.Namespace,
		},
		"spec": map[string]any{
			"runStrategy": runStrategy,
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]any{"kubevirt.io/domain": req.Name},
				},
				"spec": map[string]any{
					"domain": map[string]any{
						"cpu":    map[string]any{"cores": mustInt(cpu)},
						"memory": map[string]any{"guest": mem},
						"devices": map[string]any{
							"disks": []any{
								map[string]any{"name": "containerdisk", "disk": map[string]any{"bus": "virtio"}},
							},
							"interfaces": []any{
								map[string]any{"name": "default", "masquerade": map[string]any{}},
							},
						},
						"resources": map[string]any{
							"requests": map[string]any{"memory": mem},
						},
					},
					"networks": []any{
						map[string]any{"name": "default", "pod": map[string]any{}},
					},
					"volumes": []any{
						map[string]any{
							"name": "containerdisk",
							"containerDisk": map[string]any{
								"image": image,
							},
						},
					},
				},
			},
		},
	}}
	created, err := k.dynamic.Resource(kubevirtVMGVR).Namespace(req.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return VirtualMachine{}, err
	}
	return unstructuredToVM(*created, ""), nil
}

func (k *KubernetesProvider) DeleteVirtualMachine(ctx context.Context, ns, name string) error {
	if k.dynamic == nil {
		return fmt.Errorf("dynamic client unavailable")
	}
	return k.dynamic.Resource(kubevirtVMGVR).Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *KubernetesProvider) CreateVeleroBackup(ctx context.Context, req CreateVeleroBackupRequest) (VeleroBackup, error) {
	if k.dynamic == nil {
		return VeleroBackup{}, fmt.Errorf("dynamic client unavailable")
	}
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.Name) == "" {
		return VeleroBackup{}, fmt.Errorf("namespace and name are required")
	}
	included := make([]any, 0, len(req.IncludedNamespaces))
	for _, ns := range req.IncludedNamespaces {
		included = append(included, ns)
	}
	if len(included) == 0 {
		included = []any{"*"}
	}
	spec := map[string]any{
		"includedNamespaces": included,
	}
	if req.TTL != "" {
		spec["ttl"] = req.TTL
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "velero.io/v1",
		"kind":       "Backup",
		"metadata": map[string]any{
			"name":      req.Name,
			"namespace": req.Namespace,
		},
		"spec": spec,
	}}
	created, err := k.dynamic.Resource(veleroBackupGVR).Namespace(req.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return VeleroBackup{}, err
	}
	nsList := strings.Join(req.IncludedNamespaces, ",")
	if nsList == "" {
		nsList = "*"
	}
	return VeleroBackup{
		Namespace: created.GetNamespace(), Name: created.GetName(), Phase: "New",
		Namespaces: nsList, Age: age(created.GetCreationTimestamp().Time),
	}, nil
}

func (k *KubernetesProvider) UpgradeHelmChart(ctx context.Context, req InstallHelmRequest) (HelmActionResult, error) {
	if strings.TrimSpace(req.Namespace) == "" || strings.TrimSpace(req.ReleaseName) == "" ||
		strings.TrimSpace(req.RepoURL) == "" || strings.TrimSpace(req.ChartName) == "" {
		return HelmActionResult{}, fmt.Errorf("namespace, releaseName, repoUrl, and chartName are required")
	}
	if err := rejectFlagLike(req.Namespace, req.ReleaseName, req.RepoURL, req.ChartName, req.ChartVersion); err != nil {
		return HelmActionResult{}, err
	}
	jobNS, helmSA, helmImage := helmJobConfig()
	jobName := helmJobName("upgrade", req.ReleaseName)
	args := []string{"upgrade", req.ReleaseName, req.ChartName, "--repo", req.RepoURL, "-n", req.Namespace}
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
			Name: jobName, Namespace: jobNS,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "zyvor-axiom",
				"axiom.zyvor.dev/helm-action":  "upgrade",
				"axiom.zyvor.dev/release":      req.ReleaseName,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff, TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever, ServiceAccountName: helmSA, Volumes: volumes,
					Containers: []corev1.Container{{
						Name: "helm", Image: helmImage,
						Command: append([]string{"helm"}, args...), VolumeMounts: mounts,
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
		OK: true, Action: "upgrade", JobName: jobNS + "/" + jobName,
		Message: fmt.Sprintf("Helm upgrade started for %s/%s", req.Namespace, req.ReleaseName),
	}, nil
}

func mustInt(s string) int64 {
	var n int64
	fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	if n <= 0 {
		return 1
	}
	return n
}

func volumeFromPVC(p corev1.PersistentVolumeClaim) Volume {
	cap := ""
	if q, ok := p.Status.Capacity[corev1.ResourceStorage]; ok {
		cap = q.String()
	} else if q, ok := p.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		cap = q.String()
	}
	sc := ""
	if p.Spec.StorageClassName != nil {
		sc = *p.Spec.StorageClassName
	}
	modes := make([]string, 0, len(p.Spec.AccessModes))
	for _, m := range p.Spec.AccessModes {
		modes = append(modes, string(m))
	}
	return Volume{
		Namespace: p.Namespace, Name: p.Name, Status: string(p.Status.Phase),
		Capacity: cap, StorageClass: sc, AccessModes: strings.Join(modes, ","),
		VolumeName: p.Spec.VolumeName, Age: age(p.CreationTimestamp.Time),
	}
}

func ingressFromK8s(ing networkingv1.Ingress) IngressSummary {
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
	addr := ""
	if len(ing.Status.LoadBalancer.Ingress) > 0 {
		lb := ing.Status.LoadBalancer.Ingress[0]
		addr = lb.IP
		if addr == "" {
			addr = lb.Hostname
		}
	}
	return IngressSummary{
		Namespace: ing.Namespace, Name: ing.Name, Class: class, Hosts: hosts,
		Address: addr, TLS: len(ing.Spec.TLS) > 0, Age: age(ing.CreationTimestamp.Time),
	}
}
