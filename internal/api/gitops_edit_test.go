package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

// editBodyFrom turns a GET .../edit response into a PUT body, as the UI does:
// drop the read-only response fields, then apply mut to the map.
func editBodyFrom(t *testing.T, getBody []byte, mut func(m map[string]any)) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(getBody, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"editable", "reasons", "readOnly", "sourceObject"} {
		delete(m, k)
	}
	if mut != nil {
		mut(m)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestGitOpsEditEndpoints(t *testing.T) {
	e := newGitOpsEnv(t)
	if w := e.do("POST", "/api/v1/gitops", gitOpsBody("edit-me"), asAdmin()); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	base := "/api/v1/gitops/Kustomization/flux-system/edit-me"
	w := e.do("GET", base+"/edit", "", asAdmin())
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	var ed platform.GitOpsEditable
	decodeInto(t, w, &ed)
	if !ed.Editable || ed.Engine != "flux-kustomize" || ed.Source.Path != "deploy/prod" || ed.Source.Revision != "v1.0.0" ||
		ed.Interval != "5m" || !ed.SyncPolicy.Prune || ed.ResourceVersion == "" || len(ed.ReadOnly) != 0 {
		t.Fatalf("editable: %s", w.Body.String())
	}
	body := editBodyFrom(t, w.Body.Bytes(), func(m map[string]any) {
		m["source"].(map[string]any)["revision"] = "refs/tags/v2.0.0"
	})
	w = e.do("POST", base+"/edit/preview", body, asAdmin())
	if w.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var p platform.GitOpsEditPreview
	decodeInto(t, w, &p)
	if p.Noop || len(p.Changes) != 2 || !strings.Contains(p.YAML, "tag: v2.0.0") {
		t.Fatalf("preview: %s", w.Body.String())
	}
	w = e.do("PUT", base, body, asAdmin())
	if w.Code != http.StatusOK {
		t.Fatalf("put: %d %s", w.Code, w.Body.String())
	}
	audited := false
	for _, a := range platform.ListAxiomAudit() {
		if a.Action == "gitops.edit" && a.Target == "Kustomization/flux-system/edit-me" && a.User == idAdminUser {
			audited = true
			if a.Detail != "fields=spec.ref.tag,spec.ref.branch" && a.Detail != "fields=spec.ref.branch,spec.ref.tag" {
				t.Errorf("audit detail: %q", a.Detail)
			}
			if strings.Contains(a.Detail, "v2.0.0") {
				t.Errorf("audit contains a value: %q", a.Detail)
			}
		}
	}
	if !audited {
		t.Errorf("no audit entry: %+v", platform.ListAxiomAudit())
	}
	// The same body again carries the old resourceVersion: 409.
	if w := e.do("PUT", base, body, asAdmin()); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "the application changed since you opened it") {
		t.Fatalf("stale: %d %s", w.Code, w.Body.String())
	}
	// Immutable / unknown fields.
	w = e.do("GET", base+"/edit", "", asAdmin())
	for name, mut := range map[string]func(map[string]any){
		"name":    func(m map[string]any) { m["name"] = "renamed" },
		"engine":  func(m map[string]any) { m["engine"] = "argocd" },
		"unknown": func(m map[string]any) { m["spec"] = map[string]any{"x": 1} },
		"bad url": func(m map[string]any) { m["source"].(map[string]any)["repoUrl"] = "https://u:p4ss@github.com/o/r.git" },
		"no rv":   func(m map[string]any) { delete(m, "resourceVersion") },
	} {
		w2 := e.do("PUT", base, editBodyFrom(t, w.Body.Bytes(), mut), asAdmin())
		if w2.Code != http.StatusBadRequest || strings.Contains(w2.Body.String(), "p4ss") {
			t.Errorf("%s: %d %s", name, w2.Code, w2.Body.String())
		}
	}
	if w := e.do("GET", "/api/v1/gitops/Application/argocd/nope/edit", "", asAdmin()); w.Code != http.StatusNotFound {
		t.Errorf("404: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/gitops/Pod/argocd/x/edit", "", asAdmin()); w.Code != http.StatusBadRequest {
		t.Errorf("bad kind: %d", w.Code)
	}
	// A seed with an unmanaged, shared source: source fields read-only.
	w = e.do("GET", "/api/v1/gitops/Kustomization/flux-system/infra/edit", "", asAdmin())
	if !strings.Contains(w.Body.String(), `"readOnly":["source.repoUrl","source.revision","source.secretRef"]`) || !strings.Contains(w.Body.String(), `"editable":true`) {
		t.Fatalf("seed: %s", w.Body.String())
	}
	body = editBodyFrom(t, w.Body.Bytes(), func(m map[string]any) { m["source"].(map[string]any)["repoUrl"] = "https://github.com/evil/x.git" })
	if w := e.do("PUT", "/api/v1/gitops/Kustomization/flux-system/infra", body, asAdmin()); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "source.repoUrl cannot be changed") {
		t.Fatalf("read-only: %d %s", w.Code, w.Body.String())
	}
}

func TestGitOpsEditAccess(t *testing.T) {
	e := newGitOpsEnv(t)
	if w := e.do("POST", "/api/v1/gitops", gitOpsBody("edit-guarded"), asAdmin()); w.Code != http.StatusCreated {
		t.Fatalf("create: %d", w.Code)
	}
	base := "/api/v1/gitops/Kustomization/flux-system/edit-guarded"
	w := e.do("GET", base+"/edit", "", asAdmin())
	body := editBodyFrom(t, w.Body.Bytes(), func(m map[string]any) { m["interval"] = "10m" })
	creds := map[string][2]string{
		"operator": {"oper1", "oper-password"}, "viewer": {"viewer1", "viewer-password"},
		"scoped": {"scoped", "scoped-password"}, "scopedop": {"scopedop", "scopedop-password"},
	}
	for who, c := range creds {
		want := http.StatusOK
		if who == "scoped" || who == "scopedop" {
			want = http.StatusForbidden
		}
		if w := e.do("GET", base+"/edit", "", withBasic(c[0], c[1])); w.Code != want {
			t.Errorf("%s get: %d, want %d", who, w.Code, want)
		}
		if w := e.do("POST", base+"/edit/preview", body, withBasic(c[0], c[1])); w.Code != http.StatusForbidden {
			t.Errorf("%s preview: %d", who, w.Code)
		}
		if w := e.do("PUT", base, body, withBasic(c[0], c[1])); w.Code != http.StatusForbidden {
			t.Errorf("%s put: %d", who, w.Code)
		}
	}
	if w := e.do("PUT", base, body); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
	if w := e.do("PUT", base, body, asAdmin()); w.Code != http.StatusOK {
		t.Fatalf("admin put: %d %s", w.Code, w.Body.String())
	}
}
