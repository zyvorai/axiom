package platform

import (
	"context"
	"reflect"
	"sort"
	"testing"
)

// Kubernetes mode always serves requests through a SwitchableProvider, so an optional capability
// (a method that is not part of the Provider interface and is found by a type assertion) that
// exists on the concrete providers but not on SwitchableProvider is silently reported as
// "not available" on every real cluster. The demo provider implements every optional
// capability, so it is the reference list.
//
// switchableExempt lists demo-only methods that are deliberately not forwarded.
var switchableExempt = map[string]string{
	// Its dispatcher (ListPolicyCoverage) unwraps the SwitchableProvider itself.
	"ListPolicyCoverage": "dispatcher unwraps the switchable provider",
}

func TestSwitchableProviderForwardsEveryOptionalCapability(t *testing.T) {
	provider := reflect.TypeOf((*Provider)(nil)).Elem()
	demo := reflect.TypeOf(&DemoProvider{})
	sw := reflect.TypeOf(&SwitchableProvider{})

	var missing []string
	for i := 0; i < demo.NumMethod(); i++ {
		name := demo.Method(i).Name
		if _, inProvider := provider.MethodByName(name); inProvider {
			continue // required by the Provider interface, so SwitchableProvider must already have it
		}
		if _, ok := switchableExempt[name]; ok {
			continue
		}
		if _, ok := sw.MethodByName(name); !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("SwitchableProvider does not forward %d optional capabilit(ies) that DemoProvider implements; on a real cluster these features would report \"not available\": %v", len(missing), missing)
	}
}

// The behaviour behind the parity guard: through the SwitchableProvider (what Kubernetes mode
// serves) the optional views return the wrapped provider's data, not "not available".
func TestSwitchableProviderServesOptionalViews(t *testing.T) {
	sp := NewSwitchableProvider()
	sp.Register("demo", "Demo", NewDemoProvider(), "")
	ctx := context.Background()

	es, err := ListExternalSecretsHealth(ctx, sp, "")
	if err != nil || !es.Available || es.Rollup.Total == 0 {
		t.Fatalf("external secrets health via switchable: available=%v total=%d err=%v note=%q", es.Available, es.Rollup.Total, err, es.Note)
	}
	vap, err := ListValidatingAdmissionPolicies(ctx, sp)
	if err != nil || !vap.Available || len(vap.Policies) == 0 {
		t.Fatalf("admission policies via switchable: available=%v n=%d err=%v", vap.Available, len(vap.Policies), err)
	}
	if _, err := sp.WorkloadVolumes(ctx, "default", "web"); err != nil {
		t.Fatalf("workload volumes via switchable: %v", err)
	}
}
