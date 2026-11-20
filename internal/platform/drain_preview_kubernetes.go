//go:build kubernetes

package platform

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// PreviewNodeDrain gathers read-only lists (nodes, pods, PDBs, PVCs, PVs; the
// same reads the pods, PDB and volume views already make) and hands them to
// ComputeDrainPreview. It changes nothing.
func (k *KubernetesProvider) PreviewNodeDrain(ctx context.Context, name string) (DrainPreview, error) {
	nodes, err := k.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return DrainPreview{}, err
	}
	found := false
	in := DrainInputs{Node: name, PinnedPVCs: map[string]bool{}}
	for _, n := range nodes.Items {
		if n.Name == name {
			found = true
		}
		ni := DrainNodeInfo{Name: n.Name, Unschedulable: n.Spec.Unschedulable,
			AllocCPUMilli: n.Status.Allocatable.Cpu().MilliValue(), AllocMemBytes: n.Status.Allocatable.Memory().Value()}
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				ni.Ready = true
			}
		}
		for _, t := range n.Spec.Taints {
			if t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute {
				ni.NoSchedule = true
			}
		}
		in.Nodes = append(in.Nodes, ni)
	}
	if !found {
		return DrainPreview{}, notFoundf("node %s not found", name)
	}

	pods, err := k.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return DrainPreview{}, err
	}
	podLabels := map[string]map[string]string{}
	for i := range pods.Items {
		p := &pods.Items[i]
		dp := DrainPod{Namespace: p.Namespace, Name: p.Name, Node: p.Spec.NodeName, Phase: string(p.Status.Phase), Mirror: isMirrorPod(p)}
		for _, o := range p.OwnerReferences {
			if o.Controller != nil && *o.Controller {
				dp.OwnerKind, dp.OwnerName = o.Kind, o.Name
			}
		}
		for _, v := range p.Spec.Volumes {
			if v.EmptyDir != nil {
				dp.EmptyDir = true
			}
			if v.PersistentVolumeClaim != nil {
				dp.PVCs = append(dp.PVCs, v.PersistentVolumeClaim.ClaimName)
			}
		}
		for _, c := range p.Spec.Containers {
			dp.CPUMilli += c.Resources.Requests.Cpu().MilliValue()
			dp.MemBytes += c.Resources.Requests.Memory().Value()
		}
		in.Pods = append(in.Pods, dp)
		podLabels[drainPodKey(dp)] = p.Labels
	}

	if list, err := k.client.PolicyV1().PodDisruptionBudgets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{}); err == nil {
		for _, b := range list.Items {
			sel, err := metav1.LabelSelectorAsSelector(b.Spec.Selector)
			if err != nil || b.Spec.Selector == nil {
				continue
			}
			m := map[string]bool{}
			for key, lb := range podLabels {
				if len(key) > len(b.Namespace) && key[:len(b.Namespace)+1] == b.Namespace+"/" && sel.Matches(labels.Set(lb)) {
					m[key] = true
				}
			}
			in.PDBs = append(in.PDBs, DrainPDB{Namespace: b.Namespace, Name: b.Name, DisruptionsAllowed: b.Status.DisruptionsAllowed, Matches: m})
		}
	}

	// PVs whose node affinity names this node pin their consumers here.
	pinnedPV := map[string]bool{}
	if pvs, err := k.client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{}); err == nil {
		for _, pv := range pvs.Items {
			if pvPinnedTo(pv, name) {
				pinnedPV[pv.Name] = true
			}
		}
	}
	if len(pinnedPV) > 0 {
		if pvcs, err := k.client.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(ctx, metav1.ListOptions{}); err == nil {
			for _, c := range pvcs.Items {
				if pinnedPV[c.Spec.VolumeName] {
					in.PinnedPVCs[c.Namespace+"/"+c.Name] = true
				}
			}
		}
	}
	return ComputeDrainPreview(in), nil
}

func pvPinnedTo(pv corev1.PersistentVolume, node string) bool {
	if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return false
	}
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		for _, e := range term.MatchExpressions {
			if e.Key == "kubernetes.io/hostname" && e.Operator == corev1.NodeSelectorOpIn {
				for _, v := range e.Values {
					if v == node {
						return true
					}
				}
			}
		}
	}
	return false
}
