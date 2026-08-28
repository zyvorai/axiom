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

func TestHealthDoesNotRequireAuth(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("health expected 200 got %d: %s", w.Code, w.Body.String())
	}
}
