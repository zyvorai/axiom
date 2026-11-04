package platform

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var errBodyTooLarge = errors.New("body too large")

// readCapped reads at most max bytes and fails when there is more.
func readCapped(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errBodyTooLarge
	}
	return b, nil
}

// billingTransport sends a provider client's requests. In production http is
// newBillingHTTPClient(allowedHosts...) and baseURL is "https://"+host; unit
// tests replace both to reach an httptest server (the only way a non-fixed
// URL is ever contacted).
type billingTransport struct {
	http    *http.Client
	baseURL func(host string) string
}

func newBillingTransport(hosts ...string) billingTransport {
	return billingTransport{
		http:    newBillingHTTPClient(hosts...),
		baseURL: func(host string) string { return "https://" + host },
	}
}

// billingTestTransport points every fixed host at one test server.
func billingTestTransport(c *http.Client, base string) *billingTransport {
	base = strings.TrimRight(base, "/")
	return &billingTransport{http: c, baseURL: func(string) string { return base }}
}

// do sends req and maps transport failures to safe messages (a *url.Error
// would embed the URL, which for Azure carries the tenant and subscription).
func (t billingTransport) do(req *http.Request, what string) (*http.Response, error) {
	req.Header.Set("User-Agent", "zyvor-axiom")
	resp, err := t.http.Do(req)
	if err == nil {
		return resp, nil
	}
	var ne net.Error
	var he *billingHostError
	switch {
	case errors.As(err, &he):
		return nil, billingErrorf("refused to contact %s: %s", what, he.msg)
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return nil, billingErrorf("%s did not answer in time", what)
	case errors.Is(err, context.Canceled):
		return nil, billingErrorf("the %s request was cancelled", what)
	}
	var pe *helmPolicyError
	if errors.As(err, &pe) {
		return nil, billingErrorf("refused to contact %s: it does not resolve to a public address", what)
	}
	return nil, billingErrorf("could not reach %s", what)
}

type billingHostError struct{ msg string }

func (e *billingHostError) Error() string { return e.msg }

// billingHostGuard refuses any request that is not HTTPS to one of the
// provider's fixed API hosts on the default port.
type billingHostGuard struct {
	allowed map[string]bool
	next    http.RoundTripper
}

func (g billingHostGuard) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL == nil || r.URL.Scheme != "https" {
		return nil, &billingHostError{msg: "only https is allowed"}
	}
	if p := r.URL.Port(); p != "" && p != "443" {
		return nil, &billingHostError{msg: "only the default https port is allowed"}
	}
	if r.URL.User != nil || !g.allowed[strings.ToLower(r.URL.Hostname())] {
		return nil, &billingHostError{msg: "not one of the provider's API hosts"}
	}
	return g.next.RoundTrip(r)
}

// newBillingHTTPClient only talks to the given fixed hosts, over HTTPS, with
// no proxy and no redirects, dialing through the Helm catalog's public-address
// guard (so a poisoned resolver cannot point a provider host at the cluster
// network or the metadata service).
func newBillingHTTPClient(hosts ...string) *http.Client {
	allowed := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		allowed[strings.ToLower(h)] = true
	}
	guard := &HelmFetcher{Policy: HelmFetchPolicy{}}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           guard.dialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: billingRequestTimeout,
		MaxIdleConns:          4,
		IdleConnTimeout:       60 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Transport: billingHostGuard{allowed: allowed, next: tr},
		Timeout:   billingRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return &billingHostError{msg: "redirects are not followed"}
		},
	}
}
