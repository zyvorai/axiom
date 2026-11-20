package platform

import "testing"

func TestBuildAcceleratorsReportReady(t *testing.T) {
	nodes := []Node{{
		Name: "gpu-01", Status: "Ready", GPU: "8", AllocatableGPU: "8",
		GPUProduct: "NVIDIA-H100", SharingStrategy: "none",
	}}
	pods := []Pod{{Namespace: "ml", Name: "inf-0", GPU: "1", Status: "Running"}}
	op := AcceleratorsOperator{GPUOperator: true, DevicePlugin: true, DCGMExporter: true, ClusterPolicy: "cp"}
	rep := buildAcceleratorsReport(nodes, pods, op)
	if rep.Provider != "nvidia" || rep.Status != "ready" {
		t.Fatalf("got provider=%s status=%s", rep.Provider, rep.Status)
	}
	if rep.Counts["gpus"] != 8 || rep.Counts["pods"] != 1 || rep.Counts["nodes"] != 1 {
		t.Fatalf("counts=%v", rep.Counts)
	}
	if rep.Nodes[0].SharingStrategy != "none" {
		t.Fatalf("sharing=%q", rep.Nodes[0].SharingStrategy)
	}
}

func TestFilterGPUQuotas(t *testing.T) {
	q := filterGPUQuotas([]ResourceQuotaSummary{
		{Namespace: "ml", Name: "q", Hard: map[string]string{"requests.cpu": "10", "requests.nvidia.com/gpu": "4"}, Used: map[string]string{"requests.nvidia.com/gpu": "1"}},
		{Namespace: "a", Name: "cpu-only", Hard: map[string]string{"requests.cpu": "2"}},
	})
	if len(q) != 1 || q[0].Hard["requests.nvidia.com/gpu"] != "4" {
		t.Fatalf("%+v", q)
	}
}

func TestPodGPURequestMIG(t *testing.T) {
	got := podGPURequest(map[string]int64{"nvidia.com/mig-1g.5gb": 2})
	if got != "mig-1g.5gb=2" {
		t.Fatalf("got %q", got)
	}
}

func TestStillExecuteUnknown(t *testing.T) {
	err := StillExecute(nil, NewDemoProvider(), "ml", "Deployment", "inference", StillAction("nope"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDemoListAccelerators(t *testing.T) {
	rep, err := NewDemoProvider().ListAccelerators(nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "ready" || len(rep.Nodes) == 0 || len(rep.Quotas) == 0 {
		t.Fatalf("%+v", rep)
	}
	foundPending := false
	for _, p := range rep.Pods {
		if p.SchedulingIssue != "" {
			foundPending = true
		}
	}
	if !foundPending {
		t.Fatal("expected scheduling issue on demo pending GPU pod")
	}
}
