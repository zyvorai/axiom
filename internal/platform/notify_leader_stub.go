//go:build !kubernetes

package platform

// OpenNotifyLeader is a no-op outside the kubernetes build (demo always leads).
func OpenNotifyLeader(mode string, provider Provider) NotifyLeader {
	return nil
}
