package platform

import "context"

// demoRightsizing is fixed demo data covering over-, under- and correctly
// provisioned containers (and one with no requests), run through the real
// recommender so the numbers obey the same rules as a live cluster.
var demoRightsizing = []struct {
	c rightsizingContainer
	u containerUsage
}{
	{rightsizingContainer{Namespace: "payments", Kind: "Deployment", Workload: "payments-api", Container: "api", Replicas: 3, RequestCPUMilli: 2000, RequestMemBytes: 4096 * mib},
		containerUsage{CPUMilli: 350, MemBytes: 900 * mib, HasCPU: true, HasMem: true}},
	{rightsizingContainer{Namespace: "payments", Kind: "Deployment", Workload: "payments-api", Container: "envoy", Replicas: 3, RequestCPUMilli: 100, RequestMemBytes: 128 * mib},
		containerUsage{CPUMilli: 95, MemBytes: 60 * mib, HasCPU: true, HasMem: true}},
	{rightsizingContainer{Namespace: "commerce", Kind: "Deployment", Workload: "checkout", Container: "checkout", Replicas: 2, RequestCPUMilli: 1500, RequestMemBytes: 3072 * mib},
		containerUsage{CPUMilli: 1200, MemBytes: 2600 * mib, HasCPU: true, HasMem: true}},
	{rightsizingContainer{Namespace: "commerce", Kind: "Deployment", Workload: "cart", Container: "cart", Replicas: 2, RequestCPUMilli: 500, RequestMemBytes: 512 * mib},
		containerUsage{CPUMilli: 180, MemBytes: 300 * mib, HasCPU: true, HasMem: true}},
	{rightsizingContainer{Namespace: "data", Kind: "StatefulSet", Workload: "redis", Container: "redis", Replicas: 3, RequestCPUMilli: 1500, RequestMemBytes: 5461 * mib},
		containerUsage{CPUMilli: 300, MemBytes: 5100 * mib, HasCPU: true, HasMem: true}},
	{rightsizingContainer{Namespace: "observability", Kind: "Deployment", Workload: "otel-collector", Container: "otel-collector", Replicas: 2, RequestCPUMilli: 1000, RequestMemBytes: 4096 * mib},
		containerUsage{CPUMilli: 120, MemBytes: 700 * mib, HasCPU: true, HasMem: true}},
	{rightsizingContainer{Namespace: "observability", Kind: "DaemonSet", Workload: "node-exporter", Container: "node-exporter", Replicas: 4},
		containerUsage{CPUMilli: 15, MemBytes: 40 * mib, HasCPU: true, HasMem: true}},
}

// ListRightsizing serves the demo rightsizing report (metrics-snapshot
// semantics, deterministic numbers), with the requests a rightsizing apply
// wrote on this provider.
func (d *DemoProvider) ListRightsizing(ctx context.Context, namespace, window string) (RightsizingReport, error) {
	window, err := NormalizeRightsizingWindow(window)
	if err != nil {
		return RightsizingReport{}, err
	}
	src := snapshotUsageSource{}
	items := []RightsizingItem{}
	for _, row := range demoRightsizing {
		if namespace != "" && row.c.Namespace != namespace {
			continue
		}
		if it, ok := recommendContainer(d.demoRightsizingContainer(row.c), row.u, src.basis()); ok {
			items = append(items, it)
		}
	}
	items, sum := finishRightsizingItems(items)
	return RightsizingReport{
		Source: src.name(), Window: window, Confidence: src.confidence(window),
		Note:    "Demo data. " + src.note(window),
		Summary: sum, Items: items,
	}, nil
}
