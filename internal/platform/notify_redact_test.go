package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const secretPath = "/services/T000/B000/SuperSecretToken"

func TestWebhookErrorsNeverContainTheSecretURL(t *testing.T) {
	// non-2xx response
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	res := SendSlackMessage(context.Background(), srv.URL+secretPath, "x")
	if res.OK || res.Error == "" {
		t.Fatalf("expected an error result, got %+v", res)
	}
	if strings.Contains(res.Error, "SuperSecretToken") {
		t.Fatalf("error leaks the webhook secret: %q", res.Error)
	}
	srv.Close()

	// connection failure (closed server): *url.Error embeds the full URL
	res = SendAlertWebhook(context.Background(), srv.URL+secretPath, map[string]any{"a": 1})
	if res.OK || res.Error == "" {
		t.Fatalf("expected a connection error, got %+v", res)
	}
	if strings.Contains(res.Error, "SuperSecretToken") {
		t.Fatalf("connection error leaks the webhook secret: %q", res.Error)
	}

	// unparsable URL
	res = SendSlackMessage(context.Background(), "http://exa mple.com"+secretPath, "x")
	if strings.Contains(res.Error, "SuperSecretToken") {
		t.Fatalf("parse error leaks the webhook secret: %q", res.Error)
	}
}
