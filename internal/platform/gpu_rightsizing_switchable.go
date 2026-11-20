package platform

import "context"

// ApplyGPURightsizing delegates to the active cluster provider when it supports GPU apply.
func (sp *SwitchableProvider) ApplyGPURightsizing(ctx context.Context, req GPURightsizingApplyRequest) (GPURightsizingApplyResult, error) {
	type applier interface {
		ApplyGPURightsizing(context.Context, GPURightsizingApplyRequest) (GPURightsizingApplyResult, error)
	}
	if a, ok := sp.current(ctx).(applier); ok {
		return a.ApplyGPURightsizing(ctx, req)
	}
	return GPURightsizingApplyResult{}, rightsizingBadRequest("GPU rightsizing is not available on the active provider")
}
