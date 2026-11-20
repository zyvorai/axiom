//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigurePrometheus attaches c to the Kubernetes provider of the cluster
// Axiom starts on (in-cluster, or the kubeconfig's current context). Remote
// clusters registered later keep the snapshot source: one Prometheus scrapes
// one cluster. A nil c is a no-op.
func ConfigurePrometheus(provider Provider, c *PrometheusClient) bool {
	if c == nil {
		return false
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil {
		return false
	}
	k.prometheus = c
	return true
}

// PrometheusFromProvider returns the Prometheus client attached to the active
// Kubernetes provider, or nil when unset / demo.
func PrometheusFromProvider(provider Provider) *PrometheusClient {
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil {
		return nil
	}
	return k.prometheus
}

// podMetricsListFetcher reads metrics.k8s.io PodMetrics for one namespace
// ("" = all). A variable so tests can replace the REST call.
var podMetricsListFetcher = func(ctx context.Context, k *KubernetesProvider, namespace string) ([]byte, error) {
	path := "/apis/metrics.k8s.io/v1beta1/pods"
	if namespace != "" {
		path = fmt.Sprintf("/apis/metrics.k8s.io/v1beta1/namespaces/%s/pods", namespace)
	}
	return k.client.CoreV1().RESTClient().Get().AbsPath(path).Do(ctx).Raw()
}

type podMetricsListAPIResponse struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Containers []struct {
			Name  string `json:"name"`
			Usage struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"usage"`
		} `json:"containers"`
	} `json:"items"`
}

func (k *KubernetesProvider) snapshotUsage(ctx context.Context, namespace string) ([]podContainerUsage, error) {
	raw, err := podMetricsListFetcher(ctx, k, namespace)
	if err != nil {
		return nil, fmt.Errorf("metrics.k8s.io is not reachable (is metrics-server installed?)")
	}
	var resp podMetricsListAPIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("metrics.k8s.io returned a malformed response")
	}
	out := []podContainerUsage{}
	for _, it := range resp.Items {
		for _, c := range it.Containers {
			out = append(out, podContainerUsage{
				Namespace: it.Metadata.Namespace, Pod: it.Metadata.Name, Container: c.Name,
				CPUMilli: parseCPUMillis(c.Usage.CPU), MemBytes: parseMemoryBytes(c.Usage.Memory),
				HasCPU: c.Usage.CPU != "", HasMem: c.Usage.Memory != "",
			})
		}
	}
	return out, nil
}

// ListRightsizing compares each Deployment / StatefulSet / DaemonSet
// container's requests (taken from its newest running pod) with observed
// usage — Prometheus p95/p99 history when configured, else a metrics.k8s.io
// snapshot — and recommends new requests. The report changes nothing;
// ApplyRightsizing writes one recommendation on explicit request.
func (k *KubernetesProvider) ListRightsizing(ctx context.Context, namespace, window string) (RightsizingReport, error) {
	window, err := NormalizeRightsizingWindow(window)
	if err != nil {
		return RightsizingReport{}, err
	}
	in, err := k.gatherRightsizingInputs(ctx, namespace)
	if err != nil {
		return RightsizingReport{}, err
	}
	return runRightsizing(ctx, namespace, window, k.rightsizingSources(), in), nil
}

// gatherRightsizingInputs lists the running Deployment / StatefulSet /
// DaemonSet containers of namespace ("" = all) with the requests of each
// workload's newest running pod, and the pod-to-workload resolver.
func (k *KubernetesProvider) gatherRightsizingInputs(ctx context.Context, namespace string) (rightsizingInputs, error) {
	pods, err := k.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return rightsizingInputs{}, err
	}
	type wl struct {
		namespace, kind, name string
		replicas              int
		newest                *corev1.Pod
	}
	workloads := map[string]*wl{}
	podOwner := map[string][2]string{} // ns/pod -> kind, name
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
			continue
		}
		kind, name := workloadOwnerForPod(p)
		if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" {
			continue
		}
		key := p.Namespace + "/" + kind + "/" + name
		w := workloads[key]
		if w == nil {
			w = &wl{namespace: p.Namespace, kind: kind, name: name}
			workloads[key] = w
		}
		w.replicas++
		if w.newest == nil || p.CreationTimestamp.After(w.newest.CreationTimestamp.Time) ||
			(p.CreationTimestamp.Equal(&w.newest.CreationTimestamp) && p.Name < w.newest.Name) {
			w.newest = p
		}
		podOwner[p.Namespace+"/"+p.Name] = [2]string{kind, name}
	}
	keys := make([]string, 0, len(workloads))
	known := map[string]bool{}
	for key := range workloads {
		keys = append(keys, key)
		known[key] = true
	}
	sort.Strings(keys)
	var containers []rightsizingContainer
	for _, key := range keys {
		w := workloads[key]
		for _, c := range w.newest.Spec.Containers {
			rc := rightsizingContainer{Namespace: w.namespace, Kind: w.kind, Workload: w.name, Container: c.Name, Replicas: w.replicas}
			if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				rc.RequestCPUMilli = q.MilliValue()
			}
			if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				rc.RequestMemBytes = q.Value()
			}
			containers = append(containers, rc)
		}
	}
	in := rightsizingInputs{
		Containers: containers,
		Resolve: func(ns, pod string) (string, string, bool) {
			if o, ok := podOwner[ns+"/"+pod]; ok {
				return o[0], o[1], true
			}
			return workloadFromPodName(ns, pod, known)
		},
	}
	return in, nil
}

// rightsizingSources is the usage source order: Prometheus history when
// configured, then a metrics.k8s.io snapshot.
func (k *KubernetesProvider) rightsizingSources() []rightsizingUsageSource {
	var sources []rightsizingUsageSource
	if k.prometheus != nil {
		sources = append(sources, promUsageSource{client: k.prometheus})
	}
	return append(sources, snapshotUsageSource{fetch: k.snapshotUsage})
}
