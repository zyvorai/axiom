package platform

import (
	"fmt"
	"sort"
	"strings"
)

// Velero restore wizard: pure validation and preview logic shared by the
// preview and create endpoints, in both providers. Nothing here touches a
// cluster.

// VeleroRestore is one Velero Restore as listed.
type VeleroRestore struct {
	Namespace              string `json:"namespace"`
	Name                   string `json:"name"`
	Backup                 string `json:"backup"`
	Phase                  string `json:"phase"`
	Namespaces             string `json:"namespaces,omitempty"`
	ExistingResourcePolicy string `json:"existingResourcePolicy,omitempty"`
	Errors                 int    `json:"errors,omitempty"`
	Warnings               int    `json:"warnings,omitempty"`
	Age                    string `json:"age"`
}

// VeleroRestoreRequest is the body of both POST /velero/restores/preview and
// POST /velero/restores. Confirm is only read by create.
type VeleroRestoreRequest struct {
	Namespace              string            `json:"namespace"`
	Name                   string            `json:"name"`
	BackupName             string            `json:"backupName"`
	IncludedNamespaces     []string          `json:"includedNamespaces,omitempty"`
	ExcludedNamespaces     []string          `json:"excludedNamespaces,omitempty"`
	IncludedResources      []string          `json:"includedResources,omitempty"`
	ExcludedResources      []string          `json:"excludedResources,omitempty"`
	NamespaceMapping       map[string]string `json:"namespaceMapping,omitempty"`
	RestorePVs             *bool             `json:"restorePVs,omitempty"`
	ExistingResourcePolicy string            `json:"existingResourcePolicy,omitempty"`
	Confirm                string            `json:"confirm,omitempty"`
}

// VeleroBackupInfo is what the planner needs to know about the source Backup.
type VeleroBackupInfo struct {
	Namespace          string   `json:"namespace"`
	Name               string   `json:"name"`
	Phase              string   `json:"phase"`
	IncludedNamespaces []string `json:"includedNamespaces"`
	ExcludedNamespaces []string `json:"excludedNamespaces,omitempty"`
	IncludedResources  []string `json:"includedResources,omitempty"`
	ExcludedResources  []string `json:"excludedResources,omitempty"`
	Errors             int      `json:"errors,omitempty"`
	Warnings           int      `json:"warnings,omitempty"`
	ItemsBackedUp      int      `json:"itemsBackedUp,omitempty"`
	TotalItems         int      `json:"totalItems,omitempty"`
}

// VeleroRestoreTarget describes one source namespace of the restore.
type VeleroRestoreTarget struct {
	Source string `json:"source"`
	Target string `json:"target"`
	// Exists is true when the target namespace is already in the cluster, so
	// the restore would collide with live objects there.
	Exists bool `json:"exists"`
}

// VeleroRestorePreview is the dry-run result. Nothing is created.
type VeleroRestorePreview struct {
	Namespace              string                `json:"namespace"`
	Name                   string                `json:"name"`
	Backup                 VeleroBackupInfo      `json:"backup"`
	Targets                []VeleroRestoreTarget `json:"targets"`
	AllNamespaces          bool                  `json:"allNamespaces"`
	CollidingNamespaces    []string              `json:"collidingNamespaces"`
	IncludedResources      []string              `json:"includedResources"`
	ExcludedResources      []string              `json:"excludedResources"`
	RestorePVs             bool                  `json:"restorePVs"`
	ExistingResourcePolicy string                `json:"existingResourcePolicy"`
	// ResourceEffect says in words what happens to objects that already exist.
	ResourceEffect string   `json:"resourceEffect"`
	Warnings       []string `json:"warnings"`
	// ConfirmPhrase is what create requires as `confirm`: the restore name.
	ConfirmPhrase string `json:"confirmPhrase"`
}

const (
	veleroPolicyNone   = "none"
	veleroPolicyUpdate = "update"
)

