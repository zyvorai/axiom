package platform

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Node drain preview. Everything here is pure: the kube and demo providers
// gather DrainInputs from read-only lists and ComputeDrainPreview decides.
// Nothing in this file talks to a cluster, so the verdict is unit-testable
// with fake lists.

const (
	DrainVerdictOK      = "ok"
	DrainVerdictCaution = "caution"
	DrainVerdictBlocked = "blocked"
)

// DrainPod is one pod scheduled on the node (or, for headroom, anywhere).
type DrainPod struct {
	Namespace string
	Name      string
	Node      string
	Phase     string // Running, Pending, Succeeded, Failed
	OwnerKind string // controller owner kind; "" = bare pod
	OwnerName string
	Mirror    bool
	EmptyDir  bool
	PVCs      []string // claim names mounted
	CPUMilli  int64    // summed container requests
	MemBytes  int64
}

// DrainPDB is a PodDisruptionBudget with the pods it selects pre-resolved
// ("namespace/name"), so selector matching stays in the gatherer.
type DrainPDB struct {
	Namespace          string
	Name               string
	DisruptionsAllowed int32
	Matches            map[string]bool
}

// DrainNodeInfo is a node's schedulability and allocatable capacity.
type DrainNodeInfo struct {
	Name          string
	Ready         bool
	Unschedulable bool
	NoSchedule    bool // carries a NoSchedule/NoExecute taint
	AllocCPUMilli int64
	AllocMemBytes int64
}

// DrainInputs are the read-only lists the preview is computed from.
type DrainInputs struct {
	Node       string
	Nodes      []DrainNodeInfo
	Pods       []DrainPod // every pod in the cluster
	PDBs       []DrainPDB
	PinnedPVCs map[string]bool // "namespace/claim" bound to a PV with node affinity to Node
}

type DrainPodRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Owner     string `json:"owner,omitempty"` // Kind/name
	Reason    string `json:"reason,omitempty"`
}

type DrainEvictGroup struct {
	Namespace string   `json:"namespace"`
	Owner     string   `json:"owner"` // Kind/name, or "(none)"
	Pods      []string `json:"pods"`
}

type DrainPDBBlock struct {
	Namespace string   `json:"namespace"`
	PDB       string   `json:"pdb"`
	Allowed   int32    `json:"disruptionsAllowed"`
	Pods      []string `json:"pods"`
}

type DrainReschedule struct {
	Approximate      bool  `json:"approximate"` // always true: requests only, no affinity / taint / topology
	CandidateNodes   int   `json:"candidateNodes"`
	HeadroomCPUMilli int64 `json:"headroomCpuMilli"`
	HeadroomMemBytes int64 `json:"headroomMemBytes"`
	NeedCPUMilli     int64 `json:"needCpuMilli"`
	NeedMemBytes     int64 `json:"needMemBytes"`
	Fits             bool  `json:"fits"`
}

// DrainPreview is GET /api/v1/nodes/{name}/drain-preview.
type DrainPreview struct {
	Node          string            `json:"node"`
	Verdict       string            `json:"verdict"`
	Reasons       []string          `json:"reasons"` // stable codes for the UI to translate
	Evict         []DrainEvictGroup `json:"evict"`
	EvictCount    int               `json:"evictCount"`
	Ignored       []DrainPodRef     `json:"ignored"`      // DaemonSet, mirror, kube-system, finished
	LocalStorage  []DrainPodRef     `json:"localStorage"` // emptyDir: needs delete-emptydir-data
	Unmanaged     []DrainPodRef     `json:"unmanaged"`    // bare pods, lost for good
	PDBBlocked    []DrainPDBBlock   `json:"pdbBlocked"`
	SingleReplica []DrainPodRef     `json:"singleReplica"`
	PinnedVolumes []DrainPodRef     `json:"pinnedVolumes"`
	Reschedule    DrainReschedule   `json:"reschedule"`
}

func drainPodKey(p DrainPod) string { return p.Namespace + "/" + p.Name }

