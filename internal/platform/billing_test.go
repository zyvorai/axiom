package platform

// Billing reconciliation tests. Every provider client here runs ONLY against
// httptest fixtures shaped after the providers' documented requests and
// responses — none of this has been verified against a real AWS, Azure or
// GCP account.

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var billingNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func TestBillingPeriods(t *testing.T) {
	p := billingPeriods(billingNow)
	if len(p) != 2 || p[0].Month != "2026-10" || p[1].Month != "2026-11" ||
		!p[0].Start.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || !p[0].End.Equal(p[1].Start) ||
		!p[1].End.Equal(time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("periods %+v", p)
	}
	// January rolls back into the previous year.
	p = billingPeriods(time.Date(2027, 1, 1, 0, 30, 0, 0, time.UTC))
	if p[0].Month != "2026-12" || p[1].Month != "2027-01" {
		t.Fatalf("january %+v", p)
	}
}

// ---------------------------------------------------------------------------
// AWS

// The worked example from the AWS Signature Version 4 documentation (IAM
// ListUsers, 2015-08-30): the canonical request, string to sign and
// signature are published, so this pins the signer to AWS's own vector.
func TestSigV4KnownVector(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://iam.amazonaws.com/?Action=ListUsers&Version=2010-05-08", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	sigV4Sign(req, nil, "AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "us-east-1", "iam",
		time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/iam/aws4_request, SignedHeaders=content-type;host;x-amz-date, " +
		"Signature=5d672d79c15b13162d9279b0855cfba6789a8edb4c82c400e06b5924a6f2b5d7"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("signature mismatch\n got %s\nwant %s", got, want)
	}
	if req.Header.Get("X-Amz-Date") != "20150830T123600Z" {
		t.Fatalf("x-amz-date %q", req.Header.Get("X-Amz-Date"))
	}
}

func awsCfg() Integrations {
	return Integrations{
		BillingProvider: "aws", BillingAWSRegion: "us-east-1",
		BillingAWSAccessKeyID: "AKIAEXAMPLEKEY123456", BillingAWSSecretAccessKey: "s3cr3t/AWS+secret-key-material",
		BillingTagKey: "kubernetes-cluster", BillingTagValue: "prod",
	}.Normalize()
}

