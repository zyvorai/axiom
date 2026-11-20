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

// AWS Cost Explorer GetCostAndUsage (EXPERIMENTAL). POSTs to the partition's
// fixed endpoint, signed with Signature Version 4, asking for UnblendedCost
// per month (paginated via NextPageToken), optionally filtered by one
// cost-allocation tag. Supports STS session tokens via X-Amz-Security-Token.
// Live accounts: e2e/scripts/validate-billing-live.sh.

type awsBillingClient struct {
	t                    billingTransport
	host, region         string
	accessKey, secret    string
	sessionToken         string
	tagKey, tagValue     string
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
		sessionToken: strings.TrimSpace(cfg.BillingAWSSessionToken),
		tagKey: cfg.BillingTagKey, tagValue: cfg.BillingTagValue,
	}, nil
}

type awsCERequest struct {
	TimePeriod    struct{ Start, End string } `json:"TimePeriod"`
	Granularity   string                      `json:"Granularity"`
	Metrics       []string                    `json:"Metrics"`
	Filter        *awsCEFilter                `json:"Filter,omitempty"`
	GroupBy       []awsCEGroupBy              `json:"GroupBy,omitempty"`
	NextPageToken string                      `json:"NextPageToken,omitempty"`
}

type awsCEGroupBy struct {
	Type string `json:"Type"`
	Key  string `json:"Key"`
}

type awsCEFilter struct {
	Tags awsCETags `json:"Tags"`
}

type awsCETags struct {
	Key          string   `json:"Key"`
	Values       []string `json:"Values"`
	MatchOptions []string `json:"MatchOptions"`
}

type awsCEMetric struct {
	Amount string `json:"Amount"`
	Unit   string `json:"Unit"`
}

type awsCEResponse struct {
	ResultsByTime []struct {
		TimePeriod struct{ Start, End string } `json:"TimePeriod"`
		Total      map[string]awsCEMetric      `json:"Total"`
		Groups     []struct {
			Keys    []string                `json:"Keys"`
			Metrics map[string]awsCEMetric  `json:"Metrics"`
		} `json:"Groups"`
	} `json:"ResultsByTime"`
	NextPageToken string `json:"NextPageToken,omitempty"`
}

func (c *awsBillingClient) fetch(ctx context.Context, periods []billingPeriod, now time.Time) ([]billingMonthTotal, error) {
	var body awsCERequest
	body.TimePeriod.Start = periods[0].Start.Format("2006-01-02")
	body.TimePeriod.End = periods[len(periods)-1].End.Format("2006-01-02")
	body.Granularity = "MONTHLY"
	body.Metrics = []string{"UnblendedCost"}
	body.GroupBy = []awsCEGroupBy{{Type: "DIMENSION", Key: "SERVICE"}}
	if c.tagKey != "" {
		body.Filter = &awsCEFilter{Tags: awsCETags{Key: c.tagKey, Values: []string{c.tagValue}, MatchOptions: []string{"EQUALS"}}}
	}
	byMonth := map[string]billingMonthTotal{}
	svcByMonth := map[string]map[string]float64{}
	token := ""
	for page := 0; page < 20; page++ {
		body.NextPageToken = token
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, billingErrorf("could not build the Cost Explorer request")
		}
		pageCtx, cancel := context.WithTimeout(ctx, billingRequestTimeout)
		req, err := http.NewRequestWithContext(pageCtx, http.MethodPost, c.t.baseURL(c.host)+"/", bytes.NewReader(raw))
		if err != nil {
			cancel()
			return nil, billingErrorf("could not build the Cost Explorer request")
		}
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "AWSInsightsIndexService.GetCostAndUsage")
		req.Header.Set("Accept", "application/json")
		if c.sessionToken != "" {
			req.Header.Set("X-Amz-Security-Token", c.sessionToken)
		}
		// Sign for the fixed host even when a test reroutes the connection.
		req.Host = c.host
		sigV4Sign(req, raw, c.accessKey, c.secret, c.region, "ce", now)
		resp, err := c.t.do(req, "AWS Cost Explorer")
		if err != nil {
			cancel()
			return nil, err
		}
		var out awsCEResponse
		err = readBillingJSON(resp, "AWS Cost Explorer", &out, awsErrorCode)
		cancel() // after body read — cancelling early aborts an in-flight LimitReader
		if err != nil {
			return nil, err
		}
		for _, r := range out.ResultsByTime {
			if len(r.TimePeriod.Start) < 7 {
				continue
			}
			month := r.TimePeriod.Start[:7]
			cur := byMonth[month]
			cur.Month = month
			svc := svcByMonth[month]
			if svc == nil {
				svc = map[string]float64{}
				svcByMonth[month] = svc
			}
			if len(r.Groups) > 0 {
				for _, g := range r.Groups {
					m, ok := g.Metrics["UnblendedCost"]
					if !ok {
						continue
					}
					amt, err := strconv.ParseFloat(strings.TrimSpace(m.Amount), 64)
					if err != nil {
						return nil, billingErrorf("AWS Cost Explorer returned an amount that is not a number")
					}
					cur.Amount += amt
					if cur.Currency == "" {
						cur.Currency = m.Unit
					}
					name := "Other"
					if len(g.Keys) > 0 && strings.TrimSpace(g.Keys[0]) != "" {
						name = g.Keys[0]
					}
					svc[name] += amt
				}
			} else if m, ok := r.Total["UnblendedCost"]; ok {
				amt, err := strconv.ParseFloat(strings.TrimSpace(m.Amount), 64)
				if err != nil {
					return nil, billingErrorf("AWS Cost Explorer returned an amount that is not a number")
				}
				cur.Amount += amt
				if cur.Currency == "" {
					cur.Currency = m.Unit
				}
			}
			byMonth[month] = cur
		}
		token = strings.TrimSpace(out.NextPageToken)
		if token == "" {
			break
		}
	}
	totals := make([]billingMonthTotal, 0, len(byMonth))
	for month, m := range byMonth {
		m.Services = topBillingServices(svcByMonth[month], 10)
		totals = append(totals, m)
	}
	sort.Slice(totals, func(i, j int) bool { return totals[i].Month < totals[j].Month })
	return totals, nil
}

func topBillingServices(byName map[string]float64, n int) []BillingServiceLine {
	if len(byName) == 0 {
		return nil
	}
	out := make([]BillingServiceLine, 0, len(byName))
	for name, amt := range byName {
		out = append(out, BillingServiceLine{Name: name, Amount: roundCents(amt)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Amount == out[j].Amount {
			return out[i].Name < out[j].Name
		}
		return out[i].Amount > out[j].Amount
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
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
