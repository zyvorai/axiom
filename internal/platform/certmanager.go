package platform

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Expiry heat buckets for cert-manager Certificates.
const (
	CertHeatExpired = "expired"
	CertHeatSoon    = "lt7d"
	CertHeatMonth   = "lt30d"
	CertHeatOK      = "ok"
	CertHeatUnknown = "unknown"
)

// CertManagerCertificate is one cert-manager.io Certificate.
type CertManagerCertificate struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Issuer       string `json:"issuer,omitempty"`
	IssuerKind   string `json:"issuerKind,omitempty"`
	SecretName   string `json:"secretName,omitempty"`
	Ready        string `json:"ready"` // True | False | Unknown
	Message      string `json:"message,omitempty"`
	NotAfter     string `json:"notAfter,omitempty"`
	RenewalTime  string `json:"renewalTime,omitempty"`
	DaysToExpiry *int   `json:"daysToExpiry,omitempty"`
	ExpiryBucket string `json:"expiryBucket"`
}

// CertManagerIssuer is an Issuer or ClusterIssuer.
type CertManagerIssuer struct {
	Kind      string `json:"kind"` // Issuer | ClusterIssuer
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Type      string `json:"type,omitempty"` // acme | ca | vault | selfSigned | venafi
	Ready     string `json:"ready"`
	Message   string `json:"message,omitempty"`
}

// CertManagerFailure is a failing CertificateRequest or ACME Order.
type CertManagerFailure struct {
	Kind      string `json:"kind"` // CertificateRequest | Order
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	Message   string `json:"message,omitempty"`
}

// CertManagerInventory is GET /api/v1/certificates.
type CertManagerInventory struct {
	Available    bool                     `json:"available"`
	Note         string                   `json:"note,omitempty"`
	Certificates []CertManagerCertificate `json:"certificates"`
	Issuers      []CertManagerIssuer      `json:"issuers"`
	Failures     []CertManagerFailure     `json:"failures"`
}

func emptyCertInventory(note string) CertManagerInventory {
	return CertManagerInventory{Note: note, Certificates: []CertManagerCertificate{}, Issuers: []CertManagerIssuer{}, Failures: []CertManagerFailure{}}
}

// ListCertManager returns a read-only cert-manager inventory.
func ListCertManager(ctx context.Context, p Provider) (CertManagerInventory, error) {
	if l, ok := p.(interface {
		ListCertManager(context.Context) (CertManagerInventory, error)
	}); ok {
		return l.ListCertManager(ctx)
	}
	return emptyCertInventory("cert-manager view not available on this provider"), nil
}

// CertExpiryBucket classifies a notAfter time relative to now and returns the
// whole days remaining (negative once expired).
func CertExpiryBucket(notAfter, now time.Time) (string, int) {
	if notAfter.IsZero() {
		return CertHeatUnknown, 0
	}
	d := notAfter.Sub(now)
	days := int(math.Floor(d.Hours() / 24))
	switch {
	case d <= 0:
		return CertHeatExpired, days
	case d < 7*24*time.Hour:
		return CertHeatSoon, days
	case d < 30*24*time.Hour:
		return CertHeatMonth, days
	}
	return CertHeatOK, days
}

// certCondition extracts a condition's status and message from an
// unstructured status.conditions slice.
func certCondition(conds []any, condType string) (status, message string) {
	status = "Unknown"
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok || fmt.Sprint(m["type"]) != condType {
			continue
		}
		if s, ok := m["status"].(string); ok && s != "" {
			status = s
		}
		message, _ = m["message"].(string)
		return status, message
	}
	return status, ""
}

// parseCertificate builds a CertManagerCertificate from an unstructured object map.
func parseCertificate(obj map[string]any, now time.Time) CertManagerCertificate {
	meta, _ := obj["metadata"].(map[string]any)
	spec, _ := obj["spec"].(map[string]any)
	st, _ := obj["status"].(map[string]any)
	c := CertManagerCertificate{ExpiryBucket: CertHeatUnknown}
	c.Namespace, _ = meta["namespace"].(string)
	c.Name, _ = meta["name"].(string)
	c.SecretName, _ = spec["secretName"].(string)
	if ref, ok := spec["issuerRef"].(map[string]any); ok {
		c.Issuer, _ = ref["name"].(string)
		c.IssuerKind, _ = ref["kind"].(string)
		if c.IssuerKind == "" {
			c.IssuerKind = "Issuer"
		}
	}
	conds, _ := st["conditions"].([]any)
	c.Ready, c.Message = certCondition(conds, "Ready")
	c.NotAfter, _ = st["notAfter"].(string)
	c.RenewalTime, _ = st["renewalTime"].(string)
	if t, err := time.Parse(time.RFC3339, c.NotAfter); err == nil {
		b, days := CertExpiryBucket(t, now)
		c.ExpiryBucket = b
		c.DaysToExpiry = &days
	}
	return c
}

