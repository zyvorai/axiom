//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ClusterHealth runs every analyzer against the cluster (or one namespace).
// An analyzer that fails (typically RBAC) is reported in HealthReport.Errors
// instead of failing the whole check, so one denied list never hides the rest.
func (k *KubernetesProvider) ClusterHealth(ctx context.Context, ns string) (HealthReport, error) {
	all := ns == "" || ns == "all"
	type analyzer struct {
		name string
		run  func(context.Context, string) ([]HealthFinding, error)
	}
	analyzers := []analyzer{
		{AnalyzerPodStatus, k.analyzePods},
		{AnalyzerServiceEndpoint, k.analyzeServiceEndpoints},
		{AnalyzerPVC, k.analyzePVCs},
		{AnalyzerIngress, k.analyzeIngresses},
		{AnalyzerHPA, k.analyzeHPAs},
		{AnalyzerFinalizerGhost, k.analyzeFinalizerGhosts},
	}
	if all {
		// Nodes are cluster-scoped; a namespaced check does not report them.
		analyzers = append(analyzers, analyzer{AnalyzerNodePressure, func(ctx context.Context, _ string) ([]HealthFinding, error) {
			return k.analyzeNodes(ctx)
		}})
	}
	var findings []HealthFinding
	var errs []string
	for _, a := range analyzers {
		fs, err := a.run(ctx, ns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", a.name, err))
			continue
		}
		findings = append(findings, fs...)
	}
	findings = append(findings, analyzeWeirdFindings(ctx, k, ns)...)
	if len(errs) == len(analyzers) {
		return HealthReport{}, fmt.Errorf("health check failed: %s", strings.Join(errs, "; "))
	}
	return NewHealthReport(ns, findings, errs, time.Now()), nil
}

func (k *KubernetesProvider) analyzePods(ctx context.Context, ns string) ([]HealthFinding, error) {
	pods, err := k.client.CoreV1().Pods(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var out []HealthFinding
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodSucceeded {
			continue
		}
		images := map[string]string{}
		for _, c := range p.Spec.Containers {
			images[c.Name] = c.Image
		}
		for _, c := range p.Spec.InitContainers {
			images[c.Name] = c.Image
		}
		statuses := append(append([]corev1.ContainerStatus{}, p.Status.InitContainerStatuses...), p.Status.ContainerStatuses...)
		for _, cs := range statuses {
			var waitReason, waitMsg, termReason string
			var exit int32
			if cs.State.Waiting != nil {
				waitReason, waitMsg = cs.State.Waiting.Reason, cs.State.Waiting.Message
			}
			if t := cs.LastTerminationState.Terminated; t != nil {
				termReason, exit = t.Reason, t.ExitCode
			}
			sev, title, msg, fix, ok := containerProblem(waitReason, waitMsg, termReason, exit, cs.RestartCount, images[cs.Name])
			if !ok {
				continue
			}
			f := newFinding(AnalyzerPodStatus, sev, "Pod", p.Namespace, p.Name, title,
				fmt.Sprintf("Container %q: %s", cs.Name, msg), fix)
			f.ID += "/" + cs.Name
			out = append(out, f)
		}
		if p.Status.Phase == corev1.PodPending {
			for _, c := range p.Status.Conditions {
				if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
					out = append(out, newFinding(AnalyzerPodStatus, SeverityWarning, "Pod", p.Namespace, p.Name,
						"Pod cannot be scheduled",
						firstNonEmpty(c.Message, c.Reason, "no node satisfies the pod's requirements"),
						"Check resource requests against node capacity, node selectors/affinity, taints and tolerations, and unbound PVCs."))
					break
				}
			}
		}
	}
	return out, nil
}

