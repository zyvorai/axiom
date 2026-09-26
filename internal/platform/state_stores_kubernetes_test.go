//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Local user accounts and integration settings are persisted by the Kubernetes
// build in a Secret / ConfigMap+Secret. These tests drive the same UserHub /
// IntegrationStore code the HTTP API uses, over a fake clientset.

func newUserHubOnFake(t *testing.T) (*UserHub, *K8sUserStore) {
	t.Helper()
	_, cs := newFakeKubernetesProvider()
	store := NewK8sUserStore(cs, "axiom-system")
	hub := NewUserHub(store)
	return hub, store
}

func storedUsers(t *testing.T, store *K8sUserStore) map[string]User {
	t.Helper()
	sec, err := store.client.CoreV1().Secrets(store.ns).Get(context.Background(), usersSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("users secret not stored: %v", err)
	}
	out := map[string]User{}
	if err := json.Unmarshal(sec.Data["users.json"], &out); err != nil {
		t.Fatalf("users.json is not valid JSON: %v", err)
	}
	return out
}

func TestKubernetesUserHubCreatePersistsHashedUserInSecret(t *testing.T) {
	ctx := context.Background()
	hub, store := newUserHubOnFake(t)

	view, err := hub.Create(ctx, "Alice", "correct-horse", "operator", []string{"dev", "dev", " qa "})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.Username != "Alice" || view.Role != RoleOperator {
		t.Fatalf("unexpected view: %+v", view)
	}
	stored := storedUsers(t, store)
	u, ok := stored["alice"]
	if !ok || len(stored) != 1 {
		t.Fatalf("expected exactly one user keyed by lower-cased name, got %+v", stored)
	}
	if u.PasswordHash == "" || strings.Contains(u.PasswordHash, "correct-horse") || !CheckPassword(u.PasswordHash, "correct-horse") {
		t.Fatalf("password must be stored only as a bcrypt hash: %q", u.PasswordHash)
	}
	if len(u.Namespaces) != 2 || u.Namespaces[0] != "dev" || u.Namespaces[1] != "qa" {
		t.Fatalf("namespaces should be trimmed and de-duplicated: %+v", u.Namespaces)
	}
	sec, _ := store.client.CoreV1().Secrets(store.ns).Get(ctx, usersSecretName, metav1.GetOptions{})
	if strings.Contains(string(sec.Data["users.json"]), "correct-horse") {
		t.Fatal("the plaintext password must never be persisted")
	}
	if other, _ := store.client.CoreV1().Secrets("default").List(ctx, metav1.ListOptions{}); len(other.Items) != 0 {
		t.Fatal("the users secret must live only in the store's namespace")
	}
}

func TestKubernetesUserHubSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	hub, store := newUserHubOnFake(t)
	if _, err := hub.Create(ctx, "bob", "longenough1", "viewer", []string{"dev"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// A brand new hub over the same Secret (i.e. after a pod restart) must know the user.
	fresh := NewUserHub(NewK8sUserStore(store.client, store.ns))
	if err := fresh.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	role, nss, ok := fresh.Authenticate("BOB", "longenough1")
	if !ok || role != RoleViewer || len(nss) != 1 || nss[0] != "dev" {
		t.Fatalf("user did not survive a restart: role=%q ns=%v ok=%v", role, nss, ok)
	}
	if _, _, ok := fresh.Authenticate("bob", "wrong-password"); ok {
		t.Fatal("wrong password must not authenticate")
	}
}

func TestKubernetesUserHubUpdateRoleNamespacesAndPassword(t *testing.T) {
	ctx := context.Background()
	hub, store := newUserHubOnFake(t)
	if _, err := hub.Create(ctx, "carol", "originalpass", "viewer", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := hub.UpdateNamespaces(ctx, "carol", []string{"prod"}); err != nil {
		t.Fatalf("UpdateNamespaces: %v", err)
	}
	if got := storedUsers(t, store)["carol"].Namespaces; len(got) != 1 || got[0] != "prod" {
		t.Fatalf("namespace restriction not persisted: %+v", got)
	}
	if _, err := hub.ResetPassword(ctx, "carol", "brandnewpass"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if h := storedUsers(t, store)["carol"].PasswordHash; !CheckPassword(h, "brandnewpass") || CheckPassword(h, "originalpass") {
		t.Fatal("password reset not persisted")
	}
	if _, err := hub.UpdateRole(ctx, "carol", "admin"); err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	got := storedUsers(t, store)["carol"]
	if got.Role != RoleAdmin || len(got.Namespaces) != 0 {
		t.Fatalf("promotion to admin must persist and clear the namespace restriction: %+v", got)
	}
}

func TestKubernetesUserHubDeleteRemovesFromSecretAndKeepsOthers(t *testing.T) {
	ctx := context.Background()
	hub, store := newUserHubOnFake(t)
	for _, name := range []string{"dave", "erin"} {
		if _, err := hub.Create(ctx, name, "longenough1", "viewer", nil); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}
	if err := hub.Delete(ctx, "DAVE"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	stored := storedUsers(t, store)
	if _, has := stored["dave"]; has || len(stored) != 1 || stored["erin"].Username != "erin" {
		t.Fatalf("expected only erin to remain: %+v", stored)
	}
	if _, _, ok := hub.Authenticate("dave", "longenough1"); ok {
		t.Fatal("deleted user must no longer authenticate")
	}
	wantStatus(t, hub.Delete(ctx, "dave"), http.StatusNotFound)
}

func TestKubernetesUserStoreUpdateAndDeleteUnknownUserAreNotFound(t *testing.T) {
	_, store := newUserHubOnFake(t)
	ctx := context.Background()
	wantStatus(t, store.Update(ctx, User{Username: "ghost"}), http.StatusNotFound)
	wantStatus(t, store.Delete(ctx, "ghost"), http.StatusNotFound)
	if _, err := store.client.CoreV1().Secrets(store.ns).Get(ctx, usersSecretName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("failed operations must not create the users secret, err=%v", err)
	}
}

func TestKubernetesUserHubDuplicateUserIsConflict(t *testing.T) {
	ctx := context.Background()
	hub, _ := newUserHubOnFake(t)
	if _, err := hub.Create(ctx, "frank", "longenough1", "viewer", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := hub.Create(ctx, "FRANK", "longenough1", "viewer", nil)
	wantStatus(t, err, http.StatusConflict)
}

func TestKubernetesUserHubInvalidInputIsBadRequest(t *testing.T) {
	ctx := context.Background()
	hub, _ := newUserHubOnFake(t)
	_, err := hub.Create(ctx, "  ", "longenough1", "viewer", nil)
	wantStatus(t, err, http.StatusBadRequest)
	_, err = hub.Create(ctx, "grace", "short", "viewer", nil)
	wantStatus(t, err, http.StatusBadRequest)
}

func TestKubernetesUserHubUpdateUnknownUserIsNotFound(t *testing.T) {
	ctx := context.Background()
	hub, _ := newUserHubOnFake(t)
	_, err := hub.UpdateRole(ctx, "ghost", "admin")
	wantStatus(t, err, http.StatusNotFound)
	_, err = hub.ResetPassword(ctx, "ghost", "longenough1")
	wantStatus(t, err, http.StatusNotFound)
	_, err = hub.UpdateNamespaces(ctx, "ghost", []string{"x"})
	wantStatus(t, err, http.StatusNotFound)
}

// --- Integrations (ConfigMap + Secret) ---

func TestKubernetesIntegrationStoreSavePutsSecretsOnlyInSecret(t *testing.T) {
	ctx := context.Background()
	_, cs := newFakeKubernetesProvider()
	store := NewK8sIntegrationStore(cs, "axiom-system")

	if _, have, err := store.Load(ctx); err != nil || have {
		t.Fatalf("nothing saved yet: have=%v err=%v", have, err)
	}
	in := Integrations{
		AtlasURL: "https://atlas.example/", AtlasToken: "atlas-secret-token", AtlasConsoleURL: "https://console.example",
		PostgresHost: "pg.example", PostgresPort: 6543, PostgresDatabase: "axiom", PostgresUser: "axiom", PostgresPassword: "pg-secret-pass",
		OIDCIssuer: "https://idp.example", OIDCClientID: "axiom-ui",
	}
	if err := store.Save(ctx, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cm, err := cs.CoreV1().ConfigMaps("axiom-system").Get(ctx, integrationsConfigMap, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("configmap not stored: %v", err)
	}
	for k, v := range cm.Data {
		if strings.Contains(v, "atlas-secret-token") || strings.Contains(v, "pg-secret-pass") {
			t.Fatalf("secret material leaked into the ConfigMap key %q", k)
		}
	}
	if cm.Data["atlasUrl"] != "https://atlas.example" {
		t.Fatalf("atlas url should be normalised (trailing slash trimmed): %q", cm.Data["atlasUrl"])
	}
	sec, err := cs.CoreV1().Secrets("axiom-system").Get(ctx, integrationsSecret, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("secret not stored: %v", err)
	}
	if string(sec.Data["atlasToken"]) != "atlas-secret-token" || string(sec.Data["postgresPassword"]) != "pg-secret-pass" {
		t.Fatalf("secrets not stored in the Secret: %+v", sec.Data)
	}

	got, have, err := store.Load(ctx)
	if err != nil || !have {
		t.Fatalf("Load: have=%v err=%v", have, err)
	}
	if got.AtlasURL != "https://atlas.example" || got.AtlasToken != "atlas-secret-token" || got.PostgresPassword != "pg-secret-pass" ||
		got.PostgresPort != 6543 || got.PostgresHost != "pg.example" || got.OIDCClientID != "axiom-ui" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Saving again updates in place (no AlreadyExists) and can rotate a secret.
	in.AtlasToken = "rotated"
	if err := store.Save(ctx, in); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	got, _, _ = store.Load(ctx)
	if got.AtlasToken != "rotated" {
		t.Fatalf("second save did not update the token: %+v", got)
	}
}

func TestKubernetesIntegrationStorePersistsNotificationSettings(t *testing.T) {
	ctx := context.Background()
	_, cs := newFakeKubernetesProvider()
	store := NewK8sIntegrationStore(cs, "axiom-system")
	in := Integrations{SlackWebhookURL: "https://hooks.slack.example/T/B/x", AlertWebhookURL: "https://alerts.example/hook", NotifyOnWarnings: true}
	if err := store.Save(ctx, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _, err := NewK8sIntegrationStore(cs, "axiom-system").Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SlackWebhookURL != in.SlackWebhookURL || got.AlertWebhookURL != in.AlertWebhookURL || !got.NotifyOnWarnings {
		t.Fatalf("notification settings lost across a reload: %+v", got)
	}
}
