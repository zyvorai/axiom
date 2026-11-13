package platform

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func testBackup() VeleroBackupInfo {
	return VeleroBackupInfo{Namespace: "velero", Name: "b1", Phase: "Completed", IncludedNamespaces: []string{"forge", "forge-system"}}
}

func baseRestore() VeleroRestoreRequest {
	return VeleroRestoreRequest{Namespace: "velero", Name: "r1", BackupName: "b1"}
}

func TestNormalizeVeleroRestoreRequest(t *testing.T) {
	bad := map[string]func(*VeleroRestoreRequest){
		"name":      func(r *VeleroRestoreRequest) { r.Name = "Bad_Name" },
		"backup":    func(r *VeleroRestoreRequest) { r.BackupName = "" },
		"policy":    func(r *VeleroRestoreRequest) { r.ExistingResourcePolicy = "delete" },
		"star mix":  func(r *VeleroRestoreRequest) { r.IncludedNamespaces = []string{"*", "a"} },
		"excl star": func(r *VeleroRestoreRequest) { r.ExcludedNamespaces = []string{"*"} },
		"both":      func(r *VeleroRestoreRequest) { r.IncludedNamespaces = []string{"a"}; r.ExcludedNamespaces = []string{"a"} },
		"map dup":   func(r *VeleroRestoreRequest) { r.NamespaceMapping = map[string]string{"a": "x", "b": "x"} },
		"map bad":   func(r *VeleroRestoreRequest) { r.NamespaceMapping = map[string]string{"a": "X Y"} },
		"ns bad":    func(r *VeleroRestoreRequest) { r.IncludedNamespaces = []string{"a b"} },
	}
	for name, mut := range bad {
		r := baseRestore()
		mut(&r)
		_, err := NormalizeVeleroRestoreRequest(r)
		if err == nil || HTTPStatus(err) != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %v", name, err)
		}
	}
	n, err := NormalizeVeleroRestoreRequest(baseRestore())
	if err != nil || n.ExistingResourcePolicy != "none" {
		t.Fatalf("defaults: %v %+v", err, n)
	}
}

func TestPlanVeleroRestoreCollisions(t *testing.T) {
	req := baseRestore()
	req.ExistingResourcePolicy = "update"
	p, err := PlanVeleroRestore(req, testBackup(), map[string]bool{"forge": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Targets) != 2 || len(p.CollidingNamespaces) != 1 || p.CollidingNamespaces[0] != "forge" {
		t.Fatalf("targets/collisions: %+v", p)
	}
	joined := strings.Join(p.Warnings, "|")
	if !strings.Contains(joined, "live namespaces") || !strings.Contains(joined, "will modify live objects") {
		t.Fatalf("warnings: %v", p.Warnings)
	}
	if p.ConfirmPhrase != "r1" || !p.RestorePVs {
		t.Fatalf("confirm/restorePVs: %+v", p)
	}
}

func TestPlanVeleroRestoreMappingAvoidsCollision(t *testing.T) {
	req := baseRestore()
	req.IncludedNamespaces = []string{"forge"}
	req.NamespaceMapping = map[string]string{"forge": "forge-copy"}
	no := false
	req.RestorePVs = &no
	p, err := PlanVeleroRestore(req, testBackup(), map[string]bool{"forge": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.CollidingNamespaces) != 0 || p.Targets[0].Target != "forge-copy" || p.RestorePVs {
		t.Fatalf("%+v", p)
	}
}

func TestPlanVeleroRestoreRejects(t *testing.T) {
	b := testBackup()
	req := baseRestore()
	req.IncludedNamespaces = []string{"other"}
	if _, err := PlanVeleroRestore(req, b, nil); err == nil {
		t.Error("namespace outside backup must be rejected")
	}
	req = baseRestore()
	req.ExcludedNamespaces = []string{"forge", "forge-system"}
	if _, err := PlanVeleroRestore(req, b, nil); err == nil {
		t.Error("empty selection must be rejected")
	}
	req = baseRestore()
	req.NamespaceMapping = map[string]string{"nope": "x"}
	if _, err := PlanVeleroRestore(req, b, nil); err == nil {
		t.Error("mapping from unrestored namespace must be rejected")
	}
	b.Phase = "InProgress"
	if _, err := PlanVeleroRestore(baseRestore(), b, nil); err == nil || HTTPStatus(err) != 400 {
		t.Errorf("unfinished backup: %v", err)
	}
}

func TestPlanVeleroRestoreWildcardBackup(t *testing.T) {
	b := testBackup()
	b.IncludedNamespaces = nil
	b.Phase = "PartiallyFailed"
	b.Errors = 3
	p, err := PlanVeleroRestore(baseRestore(), b, nil)
	if err != nil || !p.AllNamespaces {
		t.Fatalf("%v %+v", err, p)
	}
	if len(p.Warnings) < 3 {
		t.Fatalf("want wildcard, PartiallyFailed and error warnings: %v", p.Warnings)
	}
}

func TestCheckVeleroRestoreConfirm(t *testing.T) {
	p := VeleroRestorePreview{ConfirmPhrase: "r1"}
	for _, c := range []string{"", "R1", "r1 "} {
		if err := CheckVeleroRestoreConfirm(p, c); err == nil || HTTPStatus(err) != 400 {
			t.Errorf("confirm %q should be rejected: %v", c, err)
		}
	}
	if err := CheckVeleroRestoreConfirm(p, "r1"); err != nil {
		t.Error(err)
	}
}

func TestBuildVeleroRestoreSpec(t *testing.T) {
	no := false
	req, _ := NormalizeVeleroRestoreRequest(VeleroRestoreRequest{
		Namespace: "velero", Name: "r1", BackupName: "b1", IncludedNamespaces: []string{"forge"},
		NamespaceMapping: map[string]string{"forge": "f2"}, RestorePVs: &no, ExistingResourcePolicy: "update",
	})
	spec := BuildVeleroRestoreSpec(req)
	if spec["backupName"] != "b1" || spec["existingResourcePolicy"] != "update" || spec["restorePVs"] != false {
		t.Fatalf("%v", spec)
	}
	if spec["namespaceMapping"].(map[string]any)["forge"] != "f2" {
		t.Fatalf("%v", spec)
	}
	if _, ok := spec["excludedNamespaces"]; ok {
		t.Fatal("empty lists must be omitted")
	}
}

func TestDemoVeleroRestoreFlow(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := VeleroRestoreRequest{Namespace: "velero", Name: "demo-restore-1", BackupName: "forge-backup-demo"}
	p, err := d.PreviewVeleroRestore(ctx, req)
	if err != nil || len(p.CollidingNamespaces) != 2 {
		t.Fatalf("preview: %v %+v", err, p)
	}
	before, _ := d.ListVeleroRestores(ctx, "")
	if _, err := d.CreateVeleroRestore(ctx, req); err == nil || HTTPStatus(err) != 400 {
		t.Fatalf("create without confirm must be 400: %v", err)
	}
	after, _ := d.ListVeleroRestores(ctx, "")
	if len(after) != len(before) {
		t.Fatal("preview/unconfirmed create must not create anything")
	}
	req.Confirm = "demo-restore-1"
	if _, err := d.CreateVeleroRestore(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateVeleroRestore(ctx, req); err == nil || HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("duplicate: %v", err)
	}
	req.BackupName = "missing"
	if _, err := d.PreviewVeleroRestore(ctx, req); err == nil || HTTPStatus(err) != 404 {
		t.Fatalf("missing backup: %v", err)
	}
}
