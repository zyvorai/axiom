//go:build !kubernetes

package platform

// ConfigurePrometheus is a no-op in the demo build: the demo provider serves
// deterministic rightsizing data and never queries Prometheus.
func ConfigurePrometheus(provider Provider, c *PrometheusClient) bool { return false }

// PrometheusFromProvider is always nil in the demo build.
func PrometheusFromProvider(provider Provider) *PrometheusClient { return nil }
