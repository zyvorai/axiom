//go:build kubernetes

package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKubernetesActiveCostOpenCost(t *testing.T) {
	now := time.Now()
	k, _ := newFakeKubernetesProvider(
		rsPod("shop", "web-6d4cf56db6-aaaaa", "ReplicaSet", "web-6d4cf56db6", now, reqContainer("app", "1", "2Gi")),
	)
	sp := NewSwitchableProvider()
	sp.Register("home", "home", k, "")
	// Nothing attached: estimate, and an attached nil client is not a source.
	if ConfigureOpenCost(sp, nil) {
		t.Fatal("nil client attached")
	}
	est, err := ActiveCost(context.Background(), sp, "30d")
	if err != nil || est.Source != CostSourceEstimate || est.TotalMonthlyUSD != 30 {
		t.Fatalf("estimate %+v %v", est, err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"data":[{
			"shop/deployment/web": {"name":"shop/deployment/web","properties":{"namespace":"shop","controllerKind":"deployment","controller":"web"},
				"minutes":10080,"cpuCost":7,"ramCost":3.5,"totalCost":10.5},
			"__idle__": {"name":"__idle__","minutes":10080,"totalCost":7}}]}`))
	}))
	defer srv.Close()
	c, _ := NewOpenCostClient(srv.URL, "")
	if !ConfigureOpenCost(sp, c) || k.opencost != c {
		t.Fatal("ConfigureOpenCost must attach the client to the active provider")
	}
	est, err = ActiveCost(context.Background(), sp, "7d")
	if err != nil || est.Source != CostSourceOpenCost || est.TotalMonthlyUSD != 45 || est.IdleMonthlyUSD != 30 {
		t.Fatalf("opencost %+v %v", est, err)
	}
	if n := est.Namespaces[0]; n.Namespace != "shop" || n.CPUCores != 1 || n.MemoryGi != 2 || *n.CPUUSD != 30 {
		t.Fatalf("row %+v", n)
	}
	if w := est.Workloads[0]; w.Workload != "web" || w.Kind != "Deployment" || w.CPUCores != 1 || w.MonthlyUSD != 45 {
		t.Fatalf("workload %+v", w)
	}

	// OpenCost going away falls back to the estimate (after the cache).
	srv.Close()
	c.mu.Lock()
	c.cache = map[string]*openCostCall{}
	c.mu.Unlock()
	est, _ = ActiveCost(context.Background(), sp, "7d")
	if est.Source != CostSourceEstimate || est.TotalMonthlyUSD != 30 || !strings.Contains(est.Note, "OpenCost") || strings.Contains(est.Note, srv.URL) {
		t.Fatalf("fallback %+v", est)
	}
}
