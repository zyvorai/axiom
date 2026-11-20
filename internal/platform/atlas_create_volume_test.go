package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// recordedRequest is what the fake Atlas gateway saw.
type recordedRequest struct {
	Method string
	Path   string
	Auth   string
	CT     string
	Body   map[string]any
}

func newFakeAtlas(t *testing.T, status int, respBody string) (*AtlasClient, func() []recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		seen = append(seen, recordedRequest{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), body})
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return NewAtlasClient(srv.URL+"/", "tok-123"), func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), seen...)
	}
}

func TestAtlasCreateVolumeSendsIntentPayload(t *testing.T) {
	c, seen := newFakeAtlas(t, http.StatusAccepted, `{"id":"vol-1","state":"provisioning"}`)
	raw, code, err := c.CreateVolume(context.Background(), CreateAtlasVolumeRequest{
		Name: "pg-data", Policy: "database", SizeGi: 50, Tenant: "acme", Namespace: "prod", CreatePVC: true,
	})
	if err != nil {
		t.Fatalf("CreateVolume: %v", err)
	}
	if code != http.StatusAccepted || !strings.Contains(string(raw), "vol-1") {
		t.Fatalf("expected the gateway response to be passed through, got code=%d body=%s", code, raw)
	}
	reqs := seen()
	if len(reqs) != 1 {
		t.Fatalf("expected exactly one request, got %d", len(reqs))
	}
	r := reqs[0]
	if r.Method != http.MethodPost || r.Path != "/api/atlas/v1/volumes" {
		t.Fatalf("unexpected request %s %s", r.Method, r.Path)
	}
	if r.Auth != "Bearer tok-123" || r.CT != "application/json" {
		t.Fatalf("missing bearer token / content type: auth=%q ct=%q", r.Auth, r.CT)
	}
	if r.Body["name"] != "pg-data" || r.Body["policy"] != "database" || r.Body["kind"] != "block" || r.Body["tenant_id"] != "acme" {
		t.Fatalf("unexpected payload: %+v", r.Body)
	}
	if size, _ := r.Body["size_bytes"].(float64); int64(size) != 50*1024*1024*1024 {
		t.Fatalf("size_bytes = %v, want 50Gi", r.Body["size_bytes"])
	}
	k8s, _ := r.Body["kubernetes"].(map[string]any)
	if k8s["create_pvc"] != true || k8s["namespace"] != "prod" {
		t.Fatalf("kubernetes section wrong: %+v", r.Body["kubernetes"])
	}
	owner, _ := r.Body["owner"].(map[string]any)
	if owner["product"] != "axiom" {
		t.Fatalf("owner must identify axiom: %+v", r.Body["owner"])
	}
}

func TestAtlasCreateVolumeDefaults(t *testing.T) {
	c, seen := newFakeAtlas(t, http.StatusOK, ``)
	raw, code, err := c.CreateVolume(context.Background(), CreateAtlasVolumeRequest{Name: "shared-data", Policy: "shared"})
	if err != nil {
		t.Fatalf("CreateVolume: %v", err)
	}
	if code != http.StatusOK || string(raw) != "{}" {
		t.Fatalf("an empty upstream body must be normalised to {}, got code=%d body=%q", code, raw)
	}
	r := seen()[0]
	if size, _ := r.Body["size_bytes"].(float64); int64(size) != 10*1024*1024*1024 {
		t.Fatalf("expected default size 10Gi, got %v", r.Body["size_bytes"])
	}
	if r.Body["kind"] != "filesystem" {
		t.Fatalf("policy 'shared' must create a filesystem volume, got %v", r.Body["kind"])
	}
	k8s, _ := r.Body["kubernetes"].(map[string]any)
	if k8s["namespace"] != "default" || k8s["create_pvc"] != false {
		t.Fatalf("expected default namespace and create_pvc=false: %+v", r.Body["kubernetes"])
	}
	if _, has := r.Body["tenant_id"]; has {
		t.Fatalf("tenant_id must be omitted when no tenant was given: %+v", r.Body)
	}
}

func TestAtlasCreateVolumeRejectsInvalidInputWithoutCallingAtlas(t *testing.T) {
	c, seen := newFakeAtlas(t, http.StatusOK, `{}`)
	for name, req := range map[string]CreateAtlasVolumeRequest{
		"no name":   {Policy: "production"},
		"no policy": {Name: "x"},
	} {
		if _, _, err := c.CreateVolume(context.Background(), req); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	if len(seen()) != 0 {
		t.Fatalf("invalid requests must not reach Atlas, got %d calls", len(seen()))
	}
}

func TestAtlasCreateVolumeNotConfigured(t *testing.T) {
	var c *AtlasClient
	if _, _, err := c.CreateVolume(context.Background(), CreateAtlasVolumeRequest{Name: "x", Policy: "production"}); err == nil {
		t.Fatal("expected an error for a nil client")
	}
	if _, _, err := (&AtlasClient{}).CreateVolume(context.Background(), CreateAtlasVolumeRequest{Name: "x", Policy: "production"}); err == nil {
		t.Fatal("expected an error for an unconfigured client")
	}
}

func TestAtlasCreateVolumeSurfacesUpstreamErrorStatusAndBody(t *testing.T) {
	c, _ := newFakeAtlas(t, http.StatusConflict, `{"error":"volume exists"}`)
	raw, code, err := c.CreateVolume(context.Background(), CreateAtlasVolumeRequest{Name: "x", Policy: "production"})
	if err == nil {
		t.Fatal("expected an error for an upstream 409")
	}
	if code != http.StatusConflict || !strings.Contains(string(raw), "volume exists") {
		t.Fatalf("the upstream status/body must be returned so the API can pass them on: code=%d body=%s", code, raw)
	}
}

func TestAtlasCreateVolumeRejectsNonJSONSuccess(t *testing.T) {
	c, _ := newFakeAtlas(t, http.StatusOK, `<html>SPA fallback</html>`)
	if _, _, err := c.CreateVolume(context.Background(), CreateAtlasVolumeRequest{Name: "x", Policy: "production"}); err == nil {
		t.Fatal("expected a non-JSON 200 response to be treated as an error")
	}
}
