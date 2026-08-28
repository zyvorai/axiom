//go:build !kubernetes

package platform

// OpenAuditStore returns nil in the demo build — the audit trail stays
// purely in-memory, which is fine for a local lab run.
func OpenAuditStore(mode string, provider Provider) AuditStore {
	return nil
}
