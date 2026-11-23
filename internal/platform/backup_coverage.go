package platform

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// Backup coverage report (GET /api/v1/backups/coverage). For every namespace
// that holds data (a PersistentVolumeClaim or a workload) it answers "when
// did a Velero Backup that includes this namespace last complete?". It is
// read-only and built only from lists the Provider already serves; it never
// creates a backup and it cannot tell whether a backup is restorable.

const (
	// BackupCoverageDefaultMaxAgeDays is how old the newest good backup may be.
	BackupCoverageDefaultMaxAgeDays = 7
	// BackupCoverageMaxAgeDaysCap caps the maxAgeDays query parameter.
	BackupCoverageMaxAgeDaysCap = 365
	backupCoverageMaxNamespaces = 500

	CoverageProtected   = "protected"
	CoverageStale       = "stale"
	CoverageFailing     = "failing"
	CoverageUnprotected = "unprotected"
)

// BackupCoverageNamespace is the verdict for one namespace.
type BackupCoverageNamespace struct {
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	// PVCs and CapacityGi say how much stateful data the namespace holds.
	PVCs       int     `json:"pvcs"`
	CapacityGi float64 `json:"capacityGi"`
	Workloads  int     `json:"workloads"`
	// LastGood is the age ("3d", "5h") of the newest Completed backup that
	// includes the namespace; empty when there is none.
	LastGood string `json:"lastGood,omitempty"`
	// Backups counts every Velero Backup that includes the namespace.
	Backups int    `json:"backups"`
	Reason  string `json:"reason"`
}

type BackupCoverageSummary struct {
	Namespaces  int            `json:"namespaces"`
	ByStatus    map[string]int `json:"byStatus"`
	DataAtRisk  float64        `json:"dataAtRiskGi"`
	Schedules   int            `json:"schedules"`
	Paused      int            `json:"pausedSchedules"`
	BackupCount int            `json:"backups"`
}

type BackupCoverageReport struct {
	MaxAgeDays int                       `json:"maxAgeDays"`
	Summary    BackupCoverageSummary     `json:"summary"`
	Items      []BackupCoverageNamespace `json:"namespaces"`
	Truncated  bool                      `json:"truncated,omitempty"`
	Note       string                    `json:"note"`
}

// BackupCoverageInput is the snapshot DetectBackupCoverage works on.
type BackupCoverageInput struct {
	Namespaces []Namespace
	Volumes    []Volume
	Workloads  []Workload
	Backups    []VeleroBackup
	Schedules  []VeleroSchedule
}

// ageHours reads provider age strings ("now", "5m", "3h", "12d"); ok is false
// for anything else (including "-" and hostile text).
func ageHours(s string) (hours float64, ok bool) {
	s = strings.TrimSpace(s)
	if s == "now" {
		return 0, true
	}
	if len(s) < 2 || len(s) > 8 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 0 {
		return 0, false
	}
	switch s[len(s)-1] {
	case 'm':
		return float64(n) / 60, true
	case 'h':
		return float64(n), true
	case 'd':
		return float64(n) * 24, true
	}
	return 0, false
}

// backupIncludes reports whether a Velero backup's includedNamespaces text
// (comma list, empty or "*" meaning every namespace) covers ns.
func backupIncludes(included, ns string) bool {
	included = strings.TrimSpace(included)
	if included == "" || included == "*" {
		return true
	}
	for _, part := range strings.Split(included, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || part == ns {
			return true
		}
	}
	return false
}