// parseIssuer builds a CertManagerIssuer; kind is Issuer or ClusterIssuer.
func parseIssuer(kind string, obj map[string]any) CertManagerIssuer {
	meta, _ := obj["metadata"].(map[string]any)
	spec, _ := obj["spec"].(map[string]any)
	st, _ := obj["status"].(map[string]any)
	i := CertManagerIssuer{Kind: kind}
	i.Namespace, _ = meta["namespace"].(string)
	i.Name, _ = meta["name"].(string)
	for _, t := range []string{"acme", "ca", "vault", "selfSigned", "venafi"} {
		if _, ok := spec[t]; ok {
			i.Type = t
			break
		}
	}
	conds, _ := st["conditions"].([]any)
	i.Ready, i.Message = certCondition(conds, "Ready")
	return i
}

// parseFailure returns a failure row for a CertificateRequest or Order when it
// is failing; ok=false when healthy or still pending.
func parseFailure(kind string, obj map[string]any) (CertManagerFailure, bool) {
	meta, _ := obj["metadata"].(map[string]any)
	st, _ := obj["status"].(map[string]any)
	f := CertManagerFailure{Kind: kind}
	f.Namespace, _ = meta["namespace"].(string)
	f.Name, _ = meta["name"].(string)
	if kind == "Order" {
		state, _ := st["state"].(string)
		switch state {
		case "invalid", "errored", "expired":
			f.State = state
			f.Message, _ = st["reason"].(string)
			return f, true
		}
		return f, false
	}
	conds, _ := st["conditions"].([]any)
	if s, msg := certCondition(conds, "Denied"); s == "True" {
		f.State, f.Message = "Denied", msg
		return f, true
	}
	if s, msg := certCondition(conds, "InvalidRequest"); s == "True" {
		f.State, f.Message = "InvalidRequest", msg
		return f, true
	}
	if s, msg := certCondition(conds, "Ready"); s == "False" {
		for _, c := range conds {
			if m, ok := c.(map[string]any); ok && m["type"] == "Ready" {
				f.State, _ = m["reason"].(string)
			}
		}
		if f.State == "Pending" {
			return f, false // still issuing, not failing
		}
		f.Message = msg
		return f, true
	}
	return f, false
}

func (d *DemoProvider) ListCertManager(ctx context.Context) (CertManagerInventory, error) {
	now := time.Now().UTC()
	mk := func(ns, name, issuer, kind, secret, ready, msg string, daysLeft int) CertManagerCertificate {
		na := now.Add(time.Duration(daysLeft) * 24 * time.Hour)
		b, days := CertExpiryBucket(na, now)
		return CertManagerCertificate{
			Namespace: ns, Name: name, Issuer: issuer, IssuerKind: kind, SecretName: secret,
			Ready: ready, Message: msg, NotAfter: na.Format(time.RFC3339),
			RenewalTime:  na.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			DaysToExpiry: &days, ExpiryBucket: b,
		}
	}
	return CertManagerInventory{
		Available: true,
		Note:      "Demo cert-manager inventory (simulated)",
		Certificates: []CertManagerCertificate{
			mk("default", "web-tls", "letsencrypt-prod", "ClusterIssuer", "web-tls", "True", "Certificate is up to date and has not expired", 62),
			mk("shop", "api-tls", "letsencrypt-prod", "ClusterIssuer", "api-tls", "True", "Certificate is up to date and has not expired", 5),
			mk("ml", "internal-ca-cert", "internal-ca", "Issuer", "internal-ca-cert", "False", "Issuing certificate as Secret does not exist", 80),
		},
		Issuers: []CertManagerIssuer{
			{Kind: "ClusterIssuer", Name: "letsencrypt-prod", Type: "acme", Ready: "True", Message: "The ACME account was registered with the ACME server"},
			{Kind: "Issuer", Namespace: "ml", Name: "internal-ca", Type: "ca", Ready: "False", Message: "Error getting keypair for CA issuer: secret \"ca-key-pair\" not found"},
		},
		Failures: []CertManagerFailure{
			{Kind: "CertificateRequest", Namespace: "ml", Name: "internal-ca-cert-1", State: "Failed", Message: "Referenced Issuer \"internal-ca\" is not ready"},
			{Kind: "Order", Namespace: "ml", Name: "internal-ca-cert-1-2918", State: "errored", Message: "Failed to finalize order: 429 rateLimited"},
		},
	}, nil
}
