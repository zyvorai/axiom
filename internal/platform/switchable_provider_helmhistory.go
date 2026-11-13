package platform

import "context"

// Delegating methods for Helm history / diff / reviewed rollback (kept out of
// the generated switchable_provider_gen.go).

func (sp *SwitchableProvider) ListHelmRevisions(a0 context.Context, a1 string, a2 string) ([]HelmRevision, error) {
	return sp.current(a0).ListHelmRevisions(a0, a1, a2)
}
func (sp *SwitchableProvider) DiffHelmRevisions(a0 context.Context, a1 string, a2 string, a3 int, a4 int) (HelmRevisionDiff, error) {
	return sp.current(a0).DiffHelmRevisions(a0, a1, a2, a3, a4)
}
func (sp *SwitchableProvider) PreviewHelmRollback(a0 context.Context, a1 string, a2 string, a3 int) (HelmRollbackPreview, error) {
	return sp.current(a0).PreviewHelmRollback(a0, a1, a2, a3)
}
func (sp *SwitchableProvider) RollbackHelm(a0 context.Context, a1 HelmRollbackRequest) (HelmActionResult, error) {
	return sp.current(a0).RollbackHelm(a0, a1)
}