// DetectBackupCoverage is pure. maxAgeDays <= 0 falls back to the default and
// values above the cap are clamped.
func DetectBackupCoverage(in BackupCoverageInput, maxAgeDays int) BackupCoverageReport {
	if maxAgeDays <= 0 {
		maxAgeDays = BackupCoverageDefaultMaxAgeDays
	}
	if maxAgeDays > BackupCoverageMaxAgeDaysCap {
		maxAgeDays = BackupCoverageMaxAgeDaysCap
	}
	type acc struct {
		pvcs      int
		gi        float64
		workloads int
	}
	data := map[string]*acc{}
	get := func(ns string) *acc {
		a := data[ns]
		if a == nil {
			a = &acc{}
			data[ns] = a
		}
		return a
	}
	for _, v := range in.Volumes {
		a := get(v.Namespace)
		a.pvcs++
		a.gi += float64(parseMemoryBytes(v.Capacity)) / (1 << 30)
	}
	for _, w := range in.Workloads {
		get(w.Namespace).workloads++
	}
	skip := func(ns string) bool {
		return ns == "" || protectedNamespace(ns) || strings.HasPrefix(ns, "kube-") || ns == "velero"
	}

	var items []BackupCoverageNamespace
	byStatus := map[string]int{}
	atRisk := 0.0
	for ns, a := range data {
		if skip(ns) {
			continue
		}
		item := BackupCoverageNamespace{Namespace: ns, PVCs: a.pvcs, CapacityGi: idleMoney(a.gi), Workloads: a.workloads}
		best, bestSet := 0.0, false
		failing := 0
		for _, b := range in.Backups {
			if !backupIncludes(b.Namespaces, ns) {
				continue
			}
			item.Backups++
			switch b.Phase {
			case "Completed":
				if h, ok := ageHours(b.Age); ok && (!bestSet || h < best) {
					best, bestSet = h, true
					item.LastGood = b.Age
				}
			case "PartiallyFailed", "Failed", "FailedValidation":
				failing++
			}
		}
		limit := float64(maxAgeDays) * 24
		switch {
		case bestSet && best <= limit:
			item.Status = CoverageProtected
			item.Reason = "A completed backup including this namespace is " + item.LastGood + " old."
		case bestSet:
			item.Status = CoverageStale
			item.Reason = "Newest completed backup is " + item.LastGood + " old, older than the " + strconv.Itoa(maxAgeDays) + "-day limit."
		case failing > 0:
			item.Status = CoverageFailing
			item.Reason = "Backups include this namespace but none completed (" + strconv.Itoa(failing) + " failed or partially failed)."
		default:
			item.Status = CoverageUnprotected
			item.Reason = "No Velero backup includes this namespace."
		}
		byStatus[item.Status]++
		if item.Status != CoverageProtected {
			atRisk += item.CapacityGi
		}
		items = append(items, item)
	}
	rank := map[string]int{CoverageUnprotected: 0, CoverageFailing: 1, CoverageStale: 2, CoverageProtected: 3}
	sort.Slice(items, func(i, j int) bool {
		if rank[items[i].Status] != rank[items[j].Status] {
			return rank[items[i].Status] < rank[items[j].Status]
		}
		if items[i].CapacityGi != items[j].CapacityGi {
			return items[i].CapacityGi > items[j].CapacityGi
		}
		return items[i].Namespace < items[j].Namespace
	})
	rep := BackupCoverageReport{MaxAgeDays: maxAgeDays}
	if len(items) > backupCoverageMaxNamespaces {
		items = items[:backupCoverageMaxNamespaces]
		rep.Truncated = true
	}
	rep.Items = items
	if rep.Items == nil {
		rep.Items = []BackupCoverageNamespace{}
	}
	sum := BackupCoverageSummary{Namespaces: len(data), ByStatus: byStatus, DataAtRisk: idleMoney(atRisk), BackupCount: len(in.Backups)}
	for _, s := range in.Schedules {
		sum.Schedules++
		if s.Paused {
			sum.Paused++
		}
	}
	for ns := range data {
		if skip(ns) {
			sum.Namespaces--
		}
	}
	rep.Summary = sum
	rep.Note = "Built from Velero Backup objects visible to Axiom. A Completed backup is not proof the data is restorable: test a restore. Schedules are counted but cannot be tied to a namespace from the objects Axiom reads. Without Velero installed every namespace shows as unprotected."
	return rep
}

// BackupCoverage collects the snapshot from the provider and runs the check.
// Velero lists are best effort: a failure there means "no backups seen".
func BackupCoverage(ctx context.Context, p Provider, maxAgeDays int) (BackupCoverageReport, error) {
	var in BackupCoverageInput
	var err error
	if in.Namespaces, err = p.ListNamespaces(ctx); err != nil {
		return BackupCoverageReport{}, err
	}
	if in.Volumes, err = p.ListVolumes(ctx, ""); err != nil {
		return BackupCoverageReport{}, err
	}
	if in.Workloads, err = p.ListWorkloads(ctx, ""); err != nil {
		return BackupCoverageReport{}, err
	}
	in.Backups, _ = p.ListVeleroBackups(ctx, "")
	in.Schedules, _ = p.ListVeleroSchedules(ctx, "")
	return DetectBackupCoverage(in, maxAgeDays), nil
}
