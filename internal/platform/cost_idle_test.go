package platform

import (
	"context"
	"strconv"
	"testing"
)

func idleFind(rep IdleReport, kind, ns, name string) *IdleFinding {
	for i := range rep.Findings {
		f := &rep.Findings[i]
		if f.Kind == kind && f.Namespace == ns && f.Name == name {
			return f
		}
	}
	return nil
}

func TestDetectIdleFakeLists(t *testing.T) {
	in := IdleInput{
		PodRefsKnown: true,
		Volumes: []Volume{
			{Namespace: "a", Name: "used", Status: "Bound", Capacity: "10Gi", Age: "30d"},
			{Namespace: "a", Name: "orphan", Status: "Bound", Capacity: "100Gi", Age: "30d"},
			{Namespace: "a", Name: "pending", Status: "Pending", Capacity: "5Gi", Age: "9d"},
			{Namespace: "a", Name: "young", Status: "Bound", Capacity: "5Gi", Age: "2d"},
		},
		Pods: []Pod{{Namespace: "a", Name: "p", PVCs: []string{"used"}, ConfigMaps: []string{"cfg-used"}, Secrets: []string{"sec-used"}}},
		Namespaces: []Namespace{
			{Name: "a", Status: "Active", Age: "50d"},
			{Name: "empty", Status: "Active", Age: "50d"},
			{Name: "default", Status: "Active", Age: "50d"},
			{Name: "kube-system", Status: "Active", Age: "50d"},
			{Name: "newempty", Status: "Active", Age: "1d"},
		},
		Workloads: []Workload{
			{Namespace: "a", Name: "zero", Kind: "Deployment", Desired: 0, Age: "20d"},
			{Namespace: "a", Name: "up", Kind: "Deployment", Desired: 2, Age: "20d"},
			{Namespace: "a", Name: "sts0", Kind: "StatefulSet", Desired: 0, Age: "20d"},
		},
		Services: []Service{
			{Namespace: "a", Name: "lb-dead", Type: "LoadBalancer", Age: "10d"},
			{Namespace: "a", Name: "lb-ok", Type: "LoadBalancer", Age: "10d", Endpoints: []string{"p"}},
			{Namespace: "a", Name: "cip", Type: "ClusterIP", Age: "10d"},
		},
		Jobs: []JobSummary{
			{Namespace: "a", Name: "done", Status: "Complete", Age: "8d"},
			{Namespace: "a", Name: "bad", Status: "Failed", Age: "8d"},
			{Namespace: "a", Name: "run", Status: "Running", Age: "8d"},
			{Namespace: "a", Name: "fresh", Status: "Complete", Age: "3d"},
		},
		ConfigMaps: []ConfigMapSummary{
			{Namespace: "a", Name: "cfg-used", Age: "40d"}, {Namespace: "a", Name: "cfg-old", Age: "40d"},
			{Namespace: "a", Name: "kube-root-ca.crt", Age: "40d"},
		},
		Secrets: []SecretSummary{
			{Namespace: "a", Name: "sec-used", Type: "Opaque", Age: "40d"}, {Namespace: "a", Name: "sec-old", Type: "Opaque", Age: "40d"},
			{Namespace: "a", Name: "sh.helm.release.v1.x", Type: "helm.sh/release.v1", Age: "40d"},
		},
	}
	rep := DetectIdle(in, "", 0)
	if rep.MinAgeDays != 7 {
		t.Fatalf("default min age: %d", rep.MinAgeDays)
	}
	want := []struct{ kind, ns, name, conf string }{
		{IdleKindPVC, "a", "orphan", "medium"},
		{IdleKindPVC, "a", "pending", "high"},
		{IdleKindNamespace, "", "empty", "medium"},
		{IdleKindService, "a", "lb-dead", "medium"},
		{IdleKindJob, "a", "done", "high"},
		{IdleKindJob, "a", "bad", "medium"},
		{IdleKindDeployment, "a", "zero", "low"},
		{IdleKindConfigMap, "a", "cfg-old", "low"},
		{IdleKindSecret, "a", "sec-old", "low"},
	}
	for _, w := range want {
		f := idleFind(rep, w.kind, w.ns, w.name)
		if f == nil || f.Confidence != w.conf || f.Suggestion == "" || f.Reason == "" {
			t.Errorf("missing/wrong finding %+v: %+v", w, f)
		}
	}
	if rep.Summary.Total != len(want) {
		t.Errorf("total %d, want %d: %+v", rep.Summary.Total, len(want), rep.Findings)
	}
	for _, no := range [][3]string{
		{IdleKindPVC, "a", "used"}, {IdleKindPVC, "a", "young"}, {IdleKindNamespace, "", "default"},
		{IdleKindNamespace, "", "kube-system"}, {IdleKindNamespace, "", "a"}, {IdleKindNamespace, "", "newempty"},
		{IdleKindService, "a", "lb-ok"}, {IdleKindService, "a", "cip"}, {IdleKindJob, "a", "run"}, {IdleKindJob, "a", "fresh"},
		{IdleKindDeployment, "a", "up"}, {IdleKindDeployment, "a", "sts0"}, {IdleKindConfigMap, "a", "cfg-used"},
		{IdleKindConfigMap, "a", "kube-root-ca.crt"}, {IdleKindSecret, "a", "sec-used"}, {IdleKindSecret, "a", "sh.helm.release.v1.x"},
	} {
		if f := idleFind(rep, no[0], no[1], no[2]); f != nil {
			t.Errorf("unexpected finding %+v", f)
		}
	}
	// 100Gi and 5Gi at the default $0.10/GiB.
	if f := idleFind(rep, IdleKindPVC, "a", "orphan"); f == nil || f.MonthlyUSD == nil || *f.MonthlyUSD != 10 {
		t.Errorf("orphan cost: %+v", f)
	}
	if rep.Summary.MonthlySavingsUSD != 10.5 {
		t.Errorf("savings %v", rep.Summary.MonthlySavingsUSD)
	}
	if rep.Findings[0].Name != "orphan" {
		t.Errorf("most expensive first, got %s", rep.Findings[0].Name)
	}

	// Namespace filter and a larger min age.
	if r := DetectIdle(in, "a", 25); idleFind(r, IdleKindJob, "a", "done") != nil || idleFind(r, IdleKindPVC, "a", "orphan") == nil || idleFind(r, IdleKindNamespace, "", "empty") != nil {
		t.Errorf("filtered report: %+v", r.Findings)
	}

	// Without pod reference data nothing is claimed about mounts or refs.
	in.PodRefsKnown = false
	r := DetectIdle(in, "", 0)
	if idleFind(r, IdleKindPVC, "a", "orphan") != nil || idleFind(r, IdleKindConfigMap, "a", "cfg-old") != nil || idleFind(r, IdleKindPVC, "a", "pending") == nil {
		t.Errorf("no-ref report: %+v", r.Findings)
	}
}

