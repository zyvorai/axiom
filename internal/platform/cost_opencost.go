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
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Actual cost from OpenCost (opencost.io, also the engine inside Kubecost).
//
// GET /cost prices resource requests at flat rates unless an OpenCost
// allocation API is configured (AXIOM_OPENCOST_URL). Then ActiveCost reads
// one accumulated allocation set per window, aggregated by namespace,
// controller kind and controller, scales it to a 30-day month and returns it
// in the same CostEstimate shape with source "opencost". Requests
// (cpuCores / memoryGi) still come from the cluster, so the table keeps its
// columns. Any OpenCost failure falls back to the estimate with a note; the
// cost page never fails because OpenCost does. Rightsizing savings keep the
// request-rate pricing (they price a change of requests, not a bill).

const (
	openCostQueryTimeout = 10 * time.Second
	openCostMaxBody      = 8 << 20
	// openCostCacheTTL is how long a successful answer is reused;
	// openCostErrorTTL how long a failure is (so a dead OpenCost is not
	// asked again on every page load, each costing up to the timeout).
	openCostCacheTTL = 60 * time.Second
	openCostErrorTTL = 15 * time.Second
	// openCostMaxRows bounds the allocation rows one answer may carry.
	openCostMaxRows = 20000
	// costMaxRowUSD clamps absurd values (overflow, unit mistakes).
	costMaxRowUSD = 1e12

	costMonthMinutes = 30 * 24 * 60

	// CostDefaultWindow is the window GET /cost reads actual cost over.
	CostDefaultWindow = "30d"
)

// costWindows are the windows GET /cost accepts, in days.
var costWindows = map[string]int{"7d": 7, "14d": 14, "30d": 30}

// NormalizeCostWindow validates ?window= for GET /cost ("" = 30d).
func NormalizeCostWindow(w string) (string, error) {
	w = strings.TrimSpace(w)
	if w == "" {
		return CostDefaultWindow, nil
	}
	if _, ok := costWindows[w]; !ok {
		return "", NewStatusError(http.StatusBadRequest, errors.New("window must be one of 7d, 14d, 30d"))
	}
	return w, nil
}

// ---------------------------------------------------------------------------
// Client

// OpenCostClient reads the OpenCost allocation API. Like PrometheusClient the
// URL is operator-configured, so in-cluster addresses are allowed; every call
// is bounded (10 s, 8 MiB), ignores proxy variables, refuses cross-host
// redirects and never puts the URL or token in an error.
type OpenCostClient struct {
	base     *url.URL
	token    string
	http     *http.Client
	timeout  time.Duration
	maxBytes int64
	now      func() time.Time

	mu    sync.Mutex
	cache map[string]*openCostCall
}

// openCostCall is one cached (or in-flight) fetch for a window.
type openCostCall struct {
	done chan struct{}
	at   time.Time
	set  openCostAllocations
	err  error
}

