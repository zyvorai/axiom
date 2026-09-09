package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

// validKubeconfigYAML is syntactically complete enough for
// clientcmd.RESTConfigFromKubeConfig (the `kubernetes`-tagged
// ProviderFromKubeconfigYAML, see internal/platform/remote_clusters_kubernetes.go)
// to build a rest.Config and client from it — building a client never
// dials the apiserver, so this works without a live cluster. It's also
// accepted as-is by the non-kubernetes stub (remote_clusters_stub.go),
// which doesn't parse kubeconfig content at all — so this fixture registers
// successfully under both build variants.
const validKubeconfigYAML = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://cluster.example.invalid:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: fake-token
`

func registerBody(t *testing.T, name, label string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"name": name, "label": label, "kubeconfig": validKubeconfigYAML})
	if err != nil {
		t.Fatalf("marshal register body: %v", err)
	}
	return string(b)
}

func TestRegisterClusterUnavailableWithoutSwitchableProvider(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "POST", "/api/v1/clusters", `{"name":"a","kubeconfig":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when the provider isn't switchable, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "multi-cluster registry is not available") {
		t.Fatalf("expected a multi-cluster-unavailable error, got: %s", w.Body.String())
	}
}

func TestUnregisterClusterUnavailableWithoutSwitchableProvider(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "DELETE", "/api/v1/clusters/whatever", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when the provider isn't switchable, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "multi-cluster registry is not available") {
		t.Fatalf("expected a multi-cluster-unavailable error, got: %s", w.Body.String())
	}
}

func TestRegisterClusterRejectsMalformedJSON(t *testing.T) {
	h := New(platform.NewDemoSwitchableProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "POST", "/api/v1/clusters", `{not valid json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON body, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegisterClusterRequiresName(t *testing.T) {
	h := New(platform.NewDemoSwitchableProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "POST", "/api/v1/clusters", `{"kubeconfig":"anything"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing name, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "name is required") {
		t.Fatalf("expected a name-required error, got: %s", w.Body.String())
	}
}

// The non-kubernetes build's ProviderFromKubeconfigYAML (see
// internal/platform/remote_clusters_stub.go) only checks that the
// kubeconfig field isn't blank — it never parses it as YAML. Full
// kubeconfig validation happens in the `kubernetes`-tagged build via
// clientcmd.RESTConfigFromKubeConfig, which requires the kubernetes build
// tag to exercise; that path is not covered here.
func TestRegisterClusterRequiresKubeconfig(t *testing.T) {
	h := New(platform.NewDemoSwitchableProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "POST", "/api/v1/clusters", `{"name":"new-cluster"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing kubeconfig, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "kubeconfig is required") {
		t.Fatalf("expected a kubeconfig-required error, got: %s", w.Body.String())
	}
}

func TestRegisterClusterDefaultsLabelToName(t *testing.T) {
	h := New(platform.NewDemoSwitchableProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "POST", "/api/v1/clusters", registerBody(t, "no-label-cluster", ""))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"label":"no-label-cluster"`) {
		t.Fatalf("expected label to default to the cluster name, got: %s", body)
	}
}

func TestRegisterAndUnregisterClusterRoundTrip(t *testing.T) {
	h := New(platform.NewDemoSwitchableProvider(), "demo", "demo", "demo", "")

	w := request(t, h, "POST", "/api/v1/clusters", registerBody(t, "stage-1", "Stage One"))
	if w.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	// "server" is the cluster's API server address, which differs between
	// build variants (a "demo://" URL on the non-kubernetes stub vs. the
	// kubeconfig's real server URL under the kubernetes build), so only
	// check that it's present, not its exact value.
	var created struct {
		OK     bool   `json:"ok"`
		Name   string `json:"name"`
		Label  string `json:"label"`
		Server string `json:"server"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode register response: %v (%s)", err, w.Body.String())
	}
	if created.Name != "stage-1" || created.Label != "Stage One" || created.Server == "" {
		t.Fatalf("unexpected register response: %+v", created)
	}

	w = request(t, h, "GET", "/api/v1/clusters", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"name":"stage-1"`) {
		t.Fatalf("expected the newly registered cluster to appear in the list: %s", w.Body.String())
	}

	found := map[string]bool{}
	for _, e := range platform.ListAxiomAudit() {
		found[e.Action] = true
	}
	if !found["cluster.register"] {
		t.Fatalf("expected cluster.register in the audit log")
	}

	w = request(t, h, "DELETE", "/api/v1/clusters/stage-1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("unregister: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("expected ok:true, got: %s", w.Body.String())
	}

	w = request(t, h, "GET", "/api/v1/clusters", "")
	if strings.Contains(w.Body.String(), `"name":"stage-1"`) {
		t.Fatalf("expected stage-1 to be gone from the list after unregister: %s", w.Body.String())
	}

	found = map[string]bool{}
	for _, e := range platform.ListAxiomAudit() {
		found[e.Action] = true
	}
	if !found["cluster.unregister"] {
		t.Fatalf("expected cluster.unregister in the audit log")
	}
}

func TestUnregisterClusterUnknownName(t *testing.T) {
	h := New(platform.NewDemoSwitchableProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "DELETE", "/api/v1/clusters/does-not-exist", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown cluster name, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unknown cluster") {
		t.Fatalf("expected an unknown-cluster error, got: %s", w.Body.String())
	}
}

func TestUnregisterClusterRefusesLastCluster(t *testing.T) {
	sp := platform.NewSwitchableProvider()
	sp.Register("only-one", "Only One", platform.NewDemoProvider(), "demo://only-one")
	h := New(sp, "demo", "demo", "demo", "")

	w := request(t, h, "DELETE", "/api/v1/clusters/only-one", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when trying to unregister the last remaining cluster, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "cannot unregister the last cluster") {
		t.Fatalf("expected a last-cluster error, got: %s", w.Body.String())
	}
}

func TestClustersEndpointsRequireAuth(t *testing.T) {
	h := New(platform.NewDemoSwitchableProvider(), "demo", "demo", "demo", "")
	r := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", strings.NewReader(`{"name":"x","kubeconfig":"y"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected register to require auth, got %d: %s", w.Code, w.Body.String())
	}
}
