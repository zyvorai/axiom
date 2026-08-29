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
