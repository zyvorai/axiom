package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Azure Cost Management query API (EXPERIMENTAL; only tested against httptest
// fixtures). A client-credentials token from Microsoft Entra ID, then one
// ActualCost query at subscription scope with Monthly granularity, optionally
// filtered by one resource tag.

const (
	azureLoginHost      = "login.microsoftonline.com"
	azureManagementHost = "management.azure.com"
	azureCostAPIVersion = "2023-03-01"
)

type azureBillingClient struct {
	t                               billingTransport
	tenant, clientID, secret, subID string
	tagKey, tagValue                string
}

func newAzureBillingClient(cfg Integrations, t *billingTransport) (*azureBillingClient, error) {
	if cfg.BillingAzureTenantID == "" || cfg.BillingAzureClientID == "" || cfg.BillingAzureSubscriptionID == "" || cfg.BillingAzureClientSecret == "" {
		return nil, billingErrorf("the Azure tenant, client, secret and subscription are all required")
	}
	// Re-validate: these go into URL paths.
	if (!billingGUIDRe.MatchString(cfg.BillingAzureTenantID) && !billingTenantRe.MatchString(cfg.BillingAzureTenantID)) ||
		!billingGUIDRe.MatchString(cfg.BillingAzureSubscriptionID) || !billingGUIDRe.MatchString(cfg.BillingAzureClientID) {
		return nil, billingErrorf("the Azure tenant, client or subscription ID is not valid")
	}
	tr := newBillingTransport(azureLoginHost, azureManagementHost)
	if t != nil {
		tr = *t
	}
	return &azureBillingClient{
		t: tr, tenant: cfg.BillingAzureTenantID, clientID: cfg.BillingAzureClientID,
		secret: cfg.BillingAzureClientSecret, subID: cfg.BillingAzureSubscriptionID,
		tagKey: cfg.BillingTagKey, tagValue: cfg.BillingTagValue,
	}, nil
}

// oauthErrorCode reads {"error":"invalid_client",...} (Entra ID, Google).
func oauthErrorCode(body []byte) string {
	var e struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	switch v := e.Error.(type) {
	case string:
		return v
	case map[string]any:
		if s, ok := v["code"].(string); ok {
			return s
		}
		if s, ok := v["status"].(string); ok {
			return s
		}
	}
	return ""
}

type oauthToken struct {
	AccessToken string `json:"access_token"`
}

func (c *azureBillingClient) token(ctx context.Context) (string, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.secret},
		"scope":         {"https://management.azure.com/.default"},
	}
	ctx, cancel := context.WithTimeout(ctx, billingRequestTimeout)
	defer cancel()
	u := c.t.baseURL(azureLoginHost) + "/" + url.PathEscape(c.tenant) + "/oauth2/v2.0/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return "", billingErrorf("could not build the Microsoft Entra ID token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.t.do(req, "Microsoft Entra ID")
	if err != nil {
		return "", err
	}
	var tok oauthToken
	if err := readBillingJSON(resp, "Microsoft Entra ID", &tok, oauthErrorCode); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", billingErrorf("Microsoft Entra ID returned no access token")
	}
	return tok.AccessToken, nil
}

type azureQueryResponse struct {
	Properties struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
		Rows [][]any `json:"rows"`
	} `json:"properties"`
}

func (c *azureBillingClient) fetch(ctx context.Context, periods []billingPeriod, now time.Time) ([]billingMonthTotal, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	dataset := map[string]any{
		"granularity": "Monthly",
		"aggregation": map[string]any{"totalCost": map[string]any{"name": "Cost", "function": "Sum"}},
	}
	if c.tagKey != "" {
		dataset["filter"] = map[string]any{"tags": map[string]any{"name": c.tagKey, "operator": "In", "values": []string{c.tagValue}}}
	}
	last := periods[len(periods)-1].End.Add(-time.Second)
	body := map[string]any{
		"type":      "ActualCost",
		"timeframe": "Custom",
		"timePeriod": map[string]string{
			"from": periods[0].Start.Format("2006-01-02T15:04:05Z"),
			"to":   last.Format("2006-01-02T15:04:05Z"),
		},
		"dataset": dataset,
	}
	raw, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, billingRequestTimeout)
	defer cancel()
	u := c.t.baseURL(azureManagementHost) + "/subscriptions/" + url.PathEscape(c.subID) +
		"/providers/Microsoft.CostManagement/query?api-version=" + azureCostAPIVersion
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return nil, billingErrorf("could not build the Azure Cost Management request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.t.do(req, "Azure Cost Management")
	if err != nil {
		return nil, err
	}
	var out azureQueryResponse
	if err := readBillingJSON(resp, "Azure Cost Management", &out, oauthErrorCode); err != nil {
		return nil, err
	}
	costCol, monthCol, curCol := -1, -1, -1
	for i, col := range out.Properties.Columns {
		switch strings.ToLower(col.Name) {
		case "cost", "pretaxcost", "costusd":
			if costCol < 0 {
				costCol = i
			}
		case "billingmonth", "usagedate":
			monthCol = i
		case "currency":
			curCol = i
		}
	}
	if costCol < 0 || monthCol < 0 {
		return nil, billingErrorf("the Azure Cost Management response has no cost or month column")
	}
	var totals []billingMonthTotal
	for _, row := range out.Properties.Rows {
		if len(row) <= costCol || len(row) <= monthCol {
			continue
		}
		amt, ok := azureNumber(row[costCol])
		if !ok {
			return nil, billingErrorf("Azure Cost Management returned an amount that is not a number")
		}
		month, ok := azureMonth(row[monthCol])
		if !ok {
			return nil, billingErrorf("Azure Cost Management returned a month it could not read")
		}
		cur := ""
		if curCol >= 0 && curCol < len(row) {
			cur, _ = row[curCol].(string)
		}
		totals = append(totals, billingMonthTotal{Month: month, Amount: amt, Currency: cur})
	}
	return totals, nil
}

func azureNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

// azureMonth accepts "2026-10-01T00:00:00" (BillingMonth) or 20261001
// (UsageDate) and returns "2026-10".
func azureMonth(v any) (string, bool) {
	switch m := v.(type) {
	case string:
		if len(m) >= 7 && m[4] == '-' {
			return m[:7], true
		}
		if len(m) == 8 {
			return m[:4] + "-" + m[4:6], true
		}
	case float64:
		s := strconv.FormatInt(int64(m), 10)
		if len(s) == 8 {
			return s[:4] + "-" + s[4:6], true
		}
	}
	return "", false
}
