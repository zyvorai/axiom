package platform

import (
	"context"
	"errors"
	"testing"
)

func TestOneFailedProbeDoesNotFlipAnEstablishedCluster(t *testing.T) {
	a := newFake(healthyOv())
	sp := newFleet(t, map[string]*fakeCluster{"edge": a}, "edge")
	ctx := context.Background()
	sp.ProbeAll(ctx)
	since := sp.healthOf("edge").Since

	a.setErr(errors.New("context deadline exceeded"))
	sp.ProbeAll(ctx)
	h := sp.healthOf("edge")
	if h.State != ClusterHealthy || h.Error != "" || h.Since != since {
		t.Fatalf("a single failed probe changed the cluster: %+v", h)
	}
	// A success in between resets the count: two failures that are not
	// consecutive never add up to an outage.
	a.setErr(nil)
	sp.ProbeAll(ctx)
	a.setErr(errors.New("timeout"))
	sp.ProbeAll(ctx)
	if got := sp.healthOf("edge").State; got != ClusterHealthy {
		t.Fatalf("non-consecutive failures flipped the cluster: %s", got)
	}
	// Two in a row do.
	sp.ProbeAll(ctx)
	if h := sp.healthOf("edge"); h.State != ClusterUnreachable || h.Error == "" {
		t.Fatalf("two consecutive failures should be an outage: %+v", h)
	}
	// An unreachable cluster that answers is healthy again at once.
	a.setErr(nil)
	sp.ProbeAll(ctx)
	if got := sp.healthOf("edge").State; got != ClusterHealthy {
		t.Fatalf("recovery should be immediate: %s", got)
	}
}

func TestADegradedClusterAlsoNeedsTwoFailures(t *testing.T) {
	a := newFake(Overview{Health: "Healthy", Nodes: 3, ReadyNodes: 2})
	sp := newFleet(t, map[string]*fakeCluster{"edge": a}, "edge")
	ctx := context.Background()
	sp.ProbeAll(ctx)
	a.setErr(errors.New("timeout"))
	sp.ProbeAll(ctx)
	if got := sp.healthOf("edge").State; got != ClusterDegraded {
		t.Fatalf("a degraded cluster should keep its state after one failure, got %s", got)
	}
	sp.ProbeAll(ctx)
	if got := sp.healthOf("edge").State; got != ClusterUnreachable {
		t.Fatalf("got %s", got)
	}
}

func TestAClusterThatNeverAnsweredIsUnreachableImmediately(t *testing.T) {
	a := newFake(healthyOv())
	a.setErr(errors.New("connection refused"))
	sp := newFleet(t, map[string]*fakeCluster{"edge": a}, "edge")
	sp.ProbeAll(context.Background())
	if got := sp.healthOf("edge").State; got != ClusterUnreachable {
		t.Fatalf("the first probe has nothing to hold on to: %s", got)
	}
}

func TestWarningEventsAloneDoNotDegradeACluster(t *testing.T) {
	// The Kubernetes provider reports "Attention" whenever there are warning
	// events; that is not an outage and must not page anyone.
	for status, want := range map[string]string{
		"Attention": ClusterHealthy, "Healthy": ClusterHealthy, "": ClusterHealthy,
		"Degraded": ClusterDegraded, "critical": ClusterDegraded, "Unhealthy": ClusterDegraded,
	} {
		h := classifyProbe(Overview{Health: status, Nodes: 3, ReadyNodes: 3, Warnings: 7}, nil)
		if h.State != want {
			t.Errorf("health %q: got %s, want %s", status, h.State, want)
		}
	}
	// Nodes that are not ready still degrade it, whatever the status word.
	if h := classifyProbe(Overview{Health: "Attention", Nodes: 3, ReadyNodes: 2}, nil); h.State != ClusterDegraded {
		t.Errorf("a node down must degrade the cluster: %+v", h)
	}
}
