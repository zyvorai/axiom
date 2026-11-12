//go:build kubernetes

package platform

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCreateVeleroBackupMatchesUpstreamCRD(t *testing.T) {
	ctx := context.Background()
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{veleroBackupGVR: "BackupList"})
	b, err := k.CreateVeleroBackup(ctx, CreateVeleroBackupRequest{
		Namespace: "velero", Name: "crd-check", IncludedNamespaces: []string{"apps"},
		TTL: "720h0m0s", StorageLocation: "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := k.dynamic.Resource(veleroBackupGVR).Namespace("velero").Get(ctx, b.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAgainstVeleroCRD(veleroTestdata("backups.velero.io.yaml"), obj); err != nil {
		t.Fatalf("Backup object rejected by upstream CRD schema: %v\nobject=%v", err, obj.Object)
	}
}

func TestCreateVeleroScheduleMatchesUpstreamCRD(t *testing.T) {
	ctx := context.Background()
	k := newFakeDynamicProvider(map[schema.GroupVersionResource]string{veleroSchedGVR: "ScheduleList"})
	s, err := k.CreateVeleroSchedule(ctx, CreateVeleroScheduleRequest{
		Namespace: "velero", Name: "nightly-crd", Schedule: "0 2 * * *", TTL: "72h",
	})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := k.dynamic.Resource(veleroSchedGVR).Namespace("velero").Get(ctx, s.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAgainstVeleroCRD(veleroTestdata("schedules.velero.io.yaml"), obj); err != nil {
		t.Fatalf("Schedule object rejected by upstream CRD schema: %v\nobject=%v", err, obj.Object)
	}
}