// NormalizeVeleroRestoreRequest validates req and fills defaults. It is the
// single validation used by preview and create.
func NormalizeVeleroRestoreRequest(req VeleroRestoreRequest) (VeleroRestoreRequest, error) {
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Name = strings.TrimSpace(req.Name)
	req.BackupName = strings.TrimSpace(req.BackupName)
	if err := checkDNSLabel("namespace", req.Namespace, 63); err != nil {
		return req, err
	}
	if err := checkDNSLabel("name", req.Name, 63); err != nil {
		return req, err
	}
	if err := checkDNSLabel("backupName", req.BackupName, 63); err != nil {
		return req, err
	}
	switch req.ExistingResourcePolicy {
	case "":
		req.ExistingResourcePolicy = veleroPolicyNone
	case veleroPolicyNone, veleroPolicyUpdate:
	default:
		return req, gitOpsBad("existingResourcePolicy must be none or update")
	}
	var err error
	if req.IncludedNamespaces, err = cleanVeleroList("includedNamespaces", req.IncludedNamespaces, true); err != nil {
		return req, err
	}
	if req.ExcludedNamespaces, err = cleanVeleroList("excludedNamespaces", req.ExcludedNamespaces, true); err != nil {
		return req, err
	}
	if req.IncludedResources, err = cleanVeleroList("includedResources", req.IncludedResources, false); err != nil {
		return req, err
	}
	if req.ExcludedResources, err = cleanVeleroList("excludedResources", req.ExcludedResources, false); err != nil {
		return req, err
	}
	for _, n := range req.ExcludedNamespaces {
		if n == "*" {
			return req, gitOpsBad("excludedNamespaces cannot contain *")
		}
	}
	if len(req.IncludedNamespaces) > 1 {
		for _, n := range req.IncludedNamespaces {
			if n == "*" {
				return req, gitOpsBad("includedNamespaces: * cannot be combined with other namespaces")
			}
		}
	}
	inc := map[string]bool{}
	for _, n := range req.IncludedNamespaces {
		inc[n] = true
	}
	for _, n := range req.ExcludedNamespaces {
		if inc[n] {
			return req, gitOpsBad("namespace %s is both included and excluded", n)
		}
	}
	targets := map[string]string{}
	srcs := make([]string, 0, len(req.NamespaceMapping))
	for src := range req.NamespaceMapping {
		srcs = append(srcs, src)
	}
	sort.Strings(srcs)
	for _, src := range srcs {
		dst := req.NamespaceMapping[src]
		if err := checkDNSLabel("namespaceMapping source", src, 63); err != nil {
			return req, err
		}
		if err := checkDNSLabel("namespaceMapping target", dst, 63); err != nil {
			return req, err
		}
		if prev, dup := targets[dst]; dup {
			return req, gitOpsBad("namespaceMapping: %s and %s both map to %s", prev, src, dst)
		}
		targets[dst] = src
	}
	return req, nil
}

func cleanVeleroList(field string, in []string, namespaces bool) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		if len(v) > 253 || strings.ContainsAny(v, " \t\n,") {
			return nil, gitOpsBad("%s contains an invalid entry", field)
		}
		if namespaces && v != "*" {
			if err := checkDNSLabel(field+" entry", v, 63); err != nil {
				return nil, err
			}
		}
		seen[v] = true
		out = append(out, v)
	}
	return out, nil
}

func isVeleroAll(l []string) bool {
	return len(l) == 0 || (len(l) == 1 && l[0] == "*")
}

// effectiveVeleroSources lists the namespaces the restore would take from the
// backup; all is true when the backup does not enumerate them.
func effectiveVeleroSources(req VeleroRestoreRequest, b VeleroBackupInfo) (list []string, all bool) {
	pick := req.IncludedNamespaces
	if isVeleroAll(pick) {
		pick = b.IncludedNamespaces
	}
	if isVeleroAll(pick) {
		return nil, true
	}
	excl := map[string]bool{}
	for _, n := range req.ExcludedNamespaces {
		excl[n] = true
	}
	for _, n := range pick {
		if !excl[n] {
			list = append(list, n)
		}
	}
	sort.Strings(list)
	return list, false
}

