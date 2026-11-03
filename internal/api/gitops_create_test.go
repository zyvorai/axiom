package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/auth"
	"github.com/zyvorai/axiom/internal/platform"
)

func newGitOpsEnv(t *testing.T) *idEnv {
	t.Helper()
	e := newIDEnv(t, auth.Guard{}, "")
	e.addUser(t, "viewer1", "viewer-password", "viewer")
	e.addUser(t, "oper1", "oper-password", "operator")
	e.addUser(t, "scoped", "scoped-password", "viewer", "payments")
	e.addUser(t, "scopedop", "scopedop-password", "operator", "payments")
	return e
}

func gitOpsBody(name string) string {
	return `{"engine":"flux-kustomize","name":"` + name + `","source":{"repoUrl":"https://github.com/org/repo.git","path":"deploy/prod","revision":"v1.0.0"},` +
		`"destination":{"namespace":"prod"},"interval":"5m","syncPolicy":{"prune":true}}`
}

func asAdmin() reqOpt { return withBasic(idAdminUser, idAdminPass) }

func TestGitOpsPreviewEndpoint(t *testing.T) {
	e := newGitOpsEnv(t)
	w := e.do("POST", "/api/v1/gitops/preview", gitOpsBody("preview-app"), asAdmin())
	if w.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var p platform.GitOpsPreview
	decodeInto(t, w, &p)
	if len(p.Objects) != 2 || p.Objects[0].Kind != "GitRepository" || p.Objects[1].Kind != "Kustomization" {
		t.Fatalf("objects: %+v", p.Objects)
	}
	if strings.Count(p.YAML, "\n---\n") != 1 || !strings.Contains(p.YAML, "kind: GitRepository") || !strings.Contains(p.YAML, "path: ./deploy/prod") {
		t.Fatalf("yaml: %s", p.YAML)
	}
	if !strings.Contains(w.Body.String(), `"warnings":[]`) {
		t.Errorf("warnings must be a JSON list: %s", w.Body.String())
	}
	// Preview is pure: nothing was created.
	var items []platform.GitOpsItem
	decodeInto(t, e.do("GET", "/api/v1/gitops", "", asAdmin()), &items)
	for _, it := range items {
		if it.Name == "preview-app" {
			t.Fatal("preview created an object")
		}
	}
}

func TestGitOpsCreateEndpoint(t *testing.T) {
	e := newGitOpsEnv(t)
	w := e.do("POST", "/api/v1/gitops", gitOpsBody("created-app"), asAdmin())
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var res platform.GitOpsCreateResult
	decodeInto(t, w, &res)
	if !res.OK || len(res.Created) != 2 || res.Created[1].Kind != "Kustomization" || res.Created[1].Namespace != "flux-system" {
		t.Fatalf("result: %+v", res)
	}
	var items []platform.GitOpsItem
	decodeInto(t, e.do("GET", "/api/v1/gitops", "", asAdmin()), &items)
	found := false
	for _, it := range items {
		found = found || (it.Name == "created-app" && it.Kind == "Kustomization")
	}
	if !found {
		t.Fatalf("not listed: %+v", items)
	}
	audited := false
	for _, a := range platform.ListAxiomAudit() {
		if a.Action == "gitops.create" && a.Target == "Kustomization/flux-system/created-app" && a.Detail == "flux-kustomize" && a.User == idAdminUser {
			audited = true
		}
	}
	if !audited {
		t.Errorf("no audit entry: %+v", platform.ListAxiomAudit())
	}
	// Duplicate.
	w = e.do("POST", "/api/v1/gitops", gitOpsBody("created-app"), asAdmin())
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "already exists") {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body.String())
	}
}

