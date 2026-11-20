//go:build kubernetes

package platform

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Billing settings: ids in the ConfigMap, every credential only in the Secret.
func TestKubernetesIntegrationStoreBillingSecretsOnlyInSecret(t *testing.T) {
	ctx := context.Background()
	_, cs := newFakeKubernetesProvider()
	store := NewK8sIntegrationStore(cs, "axiom-system")
	in := Integrations{
		BillingProvider: "azure", BillingTagKey: "cluster", BillingTagValue: "prod",
		BillingAWSAccessKeyID: "AKIAEXAMPLEKEY123456", BillingAWSSecretAccessKey: "aws-secret-material",
		BillingAzureTenantID: azTenant, BillingAzureClientID: azClient, BillingAzureSubscriptionID: azSub,
		BillingAzureClientSecret: "az-secret-material",
		BillingGCPProjectID:      "billing-proj", BillingGCPTable: "billing-proj.ds.tbl", BillingGCPServiceAccountKey: `{"type":"service_account","private_key":"gcp-secret-material"}`,
	}
	if err := store.Save(ctx, in); err != nil {
		t.Fatal(err)
	}
	cm, err := cs.CoreV1().ConfigMaps("axiom-system").Get(ctx, integrationsConfigMap, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range cm.Data {
		for _, s := range []string{"AKIAEXAMPLEKEY123456", "aws-secret-material", "az-secret-material", "gcp-secret-material"} {
			if strings.Contains(v, s) {
				t.Fatalf("credential in ConfigMap key %q", k)
			}
		}
	}
	if cm.Data["billingProvider"] != "azure" || cm.Data["billingAzureSubscriptionId"] != azSub || cm.Data["billingGcpTable"] != "billing-proj.ds.tbl" {
		t.Fatalf("configmap %+v", cm.Data)
	}
	sec, err := cs.CoreV1().Secrets("axiom-system").Get(ctx, integrationsSecret, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(sec.Data["billingAzureClientSecret"]) != "az-secret-material" || string(sec.Data["billingAwsSecretAccessKey"]) != "aws-secret-material" {
		t.Fatalf("secret %v", sec.Data)
	}
	got, _, err := NewK8sIntegrationStore(cs, "axiom-system").Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.BillingProvider != "azure" || got.BillingAzureClientSecret != "az-secret-material" || got.BillingAWSAccessKeyID != "AKIAEXAMPLEKEY123456" ||
		got.BillingGCPServiceAccountKey != in.BillingGCPServiceAccountKey || got.BillingTagValue != "prod" || got.BillingAWSRegion != "us-east-1" {
		t.Fatalf("round trip %+v", got)
	}
}
