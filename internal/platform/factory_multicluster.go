package platform

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// NewDemoSwitchableProvider registers a handful of independent demo
// clusters so the Settings → Clusters switcher has real multi-cluster
// behavior to exercise even without a live kubeconfig. Each cluster gets
// its own DemoProvider instance, so create/delete/scale actions in one
// cluster never leak into another.
func NewDemoSwitchableProvider() *SwitchableProvider {
	sp := NewSwitchableProvider()
	specs := []struct{ name, label string }{
		{"eu-west-1", "zyvor-demo"},
		{"us-east-1", "zyvor-demo-us"},
		{"ap-south-1", "zyvor-demo-apac"},
	}
	down, degraded := demoFleetSet("AXIOM_DEMO_FLEET_DOWN"), demoFleetSet("AXIOM_DEMO_FLEET_DEGRADED")
	for _, s := range specs {
		var p Provider = &clusterLabeledProvider{Provider: NewDemoProvider(), label: s.label}
		switch {
		case down[s.name]:
			p = &simulatedClusterProvider{Provider: p, server: "demo://" + s.name, down: true}
		case degraded[s.name]:
			p = &simulatedClusterProvider{Provider: p, server: "demo://" + s.name}
		}
		sp.Register(s.name, s.label, p, "demo://"+s.name)
	}
	return sp
}

// demoFleetSet parses a comma-separated list of demo cluster names from an
// environment variable (a lab/test switch, never set in production).
func demoFleetSet(env string) map[string]bool {
	out := map[string]bool{}
	for _, n := range strings.Split(os.Getenv(env), ",") {
		if n = strings.TrimSpace(n); n != "" {
			out[n] = true
		}
	}
	return out
}

// simulatedClusterProvider makes a demo cluster look unreachable
// (AXIOM_DEMO_FLEET_DOWN=name,...) or degraded (AXIOM_DEMO_FLEET_DEGRADED=...)
// so fleet health can be exercised without a broken real cluster.
type simulatedClusterProvider struct {
	Provider
	server string
	down   bool
}

func (c *simulatedClusterProvider) Overview(ctx context.Context) (Overview, error) {
	if c.down {
		return Overview{}, fmt.Errorf("Get %q: dial tcp 10.0.0.5:6443: connect: connection refused (simulated)", "https://"+strings.TrimPrefix(c.server, "demo://")+"/version")
	}
	ov, err := c.Provider.Overview(ctx)
	if err != nil {
		return ov, err
	}
	if ov.ReadyNodes > 0 {
		ov.ReadyNodes--
	}
	return ov, nil
}
