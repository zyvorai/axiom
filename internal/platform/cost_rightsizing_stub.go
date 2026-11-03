//go:build !kubernetes

package platform

// ConfigurePrometheus is a no-op in the demo build: the demo provider serves
// deterministic rightsizing data and never queries Prometheus.
func ConfigurePrometheus(provider Provider, c *PrometheusClient) bool { return false }
