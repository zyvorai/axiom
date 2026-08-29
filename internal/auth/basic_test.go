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

type fakeUserAuthenticator struct {
	users map[string]struct {
		password   string
		role       string
		namespaces []string
	}
}

func (f *fakeUserAuthenticator) Authenticate(username, password string) (string, []string, bool) {
	u, ok := f.users[username]
	if !ok || u.password != password {
		return "", nil, false
	}
	return u.role, u.namespaces, true
}

func TestGuardMultiUserAndReadOnly(t *testing.T) {
	users := &fakeUserAuthenticator{users: map[string]struct {
		password   string
		role       string
		namespaces []string
	}{
		"alice": {password: "viewerpass", role: "viewer"},
		"bob":   {password: "adminpass", role: "admin"},
	}}
	g := Guard{Username: "demo", Password: "demo", Users: users}
	var gotIdentity Identity
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity, _ = IdentityFromContext(r.Context())
		w.WriteHeader(204)
	}))

	// Viewer can read (GET).
	r := httptest.NewRequest("GET", "/api/v1/overview", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:viewerpass")))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("viewer GET: expected 204 got %d", w.Code)
	}
	if gotIdentity.User != "alice" || gotIdentity.Role != "viewer" {
		t.Fatalf("expected identity alice/viewer, got %+v", gotIdentity)
	}

	// Viewer is blocked from POST.
	r = httptest.NewRequest("POST", "/api/v1/namespaces", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:viewerpass")))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("viewer POST: expected 403 got %d", w.Code)
	}

	// Admin (from UserStore, not the env account) can POST.
	r = httptest.NewRequest("POST", "/api/v1/namespaces", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("bob:adminpass")))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("admin POST: expected 204 got %d", w.Code)
	}

	// Wrong password is rejected.
	r = httptest.NewRequest("GET", "/api/v1/overview", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:wrong")))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("wrong password: expected 401 got %d", w.Code)
	}

	// The env-configured operator account is always treated as admin.
	r = httptest.NewRequest("POST", "/api/v1/namespaces", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("demo:demo")))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("env admin POST: expected 204 got %d", w.Code)
	}
}

func TestGuardNamespaceRestriction(t *testing.T) {
	users := &fakeUserAuthenticator{users: map[string]struct {
		password   string
		role       string
		namespaces []string
	}{
		"carol": {password: "viewerpass", role: "viewer", namespaces: []string{"team-a", "team-b"}},
	}}
	g := Guard{Username: "demo", Password: "demo", Users: users}
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))

	auth := func(r *http.Request) {
		r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("carol:viewerpass")))
	}

	cases := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"allowed namespace via query param", "/api/v1/pods?namespace=team-a", 204},
		{"disallowed namespace via query param", "/api/v1/pods?namespace=team-c", 403},
		{"no namespace param on a namespaced list endpoint means all — blocked", "/api/v1/pods", 403},
		{"explicit all is blocked", "/api/v1/pods?namespace=all", 403},
		{"allowed namespace via path segment", "/api/v1/secrets/team-b/my-secret", 204},
		{"disallowed namespace via path segment", "/api/v1/secrets/team-c/my-secret", 403},
		// Regression coverage for a real gap: these six were missing from
		// namespacedResourcePrefixes entirely, so a restricted viewer could
		// read them across every namespace before this test existed.
		{"hpas: allowed namespace", "/api/v1/hpas?namespace=team-a", 204},
		{"hpas: disallowed namespace", "/api/v1/hpas?namespace=team-c", 403},
		{"networkpolicies: allowed namespace", "/api/v1/networkpolicies?namespace=team-a", 204},
		{"networkpolicies: disallowed namespace", "/api/v1/networkpolicies?namespace=team-c", 403},
		{"poddisruptionbudgets: disallowed namespace", "/api/v1/poddisruptionbudgets?namespace=team-c", 403},
		{"gateways: disallowed namespace", "/api/v1/gateways?namespace=team-c", 403},
		{"haven planes: disallowed namespace", "/api/v1/haven/planes?namespace=team-c", 403},
		{"crd instances: disallowed namespace", "/api/v1/crds/apps/v1/deployments?namespace=team-c", 403},
		{"non-namespaced endpoint is unaffected", "/api/v1/nodes", 204},
		{"users endpoint (admin-only surface) is unaffected", "/api/v1/session", 204},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tc.path, nil)
			auth(r)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.wantStatus {
				t.Fatalf("%s: expected %d got %d (body: %s)", tc.path, tc.wantStatus, w.Code, w.Body.String())
			}
		})
	}

	// An unrestricted viewer (no namespaces) must not be affected at all.
	usersUnrestricted := &fakeUserAuthenticator{users: map[string]struct {
		password   string
		role       string
		namespaces []string
	}{
		"dave": {password: "viewerpass", role: "viewer"},
	}}
	g2 := Guard{Username: "demo", Password: "demo", Users: usersUnrestricted}
	h2 := g2.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "/api/v1/pods", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("dave:viewerpass")))
	w := httptest.NewRecorder()
	h2.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("unrestricted viewer on namespaced list endpoint: expected 204 got %d", w.Code)
	}
}
