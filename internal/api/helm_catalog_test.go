package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

const helmTestIndex = `apiVersion: v1
entries:
  nginx:
  - name: nginx
    version: 1.2.0
    appVersion: "1.25"
    description: Web server
    keywords: [http]
    maintainers: [{name: Ada, email: ada@example.com}]
    urls: [charts/nginx-1.2.0.tgz]
  - name: nginx
    version: 1.1.0
    urls: [charts/nginx-1.1.0.tgz]
  redis:
  - name: redis
    version: 7.0.0
    description: Key-value store
    urls: [charts/redis-7.0.0.tgz]
`

func helmTestTgz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func newHelmRepoServer(t *testing.T) *httptest.Server {
	t.Helper()
	archive := helmTestTgz(t, map[string]string{"nginx/values.yaml": "replicaCount: 2\n", "nginx/README.md": "# nginx"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stable/index.yaml":
			_, _ = w.Write([]byte(helmTestIndex))
		case "/stable/charts/nginx-1.2.0.tgz":
			_, _ = w.Write(archive)
		case "/html/index.yaml":
			_, _ = w.Write([]byte("<html>login</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newHelmEnv(t *testing.T, store platform.HelmRepoStore) *idEnv {
	t.Helper()
	// The test repository is a local http server.
	t.Setenv("AXIOM_HELM_ALLOW_PRIVATE_REPOS", "true")
	t.Setenv("AXIOM_HELM_ALLOW_INSECURE_REPOS", "true")
	hub := platform.NewUserHub(nil)
	h := NewWithUsers(platform.NewDemoProvider(), "demo", idAdminUser, idAdminPass, "", nil, hub,
		AuthOptions{SessionSecret: "test-secret", HelmRepoStore: store})
	e := &idEnv{h: h, users: hub, store: &memRevStore{}}
	e.addUser(t, "viewer1", "viewer-password", "viewer")
	e.addUser(t, "oper1", "oper-password", "operator")
	e.addUser(t, "scoped", "scoped-password", "viewer", "payments")
	e.addUser(t, "scopedop", "scopedop-password", "operator", "payments")
	return e
}

var helmCreds = map[string][2]string{
	"admin": {idAdminUser, idAdminPass}, "operator": {"oper1", "oper-password"},
	"viewer": {"viewer1", "viewer-password"}, "scoped": {"scoped", "scoped-password"},
	"scopedop": {"scopedop", "scopedop-password"},
}

func asHelm(who string) reqOpt { return withBasic(helmCreds[who][0], helmCreds[who][1]) }

func TestHelmCatalogEndpoints(t *testing.T) {
	repo := newHelmRepoServer(t)
	store := platform.NewMemoryHelmRepoStore()
	e := newHelmEnv(t, store)

	// Empty to start with (demo mode has no built-in repositories).
	w := e.do("GET", "/api/v1/helm/repos", "", asHelm("viewer"))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("empty list: %d %q %q", w.Code, w.Body.String(), w.Header().Get("Cache-Control"))
	}

	w = e.do("POST", "/api/v1/helm/repos", jsonBody(map[string]string{"name": "stable", "url": repo.URL + "/stable/"}), asHelm("admin"))
	if w.Code != 201 {
		t.Fatalf("add: %d %s", w.Code, w.Body.String())
	}
	var added platform.HelmRepoView
	decodeInto(t, w, &added)
	if added.Name != "stable" || added.URL != repo.URL+"/stable" || added.ChartCount != 2 || added.AddedAt == "" || added.FetchedAt == "" {
		t.Fatalf("added: %+v", added)
	}
	if !strings.Contains(w.Body.String(), `"error":""`) {
		t.Errorf("repo object must carry error: %s", w.Body.String())
	}
	found := false
	for _, a := range platform.ListAxiomAudit() {
		if a.Action == "helm.repo_add" && a.Target == "stable" && a.User == idAdminUser && a.Detail == repo.URL+"/stable" {
			found = true
		}
	}
	if !found {
		t.Error("repo add was not audited")
	}

	bad := []struct {
		body string
		want int
	}{
		{jsonBody(map[string]string{"name": "stable", "url": repo.URL + "/other"}), 409},
		{jsonBody(map[string]string{"name": "again", "url": repo.URL + "/stable"}), 409},
		{jsonBody(map[string]string{"name": "Not_Valid", "url": repo.URL + "/stable"}), 400},
		{jsonBody(map[string]string{"name": "html", "url": repo.URL + "/html"}), 400},
		{jsonBody(map[string]string{"name": "gone", "url": repo.URL + "/gone"}), 400},
		{jsonBody(map[string]string{"name": "ftp", "url": "ftp://example.com"}), 400},
		{jsonBody(map[string]string{"name": "creds", "url": "https://u:p@example.com"}), 400},
		{`{"name":"x","url":"https://example.com","extra":1}`, 400},
		{`not json`, 400},
	}
	for _, b := range bad {
		if w := e.do("POST", "/api/v1/helm/repos", b.body, asHelm("admin")); w.Code != b.want {
			t.Errorf("POST %s: got %d, want %d (%s)", b.body, w.Code, b.want, w.Body.String())
		}
	}

	w = e.do("GET", "/api/v1/helm/repos", "", asHelm("viewer"))
	var repos []platform.HelmRepoView
	decodeInto(t, w, &repos)
	if len(repos) != 1 || repos[0].ChartCount != 2 {
		t.Fatalf("repos: %+v", repos)
	}

	w = e.do("GET", "/api/v1/helm/charts?q=ngi", "", asHelm("viewer"))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("search: %d %s", w.Code, w.Body.String())
	}
	var res platform.HelmChartSearch
	decodeInto(t, w, &res)
	if res.Total != 1 || res.Charts[0].Name != "nginx" || res.Charts[0].Version != "1.2.0" || res.Charts[0].Repo != "stable" || res.Errors == nil {
		t.Fatalf("search result: %+v", res)
	}
	w = e.do("GET", "/api/v1/helm/charts?repo=stable&limit=1&refresh=true", "", asHelm("operator"))
	decodeInto(t, w, &res)
	if w.Code != 200 || len(res.Charts) != 1 || res.Total != 2 {
		t.Fatalf("limit: %d %+v", w.Code, res)
	}
	for _, q := range []string{"?repo=nope", "?limit=abc", "?limit=-1"} {
		w := e.do("GET", "/api/v1/helm/charts"+q, "", asHelm("viewer"))
		want := 400
		if q == "?repo=nope" {
			want = 404
		}
		if w.Code != want {
			t.Errorf("search %s: %d", q, w.Code)
		}
	}

	w = e.do("GET", "/api/v1/helm/charts/stable/nginx", "", asHelm("viewer"))
	var d platform.HelmChartDetail
	decodeInto(t, w, &d)
	if w.Code != 200 || len(d.Versions) != 2 || d.Versions[0].Version != "1.2.0" || d.Maintainers[0].Name != "Ada" || d.Sources == nil {
		t.Fatalf("detail: %d %+v", w.Code, d)
	}
	for _, p := range []string{"/api/v1/helm/charts/stable/nope", "/api/v1/helm/charts/nope/nginx", "/api/v1/helm/charts/stable/nginx/values?version=9.9.9"} {
		if w := e.do("GET", p, "", asHelm("viewer")); w.Code != 404 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}

	w = e.do("GET", "/api/v1/helm/charts/stable/nginx/values", "", asHelm("viewer"))
	var v platform.HelmChartValues
	decodeInto(t, w, &v)
	if w.Code != 200 || v.Version != "1.2.0" || v.Values != "replicaCount: 2\n" || v.Readme != "# nginx" {
		t.Fatalf("values: %d %+v", w.Code, v)
	}
	if w := e.do("GET", "/api/v1/helm/charts/stable/nginx/values?version=1.1.0", "", asHelm("viewer")); w.Code != 502 {
		t.Errorf("archive missing upstream: %d %s", w.Code, w.Body.String())
	}

	if w := e.do("DELETE", "/api/v1/helm/repos/nope", "", asHelm("admin")); w.Code != 404 {
		t.Errorf("delete unknown: %d", w.Code)
	}
	w = e.do("DELETE", "/api/v1/helm/repos/stable", "", asHelm("admin"))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"ok":true}` {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	found = false
	for _, a := range platform.ListAxiomAudit() {
		if a.Action == "helm.repo_remove" && a.Target == "stable" {
			found = true
		}
	}
	if !found {
		t.Error("repo remove was not audited")
	}
	if l, _ := store.List(t.Context()); len(l) != 0 {
		t.Fatalf("store after delete: %+v", l)
	}
}

func TestHelmCatalogAccess(t *testing.T) {
	repo := newHelmRepoServer(t)
	e := newHelmEnv(t, nil)
	if w := e.do("POST", "/api/v1/helm/repos", jsonBody(map[string]string{"name": "stable", "url": repo.URL + "/stable"}), asHelm("admin")); w.Code != 201 {
		t.Fatalf("seed: %d %s", w.Code, w.Body.String())
	}
	addBody := jsonBody(map[string]string{"name": "other", "url": repo.URL + "/stable/x"})
	reads := []string{
		"/api/v1/helm/repos",
		"/api/v1/helm/charts",
		"/api/v1/helm/charts?repo=stable&q=nginx",
		"/api/v1/helm/charts/stable/nginx",
		"/api/v1/helm/charts/stable/nginx/values",
	}
	for _, who := range []string{"admin", "operator", "viewer"} {
		for _, p := range reads {
			if w := e.do("GET", p, "", asHelm(who)); w.Code != 200 {
				t.Errorf("GET %s as %s: %d", p, who, w.Code)
			}
		}
	}
	for _, who := range []string{"scoped", "scopedop"} {
		for _, p := range reads {
			if w := e.do("GET", p, "", asHelm(who)); w.Code != 403 {
				t.Errorf("GET %s as %s: %d, want 403", p, who, w.Code)
			}
		}
	}
	for _, who := range []string{"operator", "viewer", "scoped", "scopedop"} {
		if w := e.do("POST", "/api/v1/helm/repos", addBody, asHelm(who)); w.Code != 403 {
			t.Errorf("POST as %s: %d, want 403", who, w.Code)
		}
		if w := e.do("DELETE", "/api/v1/helm/repos/stable", "", asHelm(who)); w.Code != 403 {
			t.Errorf("DELETE as %s: %d, want 403", who, w.Code)
		}
	}
	if w := e.do("GET", "/api/v1/helm/repos", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
	// The repository is still there: none of the refused writes went through.
	var repos []platform.HelmRepoView
	decodeInto(t, e.do("GET", "/api/v1/helm/repos", "", asHelm("admin")), &repos)
	if len(repos) != 1 {
		t.Fatalf("repos: %+v", repos)
	}
}

func TestHelmCatalogRefusesPrivateReposByDefault(t *testing.T) {
	repo := newHelmRepoServer(t)
	t.Setenv("AXIOM_HELM_ALLOW_PRIVATE_REPOS", "")
	t.Setenv("AXIOM_HELM_ALLOW_INSECURE_REPOS", "")
	hub := platform.NewUserHub(nil)
	h := NewWithUsers(platform.NewDemoProvider(), "demo", idAdminUser, idAdminPass, "", nil, hub, AuthOptions{SessionSecret: "s"})
	e := &idEnv{h: h, users: hub, store: &memRevStore{}}
	for _, u := range []string{repo.URL + "/stable", strings.Replace(repo.URL, "http://", "https://", 1) + "/stable", "https://169.254.169.254/latest", "https://localhost/x"} {
		w := e.do("POST", "/api/v1/helm/repos", jsonBody(map[string]string{"name": "x", "url": u}), withBasic(idAdminUser, idAdminPass))
		if w.Code != 400 {
			t.Errorf("%s: %d %s", u, w.Code, w.Body.String())
		}
	}
}
