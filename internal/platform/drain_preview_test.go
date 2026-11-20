package platform

import (
	"context"
	"testing"
)

func drainNodes() []DrainNodeInfo {
	return []DrainNodeInfo{
		{Name: "n1", Ready: true, AllocCPUMilli: 4000, AllocMemBytes: 8 << 30},
		{Name: "n2", Ready: true, AllocCPUMilli: 4000, AllocMemBytes: 8 << 30},
	}
}

func rs(ns, name, owner, node string) DrainPod {
	return DrainPod{Namespace: ns, Name: name, Node: node, Phase: "Running", OwnerKind: "ReplicaSet", OwnerName: owner, CPUMilli: 100, MemBytes: 128 << 20}
}

func TestDrainPreviewOK(t *testing.T) {
	in := DrainInputs{Node: "n1", Nodes: drainNodes(), Pods: []DrainPod{rs("a", "w-1", "w", "n1"), rs("a", "w-2", "w", "n2")}}
	p := ComputeDrainPreview(in)
	if p.Verdict != DrainVerdictOK || p.EvictCount != 1 || len(p.Evict) != 1 || p.Evict[0].Owner != "ReplicaSet/w" {
		t.Fatalf("got %+v", p)
	}
	if !p.Reschedule.Approximate || !p.Reschedule.Fits || p.Reschedule.CandidateNodes != 1 {
		t.Fatalf("reschedule %+v", p.Reschedule)
	}
}

func TestDrainPreviewIgnoredAndCategories(t *testing.T) {
	ds := DrainPod{Namespace: "a", Name: "ds", Node: "n1", Phase: "Running", OwnerKind: "DaemonSet", OwnerName: "d"}
	mirror := DrainPod{Namespace: "a", Name: "static", Node: "n1", Phase: "Running", Mirror: true}
	sys := DrainPod{Namespace: "kube-system", Name: "dns", Node: "n1", Phase: "Running", OwnerKind: "ReplicaSet", OwnerName: "dns"}
	bare := DrainPod{Namespace: "a", Name: "bare", Node: "n1", Phase: "Running"}
	scratch := rs("a", "s-1", "s", "n1")
	scratch.EmptyDir = true
	pinned := DrainPod{Namespace: "a", Name: "db-0", Node: "n1", Phase: "Running", OwnerKind: "StatefulSet", OwnerName: "db", PVCs: []string{"data-db-0"}}
	other := rs("a", "s-2", "s", "n2")
	p := ComputeDrainPreview(DrainInputs{Node: "n1", Nodes: drainNodes(),
		Pods:       []DrainPod{ds, mirror, sys, bare, scratch, pinned, other},
		PinnedPVCs: map[string]bool{"a/data-db-0": true}})
	if len(p.Ignored) != 3 {
		t.Fatalf("ignored %+v", p.Ignored)
	}
	if len(p.Unmanaged) != 1 || p.Unmanaged[0].Name != "bare" {
		t.Fatalf("unmanaged %+v", p.Unmanaged)
	}
	if len(p.LocalStorage) != 1 || len(p.PinnedVolumes) != 1 || p.PinnedVolumes[0].Reason != "data-db-0" {
		t.Fatalf("storage %+v %+v", p.LocalStorage, p.PinnedVolumes)
	}
	if len(p.SingleReplica) != 1 || p.SingleReplica[0].Name != "db-0" {
		t.Fatalf("single %+v", p.SingleReplica)
	}
	if p.Verdict != DrainVerdictCaution {
		t.Fatalf("verdict %s", p.Verdict)
	}
}

func TestDrainPreviewPDBBlocked(t *testing.T) {
	a, b := rs("a", "w-1", "w", "n1"), rs("a", "w-2", "w", "n2")
	pdbs := []DrainPDB{{Namespace: "a", Name: "w-pdb", DisruptionsAllowed: 0, Matches: map[string]bool{"a/w-1": true, "a/w-2": true}}}
	p := ComputeDrainPreview(DrainInputs{Node: "n1", Nodes: drainNodes(), Pods: []DrainPod{a, b}, PDBs: pdbs})
	if p.Verdict != DrainVerdictBlocked || len(p.PDBBlocked) != 1 || p.PDBBlocked[0].PDB != "w-pdb" {
		t.Fatalf("got %+v", p)
	}
	// a budget that allows the eviction does not block
	pdbs[0].DisruptionsAllowed = 1
	p = ComputeDrainPreview(DrainInputs{Node: "n1", Nodes: drainNodes(), Pods: []DrainPod{a, b}, PDBs: pdbs})
	if p.Verdict != DrainVerdictOK {
		t.Fatalf("got %s %v", p.Verdict, p.Reasons)
	}
	// two pods of a budget allowing one at once is tight, not blocked
	a2 := rs("a", "w-3", "w", "n1")
	pdbs[0].Matches["a/w-3"] = true
	p = ComputeDrainPreview(DrainInputs{Node: "n1", Nodes: drainNodes(), Pods: []DrainPod{a, a2, b}, PDBs: pdbs})
	if p.Verdict != DrainVerdictCaution || p.Reasons[0] != "pdbTight" {
		t.Fatalf("got %s %v", p.Verdict, p.Reasons)
	}
}

func TestDrainPreviewHeadroom(t *testing.T) {
	big := rs("a", "w-1", "w", "n1")
	big.CPUMilli = 3000
	other := rs("a", "w-2", "w", "n2")
	other.CPUMilli = 2000 // n2 has only 2000m left
	p := ComputeDrainPreview(DrainInputs{Node: "n1", Nodes: drainNodes(), Pods: []DrainPod{big, other}})
	if p.Reschedule.Fits || p.Verdict != DrainVerdictCaution {
		t.Fatalf("got %+v %s", p.Reschedule, p.Verdict)
	}
	// cordoned / not-ready / tainted nodes are not candidates
	nodes := []DrainNodeInfo{{Name: "n1", Ready: true}, {Name: "n2", Ready: true, Unschedulable: true}, {Name: "n3"}, {Name: "n4", Ready: true, NoSchedule: true}}
	p = ComputeDrainPreview(DrainInputs{Node: "n1", Nodes: nodes, Pods: []DrainPod{rs("a", "w-1", "w", "n1"), rs("a", "w-2", "w", "n2")}})
	if p.Reschedule.CandidateNodes != 0 || p.Reschedule.Fits {
		t.Fatalf("got %+v", p.Reschedule)
	}
}

func TestCheckDrainConfirm(t *testing.T) {
	if CheckDrainConfirm("n1", "") == nil || CheckDrainConfirm("n1", "N1") == nil {
		t.Fatal("wrong confirm accepted")
	}
	if err := CheckDrainConfirm("n1", "n1"); err != nil {
		t.Fatal(err)
	}
	if HTTPStatus(CheckDrainConfirm("n1", "x")) != 400 {
		t.Fatal("want 400")
	}
}

func TestDemoPreviewNodeDrain(t *testing.T) {
	d := NewDemoProvider()
	p, err := d.PreviewNodeDrain(context.Background(), "axiom-01")
	if err != nil || p.Node != "axiom-01" || p.EvictCount == 0 || !p.Reschedule.Approximate {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := d.PreviewNodeDrain(context.Background(), "nope"); HTTPStatus(err) != 404 {
		t.Fatalf("want 404, got %v", err)
	}
}
