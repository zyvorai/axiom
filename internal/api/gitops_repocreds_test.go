package api

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/zyvorai/axiom/internal/platform"
)

func apiTestSSHKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blk, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(blk))
}

func credJSON(m map[string]string) string {
	b, _ := json.Marshal(m)
	return string(b)
}

const apiKnownHosts = "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl"

func TestGitOpsCredentialsEndpoints(t *testing.T) {
	e := newGitOpsEnv(t)
	body := credJSON(map[string]string{"engine": "flux", "name": "api-cred", "url": "https://github.com/org/repo.git", "authType": "basic", "username": "bot", "password": "pw-api-1"})
	w := e.do("POST", "/api/v1/gitops/credentials", body, asAdmin())
	if w.Code != http.StatusCreated || strings.TrimSpace(w.Body.String()) != `{"ok":true,"name":"api-cred","namespace":"flux-system","engine":"flux"}` {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/v1/gitops/credentials", body, asAdmin()); w.Code != http.StatusConflict {
		t.Fatalf("dup: %d %s", w.Code, w.Body.String())
	}
	w = e.do("GET", "/api/v1/gitops/credentials?engine=flux", "", asAdmin())
	var list []platform.GitOpsCredential
	decodeInto(t, w, &list)
	found := false
	for _, c := range list {
		found = found || (c.Name == "api-cred" && c.ManagedByAxiom && c.AuthType == "basic" && c.Kind == "git")
	}
	if !found || strings.Contains(w.Body.String(), "pw-api-1") || strings.Contains(w.Body.String(), "bot") {
		t.Fatalf("list: %s", w.Body.String())
	}
	if w := e.do("GET", "/api/v1/gitops/credentials?engine=svn", "", asAdmin()); w.Code != http.StatusBadRequest {
		t.Errorf("bad engine: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/gitops/credentials?engine=argocd&namespace=Not_DNS", "", asAdmin()); w.Code != http.StatusBadRequest {
		t.Errorf("bad namespace: %d", w.Code)
	}
	// Not Axiom's: 409, kept.
	if w := e.do("DELETE", "/api/v1/gitops/credentials/argocd/argocd/repo-platform", "", asAdmin()); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "not created by Axiom") {
		t.Fatalf("foreign delete: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("DELETE", "/api/v1/gitops/credentials/flux/flux-system/api-cred", "", asAdmin()); w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("DELETE", "/api/v1/gitops/credentials/flux/flux-system/api-cred", "", asAdmin()); w.Code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", w.Code)
	}
	// No update route.
	if w := e.do("PUT", "/api/v1/gitops/credentials/flux/flux-system/api-cred", body, asAdmin()); w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("update route exists: %d", w.Code)
	}
	// Body limits and shape.
	huge := credJSON(map[string]string{"engine": "flux", "name": "big", "url": "https://h/o/r", "authType": "basic", "username": "u", "password": strings.Repeat("x", 70<<10)})
	if w := e.do("POST", "/api/v1/gitops/credentials", huge, asAdmin()); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("huge: %d", w.Code)
	}
	for name, b := range map[string]string{
		"unknown field": `{"engine":"flux","name":"x","url":"https://h/o/r","authType":"basic","username":"u","password":"pw-unknown-field","data":{"a":"b"}}`,
		"trailing":      `{"engine":"flux","name":"x"} {"password":"pw-trailing"}`,
		"malformed":     `{"engine":"flux","password":"pw-malformed`,
	} {
		w := e.do("POST", "/api/v1/gitops/credentials", b, asAdmin())
		if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), "pw-") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
}

func TestGitOpsCredentialsAccess(t *testing.T) {
	e := newGitOpsEnv(t)
	body := credJSON(map[string]string{"engine": "flux", "name": "acc-cred", "url": "https://github.com/o/r.git", "authType": "token", "token": "tok-access"})
	creds := map[string][2]string{
		"operator": {"oper1", "oper-password"}, "viewer": {"viewer1", "viewer-password"},
		"scoped": {"scoped", "scoped-password"}, "scopedop": {"scopedop", "scopedop-password"},
	}
	for who, c := range creds {
		for _, r := range [][3]string{
			{"GET", "/api/v1/gitops/credentials", ""},
			{"GET", "/api/v1/gitops/credentials?engine=argocd", ""},
			{"POST", "/api/v1/gitops/credentials", body},
			{"DELETE", "/api/v1/gitops/credentials/argocd/argocd/repo-platform", ""},
		} {
			if w := e.do(r[0], r[1], r[2], withBasic(c[0], c[1])); w.Code != http.StatusForbidden {
				t.Errorf("%s %s %s: %d", who, r[0], r[1], w.Code)
			}
		}
	}
	if w := e.do("GET", "/api/v1/gitops/credentials", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/gitops/credentials", "", asAdmin()); w.Code != http.StatusOK {
		t.Errorf("admin: %d", w.Code)
	}
}

