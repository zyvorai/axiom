package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicMiddleware(t *testing.T) {
	h := Basic{Username: "demo", Password: "demo"}.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "/api/v1/overview", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("demo:demo")))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("expected 204 got %d", w.Code)
	}
	r = httptest.NewRequest("GET", "/api/v1/overview", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("expected 401 got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("WWW-Authenticate must not be set (browser popup)")
	}

	r = httptest.NewRequest("GET", "/api/v1/auth/providers", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("providers should be public, got %d", w.Code)
	}

	g := Guard{Username: "demo", Password: "demo", APIKey: "secret-key"}.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r = httptest.NewRequest("GET", "/api/v1/overview", nil)
	r.Header.Set("Authorization", "Bearer secret-key")
	w = httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("expected api key auth 204 got %d", w.Code)
	}

	r = httptest.NewRequest("GET", "/api/v1/overview", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("demo@zyvor.dev:demo")))
	w = httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("demo@zyvor.dev should authenticate when user is demo, got %d", w.Code)
	}
}
