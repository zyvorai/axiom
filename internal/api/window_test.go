package api

import (
	"net/http/httptest"
	"testing"
)

func TestStripWindowCredentials(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?access_token=abc&axiom_session=s&keep=1", nil)
	r.Header.Set("Authorization", "Basic abc")
	r.Header.Set("Cookie", "axiom_session=s")
	stripWindowCredentials(r)
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
		t.Fatalf("headers not stripped: %v", r.Header)
	}
	if r.URL.RawQuery != "keep=1" {
		t.Fatalf("query = %q, want keep=1", r.URL.RawQuery)
	}
}
