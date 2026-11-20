package platform

import (
	"context"
	"testing"
	"time"
)

func TestExternalSecretStale(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ago     time.Duration
		refresh string
		want    bool
	}{
		{90 * time.Minute, "1h", false},
		{3 * time.Hour, "1h", true},
		{2 * time.Hour, "1h", false}, // exactly 2x is not > 2x
		{100 * time.Hour, "0s", false},
		{100 * time.Hour, "", false},
		{100 * time.Hour, "bogus", false},
	}
	for _, c := range cases {
		if got, _ := ExternalSecretStale(now.Add(-c.ago), c.refresh, now); got != c.want {
			t.Errorf("ago=%v refresh=%q got %v want %v", c.ago, c.refresh, got, c.want)
		}
	}
	if got, _ := ExternalSecretStale(time.Time{}, "1h", now); got {
		t.Error("missing sync time must not be stale")
	}
}

func TestFinalizeRollup(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	items := []ExternalSecretHealth{
		{Namespace: "b", Name: "ok", Ready: "True", Refresh: "1h", LastSync: now.Add(-10 * time.Minute).Format(time.RFC3339)},
		{Namespace: "a", Name: "bad", Ready: "False", Refresh: "1h", LastSync: now.Add(-10 * time.Hour).Format(time.RFC3339)},
		{Namespace: "a", Name: "old", Ready: "True", Refresh: "1h", LastSync: now.Add(-5 * time.Hour).Format(time.RFC3339)},
	}
	rep := FinalizeExternalSecretsHealth(items, nil, now)
	if rep.Rollup != (ExternalSecretsRollup{Total: 3, Ready: 1, Failing: 1, Stale: 1}) {
		t.Fatalf("rollup %+v", rep.Rollup)
	}
	if rep.ExternalSecrets[0].Name != "bad" || rep.ExternalSecrets[0].Health != ESHealthFailing {
		t.Fatalf("sort/classify: %+v", rep.ExternalSecrets)
	}
	if rep.Stores == nil || !rep.Available {
		t.Fatal("stores must be non-nil and report available")
	}
}

func TestParseExternalSecretAndStore(t *testing.T) {
	es := ParseExternalSecretHealth(map[string]any{
		"metadata": map[string]any{"namespace": "n", "name": "x"},
		"spec": map[string]any{"refreshInterval": "15m",
			"secretStoreRef": map[string]any{"name": "s", "kind": "ClusterSecretStore"}},
		"status": map[string]any{"refreshTime": "2026-01-01T00:00:00Z", "conditions": []any{
			map[string]any{"type": "Ready", "status": "True", "reason": "SecretSynced", "message": "ok"}}},
	})
	if es.StoreKind != "ClusterSecretStore" || es.TargetName != "x" || es.Ready != "True" || es.Reason != "SecretSynced" {
		t.Fatalf("%+v", es)
	}
	st := ParseExternalSecretStore(map[string]any{
		"metadata": map[string]any{"name": "v"},
		"spec":     map[string]any{"provider": map[string]any{"vault": map[string]any{"auth": map[string]any{"token": "SECRET"}}}},
	}, "ClusterSecretStore")
	if st.Provider != "vault" || st.Ready != "Unknown" {
		t.Fatalf("%+v", st)
	}
}

type recNotifier struct{ got []Notification }

func (r *recNotifier) Notify(_ context.Context, n Notification) { r.got = append(r.got, n) }

type esProv struct {
	Provider
	rep ExternalSecretsHealthReport
}

func (p *esProv) ListExternalSecretsHealth(context.Context, string) (ExternalSecretsHealthReport, error) {
	return p.rep, nil
}

func TestExternalSecretsWatcherDebounce(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	mk := func(h string) ExternalSecretsHealthReport {
		return ExternalSecretsHealthReport{Available: true, ExternalSecrets: []ExternalSecretHealth{{Namespace: "n", Name: "x", Health: h}}}
	}
	p := &esProv{rep: mk(ESHealthReady)}
	rn := &recNotifier{}
	w := NewExternalSecretsWatcher(p, rn)
	w.now = func() time.Time { return now }
	ctx := context.Background()
	w.Evaluate(ctx) // primes
	p.rep = mk(ESHealthFailing)
	w.Evaluate(ctx)
	if len(rn.got) != 1 || rn.got[0].Kind != "externalsecret.failing" {
		t.Fatalf("want failing notice, got %+v", rn.got)
	}
	p.rep = mk(ESHealthReady)
	w.Evaluate(ctx)
	p.rep = mk(ESHealthFailing)
	w.Evaluate(ctx) // flap inside the debounce window
	n := 0
	for _, g := range rn.got {
		if g.Kind == "externalsecret.failing" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("flap must be debounced, got %d failing notices", n)
	}
}