// TestGitOpsCredentialSecretsNowhere posts distinctive secret values through
// every outcome (created, duplicate, validation failure, malformed) and
// asserts the bytes never appear in a response, an audit entry or the log.
func TestGitOpsCredentialSecretsNowhere(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	prevSlog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { log.SetOutput(os.Stderr); slog.SetDefault(prevSlog) })

	e := newGitOpsEnv(t)
	const (
		password = "Pa55-ZETA-9f8e7d6c-NOWHERE"
		token    = "ghp_TOKEN-OMEGA-1a2b3c4d-NOWHERE"
	)
	key := apiTestSSHKey(t)
	keyBody := strings.Split(strings.TrimSpace(key), "\n")[1] // a line of the base64 key material
	secrets := []string{password, token, keyBody}

	var responses []string
	post := func(m map[string]string) {
		w := e.do("POST", "/api/v1/gitops/credentials", credJSON(m), asAdmin())
		responses = append(responses, w.Body.String())
	}
	basic := map[string]string{"engine": "argocd", "name": "nowhere-basic", "url": "https://github.com/o/r.git", "authType": "basic", "username": "bot", "password": password}
	tok := map[string]string{"engine": "flux", "name": "nowhere-token", "url": "https://github.com/o/r.git", "authType": "token", "token": token}
	sshReq := map[string]string{"engine": "flux", "name": "nowhere-ssh", "url": "ssh://git@github.com/o/r.git", "authType": "ssh", "sshPrivateKey": key, "knownHosts": apiKnownHosts}
	post(basic)
	post(tok)
	post(sshReq)
	post(basic) // duplicate
	post(tok)   // duplicate
	// Validation failures with the secrets present.
	post(map[string]string{"engine": "flux", "name": "nowhere-bad1", "url": "https://bot:" + password + "@github.com/o/r.git", "authType": "basic", "username": "bot", "password": password})
	post(map[string]string{"engine": "flux", "name": "nowhere-bad2", "url": "https://github.com/o/r.git", "authType": "basic", "username": "bot", "password": password, "token": token})
	post(map[string]string{"engine": "flux", "name": "nowhere-bad3", "url": "ssh://git@github.com/o/r.git", "authType": "ssh", "sshPrivateKey": key})
	post(map[string]string{"engine": "flux", "name": "nowhere-bad4", "url": "ssh://git@github.com/o/r.git", "authType": "ssh", "sshPrivateKey": key[:len(key)/2] + "\n-----END OPENSSH PRIVATE KEY-----\n", "knownHosts": apiKnownHosts})
	post(map[string]string{"engine": "flux", "name": "Bad Name", "url": "https://github.com/o/r.git", "authType": "token", "token": token})
	w := e.do("POST", "/api/v1/gitops/credentials", `{"engine":"flux","token":"`+token+`","password":`+password, asAdmin())
	responses = append(responses, w.Body.String())

	for _, path := range []string{"/api/v1/gitops/credentials", "/api/v1/gitops/credentials?engine=flux", "/api/v1/gitops/credentials?engine=argocd", "/api/v1/audit"} {
		responses = append(responses, e.do("GET", path, "", asAdmin()).Body.String())
	}
	for _, n := range [][2]string{{"argocd", "nowhere-basic"}, {"flux", "nowhere-token"}, {"flux", "nowhere-ssh"}} {
		ns := "flux-system"
		if n[0] == "argocd" {
			ns = "argocd"
		}
		w := e.do("DELETE", "/api/v1/gitops/credentials/"+n[0]+"/"+ns+"/"+n[1], "", asAdmin())
		if w.Code != http.StatusOK {
			t.Errorf("delete %v: %d %s", n, w.Code, w.Body.String())
		}
		responses = append(responses, w.Body.String())
	}

	var audit strings.Builder
	creates := 0
	for _, a := range platform.ListAxiomAudit() {
		audit.WriteString(a.Action + " " + a.Target + " " + a.Detail + " " + a.User + "\n")
		if a.Action == "gitops.credential_create" && strings.Contains(a.Target, "nowhere-") {
			creates++
			if !strings.HasPrefix(a.Detail, "authType=") {
				t.Errorf("audit detail: %q", a.Detail)
			}
		}
	}
	if creates != 3 {
		t.Errorf("want 3 create audits, got %d:\n%s", creates, audit.String())
	}
	if logs.Len() == 0 {
		t.Fatal("log capture is not working (the request logger writes nothing)")
	}
	for _, s := range secrets {
		for i, r := range responses {
			if strings.Contains(r, s) {
				t.Errorf("response %d leaks %q: %s", i, s, r)
			}
		}
		if strings.Contains(audit.String(), s) {
			t.Errorf("audit leaks %q", s)
		}
		if strings.Contains(logs.String(), s) {
			t.Errorf("log leaks %q", s)
		}
	}
}