func (k *KubernetesProvider) analyzeServiceEndpoints(ctx context.Context, ns string) ([]HealthFinding, error) {
	svcs, err := k.client.CoreV1().Services(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	eps, err := k.client.CoreV1().Endpoints(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	pods, err := k.client.CoreV1().Pods(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	ready := map[string]int{}
	for _, e := range eps.Items {
		n := 0
		for _, s := range e.Subsets {
			n += len(s.Addresses)
		}
		ready[e.Namespace+"/"+e.Name] = n
	}
	var out []HealthFinding
	for _, s := range svcs.Items {
		// Selector-less services (manually managed Endpoints, ExternalName,
		// the default/kubernetes API service) are not an endpoint-selection bug.
		if s.Spec.Type == corev1.ServiceTypeExternalName || len(s.Spec.Selector) == 0 {
			continue
		}
		if ready[s.Namespace+"/"+s.Name] > 0 {
			continue
		}
		sel := labels.SelectorFromSet(s.Spec.Selector)
		matching, running := 0, 0
		for _, p := range pods.Items {
			if p.Namespace != s.Namespace || !sel.Matches(labels.Set(p.Labels)) {
				continue
			}
			matching++
			if p.Status.Phase == corev1.PodRunning {
				running++
			}
		}
		msg, fix := "", ""
		switch {
		case matching == 0:
			msg = fmt.Sprintf("Service %s/%s selects %s but no pod matches, so it has no endpoints.", s.Namespace, s.Name, labels.Set(s.Spec.Selector).String())
			fix = "Fix the Service selector or the pod labels so they match, or create the workload the Service is meant to front."
		default:
			msg = fmt.Sprintf("Service %s/%s matches %d pod(s) (%d running) but none are Ready, so it has no endpoints.", s.Namespace, s.Name, matching, running)
			fix = "Look at the matching pods' readiness probes and status; a pod that is not Ready is removed from the Service."
		}
		out = append(out, newFinding(AnalyzerServiceEndpoint, SeverityWarning, "Service", s.Namespace, s.Name, "Service has no endpoints", msg, fix))
	}
	return out, nil
}

func (k *KubernetesProvider) analyzePVCs(ctx context.Context, ns string) ([]HealthFinding, error) {
	pvcs, err := k.client.CoreV1().PersistentVolumeClaims(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	waitFirst := map[string]bool{}
	if scs, err := k.client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{}); err == nil {
		for _, sc := range scs.Items {
			if sc.VolumeBindingMode != nil && string(*sc.VolumeBindingMode) == "WaitForFirstConsumer" {
				waitFirst[sc.Name] = true
			}
		}
	}
	var out []HealthFinding
	for _, c := range pvcs.Items {
		if c.Status.Phase != corev1.ClaimPending {
			continue
		}
		class := ""
		if c.Spec.StorageClassName != nil {
			class = *c.Spec.StorageClassName
		}
		if waitFirst[class] {
			out = append(out, newFinding(AnalyzerPVC, SeverityInfo, "PersistentVolumeClaim", c.Namespace, c.Name,
				"PVC waiting for first consumer",
				fmt.Sprintf("PVC %s/%s uses StorageClass %q (WaitForFirstConsumer) and stays Pending until a pod mounts it.", c.Namespace, c.Name, class),
				"Expected until a pod references the claim. If a pod already does, check that pod's scheduling events."))
			continue
		}
		out = append(out, newFinding(AnalyzerPVC, SeverityWarning, "PersistentVolumeClaim", c.Namespace, c.Name,
			"PVC is Pending",
			fmt.Sprintf("PVC %s/%s has not been bound (StorageClass %q).", c.Namespace, c.Name, class),
			"Check that the StorageClass exists and its provisioner is healthy, and that a matching PersistentVolume has enough capacity."))
	}
	return out, nil
}

func (k *KubernetesProvider) analyzeIngresses(ctx context.Context, ns string) ([]HealthFinding, error) {
	ings, err := k.client.NetworkingV1().Ingresses(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	svcs, err := k.client.CoreV1().Services(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	svcByKey := map[string]corev1.Service{}
	for _, s := range svcs.Items {
		svcByKey[s.Namespace+"/"+s.Name] = s
	}
	var out []HealthFinding
	for _, ing := range ings.Items {
		var backends []networkingv1.IngressServiceBackend
		if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
			backends = append(backends, *ing.Spec.DefaultBackend.Service)
		}
		for _, r := range ing.Spec.Rules {
			if r.HTTP == nil {
				continue
			}
			for _, p := range r.HTTP.Paths {
				if p.Backend.Service != nil {
					backends = append(backends, *p.Backend.Service)
				}
			}
		}
		epsReady := map[string]int{}
		if eps, err := k.client.CoreV1().Endpoints(ing.Namespace).List(ctx, metav1.ListOptions{}); err == nil {
			for _, e := range eps.Items {
				n := 0
				for _, s := range e.Subsets {
					n += len(s.Addresses)
				}
				epsReady[e.Name] = n
			}
		}
		for _, b := range backends {
			svc, ok := svcByKey[ing.Namespace+"/"+b.Name]
			if !ok {
				f := newFinding(AnalyzerIngress, SeverityCritical, "Ingress", ing.Namespace, ing.Name,
					"Ingress backend Service is missing",
					fmt.Sprintf("Ingress %s/%s routes to Service %q, which does not exist, so the controller returns 503/404 for that path.", ing.Namespace, ing.Name, b.Name),
					"Create the Service, or update the Ingress rule to reference the correct Service name.")
				f.ID += "/" + b.Name
				out = append(out, f)
				continue
			}
			if !ingressPortExists(svc, b.Port) {
				f := newFinding(AnalyzerIngress, SeverityWarning, "Ingress", ing.Namespace, ing.Name,
					"Ingress backend port does not exist",
					fmt.Sprintf("Ingress %s/%s routes to port %s of Service %q, which the Service does not expose.", ing.Namespace, ing.Name, ingressPortLabel(b.Port), b.Name),
					"Use one of the Service's declared ports (name or number) in the Ingress backend.")
				f.ID += "/" + b.Name
				out = append(out, f)
			}
			if len(svc.Spec.Selector) > 0 && epsReady[b.Name] == 0 {
				f := newFinding(AnalyzerIngress, SeverityCritical, "Ingress", ing.Namespace, ing.Name,
					"Ingress backend has zero ready endpoints",
					fmt.Sprintf("Ingress %s/%s points at Service %q which has no ready endpoints — north-south traffic fails even if the Ingress object looks fine.", ing.Namespace, ing.Name, b.Name),
					"Fix the Service selector / pod readiness; this is a silent outage class.")
				f.ID += "/" + b.Name + "/empty"
				out = append(out, f)
			}
		}
	}
	return out, nil
}

func ingressPortLabel(p networkingv1.ServiceBackendPort) string {
	if p.Name != "" {
		return p.Name
	}
	return fmt.Sprint(p.Number)
}

func ingressPortExists(svc corev1.Service, p networkingv1.ServiceBackendPort) bool {
	if p.Name == "" && p.Number == 0 {
		return true
	}
	for _, sp := range svc.Spec.Ports {
		if (p.Name != "" && sp.Name == p.Name) || (p.Number != 0 && sp.Port == p.Number) {
			return true
		}
	}
	return false
}

func (k *KubernetesProvider) analyzeHPAs(ctx context.Context, ns string) ([]HealthFinding, error) {
	hpas, err := k.client.AutoscalingV2().HorizontalPodAutoscalers(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var out []HealthFinding
	for _, h := range hpas.Items {
		add := func(sev, title, msg, fix string) {
			out = append(out, newFinding(AnalyzerHPA, sev, "HorizontalPodAutoscaler", h.Namespace, h.Name, title, msg, fix))
		}
		// A missing scale target is the most common reason an HPA never acts.
		if k.hpaTargetMissing(ctx, h.Namespace, h.Spec.ScaleTargetRef.Kind, h.Spec.ScaleTargetRef.Name) {
			add(SeverityWarning, "HPA cannot scale: target not found",
				fmt.Sprintf("The HPA targets %s %s/%s, which does not exist.", h.Spec.ScaleTargetRef.Kind, h.Namespace, h.Spec.ScaleTargetRef.Name),
				"Point scaleTargetRef at an existing workload in the same namespace, or delete the HPA.")
			continue
		}
		reported := false
		for _, c := range h.Status.Conditions {
			if c.Status != corev1.ConditionFalse {
				continue
			}
			switch string(c.Type) {
			case "AbleToScale":
				add(SeverityWarning, "HPA is unable to scale",
					fmt.Sprintf("AbleToScale=False (%s): %s", c.Reason, c.Message),
					"Check the target's scale subresource and the HPA controller events (kubectl describe hpa).")
				reported = true
			case "ScalingActive":
				add(SeverityWarning, "HPA cannot compute metrics",
					fmt.Sprintf("ScalingActive=False (%s): %s", c.Reason, c.Message),
					"Make sure metrics-server (or the custom metrics adapter) is healthy and that the target's containers set resource requests for the metric used.")
				reported = true
			}
		}
		if !reported && h.Spec.MaxReplicas > 0 && h.Status.DesiredReplicas >= h.Spec.MaxReplicas && h.Status.CurrentReplicas >= h.Spec.MaxReplicas {
			add(SeverityWarning, "HPA is at its maximum replicas",
				fmt.Sprintf("%s/%s is running %d of a maximum %d replicas and cannot scale out further.", h.Namespace, h.Name, h.Status.CurrentReplicas, h.Spec.MaxReplicas),
				"Raise maxReplicas if the load is legitimate, or investigate why utilisation stays high.")
		}
	}
	return out, nil
}

// hpaTargetMissing reports true only when the API server says the target is
// NotFound; unknown kinds, forbidden reads and transient errors never produce
// a false "missing" finding.
func (k *KubernetesProvider) hpaTargetMissing(ctx context.Context, ns, kind, name string) bool {
	var err error
	switch kind {
	case "Deployment":
		_, err = k.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	case "StatefulSet":
		_, err = k.client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
	case "ReplicaSet":
		_, err = k.client.AppsV1().ReplicaSets(ns).Get(ctx, name, metav1.GetOptions{})
	default:
		return false
	}
	return err != nil && apierrors.IsNotFound(err)
}

func (k *KubernetesProvider) analyzeNodes(ctx context.Context) ([]HealthFinding, error) {
	nodes, err := k.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var out []HealthFinding
	for _, n := range nodes.Items {
		conds := make([]NodeCondition, 0, len(n.Status.Conditions))
		for _, c := range n.Status.Conditions {
			conds = append(conds, NodeCondition{Type: string(c.Type), Status: string(c.Status), Reason: c.Reason, Message: c.Message})
		}
		out = append(out, nodeFindings(Node{Name: n.Name, Conditions: conds})...)
	}
	return out, nil
}

func (k *KubernetesProvider) analyzeFinalizerGhosts(ctx context.Context, ns string) ([]HealthFinding, error) {
	pods, err := k.client.CoreV1().Pods(nsSelector(ns)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var out []HealthFinding
	cutoff := time.Now().Add(-15 * time.Minute)
	for _, p := range pods.Items {
		if p.DeletionTimestamp == nil || p.DeletionTimestamp.IsZero() {
			continue
		}
		if p.DeletionTimestamp.After(cutoff) {
			continue
		}
		if len(p.Finalizers) == 0 {
			continue
		}
		out = append(out, newFinding(AnalyzerFinalizerGhost, SeverityWarning, "Pod", p.Namespace, p.Name,
			"Finalizer ghost",
			fmt.Sprintf("Pod %s/%s has been Terminating since %s with finalizers %v.", p.Namespace, p.Name, p.DeletionTimestamp.Format(time.RFC3339), p.Finalizers),
			"Confirm the owning controller is healthy; only then clear stuck finalizers."))
	}
	return out, nil
}
