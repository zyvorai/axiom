package platform

import "testing"

func TestUniqueNonEmpty(t *testing.T) {
	got := uniqueNonEmpty([]string{" a ", "", "a", "b", " b "})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("%v", got)
	}
}

func TestRunProbeMatrixAxiomOnly(t *testing.T) {
	d := NewDemoProvider()
	res := RunProbeMatrix(nil, d, Integrations{ProbeURL: "http://example.invalid"}, ProbeMatrixRequest{})
	if len(res.Cells) != 1 {
		t.Fatalf("cells=%d", len(res.Cells))
	}
	if res.Cells[0].From != "axiom" {
		t.Fatalf("from=%s", res.Cells[0].From)
	}
}
