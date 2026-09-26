//go:build kubernetes

package platform

import (
	"context"
	"net/http"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// The demo build maps hand-written validation errors ("X is required",
// "unsupported workload kind") to HTTP 400 through a message heuristic
// (http_status_default.go). The kubernetes build instead only classifies
// *apierrors.StatusError, so every provider-side validation error falls
// through to 500. These are the same requests the demo build answers with 400.
func TestKubernetesValidationErrorsAreReportedAsBadRequest(t *testing.T) {
	ctx := context.Background()
	k, _ := newFakeKubernetesProvider(newDeployment("a", "web", 2), newDaemonSet("a", "agent"))
	kd := newFakeDynamicProvider(map[schema.GroupVersionResource]string{veleroSchedGVR: "ScheduleList", volumeSnapshotGVR: "VolumeSnapshotList"})

	cases := map[string]func() error{
		"namespace blank":     func() error { _, err := k.CreateNamespace(ctx, " "); return err },
		"launch no image":     func() error { _, err := k.Launch(ctx, LaunchRequest{Name: "x"}); return err },
		"launch bad cpu":      func() error { _, err := k.Launch(ctx, LaunchRequest{Name: "x", Image: "y", CPU: "lots"}); return err },
		"scale negative":      func() error { return k.Scale(ctx, "a", "Deployment", "web", -1) },
		"scale unsupported":   func() error { return k.Scale(ctx, "a", "DaemonSet", "agent", 1) },
		"stop unsupported":    func() error { return k.Stop(ctx, "a", "DaemonSet", "agent") },
		"restart unsupported": func() error { return k.Restart(ctx, "a", "Pod", "x") },
		"delete unsupported":  func() error { return k.DeleteWorkload(ctx, "a", "Pod", "x") },
		"rollout unknown":     func() error { return k.RolloutAction(ctx, "a", "Deployment", "web", "explode") },
		"seal unsupported":    func() error { return k.SealWorkload(ctx, "a", "DaemonSet", "agent") },
		"secret no data": func() error {
			_, err := k.CreateSecret(ctx, CreateSecretRequest{Namespace: "a", Name: "s"})
			return err
		},
		"configmap no data": func() error {
			_, err := k.CreateConfigMap(ctx, CreateConfigMapRequest{Namespace: "a", Name: "c"})
			return err
		},
		"hpa missing target": func() error { _, err := k.CreateHPA(ctx, CreateHPARequest{Namespace: "a", Name: "h"}); return err },
		"role no rules":      func() error { _, err := k.CreateRole(ctx, CreateRoleRequest{Namespace: "a", Name: "r"}); return err },
		"rolebinding no roles": func() error {
			_, err := k.CreateRoleBinding(ctx, CreateRoleBindingRequest{Namespace: "a", Name: "r"})
			return err
		},
		"helm missing fields": func() error { _, err := k.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "a"}); return err },
		"velero missing fields": func() error {
			_, err := kd.CreateVeleroSchedule(ctx, CreateVeleroScheduleRequest{Namespace: "v"})
			return err
		},
		"backup missing fields": func() error { _, err := kd.CreateBackup(ctx, CreateBackupRequest{Namespace: "a"}); return err },
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			wantStatus(t, run(), http.StatusBadRequest)
		})
	}
}
