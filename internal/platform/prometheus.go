package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PrometheusClient runs instant queries against an operator-configured
// Prometheus (AXIOM_PROMETHEUS_URL). The URL comes from the environment, so
// private / in-cluster addresses are allowed; the client still bounds every
// query (timeout, response size), ignores proxy environment variables, never
// follows a redirect to another host, and keeps the URL and bearer token out
// of every error it returns.
type PrometheusClient struct {
	base     *url.URL
	token    string
	http     *http.Client
	timeout  time.Duration
	maxBytes int64
}

const (
	prometheusQueryTimeout = 10 * time.Second
	prometheusMaxBody      = 8 << 20
	prometheusMaxRedirects = 3
	// rightsizingMaxPromQueries bounds the Prometheus queries one
	// rightsizing request may issue.
	rightsizingMaxPromQueries = 4
)

// NewPrometheusClient validates the base URL (http or https, a host, no
// userinfo) and returns a client. A nil client and nil error mean "not
// configured" (empty URL).
func NewPrometheusClient(rawURL, token string) (*PrometheusClient, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("AXIOM_PROMETHEUS_URL is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("AXIOM_PROMETHEUS_URL must use http or https")
	}
	if u.Host == "" || u.Hostname() == "" {
		return nil, errors.New("AXIOM_PROMETHEUS_URL must include a host")
	}
	if u.User != nil {
		return nil, errors.New("AXIOM_PROMETHEUS_URL must not embed credentials; use AXIOM_PROMETHEUS_TOKEN")
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	c := &PrometheusClient{base: u, token: strings.TrimSpace(token), timeout: prometheusQueryTimeout, maxBytes: prometheusMaxBody}
	transport := &http.Transport{
		Proxy:                 nil, // never route through HTTP(S)_PROXY
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: prometheusQueryTimeout,
		MaxIdleConns:          4,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	c.http = &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= prometheusMaxRedirects {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != c.base.Scheme || !strings.EqualFold(req.URL.Host, c.base.Host) {
				return errors.New("redirect to another host refused")
			}
			return nil
		},
	}
	return c, nil
}

// promSample is one element of an instant-vector result.
type promSample struct {
	Labels map[string]string
	Value  float64
}

// promError is a Prometheus failure safe to show: it never carries the URL,
// the token or the response body.
type promError struct{ msg string }

func (e *promError) Error() string { return e.msg }

func promErrorf(format string, a ...any) error { return &promError{msg: fmt.Sprintf(format, a...)} }

// Query runs one instant query and returns the vector samples.
func (c *PrometheusClient) Query(ctx context.Context, promql string) ([]promSample, error) {
	if c == nil {
		return nil, promErrorf("Prometheus is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	u := *c.base
	u.Path = c.base.Path + "/api/v1/query"
	u.RawQuery = url.Values{"query": {promql}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, promErrorf("could not build the Prometheus request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "zyvor-axiom")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// *url.Error embeds the request URL: describe the failure instead.
		var ne net.Error
		switch {
		case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
			return nil, promErrorf("Prometheus query timed out after %s", c.timeout)
		case errors.Is(err, context.Canceled):
			return nil, promErrorf("Prometheus query was cancelled")
		case strings.Contains(err.Error(), "redirect to another host refused"):
			return nil, promErrorf("Prometheus redirected to another host; refused")
		case strings.Contains(err.Error(), "too many redirects"):
			return nil, promErrorf("Prometheus redirected too many times")
		}
		return nil, promErrorf("Prometheus is unreachable")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, promErrorf("Prometheus query timed out after %s", c.timeout)
		}
		return nil, promErrorf("reading the Prometheus response failed")
	}
	if int64(len(body)) > c.maxBytes {
		return nil, promErrorf("Prometheus response exceeds %d MiB", c.maxBytes>>20)
	}
	var parsed struct {
		Status    string `json:"status"`
		ErrorType string `json:"errorType"`
		Data      struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if resp.StatusCode != http.StatusOK {
		// Prometheus answers 400/422/503 with a JSON body naming the errorType.
		if json.Unmarshal(body, &parsed) == nil && safeToken(parsed.ErrorType) != "" {
			return nil, promErrorf("Prometheus returned HTTP %d (%s)", resp.StatusCode, safeToken(parsed.ErrorType))
		}
		return nil, promErrorf("Prometheus returned HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, promErrorf("Prometheus returned a malformed response")
	}
	if parsed.Status != "success" {
		if t := safeToken(parsed.ErrorType); t != "" {
			return nil, promErrorf("Prometheus query failed (%s)", t)
		}
		return nil, promErrorf("Prometheus query failed")
	}
	if parsed.Data.ResultType != "vector" {
		return nil, promErrorf("Prometheus returned a %q result, expected a vector", safeToken(parsed.Data.ResultType))
	}
	out := make([]promSample, 0, len(parsed.Data.Result))
	for _, r := range parsed.Data.Result {
		if len(r.Value) != 2 {
			return nil, promErrorf("Prometheus returned a malformed response")
		}
		s, ok := r.Value[1].(string)
		if !ok {
			return nil, promErrorf("Prometheus returned a malformed response")
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, promErrorf("Prometheus returned a malformed response")
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		out = append(out, promSample{Labels: r.Metric, Value: v})
	}
	return out, nil
}

// safeToken keeps a short identifier-like string (an errorType such as
// "bad_data") and drops anything else.
func safeToken(s string) string {
	if len(s) == 0 || len(s) > 32 {
		return ""
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return ""
		}
	}
	return s
}

// ---------------------------------------------------------------------------
// PromQL builders

// promQuote renders s as a PromQL string literal. PromQL strings use Go's
// escaping rules, so strconv.Quote can never let a value close the literal.
func promQuote(s string) string { return strconv.Quote(s) }

// promContainerSelector selects real application containers (not the pause
// container, not the pod-level cgroup) in one namespace, or every namespace.
func promContainerSelector(namespace string) string {
	ns := `namespace!=""`
	if namespace != "" {
		ns = "namespace=" + promQuote(namespace)
	}
	return `{container!="",container!="POD",` + ns + `}`
}

// promCPUP95Query is the per-container p95 CPU usage (cores) over window.
func promCPUP95Query(namespace, window string) string {
	return fmt.Sprintf(`max by (namespace, pod, container) (quantile_over_time(0.95, sum by (namespace, pod, container) (rate(container_cpu_usage_seconds_total%s[5m]))[%s:5m]))`,
		promContainerSelector(namespace), window)
}

// promMemP99Query is the per-container p99 working-set memory (bytes) over window.
func promMemP99Query(namespace, window string) string {
	return fmt.Sprintf(`max by (namespace, pod, container) (quantile_over_time(0.99, container_memory_working_set_bytes%s[%s]))`,
		promContainerSelector(namespace), window)
}

// ---------------------------------------------------------------------------
// Prometheus usage source

type promUsageSource struct{ client *PrometheusClient }

func (promUsageSource) name() string { return rightsizingSourcePrometheus }
func (promUsageSource) basis() rightsizingBasis {
	return rightsizingBasis{CPU: "p95", Memory: "p99"}
}
func (promUsageSource) confidence(window string) string {
	if rightsizingWindows[window] >= 7 {
		return "high"
	}
	return "medium"
}
func (promUsageSource) note(window string) string {
	n := fmt.Sprintf("p95 CPU and p99 working-set memory per container over the last %s from Prometheus (cAdvisor metrics), with 20%% CPU and 15%% memory headroom.", window)
	if rightsizingWindows[window] < 7 {
		n += " A window shorter than 7 days can miss weekly peaks."
	}
	return n
}

func (p promUsageSource) usage(ctx context.Context, namespace, window string) ([]podContainerUsage, error) {
	if _, ok := rightsizingWindows[window]; !ok {
		return nil, promErrorf("unsupported window")
	}
	queries := []string{promCPUP95Query(namespace, window), promMemP99Query(namespace, window)}
	if len(queries) > rightsizingMaxPromQueries {
		return nil, promErrorf("too many Prometheus queries")
	}
	cpu, err := p.client.Query(ctx, queries[0])
	if err != nil {
		return nil, err
	}
	mem, err := p.client.Query(ctx, queries[1])
	if err != nil {
		return nil, err
	}
	byKey := map[string]*podContainerUsage{}
	get := func(l map[string]string) *podContainerUsage {
		k := l["namespace"] + "/" + l["pod"] + "/" + l["container"]
		u := byKey[k]
		if u == nil {
			u = &podContainerUsage{Namespace: l["namespace"], Pod: l["pod"], Container: l["container"]}
			byKey[k] = u
		}
		return u
	}
	for _, s := range cpu {
		u := get(s.Labels)
		u.HasCPU = true
		u.CPUMilli = int64(math.Ceil(s.Value*1000 - 1e-6))
		if u.CPUMilli < 0 {
			u.CPUMilli = 0
		}
	}
	for _, s := range mem {
		u := get(s.Labels)
		u.HasMem = true
		u.MemBytes = int64(math.Ceil(s.Value))
	}
	out := make([]podContainerUsage, 0, len(byKey))
	for _, u := range byKey {
		if namespace != "" && u.Namespace != namespace {
			continue
		}
		out = append(out, *u)
	}
	return out, nil
}