func TestAWSBillingClientFixture(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/" || r.Host != "ce.us-east-1.amazonaws.com" ||
			r.Header.Get("X-Amz-Target") != "AWSInsightsIndexService.GetCostAndUsage" ||
			r.Header.Get("Content-Type") != "application/x-amz-json-1.1" {
			t.Errorf("unexpected request %s %s host=%s target=%s", r.Method, r.URL.Path, r.Host, r.Header.Get("X-Amz-Target"))
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLEKEY123456/20261104/us-east-1/ce/aws4_request, SignedHeaders=accept;content-type;host;x-amz-date;x-amz-target, Signature=") {
			t.Errorf("authorization %q", auth)
		}
		if strings.Contains(auth, "s3cr3t") {
			t.Errorf("secret in the Authorization header")
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = io.WriteString(w, `{"ResultsByTime":[
			{"TimePeriod":{"Start":"2026-10-01","End":"2026-11-01"},"Groups":[
				{"Keys":["Amazon Elastic Compute Cloud - Compute"],"Metrics":{"UnblendedCost":{"Amount":"4000.0","Unit":"USD"}}},
				{"Keys":["Amazon Simple Storage Service"],"Metrics":{"UnblendedCost":{"Amount":"812.4012","Unit":"USD"}}}
			],"Estimated":false},
			{"TimePeriod":{"Start":"2026-11-01","End":"2026-11-05"},"Groups":[
				{"Keys":["Amazon Elastic Compute Cloud - Compute"],"Metrics":{"UnblendedCost":{"Amount":"500.0","Unit":"USD"}}},
				{"Keys":["AWS Data Transfer"],"Metrics":{"UnblendedCost":{"Amount":"112.5","Unit":"USD"}}}
			],"Estimated":true}]}`)
	}))
	defer srv.Close()
	c, err := newAWSBillingClient(awsCfg(), billingTestTransport(srv.Client(), srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	totals, err := c.fetch(context.Background(), billingPeriods(billingNow), billingNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 2 || totals[0].Month != "2026-10" || totals[0].Amount != 4812.4012 || totals[1].Month != "2026-11" || totals[1].Currency != "USD" {
		t.Fatalf("totals %+v", totals)
	}
	if len(totals[0].Services) < 2 || totals[0].Services[0].Name != "Amazon Elastic Compute Cloud - Compute" {
		t.Fatalf("services %+v", totals[0].Services)
	}
	tp := gotBody["TimePeriod"].(map[string]any)
	if tp["Start"] != "2026-10-01" || tp["End"] != "2026-11-05" || gotBody["Granularity"] != "MONTHLY" {
		t.Fatalf("body %+v", gotBody)
	}
	if m := gotBody["Metrics"].([]any); len(m) != 1 || m[0] != "UnblendedCost" {
		t.Fatalf("metrics %+v", m)
	}
	if gb := gotBody["GroupBy"].([]any); len(gb) != 1 {
		t.Fatalf("groupby %+v", gotBody["GroupBy"])
	}
	tags := gotBody["Filter"].(map[string]any)["Tags"].(map[string]any)
	if tags["Key"] != "kubernetes-cluster" || tags["Values"].([]any)[0] != "prod" {
		t.Fatalf("tag filter %+v", tags)
	}

	// Without a tag there is no filter.
	cfg := awsCfg()
	cfg.BillingTagKey = ""
	c, _ = newAWSBillingClient(cfg, billingTestTransport(srv.Client(), srv.URL))
	gotBody = nil
	if _, err := c.fetch(context.Background(), billingPeriods(billingNow), billingNow); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotBody["Filter"]; ok {
		t.Fatalf("unexpected filter %+v", gotBody)
	}
}

func TestAWSBillingClientPaginatesAndSessionToken(t *testing.T) {
	var pages int
	var sawToken bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		if r.Header.Get("X-Amz-Security-Token") != "session-token-material" {
			t.Errorf("missing session token header")
		}
		if !strings.Contains(r.Header.Get("Authorization"), "x-amz-security-token") {
			t.Errorf("session token not in SignedHeaders: %s", r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(b, &req)
		if pages == 1 {
			if _, ok := req["NextPageToken"]; ok {
				t.Errorf("first page should omit NextPageToken: %+v", req)
			}
			_, _ = io.WriteString(w, `{"ResultsByTime":[
				{"TimePeriod":{"Start":"2026-10-01","End":"2026-11-01"},"Total":{"UnblendedCost":{"Amount":"100","Unit":"USD"}}}],
				"NextPageToken":"page-2"}`)
			return
		}
		if req["NextPageToken"] != "page-2" {
			t.Errorf("expected NextPageToken page-2, got %+v", req)
		}
		sawToken = true
		_, _ = io.WriteString(w, `{"ResultsByTime":[
			{"TimePeriod":{"Start":"2026-10-01","End":"2026-11-01"},"Total":{"UnblendedCost":{"Amount":"50.5","Unit":"USD"}}},
			{"TimePeriod":{"Start":"2026-11-01","End":"2026-11-05"},"Total":{"UnblendedCost":{"Amount":"10","Unit":"USD"}}}]}`)
	}))
	defer srv.Close()
	cfg := awsCfg()
	cfg.BillingAWSSessionToken = "session-token-material"
	c, err := newAWSBillingClient(cfg, billingTestTransport(srv.Client(), srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	totals, err := c.fetch(context.Background(), billingPeriods(billingNow), billingNow)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || !sawToken {
		t.Fatalf("pages=%d sawToken=%v", pages, sawToken)
	}
	if len(totals) != 2 || totals[0].Amount != 150.5 || totals[1].Amount != 10 {
		t.Fatalf("merged totals %+v", totals)
	}
}

// TestAWSBillingLive hits real Cost Explorer when AXIOM_BILLING_LIVE=1 and
// AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY are set (optional AWS_SESSION_TOKEN).
func TestAWSBillingLive(t *testing.T) {
	if os.Getenv("AXIOM_BILLING_LIVE") != "1" {
		t.Skip("set AXIOM_BILLING_LIVE=1 to call a real AWS account")
	}
	id := strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID"))
	secret := strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY"))
	if id == "" || secret == "" {
		t.Skip("AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY required")
	}
	cfg := Integrations{
		BillingProvider: "aws",
		BillingAWSRegion: envOr("AWS_REGION", "us-east-1"),
		BillingAWSAccessKeyID: id, BillingAWSSecretAccessKey: secret,
		BillingAWSSessionToken: os.Getenv("AWS_SESSION_TOKEN"),
		BillingTagKey: os.Getenv("BILLING_TAG_KEY"), BillingTagValue: os.Getenv("BILLING_TAG_VALUE"),
	}.Normalize()
	c, err := newAWSBillingClient(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	totals, err := c.fetch(context.Background(), billingPeriods(time.Now().UTC()), time.Now().UTC())
	if err != nil {
		t.Fatalf("live Cost Explorer: %v", err)
	}
	if len(totals) == 0 {
		t.Fatal("expected at least one month total from live Cost Explorer")
	}
	t.Logf("live months: %d first=%+v", len(totals), totals[0])
}

func envOr(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}

func TestAWSBillingErrorsAreSafe(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"__type":"com.amazon.coral.service#UnrecognizedClientException","Message":"The security token AKIAEXAMPLEKEY123456 s3cr3t is invalid"}`)
	}))
	defer srv.Close()
	c, _ := newAWSBillingClient(awsCfg(), billingTestTransport(srv.Client(), srv.URL))
	_, err := c.fetch(context.Background(), billingPeriods(billingNow), billingNow)
	if err == nil || !strings.Contains(err.Error(), "HTTP 400 (UnrecognizedClientException)") {
		t.Fatalf("err %v", err)
	}
	for _, leak := range []string{"AKIAEXAMPLEKEY123456", "s3cr3t", "127.0.0.1", srv.URL} {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("error leaks %q: %v", leak, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Azure

const (
	azTenant = "11111111-2222-3333-4444-555555555555"
	azClient = "66666666-7777-8888-9999-000000000000"
	azSub    = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
)

func azureCfg() Integrations {
	return Integrations{
		BillingProvider: "azure", BillingAzureTenantID: azTenant, BillingAzureClientID: azClient,
		BillingAzureSubscriptionID: azSub, BillingAzureClientSecret: "az~client~secret",
		BillingTagKey: "cluster", BillingTagValue: "prod",
	}.Normalize()
}

func TestAzureBillingClientFixture(t *testing.T) {
	var query map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/"+azTenant+"/oauth2/v2.0/token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != azClient ||
				r.Form.Get("client_secret") != "az~client~secret" || r.Form.Get("scope") != "https://management.azure.com/.default" {
				t.Errorf("token form %v", r.Form)
			}
			_, _ = io.WriteString(w, `{"token_type":"Bearer","expires_in":3599,"access_token":"az-token"}`)
		case r.URL.Path == "/subscriptions/"+azSub+"/providers/Microsoft.CostManagement/query":
			if r.URL.Query().Get("api-version") != azureCostAPIVersion || r.Header.Get("Authorization") != "Bearer az-token" {
				t.Errorf("query request %s auth=%q", r.URL.String(), r.Header.Get("Authorization"))
			}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &query)
			_, _ = io.WriteString(w, `{"properties":{"nextLink":null,
				"columns":[{"name":"Cost","type":"Number"},{"name":"BillingMonth","type":"Datetime"},{"name":"Currency","type":"String"}],
				"rows":[[3120.55,"2026-10-01T00:00:00","EUR"],[410.2,"2026-11-01T00:00:00","EUR"]]}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c, err := newAzureBillingClient(azureCfg(), billingTestTransport(srv.Client(), srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	totals, err := c.fetch(context.Background(), billingPeriods(billingNow), billingNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 2 || totals[0].Month != "2026-10" || totals[0].Amount != 3120.55 || totals[1].Currency != "EUR" {
		t.Fatalf("totals %+v", totals)
	}
	if query["type"] != "ActualCost" || query["timeframe"] != "Custom" {
		t.Fatalf("query %+v", query)
	}
	tp := query["timePeriod"].(map[string]any)
	if tp["from"] != "2026-10-01T00:00:00Z" || tp["to"] != "2026-11-04T23:59:59Z" {
		t.Fatalf("time period %+v", tp)
	}
	ds := query["dataset"].(map[string]any)
	tags := ds["filter"].(map[string]any)["tags"].(map[string]any)
	if ds["granularity"] != "Monthly" || tags["name"] != "cluster" || tags["operator"] != "In" {
		t.Fatalf("dataset %+v", ds)
	}
}

func TestAzureBillingTokenErrorIsSafe(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret az~client~secret provided."}`)
	}))
	defer srv.Close()
	c, _ := newAzureBillingClient(azureCfg(), billingTestTransport(srv.Client(), srv.URL))
	_, err := c.fetch(context.Background(), billingPeriods(billingNow), billingNow)
	if err == nil || !strings.Contains(err.Error(), "Microsoft Entra ID answered HTTP 401 (invalid_client)") || strings.Contains(err.Error(), "az~client~secret") || strings.Contains(err.Error(), azTenant) {
		t.Fatalf("err %v", err)
	}
}

func TestAzureMonthParsing(t *testing.T) {
	for in, want := range map[any]string{"2026-10-01T00:00:00": "2026-10", float64(20261001): "2026-10", "20261101": "2026-11"} {
		if got, ok := azureMonth(in); !ok || got != want {
			t.Errorf("azureMonth(%v) = %q %v", in, got, ok)
		}
	}
	if _, ok := azureMonth(true); ok {
		t.Error("bool month accepted")
	}
}

// ---------------------------------------------------------------------------
// GCP

func gcpTestKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	sa, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "billing-proj", "private_key_id": "kid-1",
		"private_key": pemKey, "client_email": "axiom@billing-proj.iam.gserviceaccount.com",
		// A hostile key file cannot redirect the token request.
		"token_uri": "http://169.254.169.254/token",
	})
	return k, string(sa)
}

func gcpCfg(sa string) Integrations {
	return Integrations{
		BillingProvider: "gcp", BillingGCPProjectID: "billing-proj",
		BillingGCPTable:             "billing-proj.billing_export.gcp_billing_export_v1_0A1B2C_3D4E5F_6A7B8C",
		BillingGCPServiceAccountKey: sa, BillingTagKey: "cluster", BillingTagValue: "prod",
	}.Normalize()
}

func TestGCPBillingClientFixture(t *testing.T) {
	key, sa := gcpTestKey(t)
	var bq map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
				t.Errorf("grant %q", r.Form.Get("grant_type"))
			}
			parts := strings.Split(r.Form.Get("assertion"), ".")
			if len(parts) != 3 {
				t.Fatalf("assertion %q", r.Form.Get("assertion"))
			}
			sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
			sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
				t.Errorf("JWT signature: %v", err)
			}
			var hdr, claims map[string]any
			hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
			cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
			_ = json.Unmarshal(hb, &hdr)
			_ = json.Unmarshal(cb, &claims)
			if hdr["alg"] != "RS256" || hdr["kid"] != "kid-1" || claims["iss"] != "axiom@billing-proj.iam.gserviceaccount.com" ||
				claims["aud"] != "https://oauth2.googleapis.com/token" || claims["scope"] != gcpBigQueryScope ||
				claims["iat"].(float64) != float64(billingNow.Unix()) || claims["exp"].(float64) != float64(billingNow.Add(time.Hour).Unix()) {
				t.Errorf("jwt header %v claims %v", hdr, claims)
			}
			_, _ = io.WriteString(w, `{"access_token":"gcp-token","expires_in":3599,"token_type":"Bearer"}`)
		case "/bigquery/v2/projects/billing-proj/queries":
			if r.Header.Get("Authorization") != "Bearer gcp-token" {
				t.Errorf("auth %q", r.Header.Get("Authorization"))
			}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &bq)
			_, _ = io.WriteString(w, `{"kind":"bigquery#queryResponse","jobComplete":true,"totalRows":"2",
				"schema":{"fields":[{"name":"month","type":"STRING"},{"name":"currency","type":"STRING"},{"name":"total","type":"FLOAT"}]},
				"rows":[{"f":[{"v":"202610"},{"v":"USD"},{"v":"2201.75"}]},{"f":[{"v":"202611"},{"v":"USD"},{"v":"301.5"}]}]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c, err := newGCPBillingClient(gcpCfg(sa), billingTestTransport(srv.Client(), srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	totals, err := c.fetch(context.Background(), billingPeriods(billingNow), billingNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 2 || totals[0].Month != "2026-10" || totals[0].Amount != 2201.75 || totals[1].Month != "2026-11" {
		t.Fatalf("totals %+v", totals)
	}
	q := bq["query"].(string)
	if !strings.Contains(q, "FROM `billing-proj.billing_export.gcp_billing_export_v1_0A1B2C_3D4E5F_6A7B8C`") ||
		!strings.Contains(q, "invoice.month IN UNNEST(@months)") || !strings.Contains(q, "l.key = @tag_key AND l.value = @tag_value") ||
		strings.Contains(q, "prod") || bq["useLegacySql"] != false || bq["parameterMode"] != "NAMED" {
		t.Fatalf("query %q body %+v", q, bq)
	}
	params := bq["queryParameters"].([]any)
	months := params[0].(map[string]any)["parameterValue"].(map[string]any)["arrayValues"].([]any)
	if len(params) != 3 || months[0].(map[string]any)["value"] != "202610" || months[1].(map[string]any)["value"] != "202611" {
		t.Fatalf("params %+v", params)
	}
}

func TestGCPBillingJobIncomplete(t *testing.T) {
	_, sa := gcpTestKey(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"t"}`)
			return
		}
		_, _ = io.WriteString(w, `{"jobComplete":false,"jobReference":{"jobId":"x"}}`)
	}))
	defer srv.Close()
	c, _ := newGCPBillingClient(gcpCfg(sa), billingTestTransport(srv.Client(), srv.URL))
	if _, err := c.fetch(context.Background(), billingPeriods(billingNow), billingNow); err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("err %v", err)
	}
}

