//go:build !kubernetes

package platform

// OpenHelmRepoStore returns an in-memory store in the demo build: the chart
// repository list starts empty and lives as long as the process.
func OpenHelmRepoStore(mode string, provider Provider) HelmRepoStore {
	return NewMemoryHelmRepoStore()
}
