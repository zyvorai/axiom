package platform

import "context"

// clusterLabeledProvider overrides Overview.Cluster on an otherwise
// unmodified Provider, so that multiple demo clusters registered on a
// SwitchableProvider each report a distinct name in the UI without needing
// any changes to DemoProvider itself.
type clusterLabeledProvider struct {
	Provider
	label string
}

func (c *clusterLabeledProvider) Overview(ctx context.Context) (Overview, error) {
	ov, err := c.Provider.Overview(ctx)
	if err != nil {
		return ov, err
	}
	ov.Cluster = c.label
	return ov, nil
}
