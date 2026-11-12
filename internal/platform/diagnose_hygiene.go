package platform

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// analyzeHygieneFindings adds cert expiry (when listed via CRD browser is unavailable,
// we approximate via Secret tls.crt annotations / age), secret age, and node Ready skew.
func analyzeHygieneFindings(ctx context.Context, p Provider, ns string) []HealthFinding {
	var out []HealthFinding
	out = append(out, analyzeSecretAge(ctx, p, ns)...)
	out = append(out, analyzeNodeClockSkew(ctx, p)...)
	return out
}

func analyzeSecretAge(ctx context.Context, p Provider, ns string) []HealthFinding {
	secrets, err := p.ListSecrets(ctx, ns)
	if err != nil {
		return nil
	}
	var out []HealthFinding
	for _, s := range secrets {
		ageDays := parseAgeDays(s.Age)
		if ageDays < 180 {
			continue
		}
		// Skip service-account tokens and helm releases — noisy.
		if strings.HasPrefix(s.Type, "kubernetes.io/service-account") || strings.Contains(s.Type, "helm.sh") {
			continue
		}
		out = append(out, newFinding(AnalyzerSecretAge, SeverityInfo, "Secret", s.Namespace, s.Name,
			"Secret never rotated (age)",
			fmt.Sprintf("Secret %s/%s is ~%d days old (type %s) — rotation hygiene is metadata-only; values are never shown.", s.Namespace, s.Name, ageDays, s.Type),
			"Rotate credentials on a schedule and update consumers; consider External Secrets for automated rotation."))
		if len(out) >= 25 {
			break
		}
	}
	return out
}

func analyzeNodeClockSkew(ctx context.Context, p Provider) []HealthFinding {
	nodes, err := p.ListNodes(ctx)
	if err != nil || len(nodes) < 2 {
		return nil
	}
	// Without NTP metrics, flag nodes that are Ready=False or SchedulingDisabled
	// alongside a soft clock note when ages diverge wildly — keep this conservative.
	var out []HealthFinding
	for _, n := range nodes {
		for _, c := range n.Conditions {
			if c.Type == "Ready" && c.Status != "True" && strings.Contains(strings.ToLower(c.Message+c.Reason), "clock") {
				out = append(out, newFinding(AnalyzerNodeClockSkew, SeverityWarning, "Node", "", n.Name,
					"Possible node clock skew",
					fmt.Sprintf("Node %s Ready condition mentions clock issues: %s", n.Name, firstNonEmpty(c.Message, c.Reason)),
					"Sync NTP/chrony on the node; clock skew breaks TLS, leases and certificate validation."))
			}
		}
	}
	return out
}

func parseAgeDays(age string) int {
	age = strings.TrimSpace(strings.ToLower(age))
	if age == "" {
		return 0
	}
	// Formats like "41d", "12h", "3d12h" — take leading days.
	n := 0
	for _, ch := range age {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
			continue
		}
		if ch == 'd' {
			return n
		}
		break
	}
	return 0
}

// analyzeCertExpiryDemo adds a demo cert finding; kubernetes path uses dynamic CRDs when present.
func analyzeCertExpiryDemo() []HealthFinding {
	soon := time.Now().UTC().Add(10 * 24 * time.Hour).Format("2006-01-02")
	return []HealthFinding{newFinding(AnalyzerCertExpiry, SeverityWarning, "Certificate", "axiom-system", "axiom-tls",
		"Certificate will break soon",
		fmt.Sprintf("Demo certificate axiom-system/axiom-tls notAfter ~%s (< 14 days).", soon),
		"Renew with cert-manager or replace the Secret before expiry; add the deadline to your on-call calendar.")}
}
