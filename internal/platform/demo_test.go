package platform

import (
	"context"
	"testing"
)

func TestDemoLifecycle(t *testing.T) {
	ctx := context.Background()
	p := NewDemoProvider()
	w, err := p.Launch(ctx, LaunchRequest{Namespace: "test", Name: "api", Image: "nginx:1.27", Replicas: 2, Port: 80})
	if err != nil {
		t.Fatal(err)
	}
	if w.Desired != 2 || w.Ready != 2 {
		t.Fatalf("unexpected launch state: %+v", w)
	}
	if err := p.Stop(ctx, "test", "Deployment", "api"); err != nil {
		t.Fatal(err)
	}
	ws, _ := p.ListWorkloads(ctx, "test")
	if len(ws) != 1 || ws[0].Desired != 0 || ws[0].Status != "Stopped" {
		t.Fatalf("stop failed: %+v", ws)
	}
	if err := p.Start(ctx, "test", "Deployment", "api"); err != nil {
		t.Fatal(err)
	}
	ws, _ = p.ListWorkloads(ctx, "test")
	if ws[0].Desired != 2 || ws[0].Status != "Healthy" {
		t.Fatalf("start did not restore previous replica count: %+v", ws[0])
	}
	if err := p.Scale(ctx, "test", "Deployment", "api", 5); err != nil {
		t.Fatal(err)
	}
	pods, _ := p.ListPods(ctx, "test")
	if len(pods) != 5 {
		t.Fatalf("expected 5 pods, got %d", len(pods))
	}
}

func TestDemoExec(t *testing.T) {
	p := NewDemoProvider()
	out, err := p.ExecCommand(context.Background(), "payments", "payments-api-01", "payments-api", []string{"whoami"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "axiom" {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestDemoExecShellWrappedCommand(t *testing.T) {
	p := NewDemoProvider()
	out, err := p.ExecCommand(context.Background(), "payments", "payments-api-01", "payments-api", []string{"/bin/sh", "-lc", "whoami"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "axiom" {
		t.Fatalf("unexpected output %q", out)
	}
}

// A huge replica count used to crash the whole process: rebuildPodsLocked
// did make([]Pod, 0, desired) with an unvalidated caller-supplied value,
// and a multi-gigabyte allocation attempt triggers a fatal (unrecoverable)
// Go runtime out-of-memory error, not something an HTTP handler's
// deferred recover() can catch. Found via randomized/exploratory testing:
// POSTing {"replicas": 999999999} to the scale endpoint took the demo
// server down entirely.
func TestScaleRejectsHugeReplicaCount(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	err := d.Scale(ctx, "commerce", "Deployment", "checkout", 999999999)
	if err == nil {
		t.Fatal("expected Scale to reject a replica count above maxDemoReplicas, got nil error")
	}
}

func TestScaleAllowsMaxReplicaCount(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if err := d.Scale(ctx, "commerce", "Deployment", "checkout", maxDemoReplicas); err != nil {
		t.Fatalf("Scale(maxDemoReplicas) should succeed, got %v", err)
	}
	pods, err := d.ListPods(ctx, "commerce")
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	if len(pods) != maxDemoReplicas {
		t.Fatalf("expected %d pods after scaling to the cap, got %d", maxDemoReplicas, len(pods))
	}
}

// A duplicate Launch for the same namespace/kind/name used to silently
// overwrite the existing workload and report success both times, unlike
// a real cluster's API server which rejects the second create with
// AlreadyExists. Found by rapid-clicking the Launch button three times
// in the browser before the button had a chance to disable — all three
// POSTs returned 201.
func TestLaunchRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := LaunchRequest{Namespace: "default", Name: "race-launch-test", Image: "nginx:latest", Replicas: 1}
	if _, err := d.Launch(ctx, req); err != nil {
		t.Fatalf("first Launch should succeed, got %v", err)
	}
	if _, err := d.Launch(ctx, req); err == nil {
		t.Fatal("expected second Launch with the same name to fail, got nil error")
	}
}

// Same duplicate-name bug as TestLaunchRejectsDuplicateName, found to be
// systemic across every demo-mode Create method that writes straight
// into a map with no existence check: ConfigMaps, Secrets, HPAs, and
// RBAC Roles/RoleBindings all silently overwrote an existing resource
// of the same name and returned success both times, unlike a real
// cluster's 409 AlreadyExists. Confirmed live via curl before fixing.
func TestCreateConfigMapRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateConfigMapRequest{Namespace: "default", Name: "dup-cm", Data: map[string]string{"a": "1"}}
	if _, err := d.CreateConfigMap(ctx, req); err != nil {
		t.Fatalf("first CreateConfigMap should succeed, got %v", err)
	}
	if _, err := d.CreateConfigMap(ctx, req); err == nil {
		t.Fatal("expected second CreateConfigMap with the same name to fail, got nil error")
	}
}

func TestCreateSecretRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateSecretRequest{Namespace: "default", Name: "dup-secret", Data: map[string]string{"a": "1"}}
	if _, err := d.CreateSecret(ctx, req); err != nil {
		t.Fatalf("first CreateSecret should succeed, got %v", err)
	}
	if _, err := d.CreateSecret(ctx, req); err == nil {
		t.Fatal("expected second CreateSecret with the same name to fail, got nil error")
	}
}

func TestCreateHPARejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateHPARequest{Namespace: "default", Name: "dup-hpa", TargetKind: "Deployment", TargetName: "checkout", MinReplicas: 1, MaxReplicas: 5}
	if _, err := d.CreateHPA(ctx, req); err != nil {
		t.Fatalf("first CreateHPA should succeed, got %v", err)
	}
	if _, err := d.CreateHPA(ctx, req); err == nil {
		t.Fatal("expected second CreateHPA with the same name to fail, got nil error")
	}
}

func TestCreateRoleRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateRoleRequest{Namespace: "default", Name: "dup-role", Rules: []RBACPolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}}}
	if _, err := d.CreateRole(ctx, req); err != nil {
		t.Fatalf("first CreateRole should succeed, got %v", err)
	}
	if _, err := d.CreateRole(ctx, req); err == nil {
		t.Fatal("expected second CreateRole with the same name to fail, got nil error")
	}
}

func TestCreateRoleBindingRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateRoleBindingRequest{Namespace: "default", Name: "dup-rb", RoleName: "some-role", Subjects: []RBACSubject{{Kind: "User", Name: "demo"}}}
	if _, err := d.CreateRoleBinding(ctx, req); err != nil {
		t.Fatalf("first CreateRoleBinding should succeed, got %v", err)
	}
	if _, err := d.CreateRoleBinding(ctx, req); err == nil {
		t.Fatal("expected second CreateRoleBinding with the same name to fail, got nil error")
	}
}

// A repeat delete of an already-deleted (or never-created) resource used
// to silently report success, unlike a real cluster's 404 NotFound —
// and unlike this codebase's own dominant convention (DeleteSecret,
// DeleteJob, DeleteCronJob, DeleteHPA, DeleteNetworkPolicy, DeletePod
// all already check first). DeleteWorkload, DeleteRole, and
// DeleteRoleBinding were the three outliers, found by curling the same
// delete endpoint twice in a row.
func TestDeleteWorkloadRejectsMissing(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	if _, err := d.Launch(ctx, LaunchRequest{Namespace: "default", Name: "del-race", Image: "nginx:latest", Replicas: 1}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := d.DeleteWorkload(ctx, "default", "Deployment", "del-race"); err != nil {
		t.Fatalf("first DeleteWorkload should succeed, got %v", err)
	}
	if err := d.DeleteWorkload(ctx, "default", "Deployment", "del-race"); err == nil {
		t.Fatal("expected second DeleteWorkload to fail with not-found, got nil error")
	}
}

func TestDeleteRoleRejectsMissing(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateRoleRequest{Namespace: "default", Name: "del-role-race", Rules: []RBACPolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}}}
	if _, err := d.CreateRole(ctx, req); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := d.DeleteRole(ctx, "default", "del-role-race"); err != nil {
		t.Fatalf("first DeleteRole should succeed, got %v", err)
	}
	if err := d.DeleteRole(ctx, "default", "del-role-race"); err == nil {
		t.Fatal("expected second DeleteRole to fail with not-found, got nil error")
	}
}

