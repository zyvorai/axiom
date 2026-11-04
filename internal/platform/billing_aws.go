package platform

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AWS Cost Explorer GetCostAndUsage (EXPERIMENTAL; only tested against
// httptest fixtures). One POST to the partition's fixed endpoint, signed with
// Signature Version 4 implemented here, asking for UnblendedCost per month,
// optionally filtered by one cost-allocation tag.

type awsBillingClient struct {
	t                 billingTransport
	host, region      string
	accessKey, secret string
	tagKey, tagValue  string
}

func newAWSBillingClient(cfg Integrations, t *billingTransport) (*awsBillingClient, error) {
	host, ok := billingAWSRegions[cfg.BillingAWSRegion]
	if !ok {
		cfg.BillingAWSRegion = billingDefaultAWSRegion
		host = billingAWSRegions[billingDefaultAWSRegion]
	}
	if cfg.BillingAWSAccessKeyID == "" || cfg.BillingAWSSecretAccessKey == "" {
		return nil, billingErrorf("the AWS access key is not configured")
	}
	tr := newBillingTransport(host)
	if t != nil {
		tr = *t
	}
	return &awsBillingClient{
		t: tr, host: host, region: cfg.BillingAWSRegion,
		accessKey: cfg.BillingAWSAccessKeyID, secret: cfg.BillingAWSSecretAccessKey,
		tagKey: cfg.BillingTagKey, tagValue: cfg.BillingTagValue,
	}, nil
}

type awsCERequest struct {
	TimePeriod  struct{ Start, End string } `json:"TimePeriod"`
	Granularity string                      `json:"Granularity"`
	Metrics     []string                    `json:"Metrics"`
	Filter      *awsCEFilter                `json:"Filter,omitempty"`
}

type awsCEFilter struct {
	Tags awsCETags `json:"Tags"`
}

type awsCETags struct {
	Key          string   `json:"Key"`
	Values       []string `json:"Values"`
	MatchOptions []string `json:"MatchOptions"`
}

type awsCEResponse struct {
	ResultsByTime []struct {
		TimePeriod struct{ Start, End string } `json:"TimePeriod"`
		Total      map[string]struct {
			Amount string `json:"Amount"`
			Unit   string `json:"Unit"`
		} `json:"Total"`
	} `json:"ResultsByTime"`
}

func (c *awsBillingClient) fetch(ctx context.Context, periods []billingPeriod, now time.Time) ([]billingMonthTotal, error) {
	var body awsCERequest
	body.TimePeriod.Start = periods[0].Start.Format("2006-01-02")
	body.TimePeriod.End = periods[len(periods)-1].End.Format("2006-01-02")
	body.Granularity = "MONTHLY"
	body.Metrics = []string{"UnblendedCost"}
	if c.tagKey != "" {
		body.Filter = &awsCEFilter{Tags: awsCETags{Key: c.tagKey, Values: []string{c.tagValue}, MatchOptions: []string{"EQUALS"}}}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, billingErrorf("could not build the Cost Explorer request")
	}
	ctx, cancel := context.WithTimeout(ctx, billingRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.t.baseURL(c.host)+"/", bytes.NewReader(raw))
	if err != nil {
		return nil, billingErrorf("could not build the Cost Explorer request")
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSInsightsIndexService.GetCostAndUsage")
	req.Header.Set("Accept", "application/json")
	// Sign for the fixed host even when a test reroutes the connection.
	req.Host = c.host
	sigV4Sign(req, raw, c.accessKey, c.secret, c.region, "ce", now)
	resp, err := c.t.do(req, "AWS Cost Explorer")
	if err != nil {
		return nil, err
	}
	var out awsCEResponse
	if err := readBillingJSON(resp, "AWS Cost Explorer", &out, awsErrorCode); err != nil {
		return nil, err
	}
	var totals []billingMonthTotal
	for _, r := range out.ResultsByTime {
		if len(r.TimePeriod.Start) < 7 {
			continue
		}
		m, ok := r.Total["UnblendedCost"]
		if !ok {
			continue
		}
		amt, err := strconv.ParseFloat(strings.TrimSpace(m.Amount), 64)
		if err != nil {
			return nil, billingErrorf("AWS Cost Explorer returned an amount that is not a number")
		}
		totals = append(totals, billingMonthTotal{Month: r.TimePeriod.Start[:7], Amount: amt, Currency: m.Unit})
	}
	return totals, nil
}

// awsErrorCode reads "__type" ("com.amazon...#AccessDeniedException").
func awsErrorCode(body []byte) string {
	var e struct {
		Type string `json:"__type"`
	}
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	if i := strings.LastIndex(e.Type, "#"); i >= 0 {
		return e.Type[i+1:]
	}
	return e.Type
}

// ---------------------------------------------------------------------------
// Signature Version 4
// (https://docs.aws.amazon.com/IAM/latest/UserGuide/create-signed-request.html)

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// awsURIEncode percent-encodes everything except the RFC 3986 unreserved set.
func awsURIEncode(s string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && keepSlash:
			b.WriteByte(c)
		default:
			b.WriteString("%" + strings.ToUpper(hex.EncodeToString([]byte{c})))
		}
	}
	return b.String()
}

func awsCanonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vs := append([]string(nil), q[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, awsURIEncode(k, false)+"="+awsURIEncode(v, false))
		}
	}
	return strings.Join(parts, "&")
}

// sigV4Sign adds X-Amz-Date and Authorization, signing the host and every
// header already set on req.
func sigV4Sign(req *http.Request, body []byte, accessKey, secret, region, service string, now time.Time) {
	amzDate := now.UTC().Format("20060102T150405Z")
	date := amzDate[:8]
	req.Header.Set("X-Amz-Date", amzDate)
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	headers := map[string]string{"host": host}
	for k, vs := range req.Header {
		lk := strings.ToLower(k)
		if lk == "authorization" || lk == "user-agent" {
			continue
		}
		vals := make([]string, len(vs))
		for i, v := range vs {
			vals[i] = strings.Join(strings.Fields(v), " ")
		}
		headers[lk] = strings.Join(vals, ",")
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")

	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	unescaped, err := url.PathUnescape(path)
	if err == nil {
		path = awsURIEncode(unescaped, true)
	}
	canonical := strings.Join([]string{
		req.Method, path, awsCanonicalQuery(req.URL.Query()), canonHeaders.String(), signed, sha256Hex(body),
	}, "\n")
	scope := date + "/" + region + "/" + service + "/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	k := hmacSHA256([]byte("AWS4"+secret), date)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, service)
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}