func TestGCPTableValidation(t *testing.T) {
	good := []string{"my-project-1.billing.gcp_billing_export_v1_X", "abcdef.d.t"}
	bad := []string{
		"", "dataset.table", "a.b.c.d", "My-Project.d.t", "proj.d.t`; DROP TABLE x; --", "proj.d.t` OR 1=1 --",
		"proj.d.t WHERE 1=1", "example.com:proj.d.t", "proj.d-s.t", "proj.d.t\n", "short.d.t", "proj-.d.t",
		"proj.d.`t`", "proj.d.t/*", "proj.d.$t",
	}
	for _, s := range good {
		if _, err := parseBillingGCPTable(s); err != nil {
			t.Errorf("%q rejected: %v", s, err)
		}
	}
	for _, s := range bad {
		if _, err := parseBillingGCPTable(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestGCPServiceAccountKeyValidation(t *testing.T) {
	_, sa := gcpTestKey(t)
	if _, err := parseGCPServiceAccountKey([]byte(sa)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"{", `{"type":"authorized_user"}`, `{"type":"service_account","client_email":"a@b","private_key":"nope"}`} {
		if _, err := parseGCPServiceAccountKey([]byte(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		} else if strings.Contains(err.Error(), "nope") {
			t.Errorf("error quotes the key: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Outbound guard

func TestBillingHTTPClientOnlyReachesFixedHosts(t *testing.T) {
	c := newBillingHTTPClient("ce.us-east-1.amazonaws.com")
	for _, u := range []string{
		"https://evil.example/", "http://ce.us-east-1.amazonaws.com/", "https://ce.us-east-1.amazonaws.com:8443/",
		"https://user:pw@ce.us-east-1.amazonaws.com/", "https://169.254.169.254/latest/meta-data/",
	} {
		req, _ := http.NewRequest("GET", u, nil)
		_, err := c.Do(req)
		var he *billingHostError
		if !errors.As(err, &he) {
			t.Errorf("%s: want host refusal, got %v", u, err)
		}
	}
	// An allowed name that resolves to a private address is refused at dial
	// time by the Helm SSRF guard.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("request reached a private address") }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	c = newBillingHTTPClient(u.Hostname())
	tr := billingTransport{http: c, baseURL: func(h string) string { return "https://" + h }}
	req, _ := http.NewRequest("GET", "https://"+u.Hostname()+"/", nil)
	if _, err := tr.do(req, "test"); err == nil || !strings.Contains(err.Error(), "public address") {
		t.Fatalf("private dial: %v", err)
	}
}

func TestBillingNoRedirects(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://169.254.169.254/", http.StatusFound)
	}))
	defer srv.Close()
	hc := srv.Client()
	hc.CheckRedirect = newBillingHTTPClient().CheckRedirect
	tr := billingTestTransport(hc, srv.URL)
	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	if _, err := tr.do(req, "AWS Cost Explorer"); err == nil || !strings.Contains(err.Error(), "redirects are not followed") {
		t.Fatalf("redirect: %v", err)
	}
}

func TestBillingBodyCap(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ResultsByTime":[`+strings.Repeat(" ", billingMaxBody)+`]}`)
	}))
	defer srv.Close()
	c, _ := newAWSBillingClient(awsCfg(), billingTestTransport(srv.Client(), srv.URL))
	if _, err := c.fetch(context.Background(), billingPeriods(billingNow), billingNow); err == nil || !strings.Contains(err.Error(), "larger than 4 MiB") {
		t.Fatalf("cap: %v", err)
	}
}

func TestBillingTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)
	c, _ := newAWSBillingClient(awsCfg(), billingTestTransport(srv.Client(), srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := c.fetch(ctx, billingPeriods(billingNow), billingNow); err == nil || !strings.Contains(err.Error(), "did not answer in time") {
		t.Fatalf("timeout: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Settings

func TestBillingSettingsWriteOnly(t *testing.T) {
	_, sa := gcpTestKey(t)
	in := Integrations{}.Apply(IntegrationsPatch{
		BillingProvider:             billingPtr("AWS"),
		BillingAWSAccessKeyID:       billingPtr("AKIAEXAMPLEKEY123456"),
		BillingAWSSecretAccessKey:   billingPtr("s3cr3t/AWS+secret-key-material"),
		BillingAzureClientSecret:    billingPtr("az~client~secret"),
		BillingGCPServiceAccountKey: billingPtr(sa),
	})
	if in.BillingProvider != "aws" || in.BillingAWSRegion != "us-east-1" || !in.billingConfigured() {
		t.Fatalf("normalised %+v", in)
	}
	v := in.View()
	raw, _ := json.Marshal(v)
	for _, leak := range []string{"AKIAEXAMPLEKEY123456", "s3cr3t", "az~client~secret", "PRIVATE KEY"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("view leaks %q: %s", leak, raw)
		}
	}
	if !v.BillingAWSAccessKeyIDSet || !v.BillingAWSSecretAccessKeySet || !v.BillingAzureClientSecretSet || !v.BillingGCPServiceAccountKeySet || !v.BillingConfigured {
		t.Fatalf("set flags %+v", v)
	}
	// An empty value keeps the secret; Clear removes it.
	in = in.Apply(IntegrationsPatch{BillingAWSSecretAccessKey: billingPtr("  "), BillingAzureClientSecretClear: true})
	if in.BillingAWSSecretAccessKey == "" || in.BillingAzureClientSecret != "" {
		t.Fatalf("write-only semantics %+v", in)
	}
	// Unknown provider normalises to off.
	if got := (Integrations{BillingProvider: "oracle"}).Normalize(); got.BillingProvider != "" || got.billingConfigured() {
		t.Fatalf("unknown provider %+v", got)
	}
}

func billingPtr[T any](v T) *T { return &v }

func TestValidateBillingPatch(t *testing.T) {
	_, sa := gcpTestKey(t)
	ok := []IntegrationsPatch{
		{BillingProvider: billingPtr("gcp"), BillingGCPTable: billingPtr("billing-proj.ds.tbl"), BillingGCPServiceAccountKey: billingPtr(sa)},
		{BillingProvider: billingPtr(""), BillingTagKey: billingPtr("")},
		{BillingAzureTenantID: billingPtr("contoso.onmicrosoft.com"), BillingAzureClientID: billingPtr(azClient), BillingAzureSubscriptionID: billingPtr(azSub)},
		{BillingAWSRegion: billingPtr("cn-northwest-1"), BillingAWSAccessKeyID: billingPtr("AKIAEXAMPLEKEY123456"), BillingAWSSecretAccessKey: billingPtr("abcdefghijklmnop/+")},
	}
	for i, p := range ok {
		if err := ValidateBillingPatch(p); err != nil {
			t.Errorf("ok[%d]: %v", i, err)
		}
	}
	bad := []IntegrationsPatch{
		{BillingProvider: billingPtr("oracle")},
		{BillingAWSRegion: billingPtr("eu-west-1")},
		{BillingAWSAccessKeyID: billingPtr("akia lower")},
		{BillingAWSSecretAccessKey: billingPtr("short")},
		{BillingAzureTenantID: billingPtr("../../evil")},
		{BillingAzureClientID: billingPtr("not-a-guid")},
		{BillingAzureSubscriptionID: billingPtr(azSub + "/../x")},
		{BillingGCPProjectID: billingPtr("Bad_Project")},
		{BillingGCPTable: billingPtr("p.d.t`--")},
		{BillingGCPServiceAccountKey: billingPtr(`{"type":"user"}`)},
		{BillingTagKey: billingPtr("a\"b")},
		{BillingTagValue: billingPtr("line\nbreak")},
	}
	for i, p := range bad {
		err := ValidateBillingPatch(p)
		if err == nil {
			t.Errorf("bad[%d] accepted", i)
		} else if strings.Contains(err.Error(), "short") || strings.Contains(err.Error(), "akia lower") {
			t.Errorf("bad[%d] error echoes the credential: %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Service

type countingFetcher struct {
	calls  atomic.Int32
	totals []billingMonthTotal
	err    error
}

func (f *countingFetcher) fetch(context.Context, []billingPeriod, time.Time) ([]billingMonthTotal, error) {
	f.calls.Add(1)
	return f.totals, f.err
}

func newTestBillingService(f billingFetcher, now *time.Time) *BillingService {
	s := NewBillingService()
	s.now = func() time.Time { return *now }
	s.newFetcher = func(Integrations) (billingFetcher, error) { return f, nil }
	return s
}

func TestBillingServiceCachesAnHour(t *testing.T) {
	now := billingNow
	f := &countingFetcher{totals: []billingMonthTotal{{Month: "2026-10", Amount: 900, Currency: "USD"}, {Month: "2026-11", Amount: 120, Currency: "USD"}}}
	s := newTestBillingService(f, &now)
	cfg := awsCfg()
	ctx := context.Background()
	p := NewDemoProvider()
	for i := 0; i < 3; i++ {
		if rep := s.Report(ctx, cfg, p, false); rep.Error != "" {
			t.Fatal(rep.Error)
		}
	}
	if f.calls.Load() != 1 {
		t.Fatalf("calls %d, want 1", f.calls.Load())
	}
	now = now.Add(59 * time.Minute)
	s.Report(ctx, cfg, p, false)
	if f.calls.Load() != 1 {
		t.Fatalf("refetched inside the hour")
	}
	now = now.Add(2 * time.Minute)
	s.Report(ctx, cfg, p, false)
	if f.calls.Load() != 2 {
		t.Fatalf("not refetched after an hour: %d", f.calls.Load())
	}
	// A settings change is a different cache entry.
	cfg.BillingTagValue = "staging"
	s.Report(ctx, cfg, p, false)
	if f.calls.Load() != 3 {
		t.Fatalf("settings change served cached numbers: %d", f.calls.Load())
	}
}

func TestBillingServiceErrorCachedFiveMinutes(t *testing.T) {
	now := billingNow
	f := &countingFetcher{err: billingErrorf("AWS Cost Explorer answered HTTP 403 (AccessDeniedException)")}
	s := newTestBillingService(f, &now)
	rep := s.Report(context.Background(), awsCfg(), NewDemoProvider(), false)
	if rep.Error != "AWS Cost Explorer answered HTTP 403 (AccessDeniedException)" || !rep.Configured || !rep.Experimental || len(rep.Months) != 0 {
		t.Fatalf("report %+v", rep)
	}
	s.Report(context.Background(), awsCfg(), NewDemoProvider(), false)
	now = now.Add(6 * time.Minute)
	s.Report(context.Background(), awsCfg(), NewDemoProvider(), false)
	if f.calls.Load() != 2 {
		t.Fatalf("calls %d, want 2", f.calls.Load())
	}
	// A non-billing error is never shown verbatim.
	f.err = errors.New("dial tcp 10.0.0.1: secret-ish detail")
	now = now.Add(6 * time.Minute)
	if rep := s.Report(context.Background(), awsCfg(), NewDemoProvider(), false); rep.Error != "unexpected error" {
		t.Fatalf("raw error shown: %q", rep.Error)
	}
}

func TestBillingReportAssembly(t *testing.T) {
	now := billingNow // 2026-11-04 12:00 → 3.5 days into November
	f := &countingFetcher{totals: []billingMonthTotal{{Month: "2026-10", Amount: 900, Currency: "USD"}, {Month: "2026-11", Amount: 120, Currency: "usd"}}}
	s := newTestBillingService(f, &now)
	rep := s.Report(context.Background(), awsCfg(), NewDemoProvider(), false)
	// Demo estimate includes GPU rate (default 100×2) → 720 USD a month.
	const demoMonthly = 720.0
	if rep.Provider != "aws" || rep.Currency != "USD" || rep.AllocatedSource != "estimate" || len(rep.Months) != 2 || !rep.Experimental {
		t.Fatalf("report %+v", rep)
	}
	oct, nov := rep.Months[0], rep.Months[1]
	if oct.Month != "2026-10" || oct.Partial || oct.InvoiceTotal != 900 || oct.Allocated != roundCents(demoMonthly*31.0/30) || oct.Unallocated == nil || *oct.Unallocated != roundCents(900-oct.Allocated) {
		t.Fatalf("october %+v", oct)
	}
	if nov.Month != "2026-11" || !nov.Partial || nov.Allocated != roundCents(demoMonthly*3.5/30) || nov.AsOf != "2026-11-04T12:00:00Z" {
		t.Fatalf("november %+v", nov)
	}

	// A non-USD invoice shows no difference.
	f2 := &countingFetcher{totals: []billingMonthTotal{{Month: "2026-10", Amount: 900, Currency: "EUR"}}}
	rep = newTestBillingService(f2, &now).Report(context.Background(), awsCfg(), NewDemoProvider(), false)
	if rep.Currency != "EUR" || rep.Months[0].Unallocated != nil || rep.Months[1].InvoiceTotal != 0 || !strings.Contains(rep.Note, "EUR") {
		t.Fatalf("eur %+v", rep)
	}
	// Mixed currencies are refused.
	f3 := &countingFetcher{totals: []billingMonthTotal{{Month: "2026-10", Amount: 1, Currency: "EUR"}, {Month: "2026-11", Amount: 1, Currency: "USD"}}}
	if rep = newTestBillingService(f3, &now).Report(context.Background(), awsCfg(), NewDemoProvider(), false); !strings.Contains(rep.Error, "mixes currencies") {
		t.Fatalf("mixed %+v", rep)
	}
}

func TestBillingReportNotConfiguredAndDemo(t *testing.T) {
	s := NewBillingService()
	rep := s.Report(context.Background(), Integrations{}, NewDemoProvider(), false)
	if rep.Configured || !rep.Experimental || rep.Months == nil || len(rep.Months) != 0 || rep.Error != "" {
		t.Fatalf("unconfigured %+v", rep)
	}
	s.now = func() time.Time { return billingNow }
	a := s.Report(context.Background(), Integrations{}, nil, true)
	b := s.Report(context.Background(), Integrations{}, nil, true)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) || !a.Demo || !a.Configured || len(a.Months) != 2 || a.Months[0].Month != "2026-10" ||
		a.Months[0].InvoiceTotal != 4812.40 || *a.Months[0].Unallocated != 837.20 || a.Months[1].Allocated != 1288.10 {
		t.Fatalf("demo %s", ja)
	}
	res := s.Test(context.Background(), Integrations{}, false)
	if res.OK || res.Service != "billing" || res.Error == "" {
		t.Fatalf("test unconfigured %+v", res)
	}
}

func TestBillingServiceTestBypassesCache(t *testing.T) {
	now := billingNow
	f := &countingFetcher{totals: []billingMonthTotal{{Month: "2026-10", Amount: 900, Currency: "USD"}}}
	s := newTestBillingService(f, &now)
	s.Report(context.Background(), awsCfg(), NewDemoProvider(), false)
	res := s.Test(context.Background(), awsCfg(), false)
	if !res.OK || f.calls.Load() != 2 {
		t.Fatalf("test %+v calls %d", res, f.calls.Load())
	}
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), "s3cr3t") || strings.Contains(string(raw), "AKIAEXAMPLE") {
		t.Fatalf("test result leaks credentials: %s", raw)
	}
}
