//go:build !kubernetes

package platform

// OpenCostBudgetStore returns an in-memory store in the demo build: budgets
// start empty and live as long as the process.
func OpenCostBudgetStore(mode string, provider Provider) CostBudgetStore {
	return NewMemoryCostBudgetStore()
}
