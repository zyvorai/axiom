package platform

import (
	"context"
	"fmt"
)

// Optional capabilities that the concrete providers implement but that are found by a type
// assertion, not through the Provider interface. Kubernetes mode always serves requests through
// a SwitchableProvider, so each one needs a forwarder here or it reports "not available" on every
// real cluster (TestSwitchableProviderForwardsEveryOptionalCapability guards this).

func (sp *SwitchableProvider) ListExternalSecretsHealth(ctx context.Context, ns string) (ExternalSecretsHealthReport, error) {
	type lister interface {
		ListExternalSecretsHealth(context.Context, string) (ExternalSecretsHealthReport, error)
	}
	if l, ok := sp.current(ctx).(lister); ok {
		return l.ListExternalSecretsHealth(ctx, ns)
	}
	return emptyESHealth("External Secrets view not available on this provider"), nil
}

func (sp *SwitchableProvider) ListValidatingAdmissionPolicies(ctx context.Context) (VAPReport, error) {
	type lister interface {
		ListValidatingAdmissionPolicies(context.Context) (VAPReport, error)
	}
	if l, ok := sp.current(ctx).(lister); ok {
		return l.ListValidatingAdmissionPolicies(ctx)
	}
	return VAPReport{Available: false, Note: "not available", Policies: []VAPSummary{}}, nil
}

func (sp *SwitchableProvider) SealedWorkloads(ctx context.Context, ns string) (map[string]bool, error) {
	if l, ok := sp.current(ctx).(SealedWorkloadsInNamespace); ok {
		return l.SealedWorkloads(ctx, ns)
	}
	return nil, fmt.Errorf("sealed workload lookup is not available on this provider")
}

func (sp *SwitchableProvider) WorkloadVolumes(ctx context.Context, ns, workload string) ([]string, error) {
	if l, ok := sp.current(ctx).(VolumeLister); ok {
		return l.WorkloadVolumes(ctx, ns, workload)
	}
	return nil, fmt.Errorf("workload volume lookup is not available on this provider")
}