// PlanVeleroRestore validates req against the backup and the namespaces that
// exist, and returns the preview. A non-nil error means the restore must not
// be created (bad input or an unusable backup).
func PlanVeleroRestore(req VeleroRestoreRequest, b VeleroBackupInfo, existingNS map[string]bool) (VeleroRestorePreview, error) {
	req, err := NormalizeVeleroRestoreRequest(req)
	if err != nil {
		return VeleroRestorePreview{}, err
	}
	switch b.Phase {
	case "Completed", "PartiallyFailed":
	default:
		phase := b.Phase
		if phase == "" {
			phase = "in an unknown state"
		}
		return VeleroRestorePreview{}, gitOpsBad("backup %s is %s; only Completed or PartiallyFailed backups can be restored", b.Name, phase)
	}
	p := VeleroRestorePreview{
		Namespace: req.Namespace, Name: req.Name, Backup: b,
		Targets: []VeleroRestoreTarget{}, CollidingNamespaces: []string{}, Warnings: []string{},
		IncludedResources: append([]string{}, req.IncludedResources...), ExcludedResources: append([]string{}, req.ExcludedResources...),
		RestorePVs:             req.RestorePVs == nil || *req.RestorePVs,
		ExistingResourcePolicy: req.ExistingResourcePolicy, ConfirmPhrase: req.Name,
	}
	sources, all := effectiveVeleroSources(req, b)
	p.AllNamespaces = all
	if !isVeleroAll(b.IncludedNamespaces) {
		backupHas := map[string]bool{}
		for _, n := range b.IncludedNamespaces {
			backupHas[n] = true
		}
		for _, n := range req.IncludedNamespaces {
			if n != "*" && !backupHas[n] {
				return VeleroRestorePreview{}, gitOpsBad("namespace %s is not part of backup %s", n, b.Name)
			}
		}
	}
	if len(sources) == 0 && !all {
		return VeleroRestorePreview{}, gitOpsBad("the namespace selection leaves nothing to restore")
	}
	if !all {
		in := map[string]bool{}
		for _, s := range sources {
			in[s] = true
		}
		for src := range req.NamespaceMapping {
			if !in[src] {
				return VeleroRestorePreview{}, gitOpsBad("namespaceMapping source %s is not among the restored namespaces", src)
			}
		}
	}
	for _, s := range sources {
		t := s
		if m, ok := req.NamespaceMapping[s]; ok {
			t = m
		}
		ex := existingNS[t]
		p.Targets = append(p.Targets, VeleroRestoreTarget{Source: s, Target: t, Exists: ex})
		if ex {
			p.CollidingNamespaces = append(p.CollidingNamespaces, t)
		}
	}
	if all {
		p.Warnings = append(p.Warnings, "The backup does not list its namespaces; every namespace it contains would be restored and collisions cannot be fully checked.")
		for _, dst := range req.NamespaceMapping {
			if existingNS[dst] {
				p.CollidingNamespaces = append(p.CollidingNamespaces, dst)
			}
		}
		sort.Strings(p.CollidingNamespaces)
	}
	if len(p.CollidingNamespaces) > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("Restoring over live namespaces: %s.", strings.Join(p.CollidingNamespaces, ", ")))
	}
	if p.ExistingResourcePolicy == veleroPolicyUpdate {
		p.ResourceEffect = "Objects that already exist are patched to match the backup; objects not in the backup are left alone."
		if len(p.CollidingNamespaces) > 0 {
			p.Warnings = append(p.Warnings, "existingResourcePolicy=update will modify live objects in the colliding namespaces.")
		}
	} else {
		p.ResourceEffect = "Objects that already exist are skipped and reported as warnings; only missing objects are created."
	}
	if p.RestorePVs {
		p.Warnings = append(p.Warnings, "restorePVs is on: persistent volume data is restored, which can provision storage.")
	}
	if b.Phase == "PartiallyFailed" {
		p.Warnings = append(p.Warnings, "The backup is PartiallyFailed; some items may be missing from it.")
	}
	if b.Errors > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("The backup recorded %d errors.", b.Errors))
	}
	return p, nil
}

// CheckVeleroRestoreConfirm enforces the typed confirmation: create requires
// confirm to equal the restore name exactly (case-sensitive).
func CheckVeleroRestoreConfirm(p VeleroRestorePreview, confirm string) error {
	if confirm != p.ConfirmPhrase {
		return gitOpsBad("restoring changes the cluster; pass confirm=%s", p.ConfirmPhrase)
	}
	return nil
}

// BuildVeleroRestoreSpec returns the Restore .spec for a normalized request.
func BuildVeleroRestoreSpec(req VeleroRestoreRequest) map[string]any {
	toAny := func(in []string) []any {
		out := make([]any, len(in))
		for i, v := range in {
			out[i] = v
		}
		return out
	}
	spec := map[string]any{
		"backupName":             req.BackupName,
		"existingResourcePolicy": req.ExistingResourcePolicy,
	}
	if len(req.IncludedNamespaces) > 0 {
		spec["includedNamespaces"] = toAny(req.IncludedNamespaces)
	}
	if len(req.ExcludedNamespaces) > 0 {
		spec["excludedNamespaces"] = toAny(req.ExcludedNamespaces)
	}
	if len(req.IncludedResources) > 0 {
		spec["includedResources"] = toAny(req.IncludedResources)
	}
	if len(req.ExcludedResources) > 0 {
		spec["excludedResources"] = toAny(req.ExcludedResources)
	}
	if len(req.NamespaceMapping) > 0 {
		m := map[string]any{}
		for k, v := range req.NamespaceMapping {
			m[k] = v
		}
		spec["namespaceMapping"] = m
	}
	if req.RestorePVs != nil {
		spec["restorePVs"] = *req.RestorePVs
	}
	return spec
}

// RestoreSummaryDetail is the names-only audit detail for a restore.
func RestoreSummaryDetail(req VeleroRestoreRequest) string {
	return "backup=" + req.BackupName + " policy=" + req.ExistingResourcePolicy
}
