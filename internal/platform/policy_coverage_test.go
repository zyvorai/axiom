package platform

import (
	"context"
	"testing"
)

func TestDemoPolicyCoverage(t *testing.T) {
	d := NewDemoProvider()
	rep, err := d.ListPolicyCoverage(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Available || len(rep.Policies) == 0 {
		t.Fatalf("expected demo policy rows: %+v", rep)
	}
	foundFail := false
	for _, p := range rep.Policies {
		if p.Counts.Fail > 0 {
			foundFail = true
			if p.FailRate <= 0 {
				t.Fatalf("failRate should be > 0 for %+v", p)
			}
		}
	}
	if !foundFail {
		t.Fatal("expected at least one failing policy in demo coverage")
	}
}
