//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// maxGraphPods bounds how many pods a graph lists; the rest are summarised via Truncated.
const maxGraphPods = 30

// ResourceGraph builds the relationship graph around a Deployment, StatefulSet, DaemonSet,
// ReplicaSet, Job, CronJob or Pod: the owner chain down to pods (or up from a pod), the
// Services whose selector matches the pods, the Ingresses routing to those Services, HPAs that
// scale it, and the PVC/ConfigMap/Secret names its pod template references. Only names and
// short statuses are returned — never ConfigMap or Secret contents. Lookups beyond the primary
// object are best-effort: a missing RBAC grant on Ingresses must not hide the rest of the graph.
func (k *KubernetesProvider) ResourceGraph(ctx context.Context, ns, kind, name string) (ResourceGraph, error) {
	g := newGraphBuilder()
	var (
		rootID string
		// anchor is the node Services/Ingress/HPA attach to; spec/labels describe the pods.
		anchor       string
		podSpec      *corev1.PodSpec
		podLabels    map[string]string
		ownerNodes   []types.UID // controllers whose pods are listed
		ownerNodeIDs = map[types.UID]string{}
	)

	switch strings.ToLower(kind) {
	case "deployment":
		d, err := k.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceGraph{}, err
		}
		rootID, _ = g.node("Deployment", ns, d.Name, deploymentGraphStatus(d))
		anchor, podSpec, podLabels = rootID, &d.Spec.Template.Spec, d.Spec.Template.Labels
		rss, _ := k.client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
		if rss != nil {
			owned := ownedReplicaSets(rss.Items, d.UID)
			for _, rs := range owned {
				id, ok := g.node("ReplicaSet", ns, rs.Name, fmt.Sprintf("%d/%d ready", rs.Status.ReadyReplicas, derefInt32(rs.Spec.Replicas)))
				if !ok {
					continue
				}
				g.edge(rootID, id, "owns")
				ownerNodes = append(ownerNodes, rs.UID)
				ownerNodeIDs[rs.UID] = id
			}
		}
	case "statefulset":
		s, err := k.client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceGraph{}, err
		}
		rootID, _ = g.node("StatefulSet", ns, s.Name, fmt.Sprintf("%d/%d ready", s.Status.ReadyReplicas, derefInt32(s.Spec.Replicas)))
		anchor, podSpec, podLabels = rootID, &s.Spec.Template.Spec, s.Spec.Template.Labels
		ownerNodes, ownerNodeIDs[s.UID] = append(ownerNodes, s.UID), rootID
	case "daemonset":
		s, err := k.client.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceGraph{}, err
		}
		rootID, _ = g.node("DaemonSet", ns, s.Name, fmt.Sprintf("%d/%d ready", s.Status.NumberReady, s.Status.DesiredNumberScheduled))
		anchor, podSpec, podLabels = rootID, &s.Spec.Template.Spec, s.Spec.Template.Labels
		ownerNodes, ownerNodeIDs[s.UID] = append(ownerNodes, s.UID), rootID
	case "replicaset":
		rs, err := k.client.AppsV1().ReplicaSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceGraph{}, err
		}
		rootID, _ = g.node("ReplicaSet", ns, rs.Name, fmt.Sprintf("%d/%d ready", rs.Status.ReadyReplicas, derefInt32(rs.Spec.Replicas)))
		anchor, podSpec, podLabels = rootID, &rs.Spec.Template.Spec, rs.Spec.Template.Labels
		ownerNodes, ownerNodeIDs[rs.UID] = append(ownerNodes, rs.UID), rootID
		if ref := controllerRef(rs.OwnerReferences); ref != nil && ref.Kind == "Deployment" {
			if id, ok := g.node("Deployment", ns, ref.Name, ""); ok {
				g.edge(id, rootID, "owns")
				anchor = id
			}
		}
	case "job":
		j, err := k.client.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceGraph{}, err
		}
		rootID, _ = g.node("Job", ns, j.Name, jobGraphStatus(j))
		anchor, podSpec, podLabels = rootID, &j.Spec.Template.Spec, j.Spec.Template.Labels
		ownerNodes, ownerNodeIDs[j.UID] = append(ownerNodes, j.UID), rootID
		if ref := controllerRef(j.OwnerReferences); ref != nil && ref.Kind == "CronJob" {
			if id, ok := g.node("CronJob", ns, ref.Name, ""); ok {
				g.edge(id, rootID, "owns")
			}
		}
	case "cronjob":
		c, err := k.client.BatchV1().CronJobs(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceGraph{}, err
		}
		rootID, _ = g.node("CronJob", ns, c.Name, cronJobGraphStatus(c))
		anchor, podSpec, podLabels = rootID, &c.Spec.JobTemplate.Spec.Template.Spec, c.Spec.JobTemplate.Spec.Template.Labels
		if jobs, _ := k.client.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{}); jobs != nil {
			sort.Slice(jobs.Items, func(i, j int) bool {
				return jobs.Items[i].CreationTimestamp.After(jobs.Items[j].CreationTimestamp.Time)
			})
			shown := 0
			for _, j := range jobs.Items {
				if ref := controllerRef(j.OwnerReferences); ref == nil || ref.UID != c.UID {
					continue
				}
				if shown >= 5 {
					g.truncated = true
					break
				}
				shown++
				if id, ok := g.node("Job", ns, j.Name, jobGraphStatus(&j)); ok {
					g.edge(rootID, id, "owns")
					ownerNodes = append(ownerNodes, j.UID)
					ownerNodeIDs[j.UID] = id
				}
			}
		}
	case "pod":
		p, err := k.client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceGraph{}, err
		}
		rootID, _ = g.node("Pod", ns, p.Name, graphPodStatus(p))
		anchor, podSpec, podLabels = rootID, &p.Spec, p.Labels
		// walk the owner chain upwards (bounded) so the pod shows where it comes from
		child := rootID
		owners := p.OwnerReferences
		for depth := 0; depth < 4; depth++ {
			ref := controllerRef(owners)
			if ref == nil {
				break
			}
			id, ok := g.node(ref.Kind, ns, ref.Name, "")
			if !ok {
				break
			}
			g.edge(id, child, "owns")
			child, anchor = id, id
			owners = k.ownersOf(ctx, ns, ref)
		}
	default:
		return ResourceGraph{}, NewStatusError(400, fmt.Errorf("resource graph is not supported for kind %q", kind))
	}

	// pods owned by the controllers collected above
	if len(ownerNodes) > 0 {
		pods, err := k.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err == nil {
			shown := 0
			for i := range pods.Items {
				p := &pods.Items[i]
				ref := controllerRef(p.OwnerReferences)
				if ref == nil {
					continue
				}
				parent, ok := ownerNodeIDs[ref.UID]
				if !ok {
					continue
				}
				if shown >= maxGraphPods {
					g.truncated = true
					break
				}
				shown++
				if id, ok := g.node("Pod", ns, p.Name, graphPodStatus(p)); ok {
					g.edge(parent, id, "owns")
				}
			}
		}
	}

	// Services selecting the pods, and the Ingresses routing to those Services.
	serviceIDs := map[string]string{}
	if svcs, err := k.client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{}); err == nil {
		for _, s := range svcs.Items {
			if !selectorSelectsLabels(s.Spec.Selector, podLabels) {
				continue
			}
			if id, ok := g.node("Service", ns, s.Name, string(s.Spec.Type)); ok {
				serviceIDs[s.Name] = id
				g.edge(id, anchor, "selects")
			}
		}
	}
	if len(serviceIDs) > 0 {
		if ings, err := k.client.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{}); err == nil {
			for _, in := range ings.Items {
				for _, svc := range ingressBackendServices(&in) {
					sid, ok := serviceIDs[svc]
					if !ok {
						continue
					}
					status := strings.Join(ingressHosts(&in), ", ")
					if iid, ok := g.node("Ingress", ns, in.Name, status); ok {
						g.edge(iid, sid, "routes")
					}
				}
			}
		}
	}

	// HPAs targeting the root or its top owner.
	if hpas, err := k.client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{}); err == nil {
		for _, h := range hpas.Items {
			ref := h.Spec.ScaleTargetRef
			targetID := graphNodeID(ref.Kind, ns, ref.Name)
			if !g.seen[targetID] {
				continue
			}
			status := fmt.Sprintf("%d-%d replicas", derefInt32(h.Spec.MinReplicas), h.Spec.MaxReplicas)
			if hid, ok := g.node("HorizontalPodAutoscaler", ns, h.Name, status); ok {
				g.edge(hid, targetID, "scales")
			}
		}
	}

	// ConfigMaps / Secrets / PVCs referenced by the pod template (names only).
	if podSpec != nil {
		cms, secrets, pvcs := podSpecReferences(podSpec)
		for _, n := range cms {
			if id, ok := g.node("ConfigMap", ns, n, ""); ok {
				g.edge(rootID, id, "mounts")
			}
		}
		for _, n := range secrets {
			if id, ok := g.node("Secret", ns, n, ""); ok {
				g.edge(rootID, id, "mounts")
			}
		}
		for _, n := range pvcs {
			status := ""
			if pvc, err := k.client.CoreV1().PersistentVolumeClaims(ns).Get(ctx, n, metav1.GetOptions{}); err == nil {
				status = string(pvc.Status.Phase)
			} else if apierrors.IsNotFound(err) {
				status = "Missing"
			}
			if id, ok := g.node("PersistentVolumeClaim", ns, n, status); ok {
				g.edge(rootID, id, "mounts")
			}
		}
	}

	return g.build(rootID), nil
}

