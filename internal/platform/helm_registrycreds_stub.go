//go:build !kubernetes

package platform

// OpenHelmRegistryCredStore returns an in-memory store in the demo build:
// registry credentials do not outlive the process.
func OpenHelmRegistryCredStore(mode string, provider Provider) HelmRegistryCredStore {
	return NewMemoryHelmRegistryCredStore()
}
