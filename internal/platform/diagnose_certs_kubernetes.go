//go:build kubernetes

package platform

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var certManagerCertificateGVR = schema.GroupVersionResource{
	Group: "cert-manager.io", Version: "v1", Resource: "certificates",
}

func (k *KubernetesProvider) analyzeCertExpiry(ctx context.Context, ns string) ([]HealthFinding, error) {
	if k.dynamic == nil {
		return nil, nil
	}
	list, err := k.dynamic.Resource(certManagerCertificateGVR).Namespace(nsSelector(ns)).List(ctx, metav1.ListOptions{Limit: 200})
	if err != nil {
		// CRD absent is fine.
		return nil, nil
	}
	var out []HealthFinding
	cutoff := time.Now().UTC().Add(14 * 24 * time.Hour)
	for _, item := range list.Items {
		st, _ := item.Object["status"].(map[string]any)
		if st == nil {
			continue
		}
		notAfter, _ := st["notAfter"].(string)
		if notAfter == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, notAfter)
		if err != nil {
			t, err = time.Parse(time.RFC3339Nano, notAfter)
		}
		if err != nil {
			continue
		}
		if t.After(cutoff) {
			continue
		}
		sev := SeverityWarning
		if t.Before(time.Now().UTC()) {
			sev = SeverityCritical
		}
		out = append(out, newFinding(AnalyzerCertExpiry, sev, "Certificate", item.GetNamespace(), item.GetName(),
			"Certificate will break soon",
			fmt.Sprintf("Certificate %s/%s notAfter %s.", item.GetNamespace(), item.GetName(), t.Format(time.RFC3339)),
			"Renew via cert-manager (Certificate renew) or replace the TLS Secret before clients start failing."))
	}
	return out, nil
}