// ComputeDrainPreview mirrors what KubernetesProvider.Drain does (it skips
// DaemonSet, mirror and kube-system pods and evicts the rest) and reports what
// that would cost. Headroom is a requests-only estimate.
func ComputeDrainPreview(in DrainInputs) DrainPreview {
	out := DrainPreview{Node: in.Node, Verdict: DrainVerdictOK,
		Reasons: []string{}, Evict: []DrainEvictGroup{}, Ignored: []DrainPodRef{}, LocalStorage: []DrainPodRef{},
		Unmanaged: []DrainPodRef{}, PDBBlocked: []DrainPDBBlock{}, SingleReplica: []DrainPodRef{}, PinnedVolumes: []DrainPodRef{}}

	// live pods per owner across the cluster: a proxy for the desired count
	live := map[string]int{}
	for _, p := range in.Pods {
		if p.OwnerKind != "" && p.Phase != "Succeeded" && p.Phase != "Failed" {
			live[p.Namespace+"/"+p.OwnerKind+"/"+p.OwnerName]++
		}
	}

	var evict []DrainPod
	groups := map[string]*DrainEvictGroup{}
	for _, p := range in.Pods {
		if p.Node != in.Node {
			continue
		}
		ref := DrainPodRef{Namespace: p.Namespace, Name: p.Name}
		if p.OwnerKind != "" {
			ref.Owner = p.OwnerKind + "/" + p.OwnerName
		}
		switch {
		case p.OwnerKind == "DaemonSet":
			ref.Reason = "daemonset"
		case p.Mirror:
			ref.Reason = "mirror"
		case p.Namespace == "kube-system":
			ref.Reason = "kube-system"
		case p.Phase == "Succeeded" || p.Phase == "Failed":
			ref.Reason = "finished"
		}
		if ref.Reason != "" {
			out.Ignored = append(out.Ignored, ref)
			continue
		}
		evict = append(evict, p)
		owner := ref.Owner
		if owner == "" {
			owner = "(none)"
			out.Unmanaged = append(out.Unmanaged, ref)
		} else if live[p.Namespace+"/"+ref.Owner] == 1 {
			out.SingleReplica = append(out.SingleReplica, ref)
		}
		if p.EmptyDir {
			out.LocalStorage = append(out.LocalStorage, ref)
		}
		for _, c := range p.PVCs {
			if in.PinnedPVCs[p.Namespace+"/"+c] {
				r := ref
				r.Reason = c
				out.PinnedVolumes = append(out.PinnedVolumes, r)
				break
			}
		}
		gk := p.Namespace + "\x00" + owner
		g := groups[gk]
		if g == nil {
			g = &DrainEvictGroup{Namespace: p.Namespace, Owner: owner}
			groups[gk] = g
		}
		g.Pods = append(g.Pods, p.Name)
	}
	out.EvictCount = len(evict)
	for _, g := range groups {
		sort.Strings(g.Pods)
		out.Evict = append(out.Evict, *g)
	}
	sort.Slice(out.Evict, func(i, j int) bool {
		if out.Evict[i].Namespace != out.Evict[j].Namespace {
			return out.Evict[i].Namespace < out.Evict[j].Namespace
		}
		return out.Evict[i].Owner < out.Evict[j].Owner
	})

	// PDBs: blocked when nothing may be disrupted; tight when the budget's
	// pods on this node outnumber what it allows at once.
	softPDB := false
	for _, b := range in.PDBs {
		var hit []string
		for _, p := range evict {
			if b.Matches[drainPodKey(p)] {
				hit = append(hit, p.Name)
			}
		}
		if len(hit) == 0 {
			continue
		}
		sort.Strings(hit)
		if b.DisruptionsAllowed <= 0 {
			out.PDBBlocked = append(out.PDBBlocked, DrainPDBBlock{Namespace: b.Namespace, PDB: b.Name, Allowed: b.DisruptionsAllowed, Pods: hit})
		} else if int32(len(hit)) > b.DisruptionsAllowed {
			softPDB = true
		}
	}

	out.Reschedule = drainReschedule(in, evict)

	for _, ls := range [][]DrainPodRef{out.Ignored, out.LocalStorage, out.Unmanaged, out.SingleReplica, out.PinnedVolumes} {
		ls := ls
		sort.Slice(ls, func(i, j int) bool { return ls[i].Namespace+"/"+ls[i].Name < ls[j].Namespace+"/"+ls[j].Name })
	}

	add := func(code string) { out.Reasons = append(out.Reasons, code) }
	if len(out.PDBBlocked) > 0 {
		out.Verdict = DrainVerdictBlocked
		add("pdbBlocked")
	}
	caution := func(cond bool, code string) {
		if cond {
			add(code)
			if out.Verdict == DrainVerdictOK {
				out.Verdict = DrainVerdictCaution
			}
		}
	}
	caution(softPDB, "pdbTight")
	caution(len(out.Unmanaged) > 0, "unmanaged")
	caution(len(out.LocalStorage) > 0, "localStorage")
	caution(len(out.SingleReplica) > 0, "singleReplica")
	caution(len(out.PinnedVolumes) > 0, "pinnedVolumes")
	caution(out.EvictCount > 0 && !out.Reschedule.Fits, "noHeadroom")
	return out
}

func drainReschedule(in DrainInputs, evict []DrainPod) DrainReschedule {
	r := DrainReschedule{Approximate: true}
	usedCPU, usedMem := map[string]int64{}, map[string]int64{}
	for _, p := range in.Pods {
		if p.Phase == "Succeeded" || p.Phase == "Failed" {
			continue
		}
		usedCPU[p.Node] += p.CPUMilli
		usedMem[p.Node] += p.MemBytes
	}
	var maxCPU, maxMem int64
	for _, p := range evict {
		r.NeedCPUMilli += p.CPUMilli
		r.NeedMemBytes += p.MemBytes
		if p.CPUMilli > maxCPU {
			maxCPU = p.CPUMilli
		}
		if p.MemBytes > maxMem {
			maxMem = p.MemBytes
		}
	}
	fitsOne := false
	for _, n := range in.Nodes {
		if n.Name == in.Node || !n.Ready || n.Unschedulable || n.NoSchedule {
			continue
		}
		hc, hm := n.AllocCPUMilli-usedCPU[n.Name], n.AllocMemBytes-usedMem[n.Name]
		if hc < 0 {
			hc = 0
		}
		if hm < 0 {
			hm = 0
		}
		r.CandidateNodes++
		r.HeadroomCPUMilli += hc
		r.HeadroomMemBytes += hm
		if hc >= maxCPU && hm >= maxMem {
			fitsOne = true
		}
	}
	r.Fits = len(evict) == 0 ||
		(r.CandidateNodes > 0 && r.HeadroomCPUMilli >= r.NeedCPUMilli && r.HeadroomMemBytes >= r.NeedMemBytes && fitsOne)
	return r
}

// CheckDrainConfirm is the server-side typed confirmation: the caller must
// echo the node name exactly (case-sensitive).
func CheckDrainConfirm(node, confirm string) error {
	if confirm != node {
		return NewStatusError(400, fmt.Errorf("draining evicts workloads; pass confirm=%s", node))
	}
	return nil
}

// drainCPUMilli parses a CPU quantity ("500m", "2", "1.5") to millicores; 0 on error.
func drainCPUMilli(s string) int64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "m") {
		v, err := strconv.ParseInt(strings.TrimSuffix(s, "m"), 10, 64)
		if err != nil {
			return 0
		}
		return v
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * 1000)
}
