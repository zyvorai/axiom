//go:build kubernetes

package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestK8sCostBudgetStore(t *testing.T) {
	_, cs := newFakeKubernetesProvider(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: costBudgetConfigMapName, Namespace: "axiom-system"},
		Data:       map[string]string{"other": "kept"},
	})
	s := NewK8sCostBudgetStore(cs, "axiom-system")
	testCostBudgetStore(t, s)

	cm, err := cs.CoreV1().ConfigMaps("axiom-system").Get(context.Background(), costBudgetConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var stored []CostBudget
	if err := json.Unmarshal([]byte(cm.Data[costBudgetConfigMapKey]), &stored); err != nil {
		t.Fatalf("budgets.json: %v", err)
	}
	if len(stored) != 1 || stored[0].Namespace != "b" || cm.Data["other"] != "kept" {
		t.Fatalf("stored %+v data %v", stored, cm.Data)
	}
	again, err := NewK8sCostBudgetStore(cs, "axiom-system").List(context.Background())
	if err != nil || len(again) != 1 {
		t.Fatalf("reload: %v %v", again, err)
	}
}

func TestK8sCostBudgetStoreCreatesConfigMap(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "")
	_, cs := newFakeKubernetesProvider()
	s := NewK8sCostBudgetStore(cs, "")
	if s.ns != "axiom-system" {
		t.Fatalf("default namespace %q", s.ns)
	}
	if err := s.Set(context.Background(), CostBudget{Namespace: "a", MonthlyUSD: 5, WarnPercent: 80}); err != nil {
		t.Fatal(err)
	}
	cm, err := cs.CoreV1().ConfigMaps("axiom-system").Get(context.Background(), costBudgetConfigMapName, metav1.GetOptions{})
	if err != nil || cm.Labels["app"] != "zyvor-axiom" || !strings.Contains(cm.Data[costBudgetConfigMapKey], `"namespace":"a"`) {
		t.Fatalf("configmap %+v %v", cm, err)
	}
	cm.Data[costBudgetConfigMapKey] = "{broken"
	_, _ = cs.CoreV1().ConfigMaps("axiom-system").Update(context.Background(), cm, metav1.UpdateOptions{})
	if _, err := s.List(context.Background()); err == nil {
		t.Fatal("corrupt JSON must be reported")
	}
	t.Setenv("POD_NAMESPACE", "platform")
	if s := NewK8sCostBudgetStore(cs, ""); s.ns != "platform" {
		t.Fatalf("POD_NAMESPACE: %q", s.ns)
	}
}

func rsPod(ns, name, ownerKind, ownerName string, created time.Time, containers ...corev1.Container) *corev1.Pod {
	ctrl := true
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, CreationTimestamp: metav1.NewTime(created),
			OwnerReferences: []metav1.OwnerReference{{Kind: ownerKind, Name: ownerName, Controller: &ctrl}}},
		Spec:   corev1.PodSpec{Containers: containers},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func reqContainer(name, cpu, mem string) corev1.Container {
	c := corev1.Container{Name: name, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{}}}
	if cpu != "" {
		c.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if mem != "" {
		c.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(mem)
	}
	return c
}

func rightsizingFakeProvider(t *testing.T) *KubernetesProvider {
	t.Helper()
	now := time.Now()
	k, _ := newFakeKubernetesProvider(
		rsPod("shop", "web-6d4cf56db6-aaaaa", "ReplicaSet", "web-6d4cf56db6", now.Add(-time.Hour), reqContainer("app", "1", "1Gi")),
		rsPod("shop", "web-6d4cf56db6-bbbbb", "ReplicaSet", "web-6d4cf56db6", now, reqContainer("app", "1", "1Gi")),
		rsPod("shop", "db-0", "StatefulSet", "db", now, reqContainer("db", "500m", "512Mi")),
		rsPod("shop", "job-x", "Job", "job", now, reqContainer("run", "1", "1Gi")),
		rsPod("kube-system", "agent-abcde", "DaemonSet", "agent", now, reqContainer("agent", "", "")),
	)
	return k
}

func stubPodMetrics(t *testing.T, body string, err error) *[]string {
	t.Helper()
	var seen []string
	prev := podMetricsListFetcher
	podMetricsListFetcher = func(_ context.Context, _ *KubernetesProvider, ns string) ([]byte, error) {
		seen = append(seen, ns)
		return []byte(body), err
	}
	t.Cleanup(func() { podMetricsListFetcher = prev })
	return &seen
}

