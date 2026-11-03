package platform

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"sync"
)

// CostBudget is a monthly budget for one namespace, compared with the
// request-based estimate GET /cost returns.
type CostBudget struct {
	Namespace   string  `json:"namespace"`
	MonthlyUSD  float64 `json:"monthlyUsd"`
	WarnPercent float64 `json:"warnPercent"`
	UpdatedAt   string  `json:"updatedAt,omitempty"`
	UpdatedBy   string  `json:"updatedBy,omitempty"`
}

// CostBudgetRow is a budget with the namespace's current estimate.
type CostBudgetRow struct {
	CostBudget
	CurrentUSD float64 `json:"currentUsd"`
	// Percent is currentUsd / monthlyUsd x 100, rounded to one decimal.
	Percent float64 `json:"percent"`
	// State is ok, warning (>= warnPercent) or over (>= 100%).
	State string `json:"state"`
}

type UnbudgetedNamespace struct {
	Namespace  string  `json:"namespace"`
	CurrentUSD float64 `json:"currentUsd"`
}

type CostBudgetReport struct {
	Budgets    []CostBudgetRow       `json:"budgets"`
	Unbudgeted []UnbudgetedNamespace `json:"unbudgeted"`
	// TotalBudgetUSD sums the budgets; TotalCurrentUSD sums the current
	// estimate of the budgeted namespaces (so the two compare like for like).
	TotalBudgetUSD  float64 `json:"totalBudgetUsd"`
	TotalCurrentUSD float64 `json:"totalCurrentUsd"`
}

const (
	CostBudgetDefaultWarnPercent = 80.0
	costBudgetMaxMonthlyUSD      = 1e9
	costBudgetMaxCount           = 1000
	costBudgetMaxUnbudgeted      = 20

	costBudgetConfigMapName = "zyvor-axiom-cost-budgets"
	costBudgetConfigMapKey  = "budgets.json"
)

var costNamespaceRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateCostNamespace requires a DNS-1123 label (what a namespace name is).
func ValidateCostNamespace(ns string) error {
	if len(ns) == 0 || len(ns) > 63 || !costNamespaceRe.MatchString(ns) {
		return NewStatusError(http.StatusBadRequest, fmt.Errorf("namespace must be a DNS-1123 label (lowercase letters, digits and '-', at most 63 characters)"))
	}
	return nil
}

// ValidateCostBudget checks a budget about to be stored; a zero WarnPercent
// is the caller's job to default before calling.
func ValidateCostBudget(b CostBudget) error {
	if err := ValidateCostNamespace(b.Namespace); err != nil {
		return err
	}
	if math.IsNaN(b.MonthlyUSD) || b.MonthlyUSD <= 0 || b.MonthlyUSD > costBudgetMaxMonthlyUSD {
		return NewStatusError(http.StatusBadRequest, fmt.Errorf("monthlyUsd must be greater than 0 and at most 1e9"))
	}
	if math.IsNaN(b.WarnPercent) || b.WarnPercent < 1 || b.WarnPercent > 100 {
		return NewStatusError(http.StatusBadRequest, fmt.Errorf("warnPercent must be between 1 and 100"))
	}
	return nil
}

// costBudgetState classifies percent against the warning threshold.
func costBudgetState(percent, warn float64) string {
	switch {
	case percent >= 100:
		return "over"
	case percent >= warn:
		return "warning"
	}
	return "ok"
}

// CostBudgetRowFor compares one budget with an estimate. A namespace missing
// from the estimate (deleted, or nothing requested) costs 0.
func CostBudgetRowFor(b CostBudget, est CostEstimate) CostBudgetRow {
	cur := 0.0
	for _, n := range est.Namespaces {
		if n.Namespace == b.Namespace {
			cur = n.MonthlyUSD
			break
		}
	}
	if b.WarnPercent == 0 {
		b.WarnPercent = CostBudgetDefaultWarnPercent
	}
	row := CostBudgetRow{CostBudget: b, CurrentUSD: round2(cur)}
	if b.MonthlyUSD > 0 {
		row.Percent = round1(cur / b.MonthlyUSD * 100)
	}
	row.State = costBudgetState(row.Percent, b.WarnPercent)
	return row
}

