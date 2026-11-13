package platform

import (
	"context"
	"time"
)

// ListExternalSecretsHealth returns demo sync-health data with one failing and
// one stale ExternalSecret.
func (d *DemoProvider) ListExternalSecretsHealth(ctx context.Context, ns string) (ExternalSecretsHealthReport, error) {
	now := time.Now().UTC()
	ts := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }
	all := []ExternalSecretHealth{
		{Namespace: "default", Name: "app-credentials", StoreKind: "ClusterSecretStore", Store: "vault-lab", Refresh: "15m", LastSync: ts(6 * time.Minute), Ready: "True", Reason: "SecretSynced", Message: "Secret was synced", TargetName: "app-credentials"},
		{Namespace: "external-secrets", Name: "atlas-demo-gateway-auth", StoreKind: "SecretStore", Store: "vault-lab", Refresh: "1h", LastSync: ts(26 * time.Hour), Ready: "False", Reason: "SecretSyncedError", Message: "could not get secret data from provider: permission denied", TargetName: "gateway-auth"},
		{Namespace: "payments", Name: "stripe-api-key", StoreKind: "ClusterSecretStore", Store: "aws-sm", Refresh: "30m", LastSync: ts(3 * time.Hour), Ready: "True", Reason: "SecretSynced", Message: "Secret was synced", TargetName: "stripe-api-key"},
	}
	stores := []ExternalSecretStoreHealth{
		{Kind: "ClusterSecretStore", Name: "vault-lab", Provider: "vault", Ready: "True", Message: "store validated"},
		{Kind: "ClusterSecretStore", Name: "aws-sm", Provider: "aws", Ready: "True", Message: "store validated"},
		{Kind: "SecretStore", Namespace: "external-secrets", Name: "vault-lab", Provider: "vault", Ready: "False", Message: "unable to validate store"},
	}
	items := make([]ExternalSecretHealth, 0, len(all))
	for _, e := range all {
		if ns == "" || ns == "all" || e.Namespace == ns {
			items = append(items, e)
		}
	}
	return FinalizeExternalSecretsHealth(items, stores, now), nil
}
