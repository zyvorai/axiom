//go:build kubernetes

package platform

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	cmCertGVR          = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	cmIssuerGVR        = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"}
	cmClusterIssuerGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "clusterissuers"}
	cmCertReqGVR       = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificaterequests"}
	cmOrderGVR         = schema.GroupVersionResource{Group: "acme.cert-manager.io", Version: "v1", Resource: "orders"}
)

// ListCertManager reads cert-manager.io v1 objects read-only.
func (k *KubernetesProvider) ListCertManager(ctx context.Context) (CertManagerInventory, error) {
	inv := emptyCertInventory("")
	now := time.Now().UTC()
	certs, err := k.dynamic.Resource(cmCertGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		inv.Note = "cert-manager.io CRDs not installed or not readable"
		return inv, nil
	}
	inv.Available = true
	for _, it := range certs.Items {
		inv.Certificates = append(inv.Certificates, parseCertificate(it.Object, now))
	}
	if l, err := k.dynamic.Resource(cmIssuerGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 200}); err == nil {
		for _, it := range l.Items {
			inv.Issuers = append(inv.Issuers, parseIssuer("Issuer", it.Object))
		}
	}
	if l, err := k.dynamic.Resource(cmClusterIssuerGVR).List(ctx, metav1.ListOptions{Limit: 200}); err == nil {
		for _, it := range l.Items {
			inv.Issuers = append(inv.Issuers, parseIssuer("ClusterIssuer", it.Object))
		}
	}
	if l, err := k.dynamic.Resource(cmCertReqGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 500}); err == nil {
		for _, it := range l.Items {
			if f, ok := parseFailure("CertificateRequest", it.Object); ok {
				inv.Failures = append(inv.Failures, f)
			}
		}
	}
	if l, err := k.dynamic.Resource(cmOrderGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 500}); err == nil {
		for _, it := range l.Items {
			if f, ok := parseFailure("Order", it.Object); ok {
				inv.Failures = append(inv.Failures, f)
			}
		}
	}
	return inv, nil
}
