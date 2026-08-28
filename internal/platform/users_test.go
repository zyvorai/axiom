package platform

import (
	"context"
	"testing"
)

func TestUserHubCreateAuthenticateDelete(t *testing.T) {
	hub := NewUserHub(NewMemoryUserStore())
	ctx := context.Background()

	if _, err := hub.Create(ctx, "alice", "short", "viewer"); err == nil {
		t.Fatal("expected error for too-short password")
	}

	v, err := hub.Create(ctx, "alice", "longenoughpass", "viewer")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if v.Role != RoleViewer {
		t.Fatalf("expected role %q, got %q", RoleViewer, v.Role)
	}

	if _, err := hub.Create(ctx, "alice", "longenoughpass", "viewer"); err == nil {
		t.Fatal("expected error creating duplicate user")
	}

	if role, ok := hub.Authenticate("alice", "longenoughpass"); !ok || role != RoleViewer {
		t.Fatalf("expected successful auth as viewer, got role=%q ok=%v", role, ok)
	}
	if _, ok := hub.Authenticate("alice", "wrongpass"); ok {
		t.Fatal("expected auth failure for wrong password")
	}
	if _, ok := hub.Authenticate("nobody", "whatever"); ok {
		t.Fatal("expected auth failure for unknown user")
	}

	if err := hub.Delete(ctx, "alice"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := hub.Authenticate("alice", "longenoughpass"); ok {
		t.Fatal("expected auth failure after delete")
	}
	if err := hub.Delete(ctx, "alice"); err == nil {
		t.Fatal("expected error deleting already-deleted user")
	}
}

func TestUserHubUpdateRoleAndResetPassword(t *testing.T) {
	hub := NewUserHub(NewMemoryUserStore())
	ctx := context.Background()

	if _, err := hub.Create(ctx, "alice", "originalpass", "viewer"); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := hub.UpdateRole(ctx, "nobody", "admin"); err == nil {
		t.Fatal("expected error updating role of unknown user")
	}

	v, err := hub.UpdateRole(ctx, "alice", "admin")
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
	if v.Role != RoleAdmin {
		t.Fatalf("expected role %q after update, got %q", RoleAdmin, v.Role)
	}
	if role, ok := hub.Authenticate("alice", "originalpass"); !ok || role != RoleAdmin {
		t.Fatalf("expected alice to authenticate as admin after role change, got role=%q ok=%v", role, ok)
	}

	if _, err := hub.ResetPassword(ctx, "alice", "short"); err == nil {
		t.Fatal("expected error for too-short new password")
	}

	if _, err := hub.ResetPassword(ctx, "alice", "brandnewpassword"); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if _, ok := hub.Authenticate("alice", "originalpass"); ok {
		t.Fatal("expected old password to stop working after reset")
	}
	if role, ok := hub.Authenticate("alice", "brandnewpassword"); !ok || role != RoleAdmin {
		t.Fatalf("expected new password to authenticate as admin, got role=%q ok=%v", role, ok)
	}
}

func TestNormalizeRole(t *testing.T) {
	cases := map[string]string{
		"viewer": RoleViewer,
		"Viewer": RoleViewer,
		" VIEWER ": RoleViewer,
		"admin":  RoleAdmin,
		"":       RoleAdmin,
		"bogus":  RoleAdmin,
	}
	for in, want := range cases {
		if got := NormalizeRole(in); got != want {
			t.Errorf("NormalizeRole(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSwitchableProviderRegisterAndSwitch(t *testing.T) {
	sp := NewSwitchableProvider()
	a := NewDemoProvider()
	b := NewDemoProvider()
	sp.Register("a", "Cluster A", a, "server-a")
	sp.Register("b", "Cluster B", b, "server-b")

	if sp.ActiveName() != "a" {
		t.Fatalf("expected first-registered cluster active, got %q", sp.ActiveName())
	}
	if sp.Count() != 2 {
		t.Fatalf("expected 2 clusters, got %d", sp.Count())
	}

	ov, err := sp.Overview(context.Background())
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	_ = ov

	if err := sp.SetActive("b"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if sp.ActiveName() != "b" {
		t.Fatalf("expected active b, got %q", sp.ActiveName())
	}

	if err := sp.SetActive("nonexistent"); err == nil {
		t.Fatal("expected error switching to unknown cluster")
	}

	clusters := sp.Clusters()
	if len(clusters) != 2 {
		t.Fatalf("expected 2 cluster infos, got %d", len(clusters))
	}
	var currentCount int
	for _, c := range clusters {
		if c.Current {
			currentCount++
			if c.Name != "b" {
				t.Fatalf("expected current cluster to be b, got %q", c.Name)
			}
		}
	}
	if currentCount != 1 {
		t.Fatalf("expected exactly one current cluster, got %d", currentCount)
	}
}

func TestClusterLabeledProviderOverridesClusterName(t *testing.T) {
	wrapped := &clusterLabeledProvider{Provider: NewDemoProvider(), label: "custom-label"}
	ov, err := wrapped.Overview(context.Background())
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if ov.Cluster != "custom-label" {
		t.Fatalf("expected cluster label override, got %q", ov.Cluster)
	}
}
