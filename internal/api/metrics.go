package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// metricsRegistry is a minimal in-process Prometheus counter/summary store
// for Axiom's own process — separate from every other endpoint in this
// file, which reports on the *cluster* Axiom manages. A customer running
// Axiom as real infrastructure needs to monitor the tool itself too.
//
// Deliberately hand-rolled instead of pulling in prometheus/client_golang:
// this keeps the dependency-free demo build dependency-free, and the
// exposition format needed here is a handful of counters, not histograms.
type metricsRegistry struct {
	mu       sync.Mutex
	requests map[metricKey]*metricValue
}

type metricKey struct {
	method string
	route  string
	code   int
}

type metricValue struct {
	count       int64
	durationSum float64 // seconds
}

var globalMetrics = &metricsRegistry{requests: map[metricKey]*metricValue{}}

// buildVersion is overridden at build time via
// -ldflags "-X github.com/zyvorai/axiom/internal/api.buildVersion=v1.2.3"
// (see Dockerfile). Defaults to "dev" for local/unversioned builds.
var buildVersion = "dev"

// routeLabel collapses a request path to a low-cardinality label — e.g.
// "/api/v1/pods/default/checkout-01/logs" becomes "/api/v1/pods" — so a
// cluster with thousands of Pods doesn't produce thousands of distinct
// metric series.
func routeLabel(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	n := 3
	if len(segments) < n {
		n = len(segments)
	}
	return "/" + strings.Join(segments[:n], "/")
}

func (m *metricsRegistry) record(method, path string, code int, durationSeconds float64) {
	key := metricKey{method: method, route: routeLabel(path), code: code}
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.requests[key]
	if v == nil {
		v = &metricValue{}
		m.requests[key] = v
	}
	v.count++
	v.durationSum += durationSeconds
}

func (m *metricsRegistry) writeTo(w *strings.Builder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.WriteString("# HELP axiom_http_requests_total Total HTTP requests handled by this Axiom instance.\n")
	w.WriteString("# TYPE axiom_http_requests_total counter\n")
	for k, v := range m.requests {
		fmt.Fprintf(w, "axiom_http_requests_total{method=%q,route=%q,code=%q} %d\n", k.method, k.route, strconv.Itoa(k.code), v.count)
	}
	w.WriteString("# HELP axiom_http_request_duration_seconds_sum Cumulative request duration in seconds.\n")
	w.WriteString("# TYPE axiom_http_request_duration_seconds_sum counter\n")
	for k, v := range m.requests {
		fmt.Fprintf(w, "axiom_http_request_duration_seconds_sum{method=%q,route=%q,code=%q} %g\n", k.method, k.route, strconv.Itoa(k.code), v.durationSum)
	}
}

// statusRecorder wraps http.ResponseWriter to capture the status code for
// metrics — the stdlib interface has no way to read it back otherwise.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("# HELP axiom_build_info Static build/runtime info, value is always 1.\n")
	b.WriteString("# TYPE axiom_build_info gauge\n")
	fmt.Fprintf(&b, "axiom_build_info{mode=%q,version=%q} 1\n", s.mode, buildVersion)
	globalMetrics.writeTo(&b)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}
