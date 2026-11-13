//go:build kubernetes

package platform

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	secretStoreGVRs = []schema.GroupVersionResource{
		{Group: "external-secrets.io", Version: "v1", Resource: "secretstores"},
		{Group: "external-secrets.io", Version: "v1beta1", Resource: "secretstores"},
	}
	clusterSecretStoreGVRs = []schema.GroupVersionResource{
		{Group: "external-secrets.io", Version: "v1", Resource: "clustersecretstores"},
		{Group: "external-secrets.io", Version: "v1beta1", Resource: "clustersecretstores"},
	}
)

// listESObjects tries each GVR in turn; a missing CRD yields (nil, nil).
func (k *KubernetesProvider) listESObjects(ctx context.Context, gvrs []schema.GroupVersionResource, ns string) ([]unstructured.Unstructured, error) {
	for _, gvr := range gvrs {
		ri := k.dynamic.Resource(gvr)
		var list *unstructured.UnstructuredList
		var err error
		if ns == "" {
			list, err = ri.List(ctx, metav1.ListOptions{})
		} else {
			list, err = ri.Namespace(ns).List(ctx, metav1.ListOptions{})
		}
		if err == nil {
			return list.Items, nil
		}
		if !isMissingCRD(err) {
			return nil, err
		}
	}
	return nil, nil
}

// ListExternalSecretsHealth reads ExternalSecrets, SecretStores and
// ClusterSecretStores (read-only; names and status only).
func (k *KubernetesProvider) ListExternalSecretsHealth(ctx context.Context, ns string) (ExternalSecretsHealthReport, error) {
	if k.dynamic == nil {
		return emptyESHealth("dynamic client unavailable"), nil
	}
	if ns == "all" {
		ns = ""
	}
	ess, err := k.listESObjects(ctx, externalSecretGVRs, ns)
	if err != nil {
		return ExternalSecretsHealthReport{}, err
	}
	if ess == nil {
		return emptyESHealth("External Secrets Operator CRDs not found"), nil
	}
	items := make([]ExternalSecretHealth, 0, len(ess))
	for _, o := range ess {
		items = append(items, ParseExternalSecretHealth(o.Object))
	}
	var stores []ExternalSecretStoreHealth
	// Store listing is best effort: a forbidden list must not hide sync health.
	if ss, err := k.listESObjects(ctx, secretStoreGVRs, ns); err == nil {
		for _, o := range ss {
			stores = append(stores, ParseExternalSecretStore(o.Object, "SecretStore"))
		}
	}
	if cs, err := k.listESObjects(ctx, clusterSecretStoreGVRs, ""); err == nil {
		for _, o := range cs {
			stores = append(stores, ParseExternalSecretStore(o.Object, "ClusterSecretStore"))
		}
	}
	return FinalizeExternalSecretsHealth(items, stores, time.Now().UTC()), nil
}
