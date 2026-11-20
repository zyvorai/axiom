//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clienttesting "k8s.io/client-go/testing"
)

func credSecret(ns, name string, labels map[string]string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels, UID: types.UID("uid-" + name)}, Data: data}
}

func TestKubeGitOpsCredentials(t *testing.T) {
	ctx := context.Background()
	foreignArgo := credSecret("argocd", "hand-made", map[string]string{ArgoSecretTypeLabel: "repository"},
		map[string][]byte{"url": []byte("https://github.com/org/hand.git"), "password": []byte("FOREIGN-PASS"), "type": []byte("git")})
	unrelated := credSecret("flux-system", "random", nil, map[string][]byte{"password": []byte("UNRELATED-PASS")})
	argoCluster := credSecret("argocd", "cluster-x", map[string]string{ArgoSecretTypeLabel: "cluster"}, map[string][]byte{"config": []byte("CLUSTER-CFG")})
	k, cs := newFakeKubernetesProvider(foreignArgo, unrelated, argoCluster)

	res, err := k.CreateGitOpsCredential(ctx, GitOpsCredentialRequest{Engine: "flux", Name: "web-creds", URL: "https://github.com/org/web.git", AuthType: "token", Token: "TOKEN-XYZ"})
	if err != nil || !res.OK || res.Namespace != "flux-system" {
		t.Fatalf("create: %v %+v", err, res)
	}
	sec, err := cs.CoreV1().Secrets("flux-system").Get(ctx, "web-creds", metav1.GetOptions{})
	if err != nil || sec.Type != corev1.SecretTypeOpaque || sec.Labels[GitOpsManagedByLabel] != GitOpsManagedByValue ||
		sec.Labels[GitOpsCredentialPurposeLabel] != GitOpsCredentialPurposeValue || string(sec.Data["password"]) != "TOKEN-XYZ" ||
		string(sec.Data["username"]) != "git" || len(sec.Data) != 2 {
		t.Fatalf("flux secret: %v %+v", err, sec)
	}
	if _, err := k.CreateGitOpsCredential(ctx, GitOpsCredentialRequest{Engine: "argocd", Name: "argo-creds", URL: "https://github.com/org/web.git", AuthType: "basic", Username: "u", Password: "ARGO-PASS"}); err != nil {
		t.Fatal(err)
	}
	asec, _ := cs.CoreV1().Secrets("argocd").Get(ctx, "argo-creds", metav1.GetOptions{})
	if asec.Labels[ArgoSecretTypeLabel] != "repository" || asec.Labels[GitOpsManagedByLabel] != GitOpsManagedByValue || string(asec.Data["url"]) != "https://github.com/org/web.git" {
		t.Fatalf("argo secret: %+v", asec)
	}

	// Name clash: 409 and the existing Secret is neither read nor changed.
	cs.ClearActions()
	_, err = k.CreateGitOpsCredential(ctx, GitOpsCredentialRequest{Engine: "argocd", Name: "hand-made", URL: "https://github.com/org/x.git", AuthType: "basic", Username: "u", Password: "NEW"})
	if HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("exists: %v", err)
	}
	for _, a := range cs.Actions() {
		if a.GetVerb() != "create" {
			t.Errorf("unexpected %s on clash", a.GetVerb())
		}
	}
	if s, _ := cs.CoreV1().Secrets("argocd").Get(ctx, "hand-made", metav1.GetOptions{}); string(s.Data["password"]) != "FOREIGN-PASS" {
		t.Fatal("existing secret overwritten")
	}

	// Listing: only labelled Secrets, never data.
	list, err := k.ListGitOpsCredentials(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]GitOpsCredential{}
	for _, c := range list {
		names[c.Engine+"/"+c.Name] = c
	}
	if len(list) != 3 || names["argocd/hand-made"].ManagedByAxiom || names["argocd/hand-made"].URL != "https://github.com/org/hand.git" ||
		!names["flux/web-creds"].ManagedByAxiom || names["flux/web-creds"].AuthType != "token" || !names["argocd/argo-creds"].ManagedByAxiom {
		t.Fatalf("list: %+v", list)
	}
	b, _ := json.Marshal(list)
	for _, secret := range []string{"FOREIGN-PASS", "UNRELATED-PASS", "CLUSTER-CFG", "TOKEN-XYZ", "ARGO-PASS", "random", "cluster-x"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("listing leaks %s: %s", secret, b)
		}
	}
	for _, a := range cs.Actions() {
		if l, ok := a.(clienttesting.ListAction); ok && l.GetListRestrictions().Labels.Empty() {
			t.Errorf("unfiltered secret listing in %s", l.GetNamespace())
		}
	}

	// Delete: refused for Secrets Axiom did not label.
	for _, c := range [][3]string{{"argocd", "argocd", "hand-made"}, {"flux", "flux-system", "random"}, {"argocd", "argocd", "cluster-x"}} {
		if err := k.DeleteGitOpsCredential(ctx, c[0], c[1], c[2]); HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "not created by Axiom") {
			t.Errorf("%v: %v", c, err)
		}
		if _, err := cs.CoreV1().Secrets(c[1]).Get(ctx, c[2], metav1.GetOptions{}); err != nil {
			t.Errorf("%v deleted", c)
		}
	}
	if err := k.DeleteGitOpsCredential(ctx, "flux", "flux-system", "web-creds"); err != nil {
		t.Fatal(err)
	}
	if err := k.DeleteGitOpsCredential(ctx, "argocd", "argocd", "argo-creds"); err != nil {
		t.Fatal(err)
	}
	if err := k.DeleteGitOpsCredential(ctx, "flux", "flux-system", "web-creds"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("404: %v", err)
	}
	// A Flux delete can not target an Argo secret by passing engine=flux.
	if err := k.DeleteGitOpsCredential(ctx, "flux", "argocd", "hand-made"); HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("cross-engine: %v", err)
	}
}
