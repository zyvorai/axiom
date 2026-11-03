package platform

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func testSSHKey(t *testing.T, passphrase string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var blk *pem.Block
	if passphrase != "" {
		blk, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	} else {
		blk, err = ssh.MarshalPrivateKey(priv, "")
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(blk))
}

const testKnownHosts = "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl\n"

func TestGitOpsCredentialValidation(t *testing.T) {
	key := testSSHKey(t, "")
	ok := func(mut func(*GitOpsCredentialRequest)) GitOpsCredentialRequest {
		r := GitOpsCredentialRequest{Engine: "flux", Name: "repo-creds", URL: "https://github.com/org/repo.git", AuthType: "basic", Username: "bot", Password: "hunter2"}
		if mut != nil {
			mut(&r)
		}
		return r
	}
	if _, err := buildGitOpsCredential(ok(nil)); err != nil {
		t.Fatalf("valid: %v", err)
	}
	big := strings.Repeat("x", gitOpsCredMaxField+1)
	bad := map[string]func(*GitOpsCredentialRequest){
		"engine":              func(r *GitOpsCredentialRequest) { r.Engine = "helm" },
		"name":                func(r *GitOpsCredentialRequest) { r.Name = "Bad_Name" },
		"namespace":           func(r *GitOpsCredentialRequest) { r.Namespace = "kube system" },
		"kind":                func(r *GitOpsCredentialRequest) { r.Kind = "svn" },
		"no authType":         func(r *GitOpsCredentialRequest) { r.AuthType = "" },
		"bad authType":        func(r *GitOpsCredentialRequest) { r.AuthType = "oauth" },
		"no url":              func(r *GitOpsCredentialRequest) { r.URL = "" },
		"userinfo in url":     func(r *GitOpsCredentialRequest) { r.URL = "https://bot:hunter2@github.com/org/repo.git" },
		"user in https url":   func(r *GitOpsCredentialRequest) { r.URL = "https://bot@github.com/org/repo.git" },
		"file url":            func(r *GitOpsCredentialRequest) { r.URL = "file:///etc/passwd" },
		"query":               func(r *GitOpsCredentialRequest) { r.URL = "https://github.com/org/repo.git?token=x" },
		"basic no password":   func(r *GitOpsCredentialRequest) { r.Password = "" },
		"basic no username":   func(r *GitOpsCredentialRequest) { r.Username = "" },
		"basic with token":    func(r *GitOpsCredentialRequest) { r.Token = "t" },
		"basic with key":      func(r *GitOpsCredentialRequest) { r.SSHPrivateKey = key },
		"token missing":       func(r *GitOpsCredentialRequest) { r.AuthType = "token"; r.Password = "" },
		"token with password": func(r *GitOpsCredentialRequest) { r.AuthType = "token"; r.Token = "t" },
		"ssh no key": func(r *GitOpsCredentialRequest) {
			r.AuthType, r.Username, r.Password, r.URL = "ssh", "", "", "ssh://git@github.com/org/repo.git"
		},
		"ssh https url": func(r *GitOpsCredentialRequest) {
			r.AuthType, r.Username, r.Password, r.SSHPrivateKey, r.KnownHosts = "ssh", "", "", key, testKnownHosts
		},
		"ssh not a key": func(r *GitOpsCredentialRequest) {
			r.AuthType, r.Username, r.Password, r.SSHPrivateKey = "ssh", "", "", "not a key"
		},
		"ssh garbage pem": func(r *GitOpsCredentialRequest) {
			r.AuthType, r.Username, r.Password, r.SSHPrivateKey = "ssh", "", "", "-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n"
		},
		"ssh encrypted": func(r *GitOpsCredentialRequest) {
			r.AuthType, r.Username, r.Password, r.SSHPrivateKey = "ssh", "", "", testSSHKey(t, "pw")
		},
		"ssh flux no knownhst": func(r *GitOpsCredentialRequest) {
			r.AuthType, r.Username, r.Password, r.SSHPrivateKey, r.URL = "ssh", "", "", key, "ssh://git@github.com/o/r.git"
		},
		"ssh flux bad knownh": func(r *GitOpsCredentialRequest) {
			r.AuthType, r.Username, r.Password, r.SSHPrivateKey, r.URL, r.KnownHosts = "ssh", "", "", key, "ssh://git@github.com/o/r.git", "github.com"
		},
		"ssh flux scp url": func(r *GitOpsCredentialRequest) {
			r.AuthType, r.Username, r.Password, r.SSHPrivateKey, r.URL, r.KnownHosts = "ssh", "", "", key, "git@github.com:o/r.git", testKnownHosts
		},
		"ssh argo knownhosts": func(r *GitOpsCredentialRequest) {
			r.Engine, r.AuthType, r.Username, r.Password, r.SSHPrivateKey, r.URL, r.KnownHosts = "argocd", "ssh", "", "", key, "git@github.com:o/r.git", testKnownHosts
		},
		"ssh helm": func(r *GitOpsCredentialRequest) {
			r.Kind, r.AuthType, r.Username, r.Password, r.SSHPrivateKey = "helm", "ssh", "", "", key
		},
		"oci with https":    func(r *GitOpsCredentialRequest) { r.Kind = "oci" },
		"helm with oci url": func(r *GitOpsCredentialRequest) { r.Kind, r.URL = "helm", "oci://ghcr.io/org/charts" },
		"password too big":  func(r *GitOpsCredentialRequest) { r.Password = big },
		"key too big": func(r *GitOpsCredentialRequest) {
			r.AuthType, r.Username, r.Password, r.SSHPrivateKey = "ssh", "", "", big
		},
		"username spaces":  func(r *GitOpsCredentialRequest) { r.Username = "a b" },
		"password newline": func(r *GitOpsCredentialRequest) { r.Password = "a\nb" },
	}
	for name, mut := range bad {
		_, err := buildGitOpsCredential(ok(mut))
		if HTTPStatus(err) != http.StatusBadRequest {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "AAAA") {
			t.Errorf("%s: error leaks a secret: %v", name, err)
		}
	}
}

