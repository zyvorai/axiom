package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/platform"
)

func aiHandler(t *testing.T, mode string, in platform.IntegrationsPatch) http.Handler {
	t.Helper()
	hub := platform.NewIntegrationHub(platform.Integrations{}, nil)
	if _, err := hub.Update(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	return NewWithHub(platform.NewDemoProvider(), mode, "demo", "demo", "", hub)
}

func bptr(b bool) *bool { return &b }

func TestAIExplainDisabledByDefault(t *testing.T) {
	h := aiHandler(t, "demo", platform.IntegrationsPatch{})
	w := request(t, h, "GET", "/api/v1/ai/status", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	w = request(t, h, "POST", "/api/v1/ai/explain?namespace=observability", `{"kind":"event","name":"Pod/otel-collector-69d8"}`)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "disabled") {
		t.Fatalf("disabled explain must be 409: %d %s", w.Code, w.Body.String())
	}
}

func TestAIExplainDemoFlowAuditsTargetOnly(t *testing.T) {
	h := aiHandler(t, "demo", platform.IntegrationsPatch{AIEnabled: bptr(true)})
	body := `{"kind":"event","name":"Pod/otel-collector-69d8","question":"why is it failing? password=hunter2-private"}`

	// Preview: redacted payload, no answer, no audit entry.
	before := len(platform.ListAxiomAudit())
	w := request(t, h, "POST", "/api/v1/ai/explain?namespace=observability", strings.TrimSuffix(body, "}")+`,"preview":true}`)
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var res platform.AIExplainResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Answer != "" || strings.Contains(res.Payload.Prompt, "hunter2-private") || !res.Destination.Redacted || !res.Destination.Demo {
		t.Fatalf("bad preview: %+v", res)
	}
	if len(platform.ListAxiomAudit()) != before {
		t.Fatal("preview must not write an audit entry")
	}

	// Send: canned answer + audit with target only.
	w = request(t, h, "POST", "/api/v1/ai/explain?namespace=observability", body)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Demo assistant") {
		t.Fatalf("send: %d %s", w.Code, w.Body.String())
	}
	var found bool
	for _, e := range platform.ListAxiomAudit() {
		if e.Action != "ai.explain" {
			continue
		}
		found = true
		if e.Target != "observability/event/Pod/otel-collector-69d8" || e.Detail != "" {
			t.Fatalf("audit must carry the target only: %+v", e)
		}
		if strings.Contains(e.Target+e.Detail, "hunter2") {
			t.Fatal("prompt text must never reach the audit log")
		}
		break
	}
	if !found {
		t.Fatal("ai.explain audit entry missing")
	}
}

func TestAIExplainRequiresNamespaceAndRejectsBodyNamespace(t *testing.T) {
	h := aiHandler(t, "demo", platform.IntegrationsPatch{AIEnabled: bptr(true)})
	w := request(t, h, "POST", "/api/v1/ai/explain", `{"kind":"pod","name":"x"}`)
	if w.Code != 400 {
		t.Fatalf("missing namespace must be 400: %d %s", w.Code, w.Body.String())
	}
	w = request(t, h, "POST", "/api/v1/ai/explain?namespace=all", `{"kind":"pod","name":"x"}`)
	if w.Code != 400 {
		t.Fatalf("namespace=all must be 400: %d", w.Code)
	}
	// A namespace smuggled in the body is an unknown field: the guard only ever
	// authorizes the query value, so the handler must not honour another.
	w = request(t, h, "POST", "/api/v1/ai/explain?namespace=observability", `{"kind":"pod","name":"x","namespace":"kube-system"}`)
	if w.Code != 400 {
		t.Fatalf("body namespace must be rejected: %d %s", w.Code, w.Body.String())
	}
}

func TestAIExplainRateLimited(t *testing.T) {
	h := aiHandler(t, "demo", platform.IntegrationsPatch{AIEnabled: bptr(true)})
	var last *httptest.ResponseRecorder
	for i := 0; i < aiSendPerMinute+1; i++ {
		last = request(t, h, "POST", "/api/v1/ai/explain?namespace=observability", `{"kind":"event","name":"Pod/otel-collector-69d8"}`)
	}
	if last.Code != http.StatusTooManyRequests || last.Header().Get("Retry-After") == "" {
		t.Fatalf("expected 429 with Retry-After, got %d %v", last.Code, last.Header())
	}
	// Previews are throttled in their own bucket, so they still work here.
	w := request(t, h, "POST", "/api/v1/ai/explain?namespace=observability", `{"kind":"event","name":"Pod/otel-collector-69d8","preview":true}`)
	if w.Code != 200 {
		t.Fatalf("preview bucket should be independent: %d", w.Code)
	}
}

func TestAIRateLimiterWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &aiRateLimiter{now: func() time.Time { return now }}
	for i := 0; i < 3; i++ {
		if ok, _ := l.allow("u", 3, time.Minute); !ok {
			t.Fatal("within limit")
		}
	}
	if ok, wait := l.allow("u", 3, time.Minute); ok || wait <= 0 {
		t.Fatal("4th must be refused with a wait")
	}
	if ok, _ := l.allow("other", 3, time.Minute); !ok {
		t.Fatal("limit is per user")
	}
	now = now.Add(61 * time.Second)
	if ok, _ := l.allow("u", 3, time.Minute); !ok {
		t.Fatal("window must slide")
	}
}

func TestIntegrationsAIKeyNeverReturned(t *testing.T) {
	h := aiHandler(t, "demo", platform.IntegrationsPatch{})
	w := request(t, h, "PUT", "/api/v1/integrations", `{"aiEnabled":true,"aiProvider":"openai-compatible","aiBaseUrl":"https://llm.example.com/v1","aiModel":"m","aiApiKey":"sk-live-supersecretkey123"}`)
	if w.Code != 200 {
		t.Fatalf("put: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "supersecretkey123") || !strings.Contains(w.Body.String(), `"aiApiKeySet":true`) {
		t.Fatalf("api key must not be returned: %s", w.Body.String())
	}
	w = request(t, h, "GET", "/api/v1/integrations", "")
	if strings.Contains(w.Body.String(), "supersecretkey123") {
		t.Fatalf("GET leaked key: %s", w.Body.String())
	}
	// Status (visible to every user) never has it either.
	w = request(t, h, "GET", "/api/v1/ai/status", "")
	if strings.Contains(w.Body.String(), "supersecretkey123") {
		t.Fatalf("status leaked key: %s", w.Body.String())
	}
	for _, bad := range []string{
		`{"aiBaseUrl":"file:///etc/passwd"}`,
		`{"aiBaseUrl":"ftp://x"}`,
		`{"aiProvider":"skynet"}`,
	} {
		if w := request(t, h, "PUT", "/api/v1/integrations", bad); w.Code != 400 {
			t.Errorf("%s should be 400, got %d", bad, w.Code)
		}
	}
}

func TestAITestEndpointDemo(t *testing.T) {
	h := aiHandler(t, "demo", platform.IntegrationsPatch{})
	w := request(t, h, "POST", "/api/v1/integrations/ai/test", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) || !strings.Contains(w.Body.String(), `"service":"ai"`) {
		t.Fatalf("ai test: %d %s", w.Code, w.Body.String())
	}
}
