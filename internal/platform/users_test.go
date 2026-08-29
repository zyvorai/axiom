package platform

import (
	"context"
	"testing"
)

func TestUserHubCreateAuthenticateDelete(t *testing.T) {
	hub := NewUserHub(NewMemoryUserStore())
	ctx := context.Background()

	if _, err := hub.Create(ctx, "alice", "short", "viewer", nil); err == nil {
		t.Fatal("expected error for too-short password")
	}

	v, err := hub.Create(ctx, "alice", "longenoughpass", "viewer", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if v.Role != RoleViewer {
		t.Fatalf("expected role %q, got %q", RoleViewer, v.Role)
	}

	if _, err := hub.Create(ctx, "alice", "longenoughpass", "viewer", nil); err == nil {
		t.Fatal("expected error creating duplicate user")
	}

	if role, _, ok := hub.Authenticate("alice", "longenoughpass"); !ok || role != RoleViewer {
		t.Fatalf("expected successful auth as viewer, got role=%q ok=%v", role, ok)
	}
	if _, _, ok := hub.Authenticate("alice", "wrongpass"); ok {
		t.Fatal("expected auth failure for wrong password")
	}
	if _, _, ok := hub.Authenticate("nobody", "whatever"); ok {
		t.Fatal("expected auth failure for unknown user")
	}

	if err := hub.Delete(ctx, "alice"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, ok := hub.Authenticate("alice", "longenoughpass"); ok {
		t.Fatal("expected auth failure after delete")
	}
	if err := hub.Delete(ctx, "alice"); err == nil {
		t.Fatal("expected error deleting already-deleted user")
	}
}

func TestUserHubUpdateRoleAndResetPassword(t *testing.T) {
	hub := NewUserHub(NewMemoryUserStore())
	ctx := context.Background()

	if _, err := hub.Create(ctx, "alice", "originalpass", "viewer", nil); err != nil {
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
	if role, _, ok := hub.Authenticate("alice", "originalpass"); !ok || role != RoleAdmin {
		t.Fatalf("expected alice to authenticate as admin after role change, got role=%q ok=%v", role, ok)
	}

	if _, err := hub.ResetPassword(ctx, "alice", "short"); err == nil {
		t.Fatal("expected error for too-short new password")
	}

	if _, err := hub.ResetPassword(ctx, "alice", "brandnewpassword"); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if _, _, ok := hub.Authenticate("alice", "originalpass"); ok {
		t.Fatal("expected old password to stop working after reset")
	}
	if role, _, ok := hub.Authenticate("alice", "brandnewpassword"); !ok || role != RoleAdmin {
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

	ctx := context.Background()
	if sp.ActiveName(ctx) != "a" {
		t.Fatalf("expected first-registered cluster active, got %q", sp.ActiveName(ctx))
	}
	if sp.Count() != 2 {
		t.Fatalf("expected 2 clusters, got %d", sp.Count())
	}

	ov, err := sp.Overview(ctx)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	_ = ov

	if err := sp.SetActive(ctx, "b"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if sp.ActiveName(ctx) != "b" {
		t.Fatalf("expected active b, got %q", sp.ActiveName(ctx))
	}

	if err := sp.SetActive(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error switching to unknown cluster")
	}

	clusters := sp.Clusters(ctx)
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

func TestUserHubNamespaceRestriction(t *testing.T) {
	hub := NewUserHub(NewMemoryUserStore())
	ctx := context.Background()

	v, err := hub.Create(ctx, "alice", "longenoughpass", "viewer", []string{"team-a", "team-b", "team-a"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(v.Namespaces) != 2 {
		t.Fatalf("expected deduped namespaces [team-a team-b], got %v", v.Namespaces)
	}
	_, namespaces, ok := hub.Authenticate("alice", "longenoughpass")
	if !ok {
		t.Fatal("expected successful auth")
	}
	if len(namespaces) != 2 {
		t.Fatalf("expected 2 namespaces from Authenticate, got %v", namespaces)
	}

	// Promoting to admin must clear the namespace restriction.
	v, err = hub.UpdateRole(ctx, "alice", "admin")
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
	if len(v.Namespaces) != 0 {
		t.Fatalf("expected namespaces cleared after promotion to admin, got %v", v.Namespaces)
	}

	// Can't namespace-restrict an admin account.
	if _, err := hub.UpdateNamespaces(ctx, "alice", []string{"team-a"}); err == nil {
		t.Fatal("expected error restricting an admin account to namespaces")
	}

	// Demote back to viewer, then restriction should work again.
	if _, err := hub.UpdateRole(ctx, "alice", "viewer"); err != nil {
		t.Fatalf("update role back to viewer: %v", err)
	}
	v, err = hub.UpdateNamespaces(ctx, "alice", []string{"team-c"})
	if err != nil {
		t.Fatalf("update namespaces: %v", err)
	}
	if len(v.Namespaces) != 1 || v.Namespaces[0] != "team-c" {
		t.Fatalf("expected [team-c], got %v", v.Namespaces)
	}

	// Passing an empty slice removes the restriction.
	v, err = hub.UpdateNamespaces(ctx, "alice", []string{})
	if err != nil {
		t.Fatalf("clear namespaces: %v", err)
	}
	if len(v.Namespaces) != 0 {
		t.Fatalf("expected namespaces cleared, got %v", v.Namespaces)
	}
}

func TestUserHubCreateAdminIgnoresNamespaces(t *testing.T) {
	hub := NewUserHub(NewMemoryUserStore())
	ctx := context.Background()

	v, err := hub.Create(ctx, "bob", "longenoughpass", "admin", []string{"team-a"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(v.Namespaces) != 0 {
		t.Fatalf("expected admin account to ignore namespaces, got %v", v.Namespaces)
	}
}
