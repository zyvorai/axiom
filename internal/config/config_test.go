package config

import (
	"testing"
	"time"
)

func TestFleetHistoryPersistInterval(t *testing.T) {
	for v, want := range map[string]time.Duration{
		"":      5 * time.Minute,
		"off":   0,
		"Off":   0,
		"0":     0,
		"false": 0,
		"15m":   15 * time.Minute,
		"30s":   time.Minute, // clamped to the minimum
		"bogus": 5 * time.Minute,
	} {
		t.Setenv("AXIOM_FLEET_HISTORY_PERSIST_INTERVAL", v)
		if got := fleetHistoryPersistInterval(); got != want {
			t.Errorf("%q: got %v want %v", v, got, want)
		}
	}
}

func TestBudgetCheckInterval(t *testing.T) {
	for v, want := range map[string]time.Duration{
		"":      5 * time.Minute,
		"off":   0,
		"OFF":   0,
		"0":     0,
		"false": 0,
		"10m":   10 * time.Minute,
		"30s":   time.Minute, // clamped to the minimum
		"bogus": 5 * time.Minute,
		"-5m":   5 * time.Minute,
	} {
		t.Setenv("AXIOM_BUDGET_CHECK_INTERVAL", v)
		if got := budgetCheckInterval(); got != want {
			t.Errorf("%q: got %v want %v", v, got, want)
		}
	}
}
