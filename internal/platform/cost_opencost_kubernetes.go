//go:build kubernetes

package platform

import "context"

// ConfigureOpenCost attaches c to the Kubernetes provider of the cluster
// Axiom starts on. One OpenCost reports one cluster, so clusters registered
// later keep the request-based estimate. A nil c is a no-op.
func ConfigureOpenCost(provider Provider, c *OpenCostClient) bool {
	if c == nil {
		return false
	}
	k, ok := CurrentKubernetesProvider(context.Background(), provider)
	if !ok || k == nil {
		return false
	}
	k.opencost = c
	return true
}

func (k *KubernetesProvider) openCostSource() costAllocationSource {
	if k.opencost == nil {
		return nil
	}
	return k.opencost
}