func TestDetectIdleOwnNamespaceProtected(t *testing.T) {
	t.Setenv("AXIOM_NAMESPACE", "axiom-own")
	rep := DetectIdle(IdleInput{Namespaces: []Namespace{{Name: "axiom-own", Status: "Active", Age: "90d"}}}, "", 1)
	if rep.Summary.Total != 0 {
		t.Fatalf("%+v", rep.Findings)
	}
}

func TestDetectIdleCapsPerKind(t *testing.T) {
	var jobs []JobSummary
	for i := 0; i < idleMaxPerKind+5; i++ {
		jobs = append(jobs, JobSummary{Namespace: "a", Name: "j" + strconv.Itoa(i), Status: "Complete", Age: "9d"})
	}
	rep := DetectIdle(IdleInput{Jobs: jobs}, "", 0)
	if !rep.Truncated || len(rep.Findings) != idleMaxPerKind || rep.Summary.Total != idleMaxPerKind+5 {
		t.Fatalf("truncated=%v len=%d total=%d", rep.Truncated, len(rep.Findings), rep.Summary.Total)
	}
}

func TestStorageRateEnv(t *testing.T) {
	t.Setenv("AXIOM_COST_STORAGE_RATE", "0.5")
	if costStorageRatePerGi() != 0.5 {
		t.Fatal("override ignored")
	}
	t.Setenv("AXIOM_COST_STORAGE_RATE", "-1")
	if costStorageRatePerGi() != costStorageRatePerGiDefault {
		t.Fatal("negative accepted")
	}
}

func TestIdleResourcesDemo(t *testing.T) {
	rep, err := IdleResources(context.Background(), NewDemoProvider(), "", 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range [][3]string{
		{IdleKindNamespace, "", "sandbox-old"}, {IdleKindService, "labs", "preview-lb"},
		{IdleKindJob, "commerce", "migrate-schema-7731"}, {IdleKindDeployment, "labs", "legacy-worker"},
	} {
		if idleFind(rep, w[0], w[1], w[2]) == nil {
			t.Errorf("demo missing %v: %+v", w, rep.Findings)
		}
	}
	if idleFind(rep, IdleKindPVC, "data", "postgres-data") != nil {
		t.Error("mounted demo PVC flagged")
	}
	if rep.Summary.MonthlySavingsUSD <= 0 {
		t.Errorf("savings %v", rep.Summary.MonthlySavingsUSD)
	}
}
