package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// A test registry speaking the OCI Distribution API with Helm media types.

const ociTestToken = "s3cr3t-anon-token-value"

type ociReg struct {
	*httptest.Server
	t *testing.T

	mu            sync.Mutex
	manifests     map[string][]byte // repo:tag -> body
	mtypes        map[string]string // repo:tag -> Content-Type
	tags          map[string][]string
	blobs         map[string][]byte
	corrupt       map[string]bool
	catalog       []string
	catalogStatus int
	pageSize      int
	requireToken  bool
	basicOnly     bool
	realm         string // default: <self>/token
	tokenBody     []byte
	tagsBody      []byte
	manifestFail  int
	blobRedirect  bool // blobs answer with a redirect to /storage/<digest>?sig=...
	redirectTo    string
	hits          map[string]int
	scopes        []string
	tokenAuth     []string // Authorization headers seen by /token
	storageAuth   []string // Authorization headers seen by /storage
	accepts       []string
	// Registry credentials: the token endpoint wants Basic tokenUser:tokenPass
	// (and answers expires_in when tokenExpiresIn is set); basicUser makes
	// /v2/ answer a Basic challenge and accept only basicUser:basicPass.
	tokenUser, tokenPass string
	tokenExpiresIn       int
	basicUser, basicPass string
	v2Auth               []string // Authorization headers seen under /v2/
}

func newOCIReg(t *testing.T, tlsServer bool) *ociReg {
	t.Helper()
	rg := &ociReg{
		t: t, manifests: map[string][]byte{}, mtypes: map[string]string{}, tags: map[string][]string{},
		blobs: map[string][]byte{}, corrupt: map[string]bool{}, hits: map[string]int{}, pageSize: 100,
	}
	if tlsServer {
		rg.Server = httptest.NewTLSServer(rg)
	} else {
		rg.Server = httptest.NewServer(rg)
	}
	t.Cleanup(rg.Close)
	return rg
}