// ownersOf resolves the controller owner references of a ReplicaSet/Job (the only pod owners
// that themselves have owners worth following); other kinds end the chain.
func (k *KubernetesProvider) ownersOf(ctx context.Context, ns string, ref *metav1.OwnerReference) []metav1.OwnerReference {
	switch ref.Kind {
	case "ReplicaSet":
		if rs, err := k.client.AppsV1().ReplicaSets(ns).Get(ctx, ref.Name, metav1.GetOptions{}); err == nil {
			return rs.OwnerReferences
		}
	case "Job":
		if j, err := k.client.BatchV1().Jobs(ns).Get(ctx, ref.Name, metav1.GetOptions{}); err == nil {
			return j.OwnerReferences
		}
	}
	return nil
}

// controllerRef returns the controlling owner reference, or the first one when none is flagged.
func controllerRef(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			return &refs[i]
		}
	}
	if len(refs) > 0 {
		return &refs[0]
	}
	return nil
}

// ownedReplicaSets returns the ReplicaSets controlled by uid: the current one(s) with replicas
// first, then at most a couple of scaled-down revisions so rollout history is visible without
// flooding the graph.
func ownedReplicaSets(all []appsv1.ReplicaSet, uid types.UID) []appsv1.ReplicaSet {
	var active, old []appsv1.ReplicaSet
	for _, rs := range all {
		ref := controllerRef(rs.OwnerReferences)
		if ref == nil || ref.UID != uid {
			continue
		}
		if rs.Status.Replicas > 0 || derefInt32(rs.Spec.Replicas) > 0 {
			active = append(active, rs)
		} else {
			old = append(old, rs)
		}
	}
	byNewest := func(s []appsv1.ReplicaSet) {
		sort.Slice(s, func(i, j int) bool { return s[i].CreationTimestamp.After(s[j].CreationTimestamp.Time) })
	}
	byNewest(active)
	byNewest(old)
	if len(old) > 2 {
		old = old[:2]
	}
	return append(active, old...)
}