func TestGitOpsCredentialSecretShape(t *testing.T) {
	key := testSSHKey(t, "")
	// Flux basic.
	s, err := buildGitOpsCredential(GitOpsCredentialRequest{Engine: "flux", Name: "c1", URL: "https://github.com/o/r.git", AuthType: "basic", Username: "u", Password: "p"})
	if err != nil || s.Namespace != "flux-system" || string(s.Data["username"]) != "u" || string(s.Data["password"]) != "p" || len(s.Data) != 2 ||
		s.Labels[GitOpsManagedByLabel] != GitOpsManagedByValue || s.Labels[GitOpsCredentialPurposeLabel] != GitOpsCredentialPurposeValue ||
		s.Labels[ArgoSecretTypeLabel] != "" {
		t.Fatalf("flux basic: %v %+v %v", err, s.Labels, len(s.Data))
	}
	// Flux token: basic-auth with the token as password.
	s, _ = buildGitOpsCredential(GitOpsCredentialRequest{Engine: "flux", Name: "c2", URL: "https://github.com/o/r.git", AuthType: "token", Token: "tok"})
	if string(s.Data["username"]) != "git" || string(s.Data["password"]) != "tok" {
		t.Fatalf("flux token: %v", s.Data)
	}
	// Flux ssh.
	s, err = buildGitOpsCredential(GitOpsCredentialRequest{Engine: "flux", Name: "c3", Namespace: "apps", URL: "ssh://git@github.com/o/r.git", AuthType: "ssh", SSHPrivateKey: key, KnownHosts: testKnownHosts})
	if err != nil || s.Namespace != "apps" || string(s.Data["identity"]) != key || string(s.Data["known_hosts"]) != testKnownHosts || len(s.Data) != 2 {
		t.Fatalf("flux ssh: %v", err)
	}
	// Argo basic.
	s, _ = buildGitOpsCredential(GitOpsCredentialRequest{Engine: "argocd", Name: "c4", URL: "https://github.com/o/r.git", AuthType: "basic", Username: "u", Password: "p"})
	if s.Namespace != "argocd" || s.Labels[ArgoSecretTypeLabel] != "repository" || s.Labels[GitOpsManagedByLabel] != GitOpsManagedByValue ||
		string(s.Data["type"]) != "git" || string(s.Data["url"]) != "https://github.com/o/r.git" || string(s.Data["name"]) != "c4" ||
		string(s.Data["username"]) != "u" || string(s.Data["password"]) != "p" {
		t.Fatalf("argo basic: %+v", s.Labels)
	}
	// Argo ssh with scp-like URL.
	s, err = buildGitOpsCredential(GitOpsCredentialRequest{Engine: "argocd", Name: "c5", URL: "git@github.com:o/r.git", AuthType: "ssh", SSHPrivateKey: key})
	if err != nil || string(s.Data["sshPrivateKey"]) != key || s.Data["password"] != nil {
		t.Fatalf("argo ssh: %v", err)
	}
	// Argo OCI helm.
	s, err = buildGitOpsCredential(GitOpsCredentialRequest{Engine: "argocd", Name: "c6", Kind: "oci", URL: "oci://ghcr.io/org/charts", AuthType: "token", Token: "t"})
	if err != nil || string(s.Data["type"]) != "helm" || string(s.Data["enableOCI"]) != "true" || string(s.Data["url"]) != "ghcr.io/org/charts" {
		t.Fatalf("argo oci: %v", err)
	}
	// Formatting never prints the data.
	r := GitOpsCredentialRequest{Engine: "flux", Name: "x", Password: "hunter2", Token: "tok-123", SSHPrivateKey: key}
	for _, out := range []string{fmt.Sprint(r), fmt.Sprintf("%v %+v %#v %s", r, r, r, r), fmt.Sprintf("%v %+v %#v", s, s, s)} {
		if strings.Contains(out, "hunter2") || strings.Contains(out, "tok-123") || strings.Contains(out, "PRIVATE KEY") || strings.Contains(out, "ghcr") {
			t.Errorf("formatting leaks: %s", out)
		}
	}
	s.wipe()
	for k, b := range s.Data {
		for _, c := range b {
			if c != 0 {
				t.Fatalf("wipe left %s", k)
			}
		}
	}
}

