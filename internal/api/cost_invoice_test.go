package api

import (
	"strings"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func TestCostInvoiceDemoCannedData(t *testing.T) {
	e := newCostEnv(t, platform.NewMemoryCostBudgetStore())
	w := e.do("GET", "/api/v1/cost/invoice", "", asCost("viewer"))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("invoice: %d %s", w.Code, w.Body.String())
	}
	var rep platform.BillingReport
	decodeInto(t, w, &rep)
	if !rep.Experimental || !rep.Demo || !rep.Configured || rep.Provider != "aws" || rep.Currency != "USD" || len(rep.Months) != 2 ||
		rep.Months[0].InvoiceTotal != 4812.40 || rep.Months[1].Allocated != 1288.10 || !rep.Months[1].Partial {
		t.Fatalf("demo report %+v", rep)
	}
	// Cluster-wide spend: not for namespace-restricted accounts.
	if w := e.do("GET", "/api/v1/cost/invoice", "", asCost("scoped")); w.Code != 403 {
		t.Fatalf("scoped: %d", w.Code)
	}
	// Testing is an admin-only integration action.
	if w := e.do("POST", "/api/v1/integrations/billing/test", "", asCost("operator")); w.Code != 403 {
		t.Fatalf("operator test: %d", w.Code)
	}
	w = e.do("POST", "/api/v1/integrations/billing/test", "", asCost("admin"))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("admin demo test: %d %s", w.Code, w.Body.String())
	}
}

func TestCostInvoiceSettingsWriteOnly(t *testing.T) {
	integ := platform.NewIntegrationHub(platform.Integrations{}, nil)
	users := platform.NewUserHub(nil)
	h := NewWithUsers(platform.NewDemoProvider(), "kubernetes", idAdminUser, idAdminPass, "", integ, users,
		AuthOptions{SessionSecret: "test-secret"})
	e := &idEnv{h: h, users: users, store: &memRevStore{}}
	admin := withBasic(idAdminUser, idAdminPass)

	// Not configured: the card stays hidden.
	var rep platform.BillingReport
	decodeInto(t, e.do("GET", "/api/v1/cost/invoice", "", admin), &rep)
	if rep.Configured || rep.Demo || !rep.Experimental || len(rep.Months) != 0 {
		t.Fatalf("unconfigured %+v", rep)
	}
	// Malformed settings are refused before anything is stored.
	for _, bad := range []map[string]any{
		{"billingProvider": "oracle"},
		{"billingGcpTable": "p.d.t` OR 1=1"},
		{"billingAzureSubscriptionId": "../../x"},
		{"billingAwsRegion": "eu-central-1"},
	} {
		if w := e.do("PUT", "/api/v1/integrations", jsonBody(bad), admin); w.Code != 400 {
			t.Errorf("%v: %d %s", bad, w.Code, w.Body.String())
		}
	}
	w := e.do("PUT", "/api/v1/integrations", jsonBody(map[string]any{
		"billingProvider": "aws", "billingAwsAccessKeyId": "AKIAEXAMPLEKEY123456", "billingAwsSecretAccessKey": "s3cr3t/AWS+secret-key-material",
		"billingTagKey": "cluster", "billingTagValue": "prod",
	}), admin)
	if w.Code != 200 || strings.Contains(w.Body.String(), "s3cr3t") || strings.Contains(w.Body.String(), "AKIAEXAMPLEKEY123456") {
		t.Fatalf("put: %d %s", w.Code, w.Body.String())
	}
	w = e.do("GET", "/api/v1/integrations", "", admin)
	body := w.Body.String()
	if strings.Contains(body, "s3cr3t") || strings.Contains(body, "AKIAEXAMPLEKEY123456") ||
		!strings.Contains(body, `"billingAwsSecretAccessKeySet":true`) || !strings.Contains(body, `"billingConfigured":true`) {
		t.Fatalf("get: %s", body)
	}
	if got := integ.Get(); got.BillingAWSSecretAccessKey != "s3cr3t/AWS+secret-key-material" || got.BillingProvider != "aws" {
		t.Fatalf("stored %+v", got.BillingProvider)
	}
}