// NewOpenCostClient validates the base URL (http or https, a host, no
// userinfo). A nil client and nil error mean "not configured".
func NewOpenCostClient(rawURL, token string) (*OpenCostClient, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("AXIOM_OPENCOST_URL is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("AXIOM_OPENCOST_URL must use http or https")
	}
	if u.Host == "" || u.Hostname() == "" {
		return nil, errors.New("AXIOM_OPENCOST_URL must include a host")
	}
	if u.User != nil {
		return nil, errors.New("AXIOM_OPENCOST_URL must not embed credentials; use AXIOM_OPENCOST_TOKEN")
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return &OpenCostClient{
		base: u, token: strings.TrimSpace(token),
		http:    newOperatorHTTPClient(u, openCostQueryTimeout),
		timeout: openCostQueryTimeout, maxBytes: openCostMaxBody,
		now: time.Now, cache: map[string]*openCostCall{},
	}, nil
}

// openCostError is an OpenCost failure safe to show: never the URL, the
// token or the response body.
type openCostError struct{ msg string }

func (e *openCostError) Error() string { return e.msg }

func openCostErrorf(format string, a ...any) error {
	return &openCostError{msg: fmt.Sprintf(format, a...)}
}

// allocations returns the window's allocation set, from a 60 s cache. One
// fetch per window runs at a time; concurrent callers wait for it.
func (c *OpenCostClient) allocations(ctx context.Context, window string) (openCostAllocations, error) {
	c.mu.Lock()
	if e := c.cache[window]; e != nil {
		select {
		case <-e.done:
			ttl := openCostCacheTTL
			if e.err != nil {
				ttl = openCostErrorTTL
			}
			if c.now().Sub(e.at) < ttl {
				c.mu.Unlock()
				return e.set, e.err
			}
		default:
			c.mu.Unlock()
			select {
			case <-e.done:
				return e.set, e.err
			case <-ctx.Done():
				return openCostAllocations{}, openCostErrorf("the OpenCost request was cancelled")
			}
		}
	}
	e := &openCostCall{done: make(chan struct{})}
	c.cache[window] = e
	c.mu.Unlock()

	// The fetch outlives one caller's cancellation: others may be waiting.
	set, err := c.fetch(context.WithoutCancel(ctx), window)
	c.mu.Lock()
	e.set, e.err, e.at = set, err, c.now()
	c.mu.Unlock()
	close(e.done)
	return set, err
}

// fetch runs one allocation query.
func (c *OpenCostClient) fetch(ctx context.Context, window string) (openCostAllocations, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	u := *c.base
	u.Path = c.base.Path + "/allocation/compute"
	u.RawQuery = url.Values{
		"window":     {window},
		"aggregate":  {"namespace,controllerKind,controller"},
		"accumulate": {"true"},
		"shareIdle":  {"true"},
		"includeIdle": {"true"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return openCostAllocations{}, openCostErrorf("could not build the OpenCost request")
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
			return openCostAllocations{}, openCostErrorf("the OpenCost request timed out after %s", c.timeout)
		case errors.Is(err, context.Canceled):
			return openCostAllocations{}, openCostErrorf("the OpenCost request was cancelled")
		case strings.Contains(err.Error(), "redirect to another host refused"):
			return openCostAllocations{}, openCostErrorf("OpenCost redirected to another host; refused")
		case strings.Contains(err.Error(), "too many redirects"):
			return openCostAllocations{}, openCostErrorf("OpenCost redirected too many times")
		}
		return openCostAllocations{}, openCostErrorf("OpenCost is unreachable")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return openCostAllocations{}, openCostErrorf("the OpenCost request timed out after %s", c.timeout)
		}
		return openCostAllocations{}, openCostErrorf("reading the OpenCost response failed")
	}
	if int64(len(body)) > c.maxBytes {
		return openCostAllocations{}, openCostErrorf("the OpenCost response exceeds %d MiB", c.maxBytes>>20)
	}
	if resp.StatusCode != http.StatusOK {
		return openCostAllocations{}, openCostErrorf("OpenCost returned HTTP %d", resp.StatusCode)
	}
	return parseOpenCostAllocations(body, window)
}

// ---------------------------------------------------------------------------
// Parsing

// openCostRow is one allocation reduced to what Axiom shows. Costs are for
// the window, not yet scaled.
type openCostRow struct {
	Namespace, Kind, Controller string
	CPU, Memory, Storage        float64
	Network, GPU, Other, Total  float64
	Minutes                     float64
}

// openCostAllocations is a parsed allocation set.
type openCostAllocations struct {
	Rows []openCostRow
	// Idle and Unallocated are the special buckets (window cost).
	Idle, Unallocated float64
	// Minutes is how much of the window the data covers (0 = unknown).
	Minutes float64
}

// ocNum is a tolerant JSON number: null, strings ("1.5", "NaN"), overflow
// and non-finite values all decode to 0 instead of failing the row.
type ocNum float64

func (n *ocNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		*n = 0
		return nil
	}
	*n = ocNum(v)
	return nil
}

