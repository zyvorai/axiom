package platform

import (
	"context"
	"strings"
)

// PreviewNodeDrain (demo) builds the preview from the demo pods and
// workloads. Demo conventions: StatefulSet pods mount a local-path claim
// (pinned to their node) and ml-namespace pods use emptyDir scratch space.
func (d *DemoProvider) PreviewNodeDrain(ctx context.Context, name string) (DrainPreview, error) {
	if !demoNodeExists(name) {
		return DrainPreview{}, notFoundf("node %s not found", name)
	}
	nodes, err := d.ListNodes(ctx)
	if err != nil {
		return DrainPreview{}, err
	}
	pdbs, _ := d.ListPodDisruptionBudgets(ctx, "")

	d.mu.RLock()
	defer d.mu.RUnlock()
	in := DrainInputs{Node: name, PinnedPVCs: map[string]bool{}}
	for _, n := range nodes {
		ni := DrainNodeInfo{Name: n.Name, Ready: n.Status == "Ready" || n.Status == "SchedulingDisabled",
			Unschedulable: n.Unschedulable, AllocCPUMilli: drainCPUMilli(n.AllocatableCPU), AllocMemBytes: parseMemoryBytes(n.AllocatableMemory)}
		for _, t := range n.Taints {
			if t.Effect == "NoSchedule" || t.Effect == "NoExecute" {
				ni.NoSchedule = true
			}
		}
		in.Nodes = append(in.Nodes, ni)
	}
	kinds := map[string]string{}
	for _, w := range d.workloads {
		kinds[w.Namespace+"/"+w.Name] = w.Kind
	}
	for wk, ps := range d.pods {
		kind := kinds[wk]
		ns, wname, _ := strings.Cut(wk, "/")
		for _, p := range ps {
			dp := DrainPod{Namespace: ns, Name: p.Name, Node: p.Node, Phase: "Running", OwnerKind: kind, OwnerName: wname,
				CPUMilli: 250, MemBytes: 256 << 20, EmptyDir: ns == "ml"}
			if kind == "StatefulSet" {
				claim := "data-" + p.Name
				dp.PVCs = []string{claim}
				if p.Node == name {
					in.PinnedPVCs[ns+"/"+claim] = true
				}
			}
			in.Pods = append(in.Pods, dp)
		}
	}
	for _, b := range pdbs {
		base := strings.TrimSuffix(b.Name, "-pdb")
		m := map[string]bool{}
		for _, p := range in.Pods {
			if p.Namespace == b.Namespace && p.OwnerName == base {
				m[drainPodKey(p)] = true
			}
		}
		in.PDBs = append(in.PDBs, DrainPDB{Namespace: b.Namespace, Name: b.Name, DisruptionsAllowed: b.CurrentHealthy - b.DesiredHealthy, Matches: m})
	}
	return ComputeDrainPreview(in), nil
}