func ociDigest(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

func (rg *ociReg) addChart(repo, version string, cfg map[string]any, tgz []byte, docker bool) {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	if cfg == nil {
		cfg = map[string]any{}
	}
	if _, ok := cfg["version"]; !ok {
		cfg["version"] = version
	}
	cfgB, _ := json.Marshal(cfg)
	cd, ld := ociDigest(cfgB), ociDigest(tgz)
	rg.blobs[cd], rg.blobs[ld] = cfgB, tgz
	m := map[string]any{
		"schemaVersion": 2,
		"config":        map[string]any{"mediaType": helmOCIConfigMedia, "digest": cd, "size": len(cfgB)},
		"layers":        []any{map[string]any{"mediaType": helmOCIChartLayer, "digest": ld, "size": len(tgz)}},
	}
	mt := helmOCIManifestOCI
	if docker {
		mt = helmOCIManifestV2 // and no mediaType in the body: the header says it
	} else {
		m["mediaType"] = mt
	}
	body, _ := json.Marshal(m)
	tag := helmOCIVersionToTag(version)
	rg.manifests[repo+":"+tag], rg.mtypes[repo+":"+tag] = body, mt
	rg.tags[repo] = append(rg.tags[repo], tag)
}

func (rg *ociReg) addTag(repo, tag string) {
	rg.mu.Lock()
	rg.tags[repo] = append(rg.tags[repo], tag)
	rg.mu.Unlock()
}

func (rg *ociReg) paginate(w http.ResponseWriter, r *http.Request, field string, all []string) {
	n := rg.pageSize
	if v, err := strconv.Atoi(r.URL.Query().Get("n")); err == nil && v > 0 && v < n {
		n = v
	}
	start := 0
	if last := r.URL.Query().Get("last"); last != "" {
		for i, s := range all {
			if s == last {
				start = i + 1
			}
		}
	}
	end := start + n
	if end > len(all) {
		end = len(all)
	}
	page := all[start:end]
	if end < len(all) {
		w.Header().Set("Link", fmt.Sprintf(`<%s?last=%s&n=%d>; rel="next"`, r.URL.Path, url.QueryEscape(page[len(page)-1]), n))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{field: page})
}

func (rg *ociReg) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	p := r.URL.Path
	rg.hits[p]++
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p == "/token" {
		rg.scopes = append(rg.scopes, r.URL.Query().Get("scope"))
		rg.tokenAuth = append(rg.tokenAuth, r.Header.Get("Authorization"))
		if r.URL.Query().Get("service") != "test-registry" {
			w.WriteHeader(400)
			return
		}
		if rg.tokenUser != "" {
			u, pw, ok := r.BasicAuth()
			if !ok || u != rg.tokenUser || pw != rg.tokenPass {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		if rg.tokenExpiresIn != 0 {
			_ = json.NewEncoder(w).Encode(map[string]any{"token": ociTestToken, "expires_in": rg.tokenExpiresIn})
			return
		}
		if rg.tokenBody != nil {
			_, _ = w.Write(rg.tokenBody)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": ociTestToken})
		return
	}
	if strings.HasPrefix(p, "/storage/") {
		rg.storageAuth = append(rg.storageAuth, r.Header.Get("Authorization"))
		b, ok := rg.blobs[strings.TrimPrefix(p, "/storage/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
		return
	}
	if !strings.HasPrefix(p, "/v2/") {
		http.NotFound(w, r)
		return
	}
	rg.v2Auth = append(rg.v2Auth, r.Header.Get("Authorization"))
	if rg.basicUser != "" {
		if u, pw, ok := r.BasicAuth(); !ok || u != rg.basicUser || pw != rg.basicPass {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	} else if rg.requireToken || rg.basicOnly {
		if rg.basicOnly || r.Header.Get("Authorization") != "Bearer "+ociTestToken {
			if rg.basicOnly {
				w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			} else {
				realm := rg.realm
				if realm == "" {
					realm = scheme + "://" + r.Host + "/token"
				}
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s",service="test-registry",scope="repository:x:pull"`, realm))
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	rest := strings.TrimPrefix(p, "/v2/")
	switch {
	case rest == "":
		_, _ = w.Write([]byte("{}"))
	case rest == "_catalog":
		if rg.catalogStatus != 0 {
			w.WriteHeader(rg.catalogStatus)
			return
		}
		rg.paginate(w, r, "repositories", rg.catalog)
	case strings.HasSuffix(rest, "/tags/list"):
		name := strings.TrimSuffix(rest, "/tags/list")
		if rg.tagsBody != nil {
			_, _ = w.Write(rg.tagsBody)
			return
		}
		tags, ok := rg.tags[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		rg.paginate(w, r, "tags", tags)
	case strings.Contains(rest, "/manifests/"):
		i := strings.LastIndex(rest, "/manifests/")
		key := rest[:i] + ":" + rest[i+len("/manifests/"):]
		rg.accepts = append(rg.accepts, r.Header.Get("Accept"))
		if rg.manifestFail != 0 {
			w.WriteHeader(rg.manifestFail)
			return
		}
		b, ok := rg.manifests[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", rg.mtypes[key])
		_, _ = w.Write(b)
	case strings.Contains(rest, "/blobs/"):
		d := rest[strings.LastIndex(rest, "/blobs/")+len("/blobs/"):]
		b, ok := rg.blobs[d]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if rg.redirectTo != "" {
			http.Redirect(w, r, rg.redirectTo+"/storage/"+d+"?sig=SIGNED-SECRET", http.StatusTemporaryRedirect)
			return
		}
		if rg.blobRedirect {
			http.Redirect(w, r, "/storage/"+d+"?sig=SIGNED-SECRET", http.StatusTemporaryRedirect)
			return
		}
		if rg.corrupt[d] {
			b = append([]byte{}, b...)
			b[len(b)/2] ^= 0xff
		}
		_, _ = w.Write(b)
	default:
		http.NotFound(w, r)
	}
}

func (rg *ociReg) ociURL(path string) string {
	u, _ := url.Parse(rg.URL)
	return "oci://" + u.Host + path
}

func standardOCIReg(t *testing.T, tlsServer bool) *ociReg {
	t.Helper()
	rg := newOCIReg(t, tlsServer)
	tgz := makeTgz(t, tarEntry{name: "nginx/values.yaml", body: "replicaCount: 3\n"}, tarEntry{name: "nginx/README.md", body: "# nginx from OCI"})
	rg.addChart("charts/nginx", "1.0.0", map[string]any{"name": "nginx", "appVersion": "1.24"}, tgz, false)
	rg.addTag("charts/nginx", "latest")
	rg.addChart("charts/nginx", "1.2.0", map[string]any{
		"name": "nginx", "appVersion": "1.25", "description": "Web server from OCI", "home": "https://nginx.org",
		"icon": "https://example.com/i.png", "keywords": []string{"http", "web"},
		"maintainers": []any{map[string]string{"name": "Ada", "email": "ada@example.com"}},
	}, tgz, false)
	rg.addChart("charts/nginx", "2.0.0-rc.1", map[string]any{"name": "nginx", "description": "preview"}, tgz, false)
	rg.addTag("charts/nginx", "main")
	rg.addTag("charts/nginx", "sha256-abc.sig")
	rg.addChart("charts/nginx", "1.1.0", nil, tgz, false)
	rtgz := makeTgz(t, tarEntry{name: "redis/values.yaml", body: "auth: false\n"})
	rg.addChart("charts/redis", "7.0.0+build.5", map[string]any{"name": "redis", "description": "Key-value store", "deprecated": true}, rtgz, true)
	rg.set(func() {
		rg.catalog = []string{"charts/nginx", "charts/redis", "charts/sub/deep", "other/thing", "chartsy/x"}
	})
	return rg
}

func newOCICatalog(t *testing.T, f *HelmFetcher) *HelmCatalog {
	t.Helper()
	clk := &testClock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	c := NewHelmCatalog(NewMemoryHelmRepoStore(), f)
	c.now = clk.Now
	return c
}

// set changes the registry's behaviour under its lock (the handler runs on
// other goroutines).
func (rg *ociReg) set(fn func()) {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	fn()
}

func noLeak(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	for _, s := range []string{ociTestToken, "SIGNED-SECRET", "sig=", "scope=", "service="} {
		if strings.Contains(err.Error(), s) {
			t.Fatalf("error leaks %q: %v", s, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Validation

func TestHelmOCIURLValidation(t *testing.T) {
	ok := map[string]string{
		"oci://ghcr.io/org/charts":       "oci://ghcr.io/org/charts",
		"OCI://GHCR.io/org/charts/":      "oci://ghcr.io/org/charts",
		"oci://registry.example.com":     "oci://registry.example.com",
		"oci://r.example.com:5000/a_b/c": "oci://r.example.com:5000/a_b/c",
		" oci://docker.io/bitnamicharts": "oci://docker.io/bitnamicharts",
		"oci://93.184.216.34/charts":     "oci://93.184.216.34/charts",
	}
	for in, want := range ok {
		got, err := HelmFetchPolicy{}.NormalizeHelmOCIURL(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	bad := map[string]string{
		"":                                          "required",
		"https://ghcr.io/org":                       "oci://",
		"oci://":                                    "no host",
		"oci://u:p@ghcr.io/org":                     "user name",
		"oci://ghcr.io/org?x=1":                     "query",
		"oci://ghcr.io/org#frag":                    "fragment",
		"oci://ghcr.io/Org":                         "path component",
		"oci://ghcr.io/org//x":                      "path component",
		"oci://ghcr.io/o%2e":                        "escapes",
		"oci://gh cr.io/org":                        "whitespace",
		"oci://ghcr.io:99999/org":                   "port",
		"oci://ghcr.io:/org":                        "port",
		"oci://-bad.io/org":                         "DNS name",
		"oci://127.0.0.1:5000/charts":               "not a public address",
		"oci://localhost/charts":                    "not a public address",
		"oci://169.254.169.254/latest":              "not a public address",
		"oci://2130706433/charts":                   "numeric address",
		"oci://ghcr.io/" + strings.Repeat("a", 600): "too long",
	}
	for in, want := range bad {
		_, err := HelmFetchPolicy{}.NormalizeHelmOCIURL(in)
		if err == nil || HTTPStatus(err) != 400 || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want 400 containing %q, got %v", in, want, err)
		}
	}
	if _, err := openPolicy.NormalizeHelmOCIURL("oci://127.0.0.1:5000/charts"); err != nil {
		t.Errorf("private allowed: %v", err)
	}
}

func TestHelmOCIChartNameValidation(t *testing.T) {
	if got, err := ValidateHelmOCIChartNames([]string{" nginx ", "kube-prometheus.stack", "a_b"}); err != nil || len(got) != 3 || got[0] != "nginx" {
		t.Fatalf("got %v %v", got, err)
	}
	many := make([]string, 51)
	for i := range many {
		many[i] = fmt.Sprintf("c%d", i)
	}
	for _, in := range [][]string{{"Nginx"}, {""}, {"-x"}, {"x-"}, {"a/b"}, {"a", "a"}, {"a b"}, many, {strings.Repeat("a", 129)}} {
		if _, err := ValidateHelmOCIChartNames(in); err == nil || HTTPStatus(err) != 400 {
			t.Errorf("%v: want 400, got %v", in, err)
		}
	}
}

func TestHelmOCIInstallValidation(t *testing.T) {
	base := InstallHelmRequest{Namespace: "apps", ReleaseName: "web", OCIRef: "oci://ghcr.io/org/charts/nginx", ChartVersion: "1.2.0"}
	if err := ValidateHelmOCIInstall(base); err != nil {
		t.Fatalf("valid request refused: %v", err)
	}
	for name, mutate := range map[string]func(*InstallHelmRequest){
		"flag release":   func(r *InstallHelmRequest) { r.ReleaseName = "--post-renderer=/bin/sh" },
		"flag ns":        func(r *InstallHelmRequest) { r.Namespace = "-A" },
		"flag version":   func(r *InstallHelmRequest) { r.ChartVersion = "--devel" },
		"flag chart":     func(r *InstallHelmRequest) { r.ChartName = "--kubeconfig=/x" },
		"flag ref":       func(r *InstallHelmRequest) { r.OCIRef = "--repo=oci://x/y" },
		"userinfo":       func(r *InstallHelmRequest) { r.OCIRef = "oci://u:p@ghcr.io/org/nginx" },
		"whitespace":     func(r *InstallHelmRequest) { r.OCIRef = "oci://ghcr.io/org/nginx --set x=1" },
		"trailing space": func(r *InstallHelmRequest) { r.OCIRef = "oci://ghcr.io/org/nginx " },
		"newline":        func(r *InstallHelmRequest) { r.OCIRef = "oci://ghcr.io/org/nginx\n" },
		"wrong scheme":   func(r *InstallHelmRequest) { r.OCIRef = "https://ghcr.io/org/nginx" },
		"upper scheme":   func(r *InstallHelmRequest) { r.OCIRef = "OCI://ghcr.io/org/nginx" },
		"no chart":       func(r *InstallHelmRequest) { r.OCIRef = "oci://ghcr.io" },
		"trailing slash": func(r *InstallHelmRequest) { r.OCIRef = "oci://ghcr.io/org/nginx/" },
		"query":          func(r *InstallHelmRequest) { r.OCIRef = "oci://ghcr.io/org/nginx?x=1" },
		"tag in ref":     func(r *InstallHelmRequest) { r.OCIRef = "oci://ghcr.io/org/nginx:1.2.0" },
		"bad host":       func(r *InstallHelmRequest) { r.OCIRef = "oci://gh_cr.io/org/nginx" },
		"version space":  func(r *InstallHelmRequest) { r.ChartVersion = "1.0 --devel" },
		"also repoUrl":   func(r *InstallHelmRequest) { r.RepoURL = "https://charts.example.com" },
		"no namespace":   func(r *InstallHelmRequest) { r.Namespace = "" },
		"no release":     func(r *InstallHelmRequest) { r.ReleaseName = " " },
	} {
		r := base
		mutate(&r)
		if err := ValidateHelmOCIInstall(r); err == nil || HTTPStatus(err) != 400 {
			t.Errorf("%s: want 400, got %v", name, err)
		}
	}
}

func TestHelmOCIDemoInstall(t *testing.T) {
	d := NewDemoProvider()
	ctx := context.Background()
	res, err := d.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "default", ReleaseName: "oci-demo-rel", OCIRef: "oci://127.0.0.1:5000/charts/nginx", ChartVersion: "1.2.0"})
	if err != nil || !res.OK || !strings.Contains(res.Message, "oci://127.0.0.1:5000/charts") {
		t.Fatalf("install: %+v %v", res, err)
	}
	if _, err := d.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "default", ReleaseName: "oci-demo-2", OCIRef: "oci://x.io/c/n --devel"}); err == nil || HTTPStatus(err) != 400 {
		t.Fatalf("bad ref: %v", err)
	}
	if _, err := d.UpgradeHelmChart(ctx, InstallHelmRequest{Namespace: "default", ReleaseName: "oci-demo-rel", OCIRef: "https://x.io/c/n"}); err == nil {
		t.Fatal("upgrade with a bad ref must fail")
	}
	// The index path is unchanged.
	if _, err := d.InstallHelmChart(ctx, InstallHelmRequest{Namespace: "default", ReleaseName: "idx-rel"}); err == nil || !strings.Contains(err.Error(), "repoUrl, and chartName are required") {
		t.Fatalf("index validation changed: %v", err)
	}
}

func TestHelmOCIAuthChallengeParse(t *testing.T) {
	scheme, p := parseHelmAuthChallenge(`Bearer realm="https://auth.example.com/token",service="registry.example.com",scope="repository:a/b:pull,push"`)
	if scheme != "bearer" || p["realm"] != "https://auth.example.com/token" || p["service"] != "registry.example.com" || p["scope"] != "repository:a/b:pull,push" {
		t.Fatalf("%s %v", scheme, p)
	}
	scheme, p = parseHelmAuthChallenge(`Bearer realm="a\"b", service=plain`)
	if scheme != "bearer" || p["realm"] != `a"b` || p["service"] != "plain" {
		t.Fatalf("%s %v", scheme, p)
	}
	if s, _ := parseHelmAuthChallenge(`Basic realm="x"`); s != "basic" {
		t.Fatal(s)
	}
}

func TestHelmOCIVersionsFilterSemver(t *testing.T) {
	vs := helmOCIVersions("c", []string{"latest", "main", "1.0.0", "v1.1.0", "2.0.0-rc.1", "1.2.0_build.3", "sha256-abc.sig", "1.0.0", "bad tag", "1.0"})
	var got []string
	for _, v := range vs {
		got = append(got, string(v.Version))
	}
	want := "2.0.0-rc.1,1.2.0+build.3,v1.1.0,1.0.0,1.0"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// Registry client and catalog

func TestHelmOCICatalogExplicitChartsOverPlainHTTP(t *testing.T) {
	rg := standardOCIReg(t, false)
	rg.set(func() { rg.pageSize = 2 }) // force tag pagination
	c := newOCICatalog(t, &HelmFetcher{Policy: openPolicy, UserAgent: "zyvor-axiom/test"})
	ctx := context.Background()
	v, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "oci", URL: rg.ociURL("/charts/"), Charts: []string{"nginx", "redis"}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Kind != "oci" || v.ChartCount != 2 || v.URL != rg.ociURL("/charts") || len(v.Charts) != 2 || v.Error != "" {
		t.Fatalf("view: %+v", v)
	}
	rg.set(func() {
		if rg.hits["/v2/_catalog"] != 0 {
			t.Error("explicit charts must not use the catalog API")
		}
		for _, a := range rg.accepts {
			if !strings.Contains(a, helmOCIManifestOCI) || !strings.Contains(a, helmOCIManifestV2) {
				t.Errorf("manifest Accept: %q", a)
			}
		}
	})
	res, err := c.Search(ctx, "", "", 0, false)
	if err != nil || res.Total != 2 {
		t.Fatalf("search: %+v %v", res, err)
	}
	n := res.Charts[0]
	if n.Name != "nginx" || n.Version != "1.2.0" || n.AppVersion != "1.25" || n.Description != "Web server from OCI" ||
		n.Kind != "oci" || n.Ref != rg.ociURL("/charts/nginx") || n.Repo != "oci" || len(n.Keywords) != 2 {
		t.Fatalf("nginx summary: %+v", n)
	}
	r := res.Charts[1]
	if r.Name != "redis" || r.Version != "7.0.0+build.5" || !r.Deprecated || r.Kind != "oci" {
		t.Fatalf("redis summary (docker manifest, build metadata tag): %+v", r)
	}
	d, err := c.Chart(ctx, "oci", "nginx")
	if err != nil {
		t.Fatal(err)
	}
	var vs []string
	for _, x := range d.Versions {
		vs = append(vs, x.Version)
	}
	if strings.Join(vs, ",") != "2.0.0-rc.1,1.2.0,1.1.0,1.0.0" || d.Description != "Web server from OCI" || d.Kind != "oci" ||
		d.Ref != rg.ociURL("/charts/nginx") || len(d.Maintainers) != 1 || d.Maintainers[0].Name != "Ada" {
		t.Fatalf("detail: %+v", d)
	}
	vals, err := c.ChartValues(ctx, "oci", "nginx", "")
	if err != nil || vals.Version != "1.2.0" || vals.Values != "replicaCount: 3\n" || vals.Readme != "# nginx from OCI" {
		t.Fatalf("values: %+v %v", vals, err)
	}
	vals, err = c.ChartValues(ctx, "oci", "redis", "7.0.0+build.5")
	if err != nil || vals.Values != "auth: false\n" {
		t.Fatalf("redis values: %+v %v", vals, err)
	}
	if _, err := c.ChartValues(ctx, "oci", "nginx", "9.9.9"); HTTPStatus(err) != 404 {
		t.Fatalf("unknown version: %v", err)
	}
	// A repository listed with a chart the registry does not have fails at add time.
	_, err = c.AddHelmRepo(ctx, HelmRepoRequest{Name: "oci2", URL: rg.ociURL("/other"), Charts: []string{"missing"}})
	if HTTPStatus(err) != 400 || !strings.Contains(err.Error(), `chart "missing"`) {
		t.Fatalf("missing chart: %v", err)
	}
}

func TestHelmOCICatalogAPI(t *testing.T) {
	rg := standardOCIReg(t, false)
	c := newOCICatalog(t, &HelmFetcher{Policy: openPolicy})
	ctx := context.Background()
	v, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "oci", URL: rg.ociURL("/charts"), Kind: "oci"})
	if err != nil {
		t.Fatal(err)
	}
	var catalogHits int
	rg.set(func() { catalogHits = rg.hits["/v2/_catalog"] })
	if v.ChartCount != 2 || catalogHits == 0 {
		t.Fatalf("catalog listing: %+v hits=%d", v, catalogHits)
	}
	ix, _, _, err := c.index(ctx, HelmRepo{Name: "oci", URL: v.URL, Kind: "oci"}, false)
	if err != nil || strings.Join(ix.names, ",") != "nginx,redis" {
		t.Fatalf("names: %v %v", ix, err)
	}
	for _, st := range []int{401, 403, 404} {
		rg.set(func() { rg.catalogStatus = st })
		_, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: fmt.Sprintf("c%d", st), URL: rg.ociURL("/charts/x")})
		if HTTPStatus(err) != 400 || !strings.Contains(err.Error(), "name the charts explicitly") {
			t.Errorf("catalog %d: %v", st, err)
		}
	}
}

func TestHelmOCIBearerTokenFlow(t *testing.T) {
	rg := standardOCIReg(t, false)
	rg.set(func() { rg.requireToken = true })
	c := newOCICatalog(t, &HelmFetcher{Policy: openPolicy})
	ctx := context.Background()
	if _, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "oci", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}}); err != nil {
		t.Fatal(err)
	}
	rg.set(func() {
		// The token is cached per scope: one token request per catalog fetch.
		if len(rg.scopes) != 1 {
			t.Errorf("token requests: %v", rg.scopes)
		}
		for _, s := range rg.scopes {
			if s != "repository:charts/nginx:pull" {
				t.Errorf("scope %q", s)
			}
		}
		for _, a := range rg.tokenAuth {
			if a != "" {
				t.Errorf("token request must be anonymous, sent Authorization %q", a)
			}
		}
		// Values: blobs are redirected to storage; the token must not follow.
		rg.blobRedirect = true
	})
	vals, err := c.ChartValues(ctx, "oci", "nginx", "1.0.0")
	if err != nil || vals.Values != "replicaCount: 3\n" {
		t.Fatalf("values: %+v %v", vals, err)
	}
	rg.set(func() {
		if len(rg.storageAuth) == 0 {
			t.Error("blob redirect was not followed")
		}
		for _, a := range rg.storageAuth {
			if a != "" {
				t.Errorf("Authorization carried across a redirect: %q", a)
			}
		}
		// Failures after the token never print it.
		rg.manifestFail = 500
	})
	_, err = c.ChartValues(ctx, "oci", "nginx", "1.0.0")
	if err == nil {
		t.Fatal("want an error")
	}
	noLeak(t, err)
	// A registry that keeps refusing the anonymous token.
	rg.set(func() { rg.manifestFail, rg.tokenBody = 0, []byte(`{"token":"wrong-token"}`) })
	_, _, err = (&HelmFetcher{Policy: openPolicy}).FetchOCIChartValues(ctx, rg.ociURL("/charts"), "nginx", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "anonymous") || strings.Contains(err.Error(), "wrong-token") {
		t.Fatalf("denied token: %v", err)
	}
}

func TestHelmOCICredentialsRequired(t *testing.T) {
	rg := standardOCIReg(t, false)
	rg.set(func() { rg.basicOnly = true })
	c := newOCICatalog(t, &HelmFetcher{Policy: openPolicy})
	_, err := c.AddHelmRepo(context.Background(), HelmRepoRequest{Name: "oci", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}})
	if HTTPStatus(err) != 400 || !strings.Contains(err.Error(), helmOCINeedsCredentials) {
		t.Fatalf("basic auth registry: %v", err)
	}
	// A credential that does not exist (the catalog has no store at all here).
	_, err = c.AddHelmRepo(context.Background(), HelmRepoRequest{Name: "oci", URL: rg.ociURL("/charts"), CredentialsSecret: "my-registry"})
	if HTTPStatus(err) != 400 || !strings.Contains(err.Error(), `registry credential "my-registry" not found`) {
		t.Fatalf("credentialsSecret: %v", err)
	}
}

func TestHelmOCIDigestVerification(t *testing.T) {
	rg := standardOCIReg(t, false)
	c := newOCICatalog(t, &HelmFetcher{Policy: openPolicy})
	ctx := context.Background()
	if _, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "oci", URL: rg.ociURL("/charts"), Charts: []string{"nginx"}}); err != nil {
		t.Fatal(err)
	}
	// Corrupt the chart layer: values are refused.
	var m helmOCIManifest
	rg.set(func() { _ = json.Unmarshal(rg.manifests["charts/nginx:1.2.0"], &m) })
	layer, _ := m.chartLayer()
	rg.set(func() { rg.corrupt[layer.Digest] = true })
	_, err := c.ChartValues(ctx, "oci", "nginx", "1.2.0")
	if HTTPStatus(err) != 502 || !strings.Contains(err.Error(), "does not match its digest") {
		t.Fatalf("corrupt layer: %v", err)
	}
	// Corrupt the config blob: the catalog cannot be built.
	rg.set(func() { rg.corrupt[m.Config.Digest] = true })
	f := &HelmFetcher{Policy: openPolicy}
	_, err = f.FetchOCIIndex(ctx, HelmRepo{URL: rg.ociURL("/charts"), Kind: "oci", Charts: []string{"nginx"}}, true)
	if err == nil || !strings.Contains(err.Error(), "does not match its digest") {
		t.Fatalf("corrupt config: %v", err)
	}
}

func TestHelmOCICaps(t *testing.T) {
	ctx := context.Background()
	f := &HelmFetcher{Policy: openPolicy}
	t.Run("tags capped at 500", func(t *testing.T) {
		rg := standardOCIReg(t, false)
		for i := 0; i < 600; i++ {
			rg.addTag("charts/many", fmt.Sprintf("0.0.%d", i))
		}
		c, _ := newHelmOCIClient(f, rg.ociURL("/charts"))
		if err := c.ping(ctx); err != nil {
			t.Fatal(err)
		}
		tags, err := c.tags(ctx, "many")
		if err != nil || len(tags) != helmOCIMaxTags {
			t.Fatalf("tags: %d %v", len(tags), err)
		}
	})
	t.Run("tag list bytes capped", func(t *testing.T) {
		rg := standardOCIReg(t, false)
		rg.set(func() { rg.tagsBody = []byte(`{"tags":["` + strings.Repeat("a", helmOCIMaxListBytes) + `"]}`) })
		_, err := f.FetchOCIIndex(ctx, HelmRepo{URL: rg.ociURL("/charts"), Kind: "oci", Charts: []string{"nginx"}}, true)
		if HTTPStatus(err) != 413 {
			t.Fatalf("oversized tag list: %v", err)
		}
	})
	t.Run("token response capped", func(t *testing.T) {
		rg := standardOCIReg(t, false)
		rg.set(func() {
			rg.requireToken = true
			rg.tokenBody = []byte(`{"token":"` + strings.Repeat("t", helmOCIMaxToken) + `"}`)
		})
		_, err := f.FetchOCIIndex(ctx, HelmRepo{URL: rg.ociURL("/charts"), Kind: "oci", Charts: []string{"nginx"}}, true)
		if err == nil || !strings.Contains(err.Error(), "too large") || strings.Contains(err.Error(), "tttttttt") {
			t.Fatalf("oversized token: %v", err)
		}
	})
	t.Run("blob over the declared limit is not downloaded", func(t *testing.T) {
		rg := standardOCIReg(t, false)
		c, _ := newHelmOCIClient(f, rg.ociURL("/charts"))
		_ = c.ping(ctx)
		d := helmOCIDescriptor{MediaType: helmOCIChartLayer, Digest: "sha256:" + strings.Repeat("a", 64), Size: helmMaxArchiveBytes + 1}
		_, err := c.blob(ctx, "nginx", d, helmMaxArchiveBytes, "chart archive")
		if HTTPStatus(err) != 413 {
			t.Fatalf("oversized descriptor: %v", err)
		}
		rg.set(func() {
			for p := range rg.hits {
				if strings.Contains(p, "/blobs/") {
					t.Errorf("blob was requested: %s", p)
				}
			}
		})
	})
	t.Run("blob body larger than the cap", func(t *testing.T) {
		rg := standardOCIReg(t, false)
		big := []byte(strings.Repeat("x", helmOCIMaxConfig+10))
		rg.set(func() { rg.blobs[ociDigest(big)] = big })
		c, _ := newHelmOCIClient(f, rg.ociURL("/charts"))
		_ = c.ping(ctx)
		// The manifest understates the size: the stream is still cut at the cap.
		_, err := c.blob(ctx, "nginx", helmOCIDescriptor{Digest: ociDigest(big), Size: 10}, helmOCIMaxConfig, "chart config")
		if HTTPStatus(err) != 413 {
			t.Fatalf("oversized body: %v", err)
		}
	})
	t.Run("non-sha256 digest refused", func(t *testing.T) {
		rg := standardOCIReg(t, false)
		c, _ := newHelmOCIClient(f, rg.ociURL("/charts"))
		_ = c.ping(ctx)
		for _, dg := range []string{"sha512:" + strings.Repeat("a", 128), "sha256:../../x", ""} {
			if _, err := c.blob(ctx, "nginx", helmOCIDescriptor{Digest: dg}, 100, "blob"); err == nil {
				t.Errorf("%q accepted", dg)
			}
		}
	})
}

func TestHelmOCIPlainHTTPNeedsTheFlag(t *testing.T) {
	rg := standardOCIReg(t, false)
	ctx := context.Background()
	f := &HelmFetcher{Policy: HelmFetchPolicy{AllowPrivate: true}}
	_, err := f.FetchOCIIndex(ctx, HelmRepo{URL: rg.ociURL("/charts"), Kind: "oci", Charts: []string{"nginx"}}, true)
	if err == nil {
		t.Fatal("a plain-http registry must be refused without AXIOM_HELM_ALLOW_INSECURE_REPOS")
	}
	rg.set(func() {
		for p := range rg.hits {
			t.Errorf("no request may reach the registry over http, got %s", p)
		}
	})
	if _, err := (&HelmFetcher{Policy: openPolicy}).FetchOCIIndex(ctx, HelmRepo{URL: rg.ociURL("/charts"), Kind: "oci", Charts: []string{"nginx"}}, true); err != nil {
		t.Fatalf("with the flag: %v", err)
	}
	// A loopback registry is refused without the private flag.
	_, err = HelmFetchPolicy{AllowInsecure: true}.NormalizeHelmOCIURL(rg.ociURL("/charts"))
	if HTTPStatus(err) != 400 {
		t.Fatalf("loopback without the private flag: %v", err)
	}
}

func TestHelmOCISSRF(t *testing.T) {
	ctx := context.Background()
	rg := standardOCIReg(t, true)
	target := rg.Listener.Addr().String()
	res := fakeResolver{"example.com": {"93.184.216.34"}, "internal.example.com": {"10.0.0.5"}}
	strict := func() *HelmFetcher {
		f := routedFetcher(HelmFetchPolicy{}, res, target, nil)
		trustServer(f, rg.Server)
		return f
	}
	repo := HelmRepo{URL: "oci://example.com/charts", Kind: "oci", Charts: []string{"nginx"}}

	t.Run("public registry works through the guard", func(t *testing.T) {
		if _, err := strict().FetchOCIIndex(ctx, repo, true); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("same-host bearer realm", func(t *testing.T) {
		rg.set(func() { rg.requireToken, rg.realm = true, "https://example.com/token" })
		defer rg.set(func() { rg.requireToken, rg.realm = false, "" })
		if _, err := strict().FetchOCIIndex(ctx, repo, true); err != nil {
			t.Fatal(err)
		}
	})
	for name, realm := range map[string]string{
		"realm on a private IP":           "https://10.0.0.5/token",
		"realm on loopback":               "https://127.0.0.1:" + srvPort(rg.Server) + "/token",
		"realm name resolving to private": "https://internal.example.com/token?secret=REALMSECRET",
		"realm metadata":                  "https://169.254.169.254/latest/meta-data",
	} {
		t.Run(name, func(t *testing.T) {
			rg.set(func() { rg.requireToken, rg.realm = true, realm })
			defer rg.set(func() { rg.requireToken, rg.realm = false, "" })
			_, err := strict().FetchOCIIndex(ctx, repo, true)
			if err == nil || !strings.Contains(err.Error(), "not a public address") {
				t.Fatalf("want refusal, got %v", err)
			}
			noLeak(t, err)
			if strings.Contains(err.Error(), "REALMSECRET") {
				t.Fatalf("realm query leaked: %v", err)
			}
		})
	}
	t.Run("cross-host http realm", func(t *testing.T) {
		rg.set(func() { rg.requireToken, rg.realm = true, "http://auth.example.org/token" })
		defer rg.set(func() { rg.requireToken, rg.realm = false, "" })
		_, err := strict().FetchOCIIndex(ctx, repo, true)
		if err == nil || !strings.Contains(err.Error(), "must use https") {
			t.Fatalf("want refusal, got %v", err)
		}
	})
	t.Run("cross-host realm is held to the public rule even with private allowed", func(t *testing.T) {
		rg.set(func() { rg.requireToken, rg.realm = true, "https://internal.example.com/token" })
		defer rg.set(func() { rg.requireToken, rg.realm = false, "" })
		f := routedFetcher(HelmFetchPolicy{AllowPrivate: true}, res, target, nil)
		trustServer(f, rg.Server)
		_, err := f.FetchOCIIndex(ctx, repo, true)
		if err == nil || !strings.Contains(err.Error(), "not a public address") {
			t.Fatalf("want refusal, got %v", err)
		}
	})
	for name, to := range map[string]string{
		"blob redirect to loopback": "https://127.0.0.1:" + srvPort(rg.Server),
		"blob redirect to private":  "https://internal.example.com",
		"blob redirect to http":     "http://example.com",
	} {
		t.Run(name, func(t *testing.T) {
			rg.set(func() { rg.redirectTo = to })
			defer rg.set(func() { rg.redirectTo = "" })
			_, _, err := strict().FetchOCIChartValues(ctx, repo.URL, "nginx", "1.2.0")
			if err == nil || HTTPStatus(err) != 502 || !strings.Contains(err.Error(), "refused") {
				t.Fatalf("want refusal, got %v", err)
			}
			noLeak(t, err)
		})
	}
	t.Run("pagination link to another host refused", func(t *testing.T) {
		c, _ := newHelmOCIClient(strict(), repo.URL)
		cur := mustURL(t, "https://example.com/v2/charts/nginx/tags/list?n=100")
		for _, l := range []string{`<https://evil.example.org/v2/charts/nginx/tags/list?last=a>; rel="next"`, `</v2/other/tags/list?last=a>; rel="next"`} {
			if _, _, err := c.nextLink(cur, http.Header{"Link": {l}}); err == nil {
				t.Errorf("%s accepted", l)
			}
		}
		next, ok, err := c.nextLink(cur, http.Header{"Link": {`</v2/charts/nginx/tags/list?last=a&n=100>; rel="next"`}})
		if err != nil || !ok || next.String() != "https://example.com/v2/charts/nginx/tags/list?last=a&n=100" {
			t.Fatalf("%v %v %v", next, ok, err)
		}
	})
}

func TestHelmOCIAndIndexCatalogMerge(t *testing.T) {
	rs := newRepoServer(t, false)
	rg := standardOCIReg(t, false)
	c, _ := newTestCatalog(t, rs) // index repository "main"
	ctx := context.Background()
	if _, err := c.AddHelmRepo(ctx, HelmRepoRequest{Name: "a-oci", URL: rg.ociURL("/charts"), Charts: []string{"nginx", "redis"}}); err != nil {
		t.Fatal(err)
	}
	repos, _ := c.Repos(ctx)
	if len(repos) != 2 || repos[0].Kind != "index" || repos[1].Kind != "oci" {
		t.Fatalf("repos: %+v", repos)
	}
	res, err := c.Search(ctx, "", "nginx", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	// Exact name first, an OCI repo sorts by repo name among equals.
	if len(res.Charts) < 2 || res.Charts[0].Name != "nginx" || res.Charts[0].Repo != "a-oci" || res.Charts[0].Kind != "oci" ||
		res.Charts[1].Name != "nginx" || res.Charts[1].Repo != "main" || res.Charts[1].Kind != "index" || res.Charts[1].Ref != "" {
		t.Fatalf("merge order: %+v", res.Charts[:2])
	}
	res, _ = c.Search(ctx, "a-oci", "", 0, false)
	if res.Total != 2 {
		t.Fatalf("one OCI repo: %+v", res)
	}
	// The 20-repository cap is shared.
	for i := 2; i < helmMaxRepos; i++ {
		if _, err := c.AddRepo(ctx, fmt.Sprintf("r%d", i), rs.URL+fmt.Sprintf("/p%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = c.AddHelmRepo(ctx, HelmRepoRequest{Name: "full", URL: rg.ociURL("/other"), Charts: []string{"nginx"}})
	if HTTPStatus(err) != 400 || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("cap: %v", err)
	}
}

func TestHelmOCIRefreshSkipsBrokenCharts(t *testing.T) {
	rg := standardOCIReg(t, false)
	f := &HelmFetcher{Policy: openPolicy}
	ctx := context.Background()
	repo := HelmRepo{URL: rg.ociURL("/charts"), Kind: "oci", Charts: []string{"nginx", "gone"}}
	if _, err := f.FetchOCIIndex(ctx, repo, true); err == nil {
		t.Fatal("strict fetch must fail on a missing chart")
	}
	ix, err := f.FetchOCIIndex(ctx, repo, false)
	if err != nil || ix.chartCount() != 1 {
		t.Fatalf("lenient fetch: %v %v", ix, err)
	}
	repo.Charts = []string{"gone"}
	if _, err := f.FetchOCIIndex(ctx, repo, false); err == nil {
		t.Fatal("no chart at all must fail")
	}
}

func TestHelmOCIKindValidation(t *testing.T) {
	c := newOCICatalog(t, &HelmFetcher{Policy: openPolicy})
	ctx := context.Background()
	for _, req := range []HelmRepoRequest{
		{Name: "x", URL: "oci://ghcr.io/org", Kind: "index"},
		{Name: "x", URL: "https://charts.example.com", Kind: "oci"},
		{Name: "x", URL: "https://charts.example.com", Kind: "git"},
		{Name: "x", URL: "https://charts.example.com", Charts: []string{"a"}},
		{Name: "x", URL: "oci://ghcr.io/org", Charts: []string{"Bad"}},
	} {
		if _, err := c.AddHelmRepo(ctx, req); HTTPStatus(err) != 400 {
			t.Errorf("%+v: %v", req, err)
		}
	}
}