type openCostAllocation struct {
	Name       string                     `json:"name"`
	Properties map[string]json.RawMessage `json:"properties"`
	Start      string                     `json:"start"`
	End        string                     `json:"end"`
	Minutes    ocNum                      `json:"minutes"`

	CPUCost             ocNum `json:"cpuCost"`
	CPUCostAdjustment   ocNum `json:"cpuCostAdjustment"`
	GPUCost             ocNum `json:"gpuCost"`
	GPUCostAdjustment   ocNum `json:"gpuCostAdjustment"`
	RAMCost             ocNum `json:"ramCost"`
	RAMCostAdjustment   ocNum `json:"ramCostAdjustment"`
	PVCost              ocNum `json:"pvCost"`
	PVCostAdjustment    ocNum `json:"pvCostAdjustment"`
	NetworkCost         ocNum `json:"networkCost"`
	NetworkCostAdjust   ocNum `json:"networkCostAdjustment"`
	LoadBalancerCost    ocNum `json:"loadBalancerCost"`
	LoadBalancerCostAdj ocNum `json:"loadBalancerCostAdjustment"`
	SharedCost          ocNum `json:"sharedCost"`
	ExternalCost        ocNum `json:"externalCost"`
	TotalCost           ocNum `json:"totalCost"`
}

