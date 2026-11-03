package api

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func request(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("demo:demo")))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestServerOverviewAndLaunch(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "GET", "/api/v1/overview", "")
	if w.Code != 200 {
		t.Fatalf("overview: %d %s", w.Code, w.Body.String())
	}
	body := `{"namespace":"apps","name":"hello","image":"nginx:1.27","replicas":2,"port":80,"cpu":"250m","memory":"256Mi","environment":{},"expose":true}`
	w = request(t, h, "POST", "/api/v1/workloads", body)
	if w.Code != 201 {
		t.Fatalf("launch: %d %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"name":"hello"`)) {
		t.Fatalf("response missing workload: %s", w.Body.String())
	}
}

// Regression coverage for a real gap: workload/pod/node lifecycle actions
// used to call EmitStateChanged only, with no platform.RecordAxiomAudit
// call anywhere — meaning e.g. deleting a Deployment or evicting a Pod
// left zero trace in the audit log.
func TestCoreLifecycleActionsAreAudited(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")

	w := request(t, h, "POST", "/api/v1/workloads/payments/Deployment/payments-api/restart", "")
	if w.Code != http.StatusOK {
		t.Fatalf("restart: %d %s", w.Code, w.Body.String())
	}
	w = request(t, h, "POST", "/api/v1/pods/commerce/checkout-01/evict", "")
	if w.Code != http.StatusOK {
		t.Fatalf("evict: %d %s", w.Code, w.Body.String())
	}

	found := map[string]bool{}
	for _, e := range platform.ListAxiomAudit() {
		found[e.Action] = true
	}
	for _, action := range []string{"workload.restart", "pod.evict"} {
		if !found[action] {
			t.Fatalf("expected %q in the audit log, got: %v", action, found)
		}
	}
}

func TestHealthDoesNotRequireAuth(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("health expected 200 got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthProvidersExposeKubectlContext(t *testing.T) {
	h := New(platform.NewDemoSwitchableProvider(), "demo", "demo", "demo", "")
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("providers expected 200 got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"mode":"demo"`, `"context":"eu-west-1"`, `"server":"demo://eu-west-1"`, `"contexts":3`} {
		if !bytes.Contains(w.Body.Bytes(), []byte(want)) {
			t.Fatalf("providers missing %s in: %s", want, body)
		}
	}
}

func TestApplyDiffAndGraphEndpoints(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")

	w := request(t, h, "POST", "/api/v1/apply/diff", `{"manifest":"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: fresh\n  namespace: apps\n"}`)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"valid":true`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"exists":false`)) {
		t.Fatalf("diff: %d %s", w.Code, w.Body.String())
	}
	if w = request(t, h, "POST", "/api/v1/apply/diff", `{"manifest":""}`); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"valid":false`)) {
		t.Fatalf("empty diff: %d %s", w.Code, w.Body.String())
	}
	if w = request(t, h, "POST", "/api/v1/apply/diff", `not json`); w.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: %d", w.Code)
	}
	// a diff never changes anything and is not audited
	before := len(platform.ListAxiomAudit())
	request(t, h, "POST", "/api/v1/apply/diff", `{"manifest":"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n"}`)
	if after := len(platform.ListAxiomAudit()); after != before {
		t.Fatalf("diff wrote audit entries: %d -> %d", before, after)
	}

	w = request(t, h, "GET", "/api/v1/graph/payments/Deployment/payments-api", "")
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"root":"deployment/payments/payments-api"`)) {
		t.Fatalf("graph: %d %s", w.Code, w.Body.String())
	}
	if w = request(t, h, "GET", "/api/v1/graph/payments/Deployment/missing", ""); w.Code != http.StatusNotFound {
		t.Fatalf("missing graph: %d %s", w.Code, w.Body.String())
	}
}