// BuildCostBudgetReport compares every budget with the estimate. Budgets are
// sorted fullest first; unbudgeted namespaces biggest first, at most 20.
func BuildCostBudgetReport(est CostEstimate, budgets []CostBudget) CostBudgetReport {
	rep := CostBudgetReport{Budgets: []CostBudgetRow{}, Unbudgeted: []UnbudgetedNamespace{}}
	has := map[string]bool{}
	for _, b := range budgets {
		row := CostBudgetRowFor(b, est)
		rep.Budgets = append(rep.Budgets, row)
		rep.TotalBudgetUSD += b.MonthlyUSD
		rep.TotalCurrentUSD += row.CurrentUSD
		has[b.Namespace] = true
	}
	sort.SliceStable(rep.Budgets, func(i, j int) bool {
		if rep.Budgets[i].Percent != rep.Budgets[j].Percent {
			return rep.Budgets[i].Percent > rep.Budgets[j].Percent
		}
		return rep.Budgets[i].Namespace < rep.Budgets[j].Namespace
	})
	for _, n := range est.Namespaces {
		if !has[n.Namespace] {
			rep.Unbudgeted = append(rep.Unbudgeted, UnbudgetedNamespace{Namespace: n.Namespace, CurrentUSD: round2(n.MonthlyUSD)})
		}
	}
	sort.SliceStable(rep.Unbudgeted, func(i, j int) bool {
		if rep.Unbudgeted[i].CurrentUSD != rep.Unbudgeted[j].CurrentUSD {
			return rep.Unbudgeted[i].CurrentUSD > rep.Unbudgeted[j].CurrentUSD
		}
		return rep.Unbudgeted[i].Namespace < rep.Unbudgeted[j].Namespace
	})
	if len(rep.Unbudgeted) > costBudgetMaxUnbudgeted {
		rep.Unbudgeted = rep.Unbudgeted[:costBudgetMaxUnbudgeted]
	}
	rep.TotalBudgetUSD = round2(rep.TotalBudgetUSD)
	rep.TotalCurrentUSD = round2(rep.TotalCurrentUSD)
	return rep
}

// CostBudgetStore persists the per-namespace budgets.
type CostBudgetStore interface {
	List(ctx context.Context) ([]CostBudget, error)
	// Set adds or replaces the budget for b.Namespace.
	Set(ctx context.Context, b CostBudget) error
	// Delete removes a budget; a missing one is a 404 StatusError.
	Delete(ctx context.Context, namespace string) error
}

// costBudgetListSet returns list with b added or replaced. Shared by every
// store so the rules are identical.
func costBudgetListSet(list []CostBudget, b CostBudget) ([]CostBudget, error) {
	out := make([]CostBudget, 0, len(list)+1)
	replaced := false
	for _, e := range list {
		if e.Namespace == b.Namespace {
			out = append(out, b)
			replaced = true
			continue
		}
		out = append(out, e)
	}
	if !replaced {
		if len(list) >= costBudgetMaxCount {
			return nil, NewStatusError(http.StatusConflict, fmt.Errorf("at most %d budgets can be stored", costBudgetMaxCount))
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Namespace < out[j].Namespace })
	return out, nil
}

func costBudgetListDelete(list []CostBudget, ns string) ([]CostBudget, error) {
	out := make([]CostBudget, 0, len(list))
	for _, e := range list {
		if e.Namespace != ns {
			out = append(out, e)
		}
	}
	if len(out) == len(list) {
		return nil, notFoundf("no budget for namespace %q", ns)
	}
	return out, nil
}

// MemoryCostBudgetStore keeps budgets in memory (demo build, tests).
type MemoryCostBudgetStore struct {
	mu      sync.Mutex
	budgets []CostBudget
}

func NewMemoryCostBudgetStore() *MemoryCostBudgetStore { return &MemoryCostBudgetStore{} }

func (m *MemoryCostBudgetStore) List(context.Context) ([]CostBudget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]CostBudget{}, m.budgets...), nil
}

func (m *MemoryCostBudgetStore) Set(_ context.Context, b CostBudget) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	out, err := costBudgetListSet(m.budgets, b)
	if err != nil {
		return err
	}
	m.budgets = out
	return nil
}

func (m *MemoryCostBudgetStore) Delete(_ context.Context, ns string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	out, err := costBudgetListDelete(m.budgets, ns)
	if err != nil {
		return err
	}
	m.budgets = out
	return nil
}