func (a openCostAllocation) prop(k string) string {
	raw, ok := a.Properties[k]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// parseOpenCostAllocations decodes {"code":200,"data":[{name: allocation}]}.
// Unknown fields are ignored, missing numbers are 0, a malformed row is
// skipped; a non-200 code or an answer with no rows at all is an error.
func parseOpenCostAllocations(body []byte, window string) (openCostAllocations, error) {
	var env struct {
		Code *int              `json:"code"`
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return openCostAllocations{}, openCostErrorf("OpenCost returned a malformed response")
	}
	if env.Code != nil && *env.Code != http.StatusOK {
		return openCostAllocations{}, openCostErrorf("OpenCost returned code %d", *env.Code)
	}
	var out openCostAllocations
	var earliest, latest time.Time
	n := 0
	for _, raw := range env.Data {
		var set map[string]json.RawMessage
		if json.Unmarshal(raw, &set) != nil || set == nil {
			continue
		}
		keys := make([]string, 0, len(set))
		for k := range set {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			var a openCostAllocation
			if json.Unmarshal(set[key], &a) != nil {
				continue
			}
			if n++; n > openCostMaxRows {
				return openCostAllocations{}, openCostErrorf("OpenCost returned more than %d allocations", openCostMaxRows)
			}
			if a.Name == "" {
				a.Name = key
			}
			row := openCostRowFrom(a)
			if m := float64(a.Minutes); m > out.Minutes {
				out.Minutes = m
			}
			if s, err := time.Parse(time.RFC3339, a.Start); err == nil && (earliest.IsZero() || s.Before(earliest)) {
				earliest = s
			}
			if e, err := time.Parse(time.RFC3339, a.End); err == nil && e.After(latest) {
				latest = e
			}
			switch openCostBucket(a.Name, row.Namespace) {
			case "idle":
				out.Idle += row.Total
			case "unallocated":
				out.Unallocated += row.Total
			default:
				out.Rows = append(out.Rows, row)
			}
		}
	}
	if n == 0 {
		return openCostAllocations{}, openCostErrorf("OpenCost returned no allocation data")
	}
	if out.Minutes <= 0 && !earliest.IsZero() && latest.After(earliest) {
		out.Minutes = latest.Sub(earliest).Minutes()
	}
	if days, ok := costWindows[window]; ok && out.Minutes > float64(days*24*60) {
		out.Minutes = float64(days * 24 * 60)
	}
	return out, nil
}

// openCostRowFrom reads namespace / controller from properties (falling back
// to the "namespace/kind/kind:name" aggregate key) and sums the costs.
func openCostRowFrom(a openCostAllocation) openCostRow {
	parts := strings.Split(a.Name, "/")
	part := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	r := openCostRow{
		Namespace:  a.prop("namespace"),
		Kind:       a.prop("controllerKind"),
		Controller: a.prop("controller"),
		Minutes:    float64(a.Minutes),
	}
	if r.Namespace == "" {
		r.Namespace = part(0)
	}
	if r.Kind == "" {
		r.Kind = part(1)
	}
	if r.Controller == "" {
		r.Controller = part(2)
	}
	// OpenCost writes the controller aggregate as "kind:name".
	if i := strings.Index(r.Controller, ":"); i >= 0 {
		if r.Kind == "" || strings.EqualFold(r.Controller[:i], r.Kind) {
			if r.Kind == "" {
				r.Kind = r.Controller[:i]
			}
			r.Controller = r.Controller[i+1:]
		}
	}
	r.CPU = float64(a.CPUCost + a.CPUCostAdjustment)
	r.Memory = float64(a.RAMCost + a.RAMCostAdjustment)
	r.Storage = float64(a.PVCost + a.PVCostAdjustment)
	r.Network = float64(a.NetworkCost + a.NetworkCostAdjust)
	r.GPU = float64(a.GPUCost + a.GPUCostAdjustment)
	sum := r.CPU + r.Memory + r.Storage + r.Network + r.GPU +
		float64(a.LoadBalancerCost+a.LoadBalancerCostAdj+a.SharedCost+a.ExternalCost)
	r.Total = float64(a.TotalCost)
	if r.Total <= 0 {
		r.Total = sum
	}
	// Residual beyond named parts (LB, shared, external, future OpenCost kinds).
	r.Other = r.Total - (r.CPU + r.Memory + r.Storage + r.Network + r.GPU)
	if r.Other < 0 {
		r.Other = 0
	}
	return r
}

// openCostBucket classifies an allocation: "idle", "unallocated" (unmounted
// volumes, no namespace, a namespace that is not a valid name) or "" (a real
// namespace).
func openCostBucket(name, namespace string) string {
	for _, seg := range strings.Split(name, "/") {
		switch seg {
		case "__idle__":
			return "idle"
		case "__unmounted__":
			return "unallocated"
		}
	}
	if namespace == "" || namespace == "__unallocated__" || !costNamespaceRe.MatchString(namespace) || len(namespace) > 63 {
		return "unallocated"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Turning allocations into a CostEstimate

// openCostScale is the factor from window cost to a 30-day month.
func openCostScale(minutes float64, window string) float64 {
	if minutes <= 0 {
		minutes = float64(costWindows[window] * 24 * 60)
	}
	if minutes <= 0 {
		return 1
	}
	return costMonthMinutes / minutes
}

// costUSD scales, clamps and rounds one value.
func costUSD(v, scale float64) float64 {
	v *= scale
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > costMaxRowUSD || math.IsInf(v, 1) {
		v = costMaxRowUSD
	}
	return round2(v)
}

func usdPtr(v float64) *float64 { return &v }

type costParts struct{ cpu, mem, storage, network, gpu, other, total float64 }

func (p *costParts) add(r openCostRow) {
	p.cpu += r.CPU
	p.mem += r.Memory
	p.storage += r.Storage
	p.network += r.Network
	p.gpu += r.GPU
	p.other += r.Other
	p.total += r.Total
}

func (p costParts) breakdown(scale float64) CostBreakdown {
	return CostBreakdown{
		CPUUSD: usdPtr(costUSD(p.cpu, scale)), MemoryUSD: usdPtr(costUSD(p.mem, scale)),
		StorageUSD: usdPtr(costUSD(p.storage, scale)), NetworkUSD: usdPtr(costUSD(p.network, scale)),
		GPUUSD: usdPtr(costUSD(p.gpu, scale)), OtherUSD: usdPtr(costUSD(p.other, scale)),
	}
}

// openCostKind maps OpenCost's lowercase controller kinds to Kubernetes kinds.
func openCostKind(k string) string {
	switch strings.ToLower(k) {
	case "deployment":
		return "Deployment"
	case "statefulset":
		return "StatefulSet"
	case "daemonset":
		return "DaemonSet"
	case "replicaset":
		return "ReplicaSet"
	case "job":
		return "Job"
	case "cronjob":
		return "CronJob"
	case "rollout":
		return "Rollout"
	case "", "__unallocated__":
		return "Pod"
	}
	return k
}

// costUnownedWorkload names the row for pods without a controller.
const costUnownedWorkload = "(pods without a controller)"

// costFromAllocations builds the "opencost" CostEstimate. Requests come from
// est (the request-based estimate for the same cluster); a namespace the
// estimate knows but OpenCost has no cost for is listed at 0.
func costFromAllocations(est CostEstimate, set openCostAllocations, window string) CostEstimate {
	scale := openCostScale(set.Minutes, window)
	reqNS := map[string]NamespaceCost{}
	for _, n := range est.Namespaces {
		reqNS[n.Namespace] = n
	}
	reqWL := map[string]WorkloadCost{}
	for _, w := range est.Workloads {
		reqWL[w.Namespace+"/"+w.Workload] = w
	}
	nsParts := map[string]*costParts{}
	type wlKey struct{ ns, kind, name string }
	wlParts := map[wlKey]*costParts{}
	for _, r := range set.Rows {
		p := nsParts[r.Namespace]
		if p == nil {
			p = &costParts{}
			nsParts[r.Namespace] = p
		}
		p.add(r)
		k := wlKey{ns: r.Namespace, kind: openCostKind(r.Kind), name: r.Controller}
		if k.name == "" || k.name == "__unallocated__" {
			k.name, k.kind = costUnownedWorkload, "Pod"
		}
		if len(k.name) > 253 {
			k.name = k.name[:253]
		}
		wp := wlParts[k]
		if wp == nil {
			wp = &costParts{}
			wlParts[k] = wp
		}
		wp.add(r)
	}
	for ns := range reqNS {
		if nsParts[ns] == nil {
			nsParts[ns] = &costParts{}
		}
	}
	names := make([]string, 0, len(nsParts))
	for ns := range nsParts {
		names = append(names, ns)
	}
	sort.Strings(names)
	out := CostEstimate{
		CPURatePerCore: est.CPURatePerCore, MemoryRatePerGi: est.MemoryRatePerGi,
		Namespaces: make([]NamespaceCost, 0, len(names)), Workloads: make([]WorkloadCost, 0, len(wlParts)),
		Source: CostSourceOpenCost, Window: window,
		IdleMonthlyUSD:        costUSD(set.Idle, scale),
		UnallocatedMonthlyUSD: costUSD(set.Unallocated, scale),
	}
	var total float64
	for _, ns := range names {
		p := nsParts[ns]
		req := reqNS[ns]
		row := NamespaceCost{Namespace: ns, CPUCores: req.CPUCores, MemoryGi: req.MemoryGi,
			MonthlyUSD: costUSD(p.total, scale), CostBreakdown: p.breakdown(scale)}
		total += row.MonthlyUSD
		out.Namespaces = append(out.Namespaces, row)
	}
	keys := make([]wlKey, 0, len(wlParts))
	for k := range wlParts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ns != keys[j].ns {
			return keys[i].ns < keys[j].ns
		}
		if keys[i].name != keys[j].name {
			return keys[i].name < keys[j].name
		}
		return keys[i].kind < keys[j].kind
	})
	for _, k := range keys {
		p := wlParts[k]
		req := reqWL[k.ns+"/"+k.name]
		out.Workloads = append(out.Workloads, WorkloadCost{Namespace: k.ns, Workload: k.name, Kind: k.kind,
			CPUCores: req.CPUCores, MemoryGi: req.MemoryGi,
			MonthlyUSD: costUSD(p.total, scale), CostBreakdown: p.breakdown(scale)})
	}
	out.TotalMonthlyUSD = round2(total)
	days := costWindows[window]
	out.Note = fmt.Sprintf("Actual cost from OpenCost over the last %d days, scaled to a 30-day month. CPU and memory columns are current requests.", days)
	if set.Minutes > 0 && set.Minutes < float64(days*24*60)-60 {
		out.Note = fmt.Sprintf("Actual cost from OpenCost; it has %s of data for the requested %d days, scaled to a 30-day month. CPU and memory columns are current requests.",
			humanDuration(time.Duration(set.Minutes)*time.Minute), days)
	}
	return out
}

// ---------------------------------------------------------------------------
// Source selection

// costAllocationSource supplies an allocation set for a window.
type costAllocationSource interface {
	allocations(ctx context.Context, window string) (openCostAllocations, error)
}

// openCostCarrier is a provider that may carry an OpenCost source (the
// Kubernetes provider of the startup cluster; see ConfigureOpenCost).
type openCostCarrier interface {
	openCostSource() costAllocationSource
}

// costSourceFor finds the allocation source of the cluster ctx resolves to,
// or nil (estimate only).
func costSourceFor(ctx context.Context, p Provider) costAllocationSource {
	if sp, ok := p.(*SwitchableProvider); ok {
		p = sp.current(ctx)
	}
	for i := 0; i < 8 && p != nil; i++ {
		switch v := p.(type) {
		case *clusterLabeledProvider:
			p = v.Provider
		case *simulatedClusterProvider:
			p = v.Provider
		case *DemoProvider:
			if demoOpenCostEnabled() {
				return demoOpenCostSource{}
			}
			return nil
		case openCostCarrier:
			return v.openCostSource()
		default:
			return nil
		}
	}
	return nil
}

// ActiveCost is what GET /cost serves and budgets compare against: actual
// cost when the cluster has an OpenCost source and it answers, else the
// request-based estimate (with a note saying why, when OpenCost failed).
// window must already be normalised (NormalizeCostWindow).
func ActiveCost(ctx context.Context, provider Provider, window string) (CostEstimate, error) {
	if _, ok := costWindows[window]; !ok {
		window = CostDefaultWindow
	}
	est, err := provider.EstimateCost(ctx)
	if err != nil {
		return CostEstimate{}, err
	}
	src := costSourceFor(ctx, provider)
	if src == nil {
		est.Source, est.Window = CostSourceEstimate, window
		est.OpenCostConfigured = false
		return est, nil
	}
	est.OpenCostConfigured = true
	set, err := src.allocations(ctx, window)
	if err != nil {
		est.Source, est.Window = CostSourceEstimate, window
		est.OpenCostReachable = false
		est.OpenCostError = safeOpenCostReason(err)
		est.Note = "OpenCost is configured but could not be read (" + est.OpenCostError + "); showing the request-based estimate instead."
		return est, nil
	}
	out := costFromAllocations(est, set, window)
	out.OpenCostConfigured = true
	out.OpenCostReachable = true
	return out, nil
}

// safeOpenCostReason only ever surfaces our own sanitised messages.
func safeOpenCostReason(err error) string {
	var oe *openCostError
	if errors.As(err, &oe) {
		return oe.msg
	}
	return "unexpected error"
}

// ---------------------------------------------------------------------------
// Demo source (AXIOM_DEMO_OPENCOST=true)

func demoOpenCostEnabled() bool { return truthyEnv(os.Getenv("AXIOM_DEMO_OPENCOST")) }

func truthyEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// demoOpenCostSource returns fixed OpenCost-shaped actuals for the demo
// cluster: they differ from the demo estimate (payments 180, commerce 90,
// data 170, observability 80 USD) and include an idle bucket.
type demoOpenCostSource struct{}

// demoOpenCostMonthly is the demo's actual monthly cost per workload:
// namespace, kind, controller, cpu, ram, pv, network, other.
var demoOpenCostMonthly = []struct {
	ns, kind, name               string
	cpu, ram, pv, network, other float64
}{
	{"payments", "deployment", "payments-api", 92.40, 38.10, 12.00, 4.50, 0},
	{"commerce", "deployment", "checkout", 61.20, 27.30, 0, 6.75, 9.00},
	{"data", "statefulset", "redis", 48.60, 71.40, 36.00, 1.20, 0},
	{"observability", "deployment", "otel-collector", 22.80, 19.20, 8.00, 2.10, 0},
	{"observability", "", "__unallocated__", 3.10, 1.90, 0, 0, 0},
}

const demoOpenCostIdleMonthly = 57.35

func (demoOpenCostSource) allocations(_ context.Context, window string) (openCostAllocations, error) {
	days, ok := costWindows[window]
	if !ok {
		days = 30
	}
	f := float64(days) / 30
	out := openCostAllocations{Minutes: float64(days * 24 * 60), Idle: demoOpenCostIdleMonthly * f}
	for _, d := range demoOpenCostMonthly {
		r := openCostRow{Namespace: d.ns, Kind: d.kind, Controller: d.name,
			CPU: d.cpu * f, Memory: d.ram * f, Storage: d.pv * f, Network: d.network * f, Other: d.other * f, Minutes: out.Minutes}
		r.Total = r.CPU + r.Memory + r.Storage + r.Network + r.Other
		out.Rows = append(out.Rows, r)
	}
	return out, nil
}