func TestDeleteRoleBindingRejectsMissing(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateRoleBindingRequest{Namespace: "default", Name: "del-rb-race", RoleName: "some-role", Subjects: []RBACSubject{{Kind: "User", Name: "demo"}}}
	if _, err := d.CreateRoleBinding(ctx, req); err != nil {
		t.Fatalf("CreateRoleBinding: %v", err)
	}
	if err := d.DeleteRoleBinding(ctx, "default", "del-rb-race"); err != nil {
		t.Fatalf("first DeleteRoleBinding should succeed, got %v", err)
	}
	if err := d.DeleteRoleBinding(ctx, "default", "del-rb-race"); err == nil {
		t.Fatal("expected second DeleteRoleBinding to fail with not-found, got nil error")
	}
}

// Same bug class again, found by extending the duplicate-submission test
// to the remaining named-resource Create dialogs: Velero schedule (a map
// write with no existence check) and Backup (append-only, so a
// duplicate name produced two visually-identical rows rather than a
// silent overwrite, but the same underlying gap).
func TestCreateVeleroScheduleRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateVeleroScheduleRequest{Namespace: "velero", Name: "dup-sched", Schedule: "@daily"}
	if _, err := d.CreateVeleroSchedule(ctx, req); err != nil {
		t.Fatalf("first CreateVeleroSchedule should succeed, got %v", err)
	}
	if _, err := d.CreateVeleroSchedule(ctx, req); err == nil {
		t.Fatal("expected second CreateVeleroSchedule with the same name to fail, got nil error")
	}
}

func TestCreateBackupRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateBackupRequest{Namespace: "default", Name: "dup-backup", PVC: "data-pvc"}
	if _, err := d.CreateBackup(ctx, req); err != nil {
		t.Fatalf("first CreateBackup should succeed, got %v", err)
	}
	if _, err := d.CreateBackup(ctx, req); err == nil {
		t.Fatal("expected second CreateBackup with the same name to fail, got nil error")
	}
	backups, err := d.ListBackups(ctx, "default")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	count := 0
	for _, b := range backups {
		if b.Name == "dup-backup" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 backup named dup-backup, got %d", count)
	}
}

// A duplicate agent-run name is worse than the other duplicate-name bugs:
// ApproveAgentRun/DenyAgentRun look a run up by namespace+name and only
// ever act on the first match, so the second identically-named run was
// left permanently stuck at "Pending" with no way to approve or deny
// it. Confirmed live via curl before fixing.
func TestCreateAgentRunRejectsDuplicateName(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	req := CreateAgentRunRequest{Namespace: "default", Name: "dup-run", Goal: "scale checkout"}
	if _, err := d.CreateAgentRun(ctx, req); err != nil {
		t.Fatalf("first CreateAgentRun should succeed, got %v", err)
	}
	if _, err := d.CreateAgentRun(ctx, req); err == nil {
		t.Fatal("expected second CreateAgentRun with the same name to fail, got nil error")
	}
}

func TestLaunchRejectsHugeReplicaCount(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	_, err := d.Launch(ctx, LaunchRequest{Namespace: "default", Name: "huge", Image: "nginx:latest", Replicas: 999999999})
	if err == nil {
		t.Fatal("expected Launch to reject a replica count above maxDemoReplicas, got nil error")
	}
}

// rebuildPodsLocked itself must never allocate beyond the cap, even if a
// future call site somehow skips the Scale/Launch validation — this is
// the defense-in-depth clamp, tested directly against the internal
// struct field rather than through Scale/Launch's own validation.
func TestRebuildPodsLockedClampsAllocation(t *testing.T) {
	d := NewDemoProvider()
	d.mu.Lock()
	w := Workload{Namespace: "default", Name: "unchecked", Kind: "Deployment", Desired: 999999999}
	d.workloads[key(w.Namespace, w.Kind, w.Name)] = w
	d.rebuildPodsLocked(w)
	d.mu.Unlock()

	pods, err := d.ListPods(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	count := 0
	for _, p := range pods {
		if p.WorkloadID == "unchecked" {
			count++
		}
	}
	if count != maxDemoReplicas {
		t.Fatalf("rebuildPodsLocked should clamp to %d pods even with an unvalidated Desired, got %d", maxDemoReplicas, count)
	}
}
