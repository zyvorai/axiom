//go:build !kubernetes

package platform

// OpenExternalMFAStore returns an in-memory store in the demo build: directory
// users' second factors live as long as the process.
func OpenExternalMFAStore(mode string, provider Provider) *ExternalMFAStore {
	return NewMemoryExternalMFAStore()
}
