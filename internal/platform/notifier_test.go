package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureNotifier records every notification.
type captureNotifier struct {
	mu  sync.Mutex
	got []Notification
}

func (c *captureNotifier) Notify(_ context.Context, n Notification) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, n)
}

func (c *captureNotifier) take() []Notification {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.got
	c.got = nil
	return out
}

func kinds(ns []Notification) string {
	var out []string
	for _, n := range ns {
		out = append(out, n.Kind)
	}
	return strings.Join(out, ",")
}

// fakeClock is a settable clock safe for concurrent reads.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newNotifyFleet(t *testing.T) (*SwitchableProvider, *fakeCluster, *captureNotifier, *fakeClock) {
	t.Helper()
	a := newFake(healthyOv())
	sp := newFleet(t, map[string]*fakeCluster{"eu-west-1": a}, "eu-west-1")
	clk := &fakeClock{t: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)}
	sp.fleet().now = clk.Now
	n := &captureNotifier{}
	sp.SetFleetNotifier(n)
	return sp, a, n, clk
}

func TestFleetNotifyFirstProbeIsSilent(t *testing.T) {
	sp, a, n, _ := newNotifyFleet(t)
	a.setErr(errors.New("connection refused"))
	sp.ProbeAll(context.Background())
	if got := n.take(); len(got) != 0 {
		t.Fatalf("first probe notified: %+v", got)
	}
	// Coming back from a state nobody was told about announces nothing either.
	a.setErr(nil)
	sp.ProbeAll(context.Background())
	if got := n.take(); len(got) != 0 {
		t.Fatalf("unannounced recovery notified: %+v", got)
	}
}

func TestFleetNotifyDownDegradedRecovered(t *testing.T) {
	sp, a, n, clk := newNotifyFleet(t)
	ctx := context.Background()
	sp.ProbeAll(ctx) // healthy baseline

	clk.Advance(time.Minute)
	a.setErr(errors.New("dial tcp 10.0.0.1:6443: connection refused"))
	sp.ProbeAll(ctx)
	got := n.take()
	if len(got) != 1 || got[0].Kind != "fleet.cluster_unreachable" || got[0].Severity != SeverityWarning {
		t.Fatalf("down: %+v", got)
	}
	if got[0].Title != "Axiom: cluster eu-west-1 is unreachable" || !strings.Contains(got[0].Detail, "connection refused") || got[0].Path != "/fleet" {
		t.Fatalf("down message: %+v", got[0])
	}

	clk.Advance(2 * time.Minute)
	a.setErr(nil)
	a.setOv(Overview{Health: "Healthy", Nodes: 3, ReadyNodes: 2})
	sp.ProbeAll(ctx)
	got = n.take()
	if len(got) != 1 || got[0].Kind != "fleet.cluster_degraded" || !strings.Contains(got[0].Detail, "1 of 3 nodes are not ready") {
		t.Fatalf("degraded: %+v", got)
	}

	clk.Advance(9 * time.Minute)
	a.setOv(healthyOv())
	sp.ProbeAll(ctx)
	got = n.take()
	if len(got) != 1 || got[0].Kind != "fleet.cluster_recovered" || got[0].Severity != SeverityInfo {
		t.Fatalf("recovered: %+v", got)
	}
	// Down since the first failure: 2m unreachable + 9m degraded.
	if !strings.Contains(got[0].Detail, "after 11m") || !strings.Contains(got[0].Detail, "was degraded") {
		t.Fatalf("recovery detail lacks the duration: %q", got[0].Detail)
	}
}

func TestFleetNotifyDebounce(t *testing.T) {
	sp, a, n, clk := newNotifyFleet(t)
	ctx := context.Background()
	sp.ProbeAll(ctx)
	flap := func() {
		clk.Advance(time.Minute)
		a.setErr(errors.New("timeout"))
		sp.ProbeAll(ctx)
		clk.Advance(time.Minute)
		a.setErr(nil)
		sp.ProbeAll(ctx)
	}
	flap()
	if got := kinds(n.take()); got != "fleet.cluster_unreachable,fleet.cluster_recovered" {
		t.Fatalf("first flap: %s", got)
	}
	// Within 10 minutes of the last "unreachable": nothing at all (and no
	// recovery, since no outage was announced).
	flap()
	flap()
	if got := n.take(); len(got) != 0 {
		t.Fatalf("debounced flaps notified: %s", kinds(got))
	}
	clk.Advance(10 * time.Minute)
	flap()
	if got := kinds(n.take()); got != "fleet.cluster_unreachable,fleet.cluster_recovered" {
		t.Fatalf("after the window: %s", got)
	}
}

