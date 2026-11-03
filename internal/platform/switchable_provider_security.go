package platform

import "context"

// Delegating methods for the Workstream C security views. Kept out of
// switchable_provider_gen.go (whose generator script is not in the repo) so the
// generated file stays untouched; the shape is identical.

func (sp *SwitchableProvider) ListRBACSubjects(a0 context.Context) ([]RBACSubjectSummary, error) {
	return sp.current(a0).ListRBACSubjects(a0)
}
func (sp *SwitchableProvider) GetRBACSubject(a0 context.Context, a1 string, a2 string, a3 string) (RBACSubjectDetail, error) {
	return sp.current(a0).GetRBACSubject(a0, a1, a2, a3)
}
func (sp *SwitchableProvider) RBACWhoCan(a0 context.Context, a1 RBACWhoCanQuery) (RBACWhoCanResult, error) {
	return sp.current(a0).RBACWhoCan(a0, a1)
}
func (sp *SwitchableProvider) ListServiceAccounts(a0 context.Context, a1 string) ([]ServiceAccountSummary, error) {
	return sp.current(a0).ListServiceAccounts(a0, a1)
}
func (sp *SwitchableProvider) AnalyzeNetworkPolicies(a0 context.Context, a1 string) (NetworkPolicyAnalysis, error) {
	return sp.current(a0).AnalyzeNetworkPolicies(a0, a1)
}
func (sp *SwitchableProvider) ListSecurityReports(a0 context.Context, a1 string) (SecurityReportSummary, error) {
	return sp.current(a0).ListSecurityReports(a0, a1)
}
func (sp *SwitchableProvider) GetWorkloadSecurity(a0 context.Context, a1 string, a2 string, a3 string) (WorkloadSecurityDetail, error) {
	return sp.current(a0).GetWorkloadSecurity(a0, a1, a2, a3)
}
