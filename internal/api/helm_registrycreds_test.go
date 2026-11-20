package api

import (
	"bytes"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

// newPrivateHelmOCIRegistry fronts newHelmOCIRegistry with HTTP Basic: every
// /v2/ request without user:pass gets a Basic challenge. It records the
// Authorization headers it sees.
func newPrivateHelmOCIRegistry(t *testing.T, user, pass string) (*httptest.Server, *[]string) {
	t.Helper()
	inner := newHelmOCIRegistry(t)
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if u, p, ok := r.BasicAuth(); !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="private"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		req, _ := http.NewRequest(http.MethodGet, inner.URL+r.URL.RequestURI(), nil)
		req.Header.Set("Authorization", "Bearer anon-tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestHelmRegistryCredentialsEndpoints(t *testing.T) {
	const pass = "pw-endpoints-31f"
	reg, _ := newPrivateHelmOCIRegistry(t, "robot", pass)
	host := strings.TrimPrefix(reg.URL, "http://")
	ociURL := "oci://" + host + "/charts"
	e := newHelmEnv(t, platform.NewMemoryHelmRepoStore())

	w := e.do("GET", "/api/v1/helm/registry-credentials", "", asHelm("admin"))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("empty: %d %q", w.Code, w.Body.String())
	}
	body := jsonBody(map[string]string{"name": "lab", "registry": host, "username": "robot", "password": pass})
	w = e.do("POST", "/api/v1/helm/registry-credentials", body, asHelm("admin"))
	if w.Code != 201 || strings.TrimSpace(w.Body.String()) != `{"ok":true,"name":"lab","registry":"`+host+`","username":"robot"}` {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/v1/helm/registry-credentials", body, asHelm("admin")); w.Code != 409 {
		t.Fatalf("dup: %d %s", w.Code, w.Body.String())
	}
	other := jsonBody(map[string]string{"name": "ghcr", "registry": "ghcr.io", "username": "bot", "password": "x"})
	if w := e.do("POST", "/api/v1/helm/registry-credentials", other, asHelm("admin")); w.Code != 201 {
		t.Fatalf("create ghcr: %d %s", w.Code, w.Body.String())
	}

	// Without credentials the private registry says what to do.
	w = e.do("POST", "/api/v1/helm/repos", jsonBody(map[string]any{"name": "anon", "url": ociURL, "charts": []string{"nginx"}}), asHelm("admin"))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "add them under Helm") {
		t.Fatalf("anonymous add: %d %s", w.Code, w.Body.String())
	}
	w = e.do("POST", "/api/v1/helm/repos", jsonBody(map[string]any{"name": "wrong", "url": ociURL, "charts": []string{"nginx"}, "credentialsSecret": "ghcr"}), asHelm("admin"))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "credentials are for registry ghcr.io") {
		t.Fatalf("other registry: %d %s", w.Code, w.Body.String())
	}
	w = e.do("POST", "/api/v1/helm/repos", jsonBody(map[string]any{"name": "private", "url": ociURL, "charts": []string{"nginx"}, "credentialsSecret": "lab"}), asHelm("admin"))
	if w.Code != 201 {
		t.Fatalf("add with credentials: %d %s", w.Code, w.Body.String())
	}
	var added platform.HelmRepoView
	decodeInto(t, w, &added)
	if !added.Authenticated || added.CredentialsSecret != "lab" || added.ChartCount != 1 || !added.PlainHTTP {
		t.Fatalf("added: %+v", added)
	}
	if w := e.do("POST", "/api/v1/helm/repos", jsonBody(map[string]any{"name": "private", "url": ociURL, "charts": []string{"nginx"}, "credentialsSecret": "lab"}), asHelm("admin")); w.Code != 409 {
		t.Fatalf("re-add: %d", w.Code)
	}

	// Rows show the credential's NAME and nothing else about it, to every reader.
	w = e.do("GET", "/api/v1/helm/repos", "", asHelm("viewer"))
	if !strings.Contains(w.Body.String(), `"credentialsSecret":"lab","authenticated":true`) || strings.Contains(w.Body.String(), pass) || strings.Contains(w.Body.String(), "robot") {
		t.Fatalf("repo rows: %s", w.Body.String())
	}
	var vals platform.HelmChartValues
	w = e.do("GET", "/api/v1/helm/charts/private/nginx/values?version=1.0.0", "", asHelm("viewer"))
	decodeInto(t, w, &vals)
	if w.Code != 200 || vals.Values != "replicaCount: 4\n" {
		t.Fatalf("values through credentials: %d %s", w.Code, w.Body.String())
	}

	var rows []platform.HelmRegistryCredential
	w = e.do("GET", "/api/v1/helm/registry-credentials", "", asHelm("admin"))
	decodeInto(t, w, &rows)
	if len(rows) != 2 || rows[1].Name != "lab" || strings.Join(rows[1].InUse, ",") != "private" || rows[1].Registry != host || rows[1].Username != "robot" || rows[1].Age == "" {
		t.Fatalf("rows: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), pass) || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("listing leaks: %s", w.Body.String())
	}
	w = e.do("DELETE", "/api/v1/helm/registry-credentials/lab", "", asHelm("admin"))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "remove or edit repository private first") {
		t.Fatalf("delete in use: %d %s", w.Code, w.Body.String())
	}

	// Install / upgrade of a chart of that repository: accepted (the demo
	// provider ignores the access), audited with the credential's name only.
	ref := ociURL + "/nginx"
	for _, action := range []string{"install", "upgrade"} {
		w = e.do("POST", "/api/v1/helm/"+action, jsonBody(map[string]string{"namespace": "default", "releaseName": "priv-web", "ociRef": ref, "chartVersion": "1.2.0"}), asHelm("admin"))
		if w.Code != 202 {
			t.Fatalf("%s: %d %s", action, w.Code, w.Body.String())
		}
	}
	got := map[string]string{}
	for _, a := range platform.ListAxiomAudit() {
		if a.Target == "default/priv-web" {
			got[a.Action] = a.Detail
		}
	}
	for _, act := range []string{"helm.install", "helm.upgrade"} {
		if got[act] != ref+" registry-credential=lab" {
			t.Errorf("%s audit detail: %q", act, got[act])
		}
	}

	// Deleting the repository keeps the credential; then it can go.
	if w := e.do("DELETE", "/api/v1/helm/repos/private", "", asHelm("admin")); w.Code != 200 {
		t.Fatalf("repo delete: %d", w.Code)
	}
	w = e.do("GET", "/api/v1/helm/registry-credentials", "", asHelm("admin"))
	if !strings.Contains(w.Body.String(), `"name":"lab"`) {
		t.Fatalf("credential went with the repository: %s", w.Body.String())
	}
	w = e.do("DELETE", "/api/v1/helm/registry-credentials/lab", "", asHelm("admin"))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"name":"lab","ok":true,"registry":"`+host+`"}` {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("DELETE", "/api/v1/helm/registry-credentials/lab", "", asHelm("admin")); w.Code != 404 {
		t.Fatalf("delete twice: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/helm/registry-credentials/Bad_Name", "", asHelm("admin")); w.Code != 400 {
		t.Fatalf("delete bad name: %d", w.Code)
	}
	if w := e.do("PUT", "/api/v1/helm/registry-credentials/ghcr", other, asHelm("admin")); w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("update route exists: %d", w.Code)
	}
	huge := jsonBody(map[string]string{"name": "big", "registry": "ghcr.io", "username": "u", "password": strings.Repeat("x", 17<<10)})
	if w := e.do("POST", "/api/v1/helm/registry-credentials", huge, asHelm("admin")); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("huge: %d", w.Code)
	}
}

func TestHelmRegistryCredentialsAccess(t *testing.T) {
	e := newHelmEnv(t, platform.NewMemoryHelmRepoStore())
	body := jsonBody(map[string]string{"name": "acc", "registry": "ghcr.io", "username": "bot", "password": "pw-access"})
	for _, who := range []string{"operator", "viewer", "scoped", "scopedop"} {
		for _, r := range [][3]string{
			{"GET", "/api/v1/helm/registry-credentials", ""},
			{"POST", "/api/v1/helm/registry-credentials", body},
			{"DELETE", "/api/v1/helm/registry-credentials/acc", ""},
		} {
			if w := e.do(r[0], r[1], r[2], asHelm(who)); w.Code != http.StatusForbidden {
				t.Errorf("%s %s %s: %d", who, r[0], r[1], w.Code)
			}
		}
	}
	if w := e.do("GET", "/api/v1/helm/registry-credentials", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/helm/registry-credentials", body, asHelm("admin")); w.Code != 201 {
		t.Errorf("admin create: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/helm/registry-credentials", "", asHelm("admin")); w.Code != 200 {
		t.Errorf("admin list: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/helm/registry-credentials/acc", "", asHelm("admin")); w.Code != 200 {
		t.Errorf("admin delete: %d", w.Code)
	}
}

// TestHelmRegistryCredentialSecretsNowhere posts a distinctive password
// through every outcome (created, duplicate, validation failure, malformed,
// in use, install) and asserts its bytes never appear in a response, an audit
// entry or the log.
func TestHelmRegistryCredentialSecretsNowhere(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	prevSlog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { log.SetOutput(os.Stderr); slog.SetDefault(prevSlog) })

	const password = "Reg-OMEGA-5b4a3c2d-NOWHERE"
	reg, seen := newPrivateHelmOCIRegistry(t, "robot", password)
	host := strings.TrimPrefix(reg.URL, "http://")
	e := newHelmEnv(t, platform.NewMemoryHelmRepoStore())

	var responses []string
	post := func(body string) {
		w := e.do("POST", "/api/v1/helm/registry-credentials", body, asHelm("admin"))
		responses = append(responses, w.Body.String())
	}
	good := map[string]string{"name": "nowhere", "registry": host, "username": "robot", "password": password}
	post(jsonBody(good))
	post(jsonBody(good)) // duplicate
	for _, mut := range []map[string]string{
		{"registry": "https://robot:" + password + "@" + host},
		{"registry": host + "/" + password},
		{"name": "Bad " + password},
		{"username": "robot:" + password},
		{"password": password + "\n"},
		{"password": password + strings.Repeat("x", 5000)},
		{"username": ""},
	} {
		m := map[string]string{}
		for k, v := range good {
			m[k] = v
		}
		m["name"] = "nowhere-bad"
		for k, v := range mut {
			m[k] = v
		}
		post(jsonBody(m))
	}
	post(`{"name":"x","registry":"` + host + `","password":"` + password + `","extra":"field"}`)
	post(`{"name":"x"} {"password":"` + password + `"}`)
	post(`{"name":"x","password":"` + password)
	post(`{"name":"x","password":` + password + `}`)
	post(`["` + password + `"]`)

	w := e.do("POST", "/api/v1/helm/repos", jsonBody(map[string]any{"name": "nowhere-repo", "url": "oci://" + host + "/charts", "charts": []string{"nginx"}, "credentialsSecret": "nowhere"}), asHelm("admin"))
	if w.Code != 201 {
		t.Fatalf("repo add: %d %s", w.Code, w.Body.String())
	}
	responses = append(responses, w.Body.String())
	w = e.do("POST", "/api/v1/helm/install", jsonBody(map[string]string{"namespace": "default", "releaseName": "nowhere", "ociRef": "oci://" + host + "/charts/nginx"}), asHelm("admin"))
	responses = append(responses, w.Body.String())
	for _, path := range []string{"/api/v1/helm/registry-credentials", "/api/v1/helm/repos", "/api/v1/helm/charts?q=nginx", "/api/v1/audit"} {
		responses = append(responses, e.do("GET", path, "", asHelm("admin")).Body.String())
	}
	responses = append(responses, e.do("DELETE", "/api/v1/helm/registry-credentials/nowhere", "", asHelm("admin")).Body.String())
	responses = append(responses, e.do("DELETE", "/api/v1/helm/repos/nowhere-repo", "", asHelm("admin")).Body.String())
	responses = append(responses, e.do("DELETE", "/api/v1/helm/registry-credentials/nowhere", "", asHelm("admin")).Body.String())

	var audit strings.Builder
	creates, deletes := 0, 0
	for _, a := range platform.ListAxiomAudit() {
		audit.WriteString(a.Action + " " + a.Target + " " + a.Detail + " " + a.User + "\n")
		if a.Target == "nowhere" && a.Action == "helm.registry_credential_create" {
			creates++
			if a.Detail != "registry="+host {
				t.Errorf("create audit detail: %q", a.Detail)
			}
		}
		if a.Target == "nowhere" && a.Action == "helm.registry_credential_delete" {
			deletes++
		}
	}
	if creates != 1 || deletes != 1 {
		t.Errorf("want 1 create and 1 delete audit, got %d/%d:\n%s", creates, deletes, audit.String())
	}
	if logs.Len() == 0 {
		t.Fatal("log capture is not working (the request logger writes nothing)")
	}
	authenticated := false
	for _, a := range *seen {
		authenticated = authenticated || a != ""
	}
	if !authenticated {
		t.Fatal("the registry never received the credential: the test proves nothing")
	}
	for i, r := range responses {
		if strings.Contains(r, password) {
			t.Errorf("response %d leaks the password: %s", i, r)
		}
	}
	if strings.Contains(audit.String(), password) {
		t.Error("audit leaks the password")
	}
	if strings.Contains(logs.String(), password) {
		t.Error("log leaks the password")
	}
}
