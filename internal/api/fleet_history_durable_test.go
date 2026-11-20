package api

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zyvorai/axiom/internal/platform"
)

// durableHistStore is a persistent-looking FleetHistoryStore in memory.
type durableHistStore struct {
	mu    sync.Mutex
	snaps []platform.FleetHistorySnapshot
	saves int
}

func (s *durableHistStore) Load(context.Context) ([]platform.FleetHistorySnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]platform.FleetHistorySnapshot(nil), s.snaps...), nil
}

func (s *durableHistStore) Save(_ context.Context, snap platform.FleetHistorySnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	return nil
}

func (s *durableHistStore) Persistent() bool { return true }

func TestFleetHistoryDurableAndRestored(t *testing.T) {
	t.Setenv("AXIOM_DEMO_FLEET_DOWN", "")
	t.Setenv("AXIOM_DEMO_FLEET_HISTORY_SEED", "")
	sp := platform.NewDemoSwitchableProvider()
	hub := platform.NewUserHub(nil)
	e := &idEnv{h: NewWithUsers(sp, "demo", idAdminUser, idAdminPass, "", nil, hub, AuthOptions{SessionSecret: "test-secret"}), users: hub, store: &memRevStore{}}
	admin := withBasic(idAdminUser, idAdminPass)

	// Before persistence starts: not durable, nothing restored.
	w := e.do("GET", "/api/v1/fleet/history", "", admin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"durable":false`) || !strings.Contains(w.Body.String(), `"restoredFrom":null`) {
		t.Fatalf("fresh: %d %s", w.Code, w.Body.String())
	}

	now := time.Now().Unix()
	saved := now - 600
	store := &durableHistStore{snaps: []platform.FleetHistorySnapshot{{
		Version: 1, Replica: "axiom-0", SavedAt: saved, BucketSeconds: 300,
		Clusters: map[string][]platform.FleetHistoryBucket{
			"eu-west-1": {
				{FleetSample: platform.FleetSample{T: now - 1200, State: "healthy", Nodes: 3, ReadyNodes: 3}, Checks: 10},
				{FleetSample: platform.FleetSample{T: now - 900, State: "healthy", Nodes: 3, ReadyNodes: 3}, Checks: 10},
			},
			"retired-cluster": {{FleetSample: platform.FleetSample{T: now - 900, State: "healthy"}, Checks: 10}},
		},
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	done := sp.StartFleetHistoryPersistence(ctx, store, time.Minute)
	_ = e.do("GET", "/api/v1/fleet", "", admin) // one live probe each

	var h platform.FleetHistory
	decodeInto(t, e.do("GET", "/api/v1/fleet/history?cluster=eu-west-1", "", admin), &h)
	c := h.Clusters[0]
	if h.Durable || h.RestoredFrom == nil || *h.RestoredFrom != time.Unix(saved, 0).UTC().Format(time.RFC3339) {
		t.Fatalf("restored: durable=%v from=%v", h.Durable, h.RestoredFrom)
	}
	if c.Checks != 21 || len(c.Samples) != 3 || !c.Samples[0].Restored || c.Samples[2].Restored ||
		*c.UptimePercent != 100 || len(c.Incidents) != 0 || len(c.Gaps) != 1 {
		t.Fatalf("cluster %+v", c)
	}
	decodeInto(t, e.do("GET", "/api/v1/fleet/history", "", admin), &h)
	for _, c := range h.Clusters {
		if c.Cluster == "retired-cluster" {
			t.Fatal("unregistered cluster restored")
		}
	}

	// The shutdown flush is a successful save: durable from then on.
	cancel()
	<-done
	w = e.do("GET", "/api/v1/fleet/history?window=1h", "", admin)
	if !strings.Contains(w.Body.String(), `"durable":true`) || store.saves != 1 {
		t.Fatalf("after save: %d saves, %s", store.saves, w.Body.String())
	}
}
