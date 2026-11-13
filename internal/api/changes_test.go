package api

import (
	"encoding/json"
	"testing"

	"github.com/zyvorai/axiom/internal/platform"
)

func TestChangesEndpoint(t *testing.T) {
	h := New(platform.NewDemoProvider(), "demo", "demo", "demo", "")
	w := request(t, h, "GET", "/api/v1/changes?namespace=payments&workload=payments-api&window=60m", "")
	if w.Code != 200 {
		t.Fatalf("changes: %d %s", w.Code, w.Body.String())
	}
	var rep platform.ChangeReport
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Changes) < 3 {
		t.Fatalf("expected several demo changes, got %d: %s", len(rep.Changes), w.Body.String())
	}
	if rep.Changes[0].Scope != "workload" {
		t.Errorf("top change should be workload-scoped: %+v", rep.Changes[0])
	}
	for i := 1; i < len(rep.Changes); i++ {
		if rep.Changes[i].Score > rep.Changes[i-1].Score {
			t.Fatalf("not sorted by score at %d", i)
		}
	}
	if w := request(t, h, "GET", "/api/v1/changes?window=bogus", ""); w.Code != 400 {
		t.Errorf("bad window: %d", w.Code)
	}
	if w := request(t, h, "GET", "/api/v1/changes?at=nope", ""); w.Code != 400 {
		t.Errorf("bad at: %d", w.Code)
	}
}