const rsMetricsBody = `{"items":[
 {"metadata":{"name":"web-6d4cf56db6-aaaaa","namespace":"shop"},"containers":[{"name":"app","usage":{"cpu":"100m","memory":"200Mi"}}]},
 {"metadata":{"name":"web-6d4cf56db6-bbbbb","namespace":"shop"},"containers":[{"name":"app","usage":{"cpu":"150000000n","memory":"100Mi"}}]},
 {"metadata":{"name":"db-0","namespace":"shop"},"containers":[{"name":"db","usage":{"cpu":"480m","memory":"300Mi"}}]},
 {"metadata":{"name":"job-x","namespace":"shop"},"containers":[{"name":"run","usage":{"cpu":"1","memory":"1Gi"}}]},
 {"metadata":{"name":"agent-abcde","namespace":"kube-system"},"containers":[{"name":"agent","usage":{"cpu":"20m","memory":"30Mi"}}]}
]}`

func TestKubernetesListRightsizingSnapshot(t *testing.T) {
	k := rightsizingFakeProvider(t)
	seen := stubPodMetrics(t, rsMetricsBody, nil)
	rep, err := k.ListRightsizing(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Source != "metrics-snapshot" || rep.Confidence != "low" || rep.Window != "7d" || len(*seen) != 1 {
		t.Fatalf("report %+v", rep)
	}
	byName := map[string]RightsizingItem{}
	for _, it := range rep.Items {
		byName[it.Workload] = it
	}
	if len(rep.Items) != 3 {
		t.Fatalf("items (jobs excluded): %+v", rep.Items)
	}
	web := byName["web"]
	if web.Kind != "Deployment" || web.Replicas != 2 || web.CPU.UsageMilli != 150 || web.Memory.UsageMiB != 200 || web.CPU.RequestMilli != 1000 ||
		web.Memory.RequestMiB != 1024 || web.Verdict != "over" || web.CPU.Basis != "current" || web.MonthlyDeltaUSD >= 0 {
		t.Fatalf("web %+v", web)
	}
	if db := byName["db"]; db.Kind != "StatefulSet" || db.Verdict != "under" {
		t.Fatalf("db %+v", db)
	}
	if ag := byName["agent"]; ag.Kind != "DaemonSet" || ag.Verdict != "under" || ag.Reason != "no request set" {
		t.Fatalf("agent %+v", ag)
	}
	if rep.Items[0].Workload != "web" {
		t.Fatalf("savings first: %+v", rep.Items[0])
	}

	one, _ := k.ListRightsizing(context.Background(), "shop", "1d")
	if (*seen)[1] != "shop" || one.Window != "1d" {
		t.Fatalf("namespace passed to metrics: %v %+v", *seen, one)
	}
	if _, err := k.ListRightsizing(context.Background(), "", "9d"); HTTPStatus(err) != 400 {
		t.Fatalf("bad window: %v", err)
	}
}

func TestKubernetesListRightsizingNoMetrics(t *testing.T) {
	k := rightsizingFakeProvider(t)
	stubPodMetrics(t, "", errors.New("the server could not find the requested resource"))
	rep, err := k.ListRightsizing(context.Background(), "", "7d")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Source != "none" || len(rep.Items) != 0 || !strings.Contains(rep.Note, "metrics-server") {
		t.Fatalf("report %+v", rep)
	}
}

func TestKubernetesListRightsizingPrometheus(t *testing.T) {
	k := rightsizingFakeProvider(t)
	stubPodMetrics(t, rsMetricsBody, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "cpu") {
			// Includes an old pod of web (gone, resolved by name) and an unknown workload.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
				{"metric":{"namespace":"shop","pod":"web-5c9b8d7f6-zzzzz","container":"app"},"value":[1,"0.3"]},
				{"metric":{"namespace":"shop","pod":"web-6d4cf56db6-aaaaa","container":"app"},"value":[1,"0.2"]},
				{"metric":{"namespace":"shop","pod":"ghost-0","container":"x"},"value":[1,"5"]}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
			{"metric":{"namespace":"shop","pod":"web-6d4cf56db6-aaaaa","container":"app"},"value":[1,"314572800"]}]}}`))
	}))
	defer srv.Close()
	c, _ := NewPrometheusClient(srv.URL, "tok")
	sp := NewSwitchableProvider()
	sp.Register("home", "home", k, "")
	if !ConfigurePrometheus(sp, c) || k.prometheus != c {
		t.Fatal("ConfigurePrometheus must attach the client to the active provider")
	}
	rep, err := k.ListRightsizing(context.Background(), "", "7d")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Source != "prometheus" || rep.Confidence != "high" || len(rep.Items) != 1 {
		t.Fatalf("report %+v", rep)
	}
	if it := rep.Items[0]; it.Workload != "web" || it.CPU.UsageMilli != 300 || it.Memory.UsageMiB != 300 || it.CPU.Basis != "p95" {
		t.Fatalf("item %+v", it)
	}

	// Prometheus failing falls back to the snapshot.
	srv.Close()
	rep, _ = k.ListRightsizing(context.Background(), "", "7d")
	if rep.Source != "metrics-snapshot" || !strings.Contains(rep.Note, "Prometheus unavailable") || strings.Contains(rep.Note, srv.URL) {
		t.Fatalf("fallback %+v", rep)
	}
}