// podSpecReferences returns the sorted, distinct ConfigMap, Secret and PVC names a pod spec
// refers to through volumes, envFrom and env valueFrom.
func podSpecReferences(spec *corev1.PodSpec) (configMaps, secrets, pvcs []string) {
	cm, sec, pvc := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, v := range spec.Volumes {
		switch {
		case v.ConfigMap != nil:
			cm[v.ConfigMap.Name] = true
		case v.Secret != nil:
			sec[v.Secret.SecretName] = true
		case v.PersistentVolumeClaim != nil:
			pvc[v.PersistentVolumeClaim.ClaimName] = true
		case v.Projected != nil:
			for _, src := range v.Projected.Sources {
				if src.ConfigMap != nil {
					cm[src.ConfigMap.Name] = true
				}
				if src.Secret != nil {
					sec[src.Secret.Name] = true
				}
			}
		}
	}
	containers := append(append([]corev1.Container{}, spec.InitContainers...), spec.Containers...)
	for _, c := range containers {
		for _, ef := range c.EnvFrom {
			if ef.ConfigMapRef != nil {
				cm[ef.ConfigMapRef.Name] = true
			}
			if ef.SecretRef != nil {
				sec[ef.SecretRef.Name] = true
			}
		}
		for _, e := range c.Env {
			if e.ValueFrom == nil {
				continue
			}
			if e.ValueFrom.ConfigMapKeyRef != nil {
				cm[e.ValueFrom.ConfigMapKeyRef.Name] = true
			}
			if e.ValueFrom.SecretKeyRef != nil {
				sec[e.ValueFrom.SecretKeyRef.Name] = true
			}
		}
	}
	keys := func(m map[string]bool) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			if k != "" {
				out = append(out, k)
			}
		}
		sort.Strings(out)
		return out
	}
	return keys(cm), keys(sec), keys(pvc)
}

func derefInt32(p *int32) int32 {
	if p == nil {
		return 1
	}
	return *p
}

func graphPodStatus(p *corev1.Pod) string {
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
	}
	if p.DeletionTimestamp != nil {
		return "Terminating"
	}
	return string(p.Status.Phase)
}

func deploymentGraphStatus(d *appsv1.Deployment) string {
	return fmt.Sprintf("%d/%d ready", d.Status.ReadyReplicas, derefInt32(d.Spec.Replicas))
}

func jobGraphStatus(j *batchv1.Job) string {
	switch {
	case j.Status.Succeeded > 0:
		return "Complete"
	case j.Status.Failed > 0:
		return "Failed"
	case j.Status.Active > 0:
		return "Running"
	}
	return "Pending"
}

func cronJobGraphStatus(c *batchv1.CronJob) string {
	if c.Spec.Suspend != nil && *c.Spec.Suspend {
		return "Suspended"
	}
	return "Active"
}

// ingressBackendServices lists the Service names an Ingress routes to (rules and default backend).
func ingressBackendServices(in *networkingv1.Ingress) []string {
	seen := map[string]bool{}
	var out []string
	add := func(b *networkingv1.IngressBackend) {
		if b != nil && b.Service != nil && b.Service.Name != "" && !seen[b.Service.Name] {
			seen[b.Service.Name] = true
			out = append(out, b.Service.Name)
		}
	}
	add(in.Spec.DefaultBackend)
	for _, r := range in.Spec.Rules {
		if r.HTTP == nil {
			continue
		}
		for _, p := range r.HTTP.Paths {
			b := p.Backend
			add(&b)
		}
	}
	return out
}

func ingressHosts(in *networkingv1.Ingress) []string {
	var out []string
	for _, r := range in.Spec.Rules {
		if r.Host != "" {
			out = append(out, r.Host)
		}
	}
	return out
}
