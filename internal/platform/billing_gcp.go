package platform

import (
	"bytes"
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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GCP Cloud Billing via the BigQuery billing export (EXPERIMENTAL; only tested
// against httptest fixtures). A service-account JWT is exchanged for a token
// at the fixed Google token endpoint (the key file's own token_uri is
// ignored), then one fixed, parameterised query sums cost plus credits per
// invoice month over the configured export table. The table name is the only
// part of the SQL not passed as a parameter (BigQuery cannot parameterise
// identifiers), so it is validated strictly: project.dataset.table, letters,
// digits and underscores (hyphens in the project).

const (
	gcpTokenHost     = "oauth2.googleapis.com"
	gcpBigQueryHost  = "bigquery.googleapis.com"
	gcpTokenAud      = "https://oauth2.googleapis.com/token"
	gcpBigQueryScope = "https://www.googleapis.com/auth/bigquery"
)

type gcpTable struct{ Project, Dataset, Table string }

func (t gcpTable) String() string { return t.Project + "." + t.Dataset + "." + t.Table }

func parseBillingGCPTable(s string) (gcpTable, error) {
	// No trimming: callers pass already-trimmed settings, and anything else
	// (a newline, a backtick) must fail. Datasets and tables are at most 1024
	// characters each in BigQuery.
	m := billingGCPTableRe.FindStringSubmatch(s)
	if m == nil || len(m[2]) > 1024 || len(m[3]) > 1024 {
		return gcpTable{}, errors.New("billingGcpTable must be project.dataset.table (letters, digits and underscores; a lowercase project ID)")
	}
	return gcpTable{Project: m[1], Dataset: m[2], Table: m[3]}, nil
}

type gcpServiceAccount struct {
	Type         string `json:"type"`
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	PrivateKeyID string `json:"private_key_id"`
	key          *rsa.PrivateKey
}

// parseGCPServiceAccountKey validates a service-account key file. Errors
// never quote the file.
func parseGCPServiceAccountKey(b []byte) (*gcpServiceAccount, error) {
	if len(b) > billingMaxSAKeyLen {
		return nil, errors.New("the GCP service-account key is too large")
	}
	var sa gcpServiceAccount
	if err := json.Unmarshal(b, &sa); err != nil {
		return nil, errors.New("the GCP service-account key is not valid JSON")
	}
	if sa.Type != "service_account" || !strings.Contains(sa.ClientEmail, "@") || sa.PrivateKey == "" {
		return nil, errors.New("the GCP key must be a service-account key file (type service_account with client_email and private_key)")
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return nil, errors.New("the GCP service-account private key is not PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("the GCP service-account private key is not an RSA key")
		}
		key = rk
	} else if rk, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = rk
	} else {
		return nil, errors.New("the GCP service-account private key could not be parsed")
	}
	sa.key = key
	return &sa, nil
}

type gcpBillingClient struct {
	t                billingTransport
	sa               *gcpServiceAccount
	table            gcpTable
	jobProject       string
	tagKey, tagValue string
}

func newGCPBillingClient(cfg Integrations, t *billingTransport) (*gcpBillingClient, error) {
	table, err := parseBillingGCPTable(cfg.BillingGCPTable)
	if err != nil {
		return nil, billingErrorf("%s", err.Error())
	}
	sa, err := parseGCPServiceAccountKey([]byte(cfg.BillingGCPServiceAccountKey))
	if err != nil {
		return nil, billingErrorf("%s", err.Error())
	}
	project := cfg.BillingGCPProjectID
	if project == "" {
		project = table.Project
	}
	if !billingGCPProjRe.MatchString(project) {
		return nil, billingErrorf("the GCP project ID is not valid")
	}
	tr := newBillingTransport(gcpTokenHost, gcpBigQueryHost)
	if t != nil {
		tr = *t
	}
	return &gcpBillingClient{t: tr, sa: sa, table: table, jobProject: project, tagKey: cfg.BillingTagKey, tagValue: cfg.BillingTagValue}, nil
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// gcpJWT is the RS256 assertion for the JWT-bearer grant.
func gcpJWT(sa *gcpServiceAccount, now time.Time) (string, error) {
	hdr := map[string]string{"alg": "RS256", "typ": "JWT"}
	if sa.PrivateKeyID != "" {
		hdr["kid"] = sa.PrivateKeyID
	}
	claims := map[string]any{
		"iss":   sa.ClientEmail,
		"scope": gcpBigQueryScope,
		"aud":   gcpTokenAud,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	h, _ := json.Marshal(hdr)
	c, _ := json.Marshal(claims)
	signing := b64url(h) + "." + b64url(c)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, sa.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64url(sig), nil
}

func (c *gcpBillingClient) token(ctx context.Context, now time.Time) (string, error) {
	assertion, err := gcpJWT(c.sa, now)
	if err != nil {
		return "", billingErrorf("could not sign the Google token request")
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	ctx, cancel := context.WithTimeout(ctx, billingRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.t.baseURL(gcpTokenHost)+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", billingErrorf("could not build the Google token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.t.do(req, "Google OAuth")
	if err != nil {
		return "", err
	}
	var tok oauthToken
	if err := readBillingJSON(resp, "Google OAuth", &tok, oauthErrorCode); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", billingErrorf("Google OAuth returned no access token")
	}
	return tok.AccessToken, nil
}

// gcpBillingSQL is the only query ever sent. %s is the validated table; every
// value is a named parameter.
func gcpBillingSQL(table gcpTable, withTag bool) string {
	tag := ""
	if withTag {
		tag = "\n  AND EXISTS (SELECT 1 FROM UNNEST(labels) AS l WHERE l.key = @tag_key AND l.value = @tag_value)"
	}
	return fmt.Sprintf("SELECT invoice.month AS month, currency,\n"+
		"  SUM(cost) + SUM(IFNULL((SELECT SUM(c.amount) FROM UNNEST(credits) AS c), 0)) AS total\n"+
		"FROM `%s`\n"+
		"WHERE invoice.month IN UNNEST(@months)%s\n"+
		"GROUP BY month, currency\nORDER BY month", table.String(), tag)
}

type bqParam struct {
	Name           string         `json:"name"`
	ParameterType  map[string]any `json:"parameterType"`
	ParameterValue map[string]any `json:"parameterValue"`
}

type bqQueryResponse struct {
	JobComplete bool `json:"jobComplete"`
	Rows        []struct {
		F []struct {
			V any `json:"v"`
		} `json:"f"`
	} `json:"rows"`
}

func (c *gcpBillingClient) fetch(ctx context.Context, periods []billingPeriod, now time.Time) ([]billingMonthTotal, error) {
	tok, err := c.token(ctx, now)
	if err != nil {
		return nil, err
	}
	months := make([]map[string]string, 0, len(periods))
	for _, p := range periods {
		months = append(months, map[string]string{"value": strings.ReplaceAll(p.Month, "-", "")})
	}
	params := []bqParam{{
		Name:           "months",
		ParameterType:  map[string]any{"type": "ARRAY", "arrayType": map[string]string{"type": "STRING"}},
		ParameterValue: map[string]any{"arrayValues": months},
	}}
	withTag := c.tagKey != ""
	if withTag {
		params = append(params,
			bqParam{Name: "tag_key", ParameterType: map[string]any{"type": "STRING"}, ParameterValue: map[string]any{"value": c.tagKey}},
			bqParam{Name: "tag_value", ParameterType: map[string]any{"type": "STRING"}, ParameterValue: map[string]any{"value": c.tagValue}})
	}
	body := map[string]any{
		"query":           gcpBillingSQL(c.table, withTag),
		"useLegacySql":    false,
		"parameterMode":   "NAMED",
		"queryParameters": params,
		"timeoutMs":       15000,
		"maxResults":      100,
	}
	raw, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, billingRequestTimeout)
	defer cancel()
	u := c.t.baseURL(gcpBigQueryHost) + "/bigquery/v2/projects/" + url.PathEscape(c.jobProject) + "/queries"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return nil, billingErrorf("could not build the BigQuery request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.t.do(req, "BigQuery")
	if err != nil {
		return nil, err
	}
	var out bqQueryResponse
	if err := readBillingJSON(resp, "BigQuery", &out, oauthErrorCode); err != nil {
		return nil, err
	}
	if !out.JobComplete {
		return nil, billingErrorf("the BigQuery billing query did not finish in time")
	}
	var totals []billingMonthTotal
	for _, r := range out.Rows {
		if len(r.F) < 3 {
			continue
		}
		month, _ := r.F[0].V.(string)
		cur, _ := r.F[1].V.(string)
		amtStr, _ := r.F[2].V.(string)
		if len(month) != 6 {
			return nil, billingErrorf("BigQuery returned an invoice month it could not read")
		}
		amt, err := strconv.ParseFloat(amtStr, 64)
		if err != nil {
			return nil, billingErrorf("BigQuery returned an amount that is not a number")
		}
		totals = append(totals, billingMonthTotal{Month: month[:4] + "-" + month[4:], Amount: amt, Currency: cur})
	}
	return totals, nil
}
