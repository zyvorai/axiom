package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

// newHelmOCIRegistry serves charts/nginx (1.0.0, 1.2.0, latest) over the OCI
// Distribution API behind an anonymous bearer challenge.
func newHelmOCIRegistry(t *testing.T) *httptest.Server {
	t.Helper()
	blobs := map[string][]byte{}
	manifests := map[string][]byte{}
	put := func(b []byte) string {
		h := sha256.Sum256(b)
		d := "sha256:" + hex.EncodeToString(h[:])
		blobs[d] = b
		return d
	}
	tgz := helmTestTgz(t, map[string]string{"nginx/values.yaml": "replicaCount: 4\n", "nginx/README.md": "# nginx (oci)"})
	for _, v := range []string{"1.0.0", "1.2.0"} {
		cfg, _ := json.Marshal(map[string]any{"name": "nginx", "version": v, "appVersion": "1.25", "description": "Web server via OCI", "keywords": []string{"http"}})
		m, _ := json.Marshal(map[string]any{
			"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json",
			"config": map[string]any{"mediaType": "application/vnd.cncf.helm.config.v1+json", "digest": put(cfg), "size": len(cfg)},
			"layers": []any{map[string]any{"mediaType": "application/vnd.cncf.helm.chart.content.v1.tar+gzip", "digest": put(tgz), "size": len(tgz)}},
		})
		manifests[v] = m
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"token":"anon-tok"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer anon-tok" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+srv.URL+`/token",service="mock"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch p := r.URL.Path; {
		case p == "/v2/":
			_, _ = w.Write([]byte("{}"))
		case p == "/v2/charts/nginx/tags/list":
			_, _ = w.Write([]byte(`{"name":"charts/nginx","tags":["1.0.0","latest","1.2.0"]}`))
		case strings.HasPrefix(p, "/v2/charts/nginx/manifests/"):
			m, ok := manifests[strings.TrimPrefix(p, "/v2/charts/nginx/manifests/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			_, _ = w.Write(m)
		case strings.HasPrefix(p, "/v2/charts/nginx/blobs/"):
			b, ok := blobs[strings.TrimPrefix(p, "/v2/charts/nginx/blobs/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(b)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHelmCatalogOCIEndpoints(t *testing.T) {
	reg := newHelmOCIRegistry(t)
	idx := newHelmRepoServer(t)
	e := newHelmEnv(t, platform.NewMemoryHelmRepoStore())
	ociURL := "oci://" + strings.TrimPrefix(reg.URL, "http://") + "/charts"

	if w := e.do("POST", "/api/v1/helm/repos", jsonBody(map[string]string{"name": "stable", "url": idx.URL + "/stable"}), asHelm("admin")); w.Code != 201 {
		t.Fatalf("index add: %d %s", w.Code, w.Body.String())
	}
	body := jsonBody(map[string]any{"name": "ocirepo", "url": ociURL, "charts": []string{"nginx"}})
	for _, who := range []string{"operator", "viewer", "scoped"} {
		if w := e.do("POST", "/api/v1/helm/repos", body, asHelm(who)); w.Code != 403 {
			t.Errorf("POST as %s: %d", who, w.Code)
		}
	}
	w := e.do("POST", "/api/v1/helm/repos", body, asHelm("admin"))
	if w.Code != 201 {
		t.Fatalf("oci add: %d %s", w.Code, w.Body.String())
	}
	var added platform.HelmRepoView
	decodeInto(t, w, &added)
	if added.Kind != "oci" || added.URL != ociURL || added.ChartCount != 1 {
		t.Fatalf("added: %+v", added)
	}

	bad := []struct {
		body any
		want int
	}{
		{map[string]any{"name": "b1", "url": ociURL + "/sub", "charts": []string{"missing"}}, 400},
		{map[string]any{"name": "b2", "url": ociURL + "x", "charts": []string{"Bad_Name"}}, 400},
		{map[string]any{"name": "b3", "url": "oci://u:p@" + strings.TrimPrefix(reg.URL, "http://") + "/x"}, 400},
		{map[string]any{"name": "b4", "url": ociURL + "/y", "charts": []string{"nginx"}, "credentialsSecret": "reg-creds"}, 400},
		{map[string]any{"name": "b5", "url": ociURL + "/z", "kind": "index"}, 400},
		{map[string]any{"name": "ocirepo2", "url": ociURL + "/", "charts": []string{"nginx"}}, 409},
	}
	for _, b := range bad {
		if w := e.do("POST", "/api/v1/helm/repos", jsonBody(b.body), asHelm("admin")); w.Code != b.want {
			t.Errorf("POST %v: %d, want %d (%s)", b.body, w.Code, b.want, w.Body.String())
		}
	}

	var repos []platform.HelmRepoView
	decodeInto(t, e.do("GET", "/api/v1/helm/repos", "", asHelm("viewer")), &repos)
	if len(repos) != 2 || repos[0].Kind != "index" || repos[1].Kind != "oci" {
		t.Fatalf("repos: %+v", repos)
	}
	if w := e.do("GET", "/api/v1/helm/repos", "", asHelm("viewer")); !strings.Contains(w.Body.String(), `"kind":"index"`) {
		t.Errorf("index rows must carry kind: %s", w.Body.String())
	}

	var res platform.HelmChartSearch
	w = e.do("GET", "/api/v1/helm/charts?q=nginx", "", asHelm("viewer"))
	decodeInto(t, w, &res)
	if w.Code != 200 || len(res.Charts) != 2 {
		t.Fatalf("search: %d %s", w.Code, w.Body.String())
	}
	o := res.Charts[0]
	if o.Repo != "ocirepo" || o.Kind != "oci" || o.Version != "1.2.0" || o.Ref != ociURL+"/nginx" || o.Description != "Web server via OCI" {
		t.Fatalf("oci chart: %+v", o)
	}
	if res.Charts[1].Repo != "stable" || res.Charts[1].Kind != "index" {
		t.Fatalf("index chart: %+v", res.Charts[1])
	}

	var d platform.HelmChartDetail
	w = e.do("GET", "/api/v1/helm/charts/ocirepo/nginx", "", asHelm("operator"))
	decodeInto(t, w, &d)
	if w.Code != 200 || len(d.Versions) != 2 || d.Versions[0].Version != "1.2.0" || d.Ref != ociURL+"/nginx" || d.Kind != "oci" {
		t.Fatalf("detail: %d %s", w.Code, w.Body.String())
	}
	var v platform.HelmChartValues
	w = e.do("GET", "/api/v1/helm/charts/ocirepo/nginx/values?version=1.0.0", "", asHelm("viewer"))
	decodeInto(t, w, &v)
	if w.Code != 200 || v.Version != "1.0.0" || v.Values != "replicaCount: 4\n" || v.Readme != "# nginx (oci)" {
		t.Fatalf("values: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/helm/charts/ocirepo/nginx/values", "", asHelm("scoped")); w.Code != 403 {
		t.Errorf("scoped read: %d", w.Code)
	}

	// Install with the chart's ref.
	inst := jsonBody(map[string]string{"namespace": "default", "releaseName": "oci-web", "ociRef": o.Ref, "chartVersion": "1.2.0"})
	if w := e.do("POST", "/api/v1/helm/install", inst, asHelm("operator")); w.Code != 403 {
		t.Errorf("operator install: %d", w.Code)
	}
	w = e.do("POST", "/api/v1/helm/install", inst, asHelm("admin"))
	if w.Code != 202 {
		t.Fatalf("install: %d %s", w.Code, w.Body.String())
	}
	found := false
	for _, a := range platform.ListAxiomAudit() {
		if a.Action == "helm.install" && a.Target == "default/oci-web" && a.Detail == o.Ref {
			found = true
		}
	}
	if !found {
		t.Error("OCI install was not audited with its ref")
	}
	for _, ref := range []string{"oci://u:p@ghcr.io/org/nginx", "oci://ghcr.io/org/nginx --devel", "https://ghcr.io/org/nginx", "oci://ghcr.io"} {
		b := jsonBody(map[string]string{"namespace": "default", "releaseName": "bad-ref", "ociRef": ref})
		if w := e.do("POST", "/api/v1/helm/install", b, asHelm("admin")); w.Code != 400 {
			t.Errorf("install %q: %d %s", ref, w.Code, w.Body.String())
		}
	}

	if w := e.do("DELETE", "/api/v1/helm/repos/ocirepo", "", asHelm("admin")); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
}
