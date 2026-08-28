//go:build !kubernetes

package platform

func OpenUserStore(mode string, provider Provider) UserStore {
	return NewMemoryUserStore()
}
