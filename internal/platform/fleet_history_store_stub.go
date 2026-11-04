//go:build !kubernetes

package platform

// OpenFleetHistoryStore returns an in-memory store in the demo build: the
// history is not durable (the `durable` flag stays false).
func OpenFleetHistoryStore(mode string, provider Provider) FleetHistoryStore {
	return NewMemoryFleetHistoryStore(FleetHistoryReplicaID())
}
