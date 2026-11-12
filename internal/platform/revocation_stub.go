//go:build !kubernetes

package platform

import "github.com/zyvorai/axiom/internal/auth"

// OpenRevocationStore returns nil in the demo build: session revocations stay
// in memory, which is fine for a local lab run.
func OpenRevocationStore(mode string, provider Provider) auth.RevocationStore {
	return nil
}

// OpenPodProxyRedeemedStore returns nil in the demo build: redeemed pod-proxy
// tickets are remembered by this process only.
func OpenPodProxyRedeemedStore(mode string, provider Provider) auth.RevocationStore {
	return nil
}

// OpenSAMLRequestStore returns nil in the demo build.
func OpenSAMLRequestStore(mode string, provider Provider) auth.RevocationStore {
	return nil
}

// OpenStreamTicketStore returns nil in the demo build.
func OpenStreamTicketStore(mode string, provider Provider) auth.RevocationStore {
	return nil
}

// OpenLoginThrottleStore returns nil in the demo build.
func OpenLoginThrottleStore(mode string, provider Provider) auth.RevocationStore {
	return nil
}