func TestFleetNotifyRecoveryAlwaysFollowsAnnouncedOutage(t *testing.T) {
	sp, a, n, clk := newNotifyFleet(t)
	ctx := context.Background()
	sp.ProbeAll(ctx)
	clk.Advance(time.Minute)
	a.setErr(errors.New("timeout"))
	sp.ProbeAll(ctx)
	clk.Advance(30 * time.Second)
	a.setErr(nil)
	sp.ProbeAll(ctx)
	clk.Advance(30 * time.Second)
	// Second outage within the window is suppressed, but the next one that
	// is announced (after the window) is always followed by its recovery.
	a.setErr(errors.New("timeout"))
	sp.ProbeAll(ctx)
	clk.Advance(15 * time.Minute)
	a.setErr(nil)
	sp.ProbeAll(ctx)
	clk.Advance(time.Minute)
	a.setErr(errors.New("timeout"))
	sp.ProbeAll(ctx)
	clk.Advance(time.Minute)
	a.setErr(nil)
	sp.ProbeAll(ctx)
	want := "fleet.cluster_unreachable,fleet.cluster_recovered,fleet.cluster_unreachable,fleet.cluster_recovered"
	if got := kinds(n.take()); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

// --- budget watcher -------------------------------------------------------

type costFake struct {
	Provider
	mu  sync.Mutex
	usd map[string]float64
	err error
}

func (c *costFake) set(ns string, usd float64) {
	c.mu.Lock()
	c.usd[ns] = usd
	c.mu.Unlock()
}

func (c *costFake) EstimateCost(context.Context) (CostEstimate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return CostEstimate{}, c.err
	}
	var est CostEstimate
	for ns, v := range c.usd {
		est.Namespaces = append(est.Namespaces, NamespaceCost{Namespace: ns, MonthlyUSD: v})
	}
	return est, nil
}

// newestAudit returns the most recent audit entry (zero if none).
func newestAudit() AuditEntry {
	if l := ListAxiomAudit(); len(l) > 0 {
		return l[0]
	}
	return AuditEntry{}
}

// auditAdded returns the action of the watcher entry for ns recorded since
// before (each evaluation records at most one per namespace), or "".
func auditAdded(before AuditEntry, ns string) string {
	a := newestAudit()
	if a == before || a.Target != ns || a.User != "budget-watcher" {
		return ""
	}
	return a.Action
}

func TestBudgetWatcherTransitions(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryCostBudgetStore()
	_ = store.Set(ctx, CostBudget{Namespace: "team-a", MonthlyUSD: 100, WarnPercent: 80})
	cost := &costFake{usd: map[string]float64{"team-a": 50}}
	n := &captureNotifier{}
	w := NewBudgetWatcher(cost, store, n)
	clk := &fakeClock{t: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)}
	w.now = clk.Now

	steps := []struct {
		usd     float64
		advance time.Duration
		want    string // notification kinds
		audit   string // audit action recorded by this step ("" = none)
	}{
		{50, 0, "", ""}, // first evaluation: silent
		{85, time.Minute, "cost.budget_warning", "cost.budget_warning"}, // ok -> warning
		{85, time.Minute, "", ""},                                  // no change
		{120, time.Minute, "cost.budget_over", "cost.budget_over"}, // warning -> over
		{90, time.Minute, "", ""},                                  // over -> warning: quiet
		{40, time.Minute, "cost.budget_ok", "cost.budget_ok"},      // -> ok: announced
		{130, time.Minute, "", "cost.budget_over"},                 // ok -> over within 10m of the last "over": audited, not sent
		{40, time.Minute, "", "cost.budget_ok"},                    // no outstanding announcement: no ok message
		{130, 15 * time.Minute, "cost.budget_over", "cost.budget_over"},
	}
	for i, s := range steps {
		clk.Advance(s.advance)
		before := newestAudit()
		cost.set("team-a", s.usd)
		w.Evaluate(ctx)
		got := n.take()
		if kinds(got) != s.want {
			t.Fatalf("step %d ($%v): notified %q, want %q", i, s.usd, kinds(got), s.want)
		}
		if added := auditAdded(before, "team-a"); added != s.audit {
			t.Fatalf("step %d: audited %q, want %q", i, added, s.audit)
		}
		for _, m := range got {
			if m.Path != "/ops/cost" || !strings.HasPrefix(m.Title, "Axiom: namespace team-a") {
				t.Fatalf("step %d: message %+v", i, m)
			}
		}
	}
	last := ListAxiomAudit()[0]
	if last.Action != "cost.budget_over" || last.Detail != "$130.00 of $100.00 (130%)" || last.User != "budget-watcher" {
		t.Fatalf("audit entry %+v", last)
	}
}

