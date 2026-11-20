package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Actual-bill reconciliation (EXPERIMENTAL).
//
// GET /cost shows what the cluster's workloads cost — priced requests or
// OpenCost allocation. This compares that with what the cloud provider
// actually invoiced for the current and the previous calendar month:
//
//   - AWS Cost Explorer GetCostAndUsage (UnblendedCost, MONTHLY), SigV4
//     signed by hand (billing_aws.go)
//   - Azure Cost Management query API (ActualCost, Monthly) with a
//     client-credentials token (billing_azure.go)
//   - GCP Cloud Billing via a fixed, parameterised BigQuery query over the
//     billing export table, with a service-account JWT token (billing_gcp.go)
//
// Each client is a few hundred lines of standard library — no cloud SDKs —
// and has only been exercised against httptest fixtures that mimic the
// documented request and response shapes, never against a real account.
// Hence "experimental" everywhere it surfaces.
//
// Outbound requests go only to each provider's fixed API host names (there is
// no configurable endpoint), over HTTPS, without proxies or redirects, through
// the same public-address dialer as the Helm catalog. Each request is bounded
// (20 s, 4 MiB); the invoice answer is cached for an hour (a failure for five
// minutes). Credentials are never logged and never part of an error: errors
// carry only our own text plus the provider's error code.
//
// "Allocated" is the current monthly run-rate from ActiveCost (OpenCost when
// configured, else the request-based estimate) prorated to the days the month
// covers so far — Axiom keeps no allocation history, so the previous month
// uses today's run-rate too. The difference is shown only when the invoice is
// in USD, the currency allocation is priced in.

const (
	BillingProviderAWS   = "aws"
	BillingProviderAzure = "azure"
	BillingProviderGCP   = "gcp"

	billingRequestTimeout = 20 * time.Second
	billingOverallTimeout = 45 * time.Second
	billingMaxBody        = 4 << 20
	billingCacheTTL       = time.Hour
	billingErrorTTL       = 5 * time.Minute
	billingMaxCache       = 16
	billingMaxAmount      = 1e12

	billingMaxSecretLen = 512
	billingMaxSAKeyLen  = 16 << 10
)

// billingAWSRegions maps the Cost Explorer regions we accept to their fixed
// endpoint host. Cost Explorer is a global service with one endpoint per
// partition.
var billingAWSRegions = map[string]string{
	"us-east-1":      "ce.us-east-1.amazonaws.com",
	"cn-northwest-1": "ce.cn-northwest-1.amazonaws.com.cn",
}

const billingDefaultAWSRegion = "us-east-1"