func TestGitOpsCredentialDeletableAndSummary(t *testing.T) {
	m := GitOpsManagedByLabel
	for _, c := range []struct {
		engine string
		labels map[string]string
		ok     bool
	}{
		{"flux", map[string]string{m: GitOpsManagedByValue, GitOpsCredentialPurposeLabel: GitOpsCredentialPurposeValue}, true},
		{"flux", map[string]string{m: GitOpsManagedByValue}, false},
		{"flux", map[string]string{}, false},
		{"argocd", map[string]string{ArgoSecretTypeLabel: "repository", m: GitOpsManagedByValue}, true},
		{"argocd", map[string]string{ArgoSecretTypeLabel: "repo-creds", m: GitOpsManagedByValue}, true},
		{"argocd", map[string]string{ArgoSecretTypeLabel: "repository"}, false},
		{"argocd", map[string]string{ArgoSecretTypeLabel: "cluster", m: GitOpsManagedByValue}, false},
	} {
		err := checkGitOpsCredentialDeletable(c.engine, c.labels)
		if (err == nil) != c.ok || (err != nil && (HTTPStatus(err) != http.StatusConflict || !strings.Contains(err.Error(), "not created by Axiom"))) {
			t.Errorf("%s %v: %v", c.engine, c.labels, err)
		}
	}
	sum := gitOpsCredentialSummary("argocd", "argocd", "r", map[string]string{ArgoSecretTypeLabel: "repository"}, nil,
		map[string][]byte{"url": []byte("https://h/o/r"), "type": []byte("helm"), "enableOCI": []byte("true"), "password": []byte("hunter2")}, "3d")
	if sum.URL != "https://h/o/r" || sum.Kind != "oci" || sum.AuthType != "basic" || sum.ManagedByAxiom || sum.Age != "3d" {
		t.Errorf("summary: %+v", sum)
	}
	if strings.Contains(fmt.Sprintf("%+v", sum), "hunter2") {
		t.Error("summary leaks")
	}
	if _, err := gitOpsCredListScopes("", "argocd"); HTTPStatus(err) != http.StatusBadRequest {
		t.Errorf("namespace without engine: %v", err)
	}
	if _, err := gitOpsCredListScopes("flux", "Bad NS"); HTTPStatus(err) != http.StatusBadRequest {
		t.Errorf("bad namespace: %v", err)
	}
}

func TestDemoGitOpsCredentials(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	res, err := d.CreateGitOpsCredential(ctx, GitOpsCredentialRequest{Engine: "flux", Name: "demo-cred", URL: "https://github.com/o/r.git", AuthType: "token", Token: "tok-demo"})
	if err != nil || !res.OK || res.Namespace != "flux-system" {
		t.Fatalf("create: %v %+v", err, res)
	}
	if _, err := d.CreateGitOpsCredential(ctx, GitOpsCredentialRequest{Engine: "flux", Name: "demo-cred", URL: "https://github.com/o/r.git", AuthType: "token", Token: "x"}); HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("dup: %v", err)
	}
	list, err := d.ListGitOpsCredentials(ctx, "flux", "")
	if err != nil || len(list) != 1 || list[0].Name != "demo-cred" || !list[0].ManagedByAxiom || list[0].AuthType != "token" || list[0].URL != "https://github.com/o/r.git" {
		t.Fatalf("list: %v %+v", err, list)
	}
	all, _ := d.ListGitOpsCredentials(ctx, "", "")
	if len(all) < 2 {
		t.Fatalf("all: %+v", all)
	}
	if err := d.DeleteGitOpsCredential(ctx, "argocd", "argocd", "repo-platform"); HTTPStatus(err) != http.StatusConflict {
		t.Fatalf("delete foreign: %v", err)
	}
	if err := d.DeleteGitOpsCredential(ctx, "flux", "flux-system", "demo-cred"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := d.DeleteGitOpsCredential(ctx, "flux", "flux-system", "demo-cred"); HTTPStatus(err) != http.StatusNotFound {
		t.Fatalf("delete twice: %v", err)
	}
}
