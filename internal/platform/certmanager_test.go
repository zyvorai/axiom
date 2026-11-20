package platform

import (
	"context"
	"testing"
	"time"
)

func TestCertExpiryBucket(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		in   time.Duration
		want string
	}{
		{-time.Hour, CertHeatExpired},
		{3 * 24 * time.Hour, CertHeatSoon},
		{7 * 24 * time.Hour, CertHeatMonth},
		{29 * 24 * time.Hour, CertHeatMonth},
		{30 * 24 * time.Hour, CertHeatOK},
	}
	for _, c := range cases {
		if got, _ := CertExpiryBucket(now.Add(c.in), now); got != c.want {
			t.Errorf("%v: got %s want %s", c.in, got, c.want)
		}
	}
	if b, _ := CertExpiryBucket(time.Time{}, now); b != CertHeatUnknown {
		t.Errorf("zero time: %s", b)
	}
	if _, d := CertExpiryBucket(now.Add(-36*time.Hour), now); d != -2 {
		t.Errorf("days = %d", d)
	}
}

func TestParseCertificate(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	obj := map[string]any{
		"metadata": map[string]any{"namespace": "a", "name": "c"},
		"spec":     map[string]any{"secretName": "s", "issuerRef": map[string]any{"name": "le", "kind": "ClusterIssuer"}},
		"status": map[string]any{
			"notAfter":    "2026-01-04T00:00:00Z",
			"renewalTime": "2025-12-20T00:00:00Z",
			"conditions":  []any{map[string]any{"type": "Ready", "status": "True", "message": "ok"}},
		},
	}
	c := parseCertificate(obj, now)
	if c.Issuer != "le" || c.IssuerKind != "ClusterIssuer" || c.SecretName != "s" || c.Ready != "True" {
		t.Fatalf("bad parse: %+v", c)
	}
	if c.ExpiryBucket != CertHeatSoon || c.DaysToExpiry == nil || *c.DaysToExpiry != 3 {
		t.Fatalf("bad expiry: %+v", c)
	}
	empty := parseCertificate(map[string]any{}, now)
	if empty.Ready != "Unknown" || empty.ExpiryBucket != CertHeatUnknown || empty.DaysToExpiry != nil {
		t.Fatalf("bad empty: %+v", empty)
	}
}

func TestParseIssuerAndFailure(t *testing.T) {
	i := parseIssuer("Issuer", map[string]any{
		"metadata": map[string]any{"name": "x"},
		"spec":     map[string]any{"ca": map[string]any{}},
		"status":   map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "False", "message": "boom"}}},
	})
	if i.Type != "ca" || i.Ready != "False" || i.Message != "boom" {
		t.Fatalf("issuer: %+v", i)
	}
	cr := map[string]any{
		"metadata": map[string]any{"namespace": "n", "name": "r"},
		"status":   map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "False", "reason": "Failed", "message": "bad"}}},
	}
	if f, ok := parseFailure("CertificateRequest", cr); !ok || f.Message != "bad" || f.State != "Failed" {
		t.Fatalf("cr: %+v %v", f, ok)
	}
	pending := map[string]any{"status": map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "False", "reason": "Pending"}}}}
	if _, ok := parseFailure("CertificateRequest", pending); ok {
		t.Fatal("pending should not fail")
	}
	if f, ok := parseFailure("Order", map[string]any{"status": map[string]any{"state": "errored", "reason": "x"}}); !ok || f.Message != "x" {
		t.Fatalf("order: %+v", f)
	}
	if _, ok := parseFailure("Order", map[string]any{"status": map[string]any{"state": "valid"}}); ok {
		t.Fatal("valid order should not fail")
	}
}

func TestDemoCertManager(t *testing.T) {
	inv, err := (&DemoProvider{}).ListCertManager(context.Background())
	if err != nil || !inv.Available {
		t.Fatal(err)
	}
	var soon, failing bool
	for _, c := range inv.Certificates {
		soon = soon || c.ExpiryBucket == CertHeatSoon
		failing = failing || c.Ready == "False"
	}
	if !soon || !failing || len(inv.Failures) == 0 {
		t.Fatalf("demo data lacks expiring/failing: %+v", inv)
	}
}