var (
	billingGUIDRe      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	billingTenantRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,62}(\.[A-Za-z0-9][A-Za-z0-9-]{0,62})*$`)
	billingAWSKeyIDRe  = regexp.MustCompile(`^[A-Z0-9]{16,128}$`)
	billingGCPProjRe   = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	billingGCPTableRe  = regexp.MustCompile(`^([a-z][a-z0-9-]{4,28}[a-z0-9])\.([A-Za-z0-9_]+)\.([A-Za-z0-9_]+)$`)
	billingErrorCodeRe = regexp.MustCompile(`[^A-Za-z0-9._:/-]`)
)

func isKnownBillingProvider(p string) bool {
	return p == BillingProviderAWS || p == BillingProviderAzure || p == BillingProviderGCP
}

// ---------------------------------------------------------------------------
// Settings (part of Integrations)

func (i Integrations) normalizeBilling() Integrations {
	i.BillingProvider = strings.ToLower(strings.TrimSpace(i.BillingProvider))
	if !isKnownBillingProvider(i.BillingProvider) {
		i.BillingProvider = ""
	}
	i.BillingTagKey = strings.TrimSpace(i.BillingTagKey)
	i.BillingTagValue = strings.TrimSpace(i.BillingTagValue)
	i.BillingAWSRegion = strings.ToLower(strings.TrimSpace(i.BillingAWSRegion))
	if _, ok := billingAWSRegions[i.BillingAWSRegion]; !ok {
		i.BillingAWSRegion = billingDefaultAWSRegion
	}
	i.BillingAWSAccessKeyID = strings.TrimSpace(i.BillingAWSAccessKeyID)
	i.BillingAWSSecretAccessKey = strings.TrimSpace(i.BillingAWSSecretAccessKey)
	i.BillingAWSSessionToken = strings.TrimSpace(i.BillingAWSSessionToken)
	i.BillingAzureTenantID = strings.TrimSpace(i.BillingAzureTenantID)
	i.BillingAzureClientID = strings.TrimSpace(i.BillingAzureClientID)
	i.BillingAzureSubscriptionID = strings.TrimSpace(i.BillingAzureSubscriptionID)
	i.BillingAzureClientSecret = strings.TrimSpace(i.BillingAzureClientSecret)
	i.BillingGCPProjectID = strings.TrimSpace(i.BillingGCPProjectID)
	i.BillingGCPTable = strings.TrimSpace(i.BillingGCPTable)
	i.BillingGCPServiceAccountKey = strings.TrimSpace(i.BillingGCPServiceAccountKey)
	return i
}

// billingConfigured reports whether the selected provider has everything it
// needs to make a call.
func (i Integrations) billingConfigured() bool {
	switch i.BillingProvider {
	case BillingProviderAWS:
		return i.BillingAWSAccessKeyID != "" && i.BillingAWSSecretAccessKey != ""
	case BillingProviderAzure:
		return i.BillingAzureTenantID != "" && i.BillingAzureClientID != "" &&
			i.BillingAzureSubscriptionID != "" && i.BillingAzureClientSecret != ""
	case BillingProviderGCP:
		return i.BillingGCPTable != "" && i.BillingGCPServiceAccountKey != ""
	}
	return false
}

func (i Integrations) billingView(v IntegrationsView) IntegrationsView {
	v.BillingProvider = i.BillingProvider
	v.BillingTagKey = i.BillingTagKey
	v.BillingTagValue = i.BillingTagValue
	v.BillingAWSRegion = i.BillingAWSRegion
	if v.BillingAWSRegion == "" {
		v.BillingAWSRegion = billingDefaultAWSRegion
	}
	v.BillingAWSAccessKeyIDSet = strings.TrimSpace(i.BillingAWSAccessKeyID) != ""
	v.BillingAWSSecretAccessKeySet = strings.TrimSpace(i.BillingAWSSecretAccessKey) != ""
	v.BillingAWSSessionTokenSet = strings.TrimSpace(i.BillingAWSSessionToken) != ""
	v.BillingAzureTenantID = i.BillingAzureTenantID
	v.BillingAzureClientID = i.BillingAzureClientID
	v.BillingAzureSubscriptionID = i.BillingAzureSubscriptionID
	v.BillingAzureClientSecretSet = strings.TrimSpace(i.BillingAzureClientSecret) != ""
	v.BillingGCPProjectID = i.BillingGCPProjectID
	v.BillingGCPTable = i.BillingGCPTable
	v.BillingGCPServiceAccountKeySet = strings.TrimSpace(i.BillingGCPServiceAccountKey) != ""
	v.BillingConfigured = i.billingConfigured()
	return v
}

func applyBillingSecret(cur string, val *string, clear bool) string {
	if clear {
		return ""
	}
	if val != nil && strings.TrimSpace(*val) != "" {
		return *val
	}
	return cur
}

func (i Integrations) applyBillingPatch(p IntegrationsPatch) Integrations {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&i.BillingProvider, p.BillingProvider)
	set(&i.BillingTagKey, p.BillingTagKey)
	set(&i.BillingTagValue, p.BillingTagValue)
	set(&i.BillingAWSRegion, p.BillingAWSRegion)
	set(&i.BillingAzureTenantID, p.BillingAzureTenantID)
	set(&i.BillingAzureClientID, p.BillingAzureClientID)
	set(&i.BillingAzureSubscriptionID, p.BillingAzureSubscriptionID)
	set(&i.BillingGCPProjectID, p.BillingGCPProjectID)
	set(&i.BillingGCPTable, p.BillingGCPTable)
	i.BillingAWSAccessKeyID = applyBillingSecret(i.BillingAWSAccessKeyID, p.BillingAWSAccessKeyID, p.BillingAWSAccessKeyIDClear)
	i.BillingAWSSecretAccessKey = applyBillingSecret(i.BillingAWSSecretAccessKey, p.BillingAWSSecretAccessKey, p.BillingAWSSecretAccessKeyClear)
	i.BillingAWSSessionToken = applyBillingSecret(i.BillingAWSSessionToken, p.BillingAWSSessionToken, p.BillingAWSSessionTokenClear)
	i.BillingAzureClientSecret = applyBillingSecret(i.BillingAzureClientSecret, p.BillingAzureClientSecret, p.BillingAzureClientSecretClear)
	i.BillingGCPServiceAccountKey = applyBillingSecret(i.BillingGCPServiceAccountKey, p.BillingGCPServiceAccountKey, p.BillingGCPServiceAccountKeyClear)
	return i
}

func billingTrimmed(v *string) (string, bool) {
	if v == nil {
		return "", false
	}
	s := strings.TrimSpace(*v)
	return s, s != ""
}

func billingPlainText(s string, max int) bool {
	if len(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// ValidateBillingPatch rejects malformed billing settings before they are
// stored. Error messages never echo a credential.
func ValidateBillingPatch(p IntegrationsPatch) error {
	if v, ok := billingTrimmed(p.BillingProvider); ok && !isKnownBillingProvider(strings.ToLower(v)) {
		return errors.New("billingProvider must be one of aws, azure, gcp (or empty to turn reconciliation off)")
	}
	if v, ok := billingTrimmed(p.BillingTagKey); ok && (!billingPlainText(v, 128) || strings.ContainsAny(v, `"'\`)) {
		return errors.New("billingTagKey must be at most 128 characters without quotes or control characters")
	}
	if v, ok := billingTrimmed(p.BillingTagValue); ok && (!billingPlainText(v, 256) || strings.ContainsAny(v, `"'\`)) {
		return errors.New("billingTagValue must be at most 256 characters without quotes or control characters")
	}
	if v, ok := billingTrimmed(p.BillingAWSRegion); ok {
		if _, known := billingAWSRegions[strings.ToLower(v)]; !known {
			return errors.New("billingAwsRegion must be us-east-1 (Cost Explorer's endpoint) or cn-northwest-1 (China)")
		}
	}
	if v, ok := billingTrimmed(p.BillingAWSAccessKeyID); ok && !billingAWSKeyIDRe.MatchString(v) {
		return errors.New("the AWS access key ID is not in the expected format")
	}
	if v, ok := billingTrimmed(p.BillingAWSSecretAccessKey); ok && (len(v) < 16 || !billingPlainText(v, 128) || strings.ContainsAny(v, " \t")) {
		return errors.New("the AWS secret access key is not in the expected format")
	}
	if v, ok := billingTrimmed(p.BillingAzureTenantID); ok && !billingGUIDRe.MatchString(v) && !billingTenantRe.MatchString(v) {
		return errors.New("billingAzureTenantId must be a tenant GUID or a domain such as contoso.onmicrosoft.com")
	}
	if v, ok := billingTrimmed(p.BillingAzureClientID); ok && !billingGUIDRe.MatchString(v) {
		return errors.New("billingAzureClientId must be an application (client) GUID")
	}
	if v, ok := billingTrimmed(p.BillingAzureSubscriptionID); ok && !billingGUIDRe.MatchString(v) {
		return errors.New("billingAzureSubscriptionId must be a subscription GUID")
	}
	if v, ok := billingTrimmed(p.BillingAzureClientSecret); ok && !billingPlainText(v, billingMaxSecretLen) {
		return errors.New("the Azure client secret is not in the expected format")
	}
	if v, ok := billingTrimmed(p.BillingGCPProjectID); ok && !billingGCPProjRe.MatchString(v) {
		return errors.New("billingGcpProjectId must be a Google Cloud project ID (6-30 lowercase letters, digits or hyphens)")
	}
	if v, ok := billingTrimmed(p.BillingGCPTable); ok {
		if _, err := parseBillingGCPTable(v); err != nil {
			return err
		}
	}
	if v, ok := billingTrimmed(p.BillingGCPServiceAccountKey); ok {
		if _, err := parseGCPServiceAccountKey([]byte(v)); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Report

// BillingServiceLine is one top service/SKU slice of an experimental invoice.
type BillingServiceLine struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

// BillingMonth is one calendar month of the reconciliation.
type BillingMonth struct {
	Month        string  `json:"month"` // YYYY-MM
	InvoiceTotal float64 `json:"invoiceTotal"`
	Allocated    float64 `json:"allocated"`
	// Unallocated is invoiceTotal - allocated; nil when the two are in
	// different currencies or the allocation could not be read.
	Unallocated *float64 `json:"unallocated"`
	AsOf        string   `json:"asOf"`
	// Partial marks the month in progress (month-to-date).
	Partial bool `json:"partial"`
	// Services is an optional top-N service breakdown (experimental).
	Services []BillingServiceLine `json:"services,omitempty"`
}

// BillingReport is GET /api/v1/cost/invoice.
type BillingReport struct {
	Provider          string         `json:"provider"`
	Configured        bool           `json:"configured"`
	Currency          string         `json:"currency"`
	AllocatedCurrency string         `json:"allocatedCurrency"`
	AllocatedSource   string         `json:"allocatedSource,omitempty"`
	Months            []BillingMonth `json:"months"`
	Experimental      bool           `json:"experimental"`
	Demo              bool           `json:"demo,omitempty"`
	Error             string         `json:"error,omitempty"`
	Note              string         `json:"note,omitempty"`
}

// billingPeriod is one month to ask for: [Start, End) in UTC.
type billingPeriod struct {
	Month      string // YYYY-MM
	Start, End time.Time
}

// billingMonthTotal is what a provider client returns per month.
type billingMonthTotal struct {
	Month    string
	Amount   float64
	Currency string
	Services []BillingServiceLine
}

// billingFetcher is one provider client.
type billingFetcher interface {
	fetch(ctx context.Context, periods []billingPeriod, now time.Time) ([]billingMonthTotal, error)
}

// billingError is a failure safe to show: our own text plus, at most, the
// provider's error code. Never a URL, a credential or a response body.
type billingError struct{ msg string }

func (e *billingError) Error() string { return e.msg }

func billingErrorf(format string, a ...any) error {
	return &billingError{msg: fmt.Sprintf(format, a...)}
}

func safeBillingReason(err error) string {
	var be *billingError
	if errors.As(err, &be) {
		return be.msg
	}
	return "unexpected error"
}

// sanitizeBillingCode keeps a provider error code short and inert.
func sanitizeBillingCode(code string) string {
	code = billingErrorCodeRe.ReplaceAllString(strings.TrimSpace(code), "")
	if len(code) > 80 {
		code = code[:80]
	}
	return code
}

// billingPeriods returns the previous and the current month, the current one
// ending tomorrow (UTC) so today's charges are included.
func billingPeriods(now time.Time) []billingPeriod {
	now = now.UTC()
	cur := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	prev := cur.AddDate(0, -1, 0)
	tomorrow := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	return []billingPeriod{
		{Month: prev.Format("2006-01"), Start: prev, End: cur},
		{Month: cur.Format("2006-01"), Start: cur, End: tomorrow},
	}
}

func roundCents(v float64) float64 { return math.Round(v*100) / 100 }

// ---------------------------------------------------------------------------
// Service (cache + assembly)

type billingCall struct {
	done   chan struct{}
	at     time.Time
	totals []billingMonthTotal
	err    error
}

// BillingService caches invoice answers and assembles the report. The zero
// value is not usable; call NewBillingService.
type BillingService struct {
	mu    sync.Mutex
	cache map[string]*billingCall
	now   func() time.Time
	// newFetcher builds the provider client (tests swap it for one pointed
	// at an httptest server).
	newFetcher func(Integrations) (billingFetcher, error)
}

func NewBillingService() *BillingService {
	return &BillingService{cache: map[string]*billingCall{}, now: time.Now, newFetcher: newBillingFetcher}
}

func newBillingFetcher(cfg Integrations) (billingFetcher, error) {
	switch cfg.BillingProvider {
	case BillingProviderAWS:
		return newAWSBillingClient(cfg, nil)
	case BillingProviderAzure:
		return newAzureBillingClient(cfg, nil)
	case BillingProviderGCP:
		return newGCPBillingClient(cfg, nil)
	}
	return nil, billingErrorf("no billing provider is selected")
}

// billingFingerprint identifies a configuration (credentials included, hashed)
// so a settings change never serves the previous account's numbers.
func billingFingerprint(cfg Integrations, month string) string {
	h := sha256.New()
	for _, s := range []string{
		cfg.BillingProvider, cfg.BillingTagKey, cfg.BillingTagValue, cfg.BillingAWSRegion,
		cfg.BillingAWSAccessKeyID, cfg.BillingAWSSecretAccessKey, cfg.BillingAWSSessionToken,
		cfg.BillingAzureTenantID, cfg.BillingAzureClientID, cfg.BillingAzureSubscriptionID, cfg.BillingAzureClientSecret,
		cfg.BillingGCPProjectID, cfg.BillingGCPTable, cfg.BillingGCPServiceAccountKey, month,
	} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// invoices returns the provider's monthly totals, from the cache when fresh.
// Concurrent callers share one in-flight fetch.
func (s *BillingService) invoices(ctx context.Context, cfg Integrations) ([]billingMonthTotal, time.Time, error) {
	now := s.now()
	periods := billingPeriods(now)
	key := billingFingerprint(cfg, periods[1].Month)
	s.mu.Lock()
	if e := s.cache[key]; e != nil {
		select {
		case <-e.done:
			ttl := billingCacheTTL
			if e.err != nil {
				ttl = billingErrorTTL
			}
			if now.Sub(e.at) < ttl {
				s.mu.Unlock()
				return e.totals, e.at, e.err
			}
		default:
			s.mu.Unlock()
			select {
			case <-e.done:
				return e.totals, e.at, e.err
			case <-ctx.Done():
				return nil, time.Time{}, billingErrorf("the billing request was cancelled")
			}
		}
	}
	if len(s.cache) >= billingMaxCache {
		for k, e := range s.cache {
			select {
			case <-e.done:
				delete(s.cache, k)
			default:
			}
		}
	}
	e := &billingCall{done: make(chan struct{})}
	s.cache[key] = e
	s.mu.Unlock()

	totals, err := s.fetchNow(context.WithoutCancel(ctx), cfg, periods, now)
	s.mu.Lock()
	e.totals, e.err, e.at = totals, err, s.now()
	s.mu.Unlock()
	close(e.done)
	return totals, e.at, err
}

func (s *BillingService) fetchNow(ctx context.Context, cfg Integrations, periods []billingPeriod, now time.Time) ([]billingMonthTotal, error) {
	f, err := s.newFetcher(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, billingOverallTimeout)
	defer cancel()
	totals, err := f.fetch(ctx, periods, now)
	if err != nil {
		return nil, err
	}
	return validateBillingTotals(totals)
}

// validateBillingTotals rejects absurd values and mixed currencies.
func validateBillingTotals(in []billingMonthTotal) ([]billingMonthTotal, error) {
	cur := ""
	for _, t := range in {
		if math.IsNaN(t.Amount) || math.IsInf(t.Amount, 0) || math.Abs(t.Amount) > billingMaxAmount {
			return nil, billingErrorf("the provider returned an implausible amount")
		}
		c := strings.ToUpper(strings.TrimSpace(t.Currency))
		if cur == "" {
			cur = c
		} else if c != "" && c != cur {
			return nil, billingErrorf("the invoice mixes currencies (%s and %s); reconciliation needs one", sanitizeBillingCode(cur), sanitizeBillingCode(c))
		}
	}
	return in, nil
}

// Report builds GET /cost/invoice. It never fails: errors go in .Error.
func (s *BillingService) Report(ctx context.Context, cfg Integrations, provider Provider, demo bool) BillingReport {
	now := s.now().UTC()
	if demo {
		return demoBillingReport(now)
	}
	rep := BillingReport{Provider: cfg.BillingProvider, Configured: cfg.billingConfigured(), AllocatedCurrency: "USD", Experimental: true, Months: []BillingMonth{}}
	if !rep.Configured {
		return rep
	}
	totals, asOf, err := s.invoices(ctx, cfg)
	if err != nil {
		rep.Error = safeBillingReason(err)
		return rep
	}
	rate, src, allocErr := billingAllocationRate(ctx, provider)
	rep.AllocatedSource = src
	rep.Months, rep.Currency = assembleBillingMonths(billingPeriods(now), totals, rate, allocErr == nil, now, asOf)
	switch {
	case allocErr != nil:
		rep.Note = "the allocated cost could not be read, so no difference is shown"
	case rep.Currency != "" && rep.Currency != "USD":
		rep.Note = "the invoice is in " + rep.Currency + " and allocation is priced in USD, so no difference is shown"
	}
	return rep
}

// billingAllocationRate is the cluster's current monthly allocated cost.
func billingAllocationRate(ctx context.Context, provider Provider) (float64, string, error) {
	if provider == nil {
		return 0, "", errors.New("no provider")
	}
	est, err := ActiveCost(ctx, provider, CostDefaultWindow)
	if err != nil {
		return 0, "", err
	}
	return est.TotalMonthlyUSD, est.Source, nil
}

func assembleBillingMonths(periods []billingPeriod, totals []billingMonthTotal, monthlyRate float64, haveRate bool, now, asOf time.Time) ([]BillingMonth, string) {
	byMonth := map[string]billingMonthTotal{}
	currency := ""
	for _, t := range totals {
		prev := byMonth[t.Month]
		prev.Month, prev.Amount = t.Month, prev.Amount+t.Amount
		if prev.Currency == "" {
			prev.Currency = strings.ToUpper(strings.TrimSpace(t.Currency))
		}
		if len(t.Services) > 0 {
			prev.Services = append(prev.Services, t.Services...)
		}
		byMonth[t.Month] = prev
		if currency == "" {
			currency = prev.Currency
		}
	}
	out := make([]BillingMonth, 0, len(periods))
	for i, p := range periods {
		partial := i == len(periods)-1
		days := p.End.Sub(p.Start).Hours() / 24
		if partial {
			days = now.Sub(p.Start).Hours() / 24
		}
		raw := byMonth[p.Month]
		m := BillingMonth{
			Month:        p.Month,
			InvoiceTotal: roundCents(raw.Amount),
			Allocated:    roundCents(monthlyRate * days / 30),
			AsOf:         asOf.UTC().Format(time.RFC3339),
			Partial:      partial,
			Services:     mergeBillingServices(raw.Services, 10),
		}
		if haveRate && (currency == "" || currency == "USD") {
			d := roundCents(m.InvoiceTotal - m.Allocated)
			m.Unallocated = &d
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Month < out[b].Month })
	if currency == "" {
		currency = "USD"
	}
	return out, currency
}

func mergeBillingServices(in []BillingServiceLine, n int) []BillingServiceLine {
	if len(in) == 0 {
		return nil
	}
	by := map[string]float64{}
	for _, s := range in {
		by[s.Name] += s.Amount
	}
	return topBillingServices(by, n)
}

// Test runs one uncached call with the saved settings for
// POST /integrations/billing/test.
func (s *BillingService) Test(ctx context.Context, cfg Integrations, demo bool) ConnectionTestResult {
	start := time.Now()
	if !cfg.billingConfigured() {
		if demo {
			return ConnectionTestResult{OK: true, Service: "billing", Hint: "Demo mode serves canned invoice data; nothing is contacted."}
		}
		return ConnectionTestResult{Service: "billing", Error: "billing reconciliation is not configured", Hint: "Choose a provider and enter its credentials, then save."}
	}
	now := s.now()
	totals, err := s.fetchNow(ctx, cfg, billingPeriods(now), now)
	if err != nil {
		return ConnectionTestResult{Service: "billing", Error: safeBillingReason(err), LatencyMS: time.Since(start).Milliseconds()}
	}
	// A successful test also refreshes the cache.
	s.mu.Lock()
	done := make(chan struct{})
	close(done)
	s.cache[billingFingerprint(cfg, billingPeriods(now)[1].Month)] = &billingCall{done: done, at: s.now(), totals: totals}
	s.mu.Unlock()
	months := make([]map[string]any, 0, len(totals))
	for _, t := range totals {
		months = append(months, map[string]any{"month": t.Month, "total": roundCents(t.Amount), "currency": t.Currency})
	}
	return ConnectionTestResult{
		OK: true, Service: "billing", LatencyMS: time.Since(start).Milliseconds(),
		Detail: map[string]any{"provider": cfg.BillingProvider, "months": months},
		Hint:   "Experimental: compare these totals with your invoice before relying on them.",
	}
}

// demoBillingReport is deterministic canned data for demo mode: fixed amounts
// for the previous and current month (relative to the clock).
func demoBillingReport(now time.Time) BillingReport {
	periods := billingPeriods(now)
	asOf := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	canned := []struct{ invoice, allocated float64 }{{4812.40, 3975.20}, {1603.75, 1288.10}}
	months := make([]BillingMonth, 0, len(periods))
	for i, p := range periods {
		d := roundCents(canned[i].invoice - canned[i].allocated)
		months = append(months, BillingMonth{
			Month: p.Month, InvoiceTotal: canned[i].invoice, Allocated: canned[i].allocated,
			Unallocated: &d, AsOf: asOf, Partial: i == len(periods)-1,
		})
	}
	return BillingReport{
		Provider: BillingProviderAWS, Configured: true, Currency: "USD", AllocatedCurrency: "USD",
		AllocatedSource: CostSourceEstimate, Months: months, Experimental: true, Demo: true,
		Note: "Demo data: canned figures, no cloud account is contacted.",
	}
}

// ---------------------------------------------------------------------------
// Shared HTTP helpers for the provider clients

// readBillingJSON reads a capped body and decodes it into out. On a non-2xx
// status it asks codeOf for the provider's error code and returns a
// billingError naming only that.
func readBillingJSON(resp *http.Response, what string, out any, codeOf func([]byte) string) error {
	defer resp.Body.Close()
	body, err := readCapped(resp.Body, billingMaxBody)
	if err != nil {
		if errors.Is(err, errBodyTooLarge) {
			return billingErrorf("the %s response is larger than %d MiB", what, billingMaxBody>>20)
		}
		return billingErrorf("could not read the %s response", what)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		code := ""
		if codeOf != nil {
			code = sanitizeBillingCode(codeOf(body))
		}
		if code != "" {
			return billingErrorf("%s answered HTTP %d (%s)", what, resp.StatusCode, code)
		}
		return billingErrorf("%s answered HTTP %d", what, resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return billingErrorf("the %s response is not the expected JSON", what)
	}
	return nil
}
