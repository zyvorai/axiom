package platform

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

var (
	demoVeleroRestoreMu sync.Mutex
	demoVeleroRestores  = map[string]VeleroRestore{
		"velero/forge-backup-demo-20260101": {
			Namespace: "velero", Name: "forge-backup-demo-20260101", Backup: "forge-backup-demo", Phase: "Completed",
			Namespaces: "forge", ExistingResourcePolicy: "none", Warnings: 2, Age: "9d",
		},
	}
	demoLiveNamespaces = map[string]bool{
		"default": true, "kube-system": true, "velero": true, "forge": true, "forge-system": true,
	}
)

func demoBackupInfo(ns, name string) (VeleroBackupInfo, error) {
	if ns == "velero" && name == "forge-backup-demo" {
		return VeleroBackupInfo{
			Namespace: ns, Name: name, Phase: "Completed",
			IncludedNamespaces: []string{"forge", "forge-system"}, Warnings: 1,
			ItemsBackedUp: 184, TotalItems: 184,
		}, nil
	}
	demoVeleroBackupMu.Lock()
	b, ok := demoVeleroBackupExtra[ns+"/"+name]
	demoVeleroBackupMu.Unlock()
	if !ok {
		return VeleroBackupInfo{}, notFoundf("velero backup %s/%s not found", ns, name)
	}
	var inc []string
	if b.Namespaces != "" && b.Namespaces != "*" {
		inc = strings.Split(b.Namespaces, ",")
	}
	return VeleroBackupInfo{Namespace: ns, Name: name, Phase: b.Phase, IncludedNamespaces: inc}, nil
}

func (d *DemoProvider) ListVeleroRestores(ctx context.Context, ns string) ([]VeleroRestore, error) {
	demoVeleroRestoreMu.Lock()
	defer demoVeleroRestoreMu.Unlock()
	out := make([]VeleroRestore, 0, len(demoVeleroRestores))
	for _, r := range demoVeleroRestores {
		if ns == "" || ns == "all" || r.Namespace == ns {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (d *DemoProvider) demoPlanRestore(req VeleroRestoreRequest) (VeleroRestorePreview, error) {
	nr, err := NormalizeVeleroRestoreRequest(req)
	if err != nil {
		return VeleroRestorePreview{}, err
	}
	b, err := demoBackupInfo(nr.Namespace, nr.BackupName)
	if err != nil {
		return VeleroRestorePreview{}, err
	}
	existing := map[string]bool{}
	for k, v := range demoLiveNamespaces {
		existing[k] = v
	}
	return PlanVeleroRestore(nr, b, existing)
}

func (d *DemoProvider) PreviewVeleroRestore(ctx context.Context, req VeleroRestoreRequest) (VeleroRestorePreview, error) {
	return d.demoPlanRestore(req)
}

func (d *DemoProvider) CreateVeleroRestore(ctx context.Context, req VeleroRestoreRequest) (VeleroRestore, error) {
	p, err := d.demoPlanRestore(req)
	if err != nil {
		return VeleroRestore{}, err
	}
	if err := CheckVeleroRestoreConfirm(p, req.Confirm); err != nil {
		return VeleroRestore{}, err
	}
	nr, _ := NormalizeVeleroRestoreRequest(req)
	demoVeleroRestoreMu.Lock()
	defer demoVeleroRestoreMu.Unlock()
	key := p.Namespace + "/" + p.Name
	if _, ok := demoVeleroRestores[key]; ok {
		return VeleroRestore{}, NewStatusError(http.StatusConflict, fmt.Errorf("velero restore %s already exists", key))
	}
	r := VeleroRestore{
		Namespace: p.Namespace, Name: p.Name, Backup: p.Backup.Name, Phase: "New",
		Namespaces: strings.Join(nr.IncludedNamespaces, ","), ExistingResourcePolicy: p.ExistingResourcePolicy, Age: "now",
	}
	demoVeleroRestores[key] = r
	return r, nil
}
