package platform

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
	for _, s := range specs {
		sp.Register(s.name, s.label, &clusterLabeledProvider{Provider: NewDemoProvider(), label: s.label}, "demo://"+s.name)
	}
	return sp
}