func TestBudgetWatcherRestartIsSilent(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryCostBudgetStore()
	_ = store.Set(ctx, CostBudget{Namespace: "team-r", MonthlyUSD: 100, WarnPercent: 80})
	cost := &costFake{usd: map[string]float64{"team-r": 150}}
	n := &captureNotifier{}
	before := newestAudit()
	// A fresh watcher (a restart) over an already-blown budget says nothing.
	for i := 0; i < 2; i++ {
		w := NewBudgetWatcher(cost, store, n)
		w.Evaluate(ctx)
		w.Evaluate(ctx)
	}
	if got := n.take(); len(got) != 0 {
		t.Fatalf("restart notified: %+v", got)
	}
	if auditAdded(before, "team-r") != "" {
		t.Fatalf("restart audited")
	}

	// A failed first evaluation does not prime the watcher.
	w := NewBudgetWatcher(cost, store, n)
	cost.mu.Lock()
	cost.err = errors.New("metrics unavailable")
	cost.mu.Unlock()
	w.Evaluate(ctx)
	cost.mu.Lock()
	cost.err = nil
	cost.mu.Unlock()
	w.Evaluate(ctx) // this is the priming one
	if got := n.take(); len(got) != 0 {
		t.Fatalf("evaluation after a failed one notified: %+v", got)
	}
}

func TestBudgetWatcherRemovedOrChangedBudgetResets(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryCostBudgetStore()
	_ = store.Set(ctx, CostBudget{Namespace: "team-c", MonthlyUSD: 100, WarnPercent: 80})
	cost := &costFake{usd: map[string]float64{"team-c": 50}}
	n := &captureNotifier{}
	w := NewBudgetWatcher(cost, store, n)
	clk := &fakeClock{t: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)}
	w.now = clk.Now
	w.Evaluate(ctx)

	cost.set("team-c", 120)
	w.Evaluate(ctx)
	if got := kinds(n.take()); got != "cost.budget_over" {
		t.Fatalf("over: %s", got)
	}
	// Raising the budget resets the namespace: its new state is the
	// baseline, the stale "over" announcement no longer awaits an "ok".
	_ = store.Set(ctx, CostBudget{Namespace: "team-c", MonthlyUSD: 500, WarnPercent: 80})
	clk.Advance(time.Minute)
	w.Evaluate(ctx)
	if got := n.take(); len(got) != 0 {
		t.Fatalf("changed budget notified: %+v", got)
	}
	// ... and the debounce memory is gone too: a new "over" is sent at once.
	cost.set("team-c", 600)
	clk.Advance(time.Minute)
	w.Evaluate(ctx)
	if got := kinds(n.take()); got != "cost.budget_over" {
		t.Fatalf("over after change: %s", got)
	}

	// Removed, then re-added while over: baseline again, no notification.
	_ = store.Delete(ctx, "team-c")
	w.Evaluate(ctx)
	_ = store.Set(ctx, CostBudget{Namespace: "team-c", MonthlyUSD: 500, WarnPercent: 80})
	w.Evaluate(ctx)
	if got := n.take(); len(got) != 0 {
		t.Fatalf("removed/re-added budget notified: %+v", got)
	}
	w.mu.Lock()
	_, ok := w.memo["team-c"]
	w.mu.Unlock()
	if !ok {
		t.Fatalf("re-added budget not remembered")
	}
	_ = store.Delete(ctx, "team-c")
	w.Evaluate(ctx)
	w.mu.Lock()
	_, ok = w.memo["team-c"]
	w.mu.Unlock()
	if ok {
		t.Fatalf("removed budget still remembered")
	}
}