func TestGitOpsCreateBadInput(t *testing.T) {
	e := newGitOpsEnv(t)
	cases := map[string]string{
		"malformed json":  `{"engine":`,
		"unknown field":   `{"engine":"argocd","name":"x","source":{"repoUrl":"https://h/o/r"},"spec":{"anything":true}}`,
		"raw manifest":    `{"engine":"argocd","name":"x","yaml":"kind: Pod","source":{"repoUrl":"https://h/o/r"}}`,
		"bad engine":      `{"engine":"helm","name":"x","source":{"repoUrl":"https://h/o/r"}}`,
		"bad name":        `{"engine":"argocd","name":"Not_DNS","source":{"repoUrl":"https://h/o/r"}}`,
		"bad url":         `{"engine":"argocd","name":"x","source":{"repoUrl":"file:///etc/passwd"}}`,
		"escaping path":   `{"engine":"argocd","name":"x","source":{"repoUrl":"https://h/o/r","path":"../../x"}}`,
		"bad interval":    `{"engine":"flux-kustomize","name":"x","source":{"repoUrl":"https://h/o/r"},"interval":"1s"}`,
		"bad values":      `{"engine":"flux-helm","name":"x","source":{"repoUrl":"https://h/c","chart":"c"},"values":"- a"}`,
		"inconsistent":    `{"engine":"argocd","name":"x","source":{"repoUrl":"https://h/o/r","secretRef":"creds"}}`,
		"chart with path": `{"engine":"argocd","name":"x","source":{"repoUrl":"https://h/c","chart":"c","chartVersion":"1","path":"p"}}`,
	}
	for name, body := range cases {
		for _, path := range []string{"/api/v1/gitops", "/api/v1/gitops/preview"} {
			w := e.do("POST", path, body, asAdmin())
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s %s: %d %s", name, path, w.Code, w.Body.String())
			}
			var m map[string]string
			if json.Unmarshal(w.Body.Bytes(), &m) != nil || m["error"] == "" {
				t.Errorf("%s %s: no error message: %s", name, path, w.Body.String())
			}
		}
	}
}

func TestGitOpsCreateCredentialsNeverEchoed(t *testing.T) {
	e := newGitOpsEnv(t)
	body := `{"engine":"argocd","name":"leaky","source":{"repoUrl":"https://deploy:s3cr3t-T0ken@github.com/org/repo.git"}}`
	for _, path := range []string{"/api/v1/gitops", "/api/v1/gitops/preview"} {
		w := e.do("POST", path, body, asAdmin())
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "s3cr3t") || !strings.Contains(w.Body.String(), "secretRef") {
			t.Errorf("%s: %s", path, w.Body.String())
		}
	}
	for _, a := range platform.ListAxiomAudit() {
		if strings.Contains(a.Target+a.Detail, "s3cr3t") {
			t.Errorf("audit leaks credentials: %+v", a)
		}
	}
}

func TestGitOpsCreateAccess(t *testing.T) {
	e := newGitOpsEnv(t)
	creds := map[string][2]string{
		"operator": {"oper1", "oper-password"}, "viewer": {"viewer1", "viewer-password"},
		"scoped": {"scoped", "scoped-password"}, "scopedop": {"scopedop", "scopedop-password"},
	}
	for who, c := range creds {
		for _, path := range []string{"/api/v1/gitops", "/api/v1/gitops/preview"} {
			w := e.do("POST", path, gitOpsBody("access-"+who), withBasic(c[0], c[1]))
			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s: %d", who, path, w.Code)
			}
		}
	}
	// Listing and sync keep their behaviour for operators.
	if w := e.do("GET", "/api/v1/gitops", "", withBasic("oper1", "oper-password")); w.Code != 200 {
		t.Errorf("operator list: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/gitops/Application/argocd/platform-apps/sync", "", withBasic("oper1", "oper-password")); w.Code != 200 {
		t.Errorf("operator sync: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/gitops", "", withBasic("scoped", "scoped-password")); w.Code != 403 {
		t.Errorf("restricted list: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/gitops", gitOpsBody("anon")); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
}
