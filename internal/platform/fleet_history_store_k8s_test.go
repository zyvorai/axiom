//go:build kubernetes

package platform

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestK8sFleetHistoryStoreRoundTrip(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	a, b := newFake(healthyOv()), newFake(healthyOv())
	sp, clk := newHistFleet(t, map[string]*fakeCluster{"a": a, "b": b}, "a", "b")
	probeFor(sp, clk, time.Hour, 30*time.Second)
	b.setErr(errors.New("connection refused"))
	probeFor(sp, clk, 10*time.Minute, 30*time.Second)

	storeA := NewK8sFleetHistoryStore(cs, "axiom", "zyvor-axiom-7c9d-a")
	if !storeA.Persistent() {
		t.Fatal("the ConfigMap store is persistent")
	}
	if err := sp.saveFleetHistory(context.Background(), storeA); err != nil {
		t.Fatal(err)
	}
	cm, err := cs.CoreV1().ConfigMaps("axiom").Get(context.Background(), FleetHistoryConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := cm.Data["history-zyvor-axiom-7c9d-a"]
	if !ok || len(cm.Data) != 1 || cm.Labels["app"] != "zyvor-axiom" {
		t.Fatalf("configmap %+v", cm.ObjectMeta)
	}
	if _, err := base64.StdEncoding.DecodeString(raw); err != nil {
		t.Fatalf("not base64: %v", err)
	}

	// A second replica writes its own key next to the first (the update is
	// retried after a conflict) and both are loaded.
	var conflicts atomic.Int32
	cs.PrependReactor("update", "configmaps", func(clienttesting.Action) (bool, runtime.Object, error) {
		if conflicts.Add(1) == 1 {
			return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, FleetHistoryConfigMapName, errors.New("stale"))
		}
		return false, nil, nil
	})
	storeB := NewK8sFleetHistoryStore(cs, "axiom", "zyvor-axiom-7c9d-b")
	if err := storeB.Save(context.Background(), FleetHistorySnapshot{Version: 1, SavedAt: clk.Now().Unix(),
		Clusters: map[string][]FleetHistoryBucket{"b": {bkt(clk.Now().Unix()-45, ClusterUnreachable, 1, 1)}}}); err != nil {
		t.Fatal(err)
	}
	if conflicts.Load() < 2 {
		t.Fatalf("conflict not retried: %d", conflicts.Load())
	}
	snaps, err := storeA.Load(context.Background())
	if err != nil || len(snaps) != 2 {
		t.Fatalf("load %d %v", len(snaps), err)
	}

	// A fresh process restores the identical downsampled data.
	want := sp.fleetHistorySnapshot()
	sp2 := newFleet(t, map[string]*fakeCluster{"a": a, "b": b}, "a", "b")
	sp2.fleet().now = clk.Now
	sp2.fleet().interval = 30 * time.Second
	if _, err := sp2.RestoreFleetHistory(context.Background(), NewK8sFleetHistoryStore(cs, "axiom", "zyvor-axiom-7c9d-c")); err != nil {
		t.Fatal(err)
	}
	got := sp2.fleetHistorySnapshot()
	if !reflect.DeepEqual(want.Clusters["a"], got.Clusters["a"]) {
		t.Fatalf("cluster a differs after the round trip")
	}
	// b merges replica B's extra bucket into its last bucket (same 5 minutes).
	var wantChecks, gotChecks int
	for _, x := range want.Clusters["b"] {
		wantChecks += x.Checks
	}
	for _, x := range got.Clusters["b"] {
		gotChecks += x.Checks
	}
	if gotChecks != wantChecks+1 || len(got.Clusters["b"]) != len(want.Clusters["b"]) {
		t.Fatalf("b: %d checks in %d buckets, want %d+1 in %d", gotChecks, len(got.Clusters["b"]), wantChecks, len(want.Clusters["b"]))
	}

	// An empty namespace has nothing to restore.
	if snaps, err := NewK8sFleetHistoryStore(cs, "elsewhere", "x").Load(context.Background()); err != nil || len(snaps) != 0 {
		t.Fatalf("empty: %v %v", snaps, err)
	}
}

func TestOpenFleetHistoryStoreDemoMode(t *testing.T) {
	if OpenFleetHistoryStore("demo", nil).Persistent() {
		t.Fatal("demo mode must not claim durability")
	}
}
