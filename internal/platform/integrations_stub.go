//go:build !kubernetes

package platform

func OpenIntegrationStore(mode string, provider Provider) IntegrationStore {
	return NewMemoryIntegrationStore()
}