func TestBudgetWatcherStartDisabledAndRuns(t *testing.T) {
	store := NewMemoryCostBudgetStore()
	cost := &costFake{usd: map[string]float64{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Disabled: never evaluates, so it never primes.
	w := StartBudgetWatcher(ctx, 0, cost, store, nil)
	time.Sleep(20 * time.Millisecond)
	w.mu.Lock()
	primed := w.primed
	w.mu.Unlock()
	if primed {
		t.Fatalf("disabled watcher evaluated")
	}
	// Enabled: evaluates straight away.
	w = StartBudgetWatcher(ctx, time.Second, cost, store, nil)
	deadline := time.Now().Add(2 * time.Second)
	for {
		w.mu.Lock()
		primed = w.primed
		w.mu.Unlock()
		if primed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("watcher never evaluated")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// --- formatting, delivery, timeline ---------------------------------------

func TestFormatNotificationHasNoURLs(t *testing.T) {
	n := Notification{
		Kind: "fleet.cluster_unreachable", Severity: SeverityWarning,
		Title:  "Axiom: cluster eu-west-1 is unreachable",
		Detail: "Get \"https://10.0.0.1:6443/api?token=abc123\": dial tcp:\n connection refused",
		Path:   "/fleet",
	}
	text, payload := formatNotification(n)
	raw, _ := json.Marshal(payload)
	for _, s := range []string{text, string(raw)} {
		if strings.Contains(s, "token=abc123") || strings.Contains(s, "/api") {
			t.Fatalf("URL path/query leaked: %s", s)
		}
		if !strings.Contains(s, "eu-west-1") || !strings.Contains(s, "/fleet") {
			t.Fatalf("missing content: %s", s)
		}
	}
	if strings.Count(text, "\n") != 2 {
		t.Fatalf("want title, detail and path lines: %q", text)
	}
	if !strings.HasPrefix(text, ":warning: *Axiom: cluster eu-west-1 is unreachable*") {
		t.Fatalf("slack text %q", text)
	}
	// A path that is not Axiom-relative is dropped.
	if _, p := formatNotification(Notification{Title: "x", Path: "https://evil.example/x"}); p["path"] != "" {
		t.Fatalf("absolute path kept: %v", p["path"])
	}
}

func TestHubNotifierDeliversWithoutLeakingSecret(t *testing.T) {
	var mu sync.Mutex
	var slackBody map[string]string
	var hookBody map[string]any
	slack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		_ = json.NewDecoder(r.Body).Decode(&slackBody)
		mu.Unlock()
	}))
	defer slack.Close()
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		_ = json.NewDecoder(r.Body).Decode(&hookBody)
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError) // a failure that gets logged
	}))
	defer hook.Close()

	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prev)

	slackURL := slack.URL + secretPath
	hookURL := hook.URL + "/hooks/AnotherSecretToken"
	hub := NewIntegrationHub(Integrations{SlackWebhookURL: slackURL, AlertWebhookURL: hookURL, NotifyOnWarnings: true}, nil)
	msg := Notification{Kind: "cost.budget_over", Severity: SeverityWarning, Title: "Axiom: namespace a is over its cost budget", Detail: "$120.00 of $100.00 (120%)", Path: "/ops/cost"}
	NewHubNotifier(hub).Notify(context.Background(), msg)

	mu.Lock()
	gotSlack, gotHook := slackBody["text"], hookBody
	mu.Unlock()
	if !strings.Contains(gotSlack, "Axiom: namespace a is over its cost budget") || !strings.Contains(gotSlack, "$120.00 of $100.00") {
		t.Fatalf("slack text %q", gotSlack)
	}
	if gotHook["kind"] != "cost.budget_over" || gotHook["path"] != "/ops/cost" || gotHook["source"] != "zyvor-axiom" {
		t.Fatalf("webhook payload %v", gotHook)
	}
	out := logs.String()
	if !strings.Contains(out, "webhook dispatch failed") {
		t.Fatalf("expected the webhook failure to be logged, got %q", out)
	}
	for _, secret := range []string{"SuperSecretToken", "AnotherSecretToken"} {
		if strings.Contains(out, secret) || strings.Contains(gotSlack, secret) {
			t.Fatalf("secret %s leaked: logs=%q", secret, out)
		}
	}

	// Disabled or unconfigured: nothing is sent.
	mu.Lock()
	slackBody = nil
	mu.Unlock()
	off := NewIntegrationHub(Integrations{SlackWebhookURL: slackURL, NotifyOnWarnings: false}, nil)
	NewHubNotifier(off).Notify(context.Background(), msg)
	NewHubNotifier(NewIntegrationHub(Integrations{NotifyOnWarnings: true}, nil)).Notify(context.Background(), msg)
	NewHubNotifier(nil).Notify(context.Background(), msg)
	mu.Lock()
	defer mu.Unlock()
	if slackBody != nil {
		t.Fatalf("disabled notifier sent %v", slackBody)
	}
}

func TestNotificationAuditEntriesTimelineSeverity(t *testing.T) {
	cases := []struct {
		action, target, ns, sev string
	}{
		{"fleet.cluster_unreachable", "eu-west-1", "", SeverityWarning},
		{"fleet.cluster_degraded", "eu-west-1", "", SeverityWarning},
		{"fleet.cluster_recovered", "eu-west-1", "", SeverityInfo},
		{"cost.budget_warning", "team-a", "team-a", SeverityWarning},
		{"cost.budget_over", "team-a", "team-a", SeverityWarning},
		{"cost.budget_ok", "team-a", "team-a", SeverityInfo},
	}
	for _, c := range cases {
		got := TimelineFromAudit(AuditEntry{Action: c.action, Target: c.target, Detail: "d", User: "budget-watcher"})
		if got.Severity != c.sev || got.Namespace != c.ns || got.Object != c.target || !strings.Contains(got.Summary, c.action) {
			t.Errorf("%s -> %+v", c.action, got)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		45 * time.Second: "45s", 12 * time.Minute: "12m", 3*time.Hour + 5*time.Minute: "3h 5m",
		2 * time.Hour: "2h", 52 * time.Hour: "2d 4h", 48 * time.Hour: "2d", -time.Second: "0s",
	} {
		if got := humanDuration(d); got != want {
			t.Errorf("%v: %q want %q", d, got, want)
		}
	}
}
