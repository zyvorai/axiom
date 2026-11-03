package platform

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Budget watcher: evaluates every cost budget in the background (the same
// CostBudgetReportFor GET /cost/budgets serves) and, when a namespace crosses
// a threshold, writes an audit entry and sends a notification.
//
// The watcher evaluates with a session-less context, so on a multi-cluster
// SwitchableProvider it reads the shared default active cluster — the first
// cluster registered (the one Axiom runs in, or the kubeconfig's current
// context) unless a session-less API client switched the default. That is
// also the cluster whose ConfigMap holds the budgets.

const (
	// DefaultBudgetCheckInterval is how often budgets are evaluated.
	DefaultBudgetCheckInterval = 5 * time.Minute
	// MinBudgetCheckInterval is the shortest accepted interval.
	MinBudgetCheckInterval = time.Minute
	// budgetWatcherUser is the audit user for watcher entries.
	budgetWatcherUser = "budget-watcher"
	budgetEvalTimeout = 30 * time.Second
)

type budgetMemo struct {
	monthlyUSD  float64
	warnPercent float64
	state       string
}

// BudgetWatcher remembers each budget's last state between evaluations.
type BudgetWatcher struct {
	provider Provider
	store    CostBudgetStore
	notifier Notifier
	now      func() time.Time

	mu     sync.Mutex
	primed bool
	memo   map[string]budgetMemo
	gate   *notifyGate
}

// NewBudgetWatcher builds a watcher; a nil notifier only audits.
func NewBudgetWatcher(provider Provider, store CostBudgetStore, n Notifier) *BudgetWatcher {
	if n == nil {
		n = NopNotifier{}
	}
	return &BudgetWatcher{provider: provider, store: store, notifier: n, now: time.Now, memo: map[string]budgetMemo{}, gate: newNotifyGate()}
}

// StartBudgetWatcher evaluates the budgets now and then every interval until
// ctx is cancelled. An interval <= 0 (or a nil provider/store) disables it.
func StartBudgetWatcher(ctx context.Context, interval time.Duration, provider Provider, store CostBudgetStore, n Notifier) *BudgetWatcher {
	w := NewBudgetWatcher(provider, store, n)
	w.Start(ctx, interval)
	return w
}

// Start runs the watcher loop in the background.
func (w *BudgetWatcher) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 || w.provider == nil || w.store == nil {
		return
	}
	if interval < MinBudgetCheckInterval {
		interval = MinBudgetCheckInterval
	}
	go func() {
		w.Evaluate(ctx)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				w.Evaluate(ctx)
			}
		}
	}()
}

// Evaluate runs one check. The first successful evaluation only records the
// states (a restart must not re-announce existing conditions); a failed
// evaluation changes nothing.
func (w *BudgetWatcher) Evaluate(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("budget watcher: recovered from panic: %v", r)
		}
	}()
	ectx, cancel := context.WithTimeout(ctx, budgetEvalTimeout)
	defer cancel()
	rep, err := CostBudgetReportFor(ectx, w.provider, w.store)
	if err != nil {
		log.Printf("budget watcher: evaluate: %v", err)
		return
	}
	now := w.now()

	var out []Notification
	w.mu.Lock()
	seen := map[string]bool{}
	for _, row := range rep.Budgets {
		ns := row.Namespace
		seen[ns] = true
		cur := budgetMemo{monthlyUSD: row.MonthlyUSD, warnPercent: row.WarnPercent, state: row.State}
		prev, had := w.memo[ns]
		w.memo[ns] = cur
		if !w.primed {
			continue
		}
		if !had || prev.monthlyUSD != cur.monthlyUSD || prev.warnPercent != cur.warnPercent {
			// New or changed budget: start over from its current state.
			w.gate.forget(ns)
			continue
		}
		if prev.state == cur.state {
			continue
		}
		if n, ok := w.transition(ns, prev.state, row, now); ok {
			out = append(out, n)
		}
	}
	for ns := range w.memo {
		if !seen[ns] {
			delete(w.memo, ns)
			w.gate.forget(ns)
		}
	}
	w.primed = true
	w.mu.Unlock()

	for _, n := range out {
		w.notifier.Notify(ctx, n)
	}
}

// transition audits a state change and returns the notification to send, if
// the change is one that is announced and the debounce rule allows it.
// Called with w.mu held.
func (w *BudgetWatcher) transition(ns, from string, row CostBudgetRow, now time.Time) (Notification, bool) {
	detail := budgetDetail(row)
	n := Notification{Detail: detail, Path: "/ops/cost", Time: now}
	var send bool
	switch {
	case row.State == "over":
		n.Kind, n.Severity = "cost.budget_over", SeverityWarning
		n.Title = "Axiom: namespace " + ns + " is over its cost budget"
		RecordAxiomAudit(n.Kind, ns, detail, budgetWatcherUser)
		send = w.gate.allowBad(ns, row.State, now)
	case row.State == "warning" && from == "ok":
		n.Kind, n.Severity = "cost.budget_warning", SeverityWarning
		n.Title = fmt.Sprintf("Axiom: namespace %s is at %s%% of its cost budget", ns, trimFloat(row.Percent))
		RecordAxiomAudit(n.Kind, ns, detail, budgetWatcherUser)
		send = w.gate.allowBad(ns, row.State, now)
	case row.State == "ok":
		n.Kind, n.Severity = "cost.budget_ok", SeverityInfo
		n.Title = "Axiom: namespace " + ns + " is back within its cost budget"
		RecordAxiomAudit(n.Kind, ns, detail, budgetWatcherUser)
		send = w.gate.allowGood(ns)
	}
	// over -> warning is a quiet step down: remembered, not announced.
	return n, send
}

// budgetDetail renders "$X of $Y (Z%)".
func budgetDetail(row CostBudgetRow) string {
	return fmt.Sprintf("$%.2f of $%.2f (%s%%)", row.CurrentUSD, row.MonthlyUSD, trimFloat(row.Percent))
}

func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%.1f", f)
}
