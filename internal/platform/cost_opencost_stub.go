//go:build !kubernetes

package platform

// ConfigureOpenCost is a no-op in the demo build: the demo provider serves
// its own OpenCost-shaped data when AXIOM_DEMO_OPENCOST=true.
func ConfigureOpenCost(provider Provider, c *OpenCostClient) bool { return false }
