package platform

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ClusterHealth runs the demo analyzers. They are real rules over the demo
// provider's own data (not canned output), so a state change — a scaled-to-zero
// service, a deleted workload an HPA still targets — shows up as a finding.
func (d *DemoProvider) ClusterHealth(ctx context.Context, ns string) (HealthReport, error) {
	var findings []HealthFinding
	all := ns == "" || ns == "all"

	pods, _ := d.ListPods(ctx, ns)
	for _, p := range pods {
		if p.Status == "CrashLoopBackOff" || p.Status == "CrashLoop" {
			sev, title, msg, fix, ok := containerProblem("CrashLoopBackOff", "", "", 1, p.Restarts, p.Image)
			if ok {
				findings = append(findings, newFinding(AnalyzerPodStatus, sev, "Pod", p.Namespace, p.Name, title, msg, fix))
			}
		}
	}

	svcs, _ := d.ListServices(ctx, ns)
	for _, s := range svcs {
		if len(s.Selector) > 0 && len(s.Endpoints) == 0 {
			findings = append(findings, newFinding(AnalyzerServiceEndpoint, SeverityWarning, "Service", s.Namespace, s.Name,
				"Service has no endpoints",
				fmt.Sprintf("Service %s/%s selects pods but none are ready, so traffic to it fails.", s.Namespace, s.Name),
				"Check that pods matching the selector exist and are Ready; scale the backing workload up or fix the selector labels."))
		}
	}

	classes, _ := d.ListStorageClasses(ctx)
	waitFirst := map[string]bool{}
	for _, c := range classes {
		if c.VolumeBindingMode == "WaitForFirstConsumer" {
			waitFirst[c.Name] = true
		}
	}
	vols, _ := d.ListVolumes(ctx, ns)
	for _, v := range vols {
		if v.Status != "Pending" {
			continue
		}
		if waitFirst[v.StorageClass] {
			findings = append(findings, newFinding(AnalyzerPVC, SeverityInfo, "PersistentVolumeClaim", v.Namespace, v.Name,
				"PVC waiting for first consumer",
				fmt.Sprintf("PVC %s/%s uses StorageClass %q (WaitForFirstConsumer) and stays Pending until a pod mounts it.", v.Namespace, v.Name, v.StorageClass),
				"Expected until a pod references the claim. If a pod already does, check that pod's scheduling events."))
			continue
		}
		findings = append(findings, newFinding(AnalyzerPVC, SeverityWarning, "PersistentVolumeClaim", v.Namespace, v.Name,
			"PVC is Pending",
			fmt.Sprintf("PVC %s/%s has not been bound (StorageClass %q).", v.Namespace, v.Name, v.StorageClass),
			"Check that the StorageClass exists and its provisioner is healthy, and that a matching PersistentVolume has enough capacity."))
	}

	hpas, _ := d.ListHPAs(ctx, ns)
	d.mu.RLock()
	for _, h := range hpas {
		target := key(h.Namespace, h.TargetKind, h.TargetName)
		if _, ok := d.workloads[target]; !ok {
			findings = append(findings, newFinding(AnalyzerHPA, SeverityWarning, "HorizontalPodAutoscaler", h.Namespace, h.Name,
				"HPA cannot scale: target not found",
				fmt.Sprintf("The HPA targets %s %s/%s, which does not exist.", h.TargetKind, h.Namespace, h.TargetName),
				"Point scaleTargetRef at an existing workload in the same namespace, or delete the HPA."))
			continue
		}
		if h.MaxReplicas > 0 && h.CurrentReplicas >= h.MaxReplicas {
			findings = append(findings, newFinding(AnalyzerHPA, SeverityWarning, "HorizontalPodAutoscaler", h.Namespace, h.Name,
				"HPA is at its maximum replicas",
				fmt.Sprintf("%s/%s is running %d of a maximum %d replicas and cannot scale out further.", h.Namespace, h.Name, h.CurrentReplicas, h.MaxReplicas),
				"Raise maxReplicas if the load is legitimate, or investigate why utilisation stays high."))
		}
	}
	d.mu.RUnlock()

	if all {
		nodes, _ := d.ListNodes(ctx)
		for _, n := range nodes {
			findings = append(findings, nodeFindings(n)...)
		}
	}

	findings = append(findings, analyzeWeirdFindings(ctx, d, ns)...)
	// Demo finalizer ghost: pretend a stuck Terminating namespace object via a sealed leftover name.
	if all || ns == "observability" || ns == "" {
		findings = append(findings, newFinding(AnalyzerFinalizerGhost, SeverityWarning, "Pod", "observability", "otel-collector-ghost",
			"Finalizer ghost",
			"Demo object observability/otel-collector-ghost has been Terminating with finalizers for >15m.",
			"Inspect finalizers on the object; remove stuck finalizers only after confirming the owning controller is gone."))
	}

	return NewHealthReport(ns, findings, nil, time.Now()), nil
}

// nodeFindings turns a node's conditions into findings. Shared by the demo and
// Kubernetes node-pressure analyzers.
func nodeFindings(n Node) []HealthFinding {
	var out []HealthFinding
	for _, c := range n.Conditions {
		pressure := strings.HasSuffix(c.Type, "Pressure") || c.Type == "NetworkUnavailable"
		switch {
		case c.Type == "Ready" && c.Status != "True":
			out = append(out, newFinding(AnalyzerNodePressure, SeverityCritical, "Node", "", n.Name,
				"Node is not Ready",
				fmt.Sprintf("Node %s reports Ready=%s. Pods on it may be evicted or unreachable.", n.Name, c.Status),
				"Check kubelet and container runtime on the node, and its network connectivity to the control plane; cordon and drain if it will not recover."))
		case pressure && c.Status == "True":
			out = append(out, newFinding(AnalyzerNodePressure, SeverityWarning, "Node", "", n.Name,
				fmt.Sprintf("Node under %s", c.Type),
				fmt.Sprintf("Node %s reports %s=True; the kubelet may evict pods and stops scheduling new ones.", n.Name, c.Type),
				pressureFix(c.Type)))
		}
	}
	return out
}

func pressureFix(condition string) string {
	switch condition {
	case "MemoryPressure":
		return "Find memory-hungry pods on the node (Node detail, Pods) and lower their usage or limits, or add node capacity."
	case "DiskPressure":
		return "Free disk on the node: prune unused images and logs, or enlarge the root/containerd volume."
	case "PIDPressure":
		return "Look for pods forking excessively and set a podPidsLimit; restart the offending workload."
	default:
		return "Check the node's network plugin and its connectivity."
	}
}
