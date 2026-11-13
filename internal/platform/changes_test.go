package platform

import (
	"context"
	"testing"
	"time"
)

func TestRankChanges(t *testing.T) {
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m := func(n int) time.Time { return at.Add(-time.Duration(n) * time.Minute) }
	cands := []ChangeCandidate{
		{Source: ChangeRollout, Time: m(50), Namespace: "shop", Object: "Deployment/api", Summary: "old workload"},
		{Source: ChangeHelm, Time: m(5), Namespace: "shop", Object: "HelmRelease/other", Summary: "ns recent"},
		{Source: ChangeNode, Time: m(2), Object: "Node/n1", Summary: "cluster"},
		{Source: ChangeConfig, Time: m(90), Namespace: "shop", Object: "ConfigMap/api", Summary: "outside window"},
		{Source: ChangeGitOps, Time: m(3), Namespace: "other", Object: "Application/x", Summary: "other ns"},
		{Source: ChangeAudit, Time: at.Add(time.Minute), Namespace: "shop", Summary: "future"},
	}
	got := RankChanges(cands, "shop", "api", at, time.Hour)
	if len(got) != 3 {
		t.Fatalf("want 3 got %d: %+v", len(got), got)
	}
	if got[0].Summary != "old workload" || got[0].Scope != "workload" {
		t.Errorf("workload match should rank first: %+v", got)
	}
	if got[1].Summary != "ns recent" || got[2].Summary != "cluster" {
		t.Errorf("order: %+v", got)
	}
	two := RankChanges([]ChangeCandidate{
		{Time: m(40), Namespace: "shop", Object: "Deployment/api"},
		{Time: m(10), Namespace: "shop", Object: "Deployment/api"},
	}, "shop", "api", at, time.Hour)
	if two[0].Time != m(10) {
		t.Errorf("closer change should rank higher")
	}
	if got[0].Before != "50m" {
		t.Errorf("before = %q", got[0].Before)
	}
}

func TestCollectChangesDemo(t *testing.T) {
	r := CollectChanges(context.Background(), NewDemoProvider(), "", "", time.Now(), 7*24*time.Hour)
	if r.Changes == nil {
		t.Fatal("nil changes")
	}
	for _, c := range r.Changes {
		t.Logf("%d %s %s %s %s", c.Score, c.Source, c.Before, c.Object, c.Summary)
	}
}
