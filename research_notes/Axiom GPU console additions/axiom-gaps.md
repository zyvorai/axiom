# Axiom GPU / NVIDIA / accelerators — codebase gap map

Scope: `/Volumes/Storage/tt/axiom` (medium thoroughness). Sources are in-repo paths (no external NVIDIA docs consulted). Dirty-tree status noted where relevant: `internal/platform/nvidia_gpu.go` and `nvidia_gpu_kubernetes.go` are **untracked** (`??`) as of this survey — partial NVIDIA work already in the working tree but not yet committed.

## What GPU fields exist on Node/Pod/types and ListAccelerators?

### Takeaway
Platform types and providers already model NVIDIA GPU inventory end-to-end (Node/Pod fields + `ListAccelerators` / `AcceleratorsReport`), including demo data and operator detection — but this is inventory-only (capacity/requests/labels), not utilization or cost.

### Cited Findings
- `Pod` carries `GPU string \`json:"gpu,omitempty"\`` documented as nvidia.com/gpu request (or MIG). — [types.go](file:///Volumes/Storage/tt/axiom/internal/platform/types.go)
- `Node` carries `GPU`, `AllocatableGPU`, `GPUProduct`, `GPUDriver`, and `MIG map[string]string` (nvidia.com/mig-* → quantity). — [types.go](file:///Volumes/Storage/tt/axiom/internal/platform/types.go)
- Dedicated report types exist: `AcceleratorsReport` (provider/status/summary/operator/nodes/pods/counts), `AcceleratorsOperator` (`gpuOperator`, `devicePlugin`, `dcgmExporter`, `clusterPolicy`, `note`), `AcceleratorNode`, `AcceleratorPod`. Comment on `AcceleratorsReport` documents intended route `GET /api/v1/accelerators`. — [types.go](file:///Volumes/Storage/tt/axiom/internal/platform/types.go)
- Constants and helpers in untracked `nvidia_gpu.go`: resource `nvidia.com/gpu`, labels `nvidia.com/gpu.product`, `nvidia.com/cuda.driver-version.full`, `nvidia.com/gpu.driver`; `applyNodeGPUFields`, `podGPURequest`, `buildAcceleratorsReport` (status `ready`/`degraded`/`missing`). — [nvidia_gpu.go](file:///Volumes/Storage/tt/axiom/internal/platform/nvidia_gpu.go)
- Kubernetes wiring (untracked, `//go:build kubernetes`): capacity/allocatable GPU+MIG maps, container request/limit fallback, DaemonSet heuristic for device-plugin/DCGM/gpu-operator, ClusterPolicy GVR `nvidia.com/v1` `clusterpolicies`, `ListAccelerators` → `buildAcceleratorsReport`. — [nvidia_gpu_kubernetes.go](file:///Volumes/Storage/tt/axiom/internal/platform/nvidia_gpu_kubernetes.go)
- Live list paths already populate fields: pods set `GPU: podGPURequest(containerGPURequests(p))`; nodes call `applyNodeGPUFields(...)`. — [kubernetes.go](file:///Volumes/Storage/tt/axiom/internal/platform/kubernetes.go)
- `Provider` interface requires `ListAccelerators`; `SwitchableProvider` and `DemoProvider` implement it (demo node `gpu-01` with 8× H100 + taint `nvidia.com/gpu`). — [provider.go](file:///Volumes/Storage/tt/axiom/internal/platform/provider.go); [switchable_provider_gen.go](file:///Volumes/Storage/tt/axiom/internal/platform/switchable_provider_gen.go); [demo.go](file:///Volumes/Storage/tt/axiom/internal/platform/demo.go); [nvidia_gpu.go](file:///Volumes/Storage/tt/axiom/internal/platform/nvidia_gpu.go)

### Inferences
- Backend inventory is substantially implemented in the dirty tree; the main missing vertical slice is HTTP + UI + docs/roadmap acknowledgment, not the data model.
- MIG is represented as capacity map / request string only; no MIG profile management or time-sliced GPU scheduling APIs.

### Gaps
- No separate GPU utilization / memory / temperature / ECC fields on Node or Pod types (inventory ≠ telemetry).
- Whether non-kubernetes stub builds compile `ListAccelerators` beyond Demo/Kubernetes was not exhaustively audited beyond the two nvidia files + interface.

---

## Is GET /api/v1/accelerators registered in internal/api?

### Takeaway
**No.** Types and providers advertise `ListAccelerators` / `GET /api/v1/accelerators`, but `internal/api` has zero accelerators handlers or mux registrations.

### Cited Findings
- Ripgrep over `internal/api` for `accelerators|Accelerators|ListAccelerators` returned **no matches**. — [internal/api](file:///Volumes/Storage/tt/axiom/internal/api)
- `server.go` registers many `/api/v1/...` routes (nodes, pods, cost, gateways, metrics/history, agent-runs, alerts, etc.) but none named accelerators. — [server.go](file:///Volumes/Storage/tt/axiom/internal/api/server.go)
- `docs/API.md` documents cost/OpenCost/`otherUsd` extensively; no accelerators endpoint section found in the same survey. — [API.md](file:///Volumes/Storage/tt/axiom/docs/API.md)
- `AcceleratorsReport` comment still claims `GET /api/v1/accelerators`. — [types.go](file:///Volumes/Storage/tt/axiom/internal/platform/types.go)

### Inferences
- Smallest next API step for a GPU console pack: thin handler calling `provider.ListAccelerators` + mux line + API.md row — platform work is already ahead of the API surface.

### Gaps
- Authz / Cache-Control / role restrictions for a future accelerators route are undefined (no handler to inspect).

---

## Any frontend GPU/Accelerators UI?

### Takeaway
**No dedicated Accelerators page/tab and no GPU columns on Nodes/PodDetail.** Cost UI omits `otherUsd` even when OpenCost breakdown exists; the only “gpu” frontend hit in NodeDetail is a taint-value placeholder.

### Cited Findings
- Frontend search for `accelerators|Accelerator|gpu|GPU|nvidia` under `frontend/**/*.{ts,tsx}` returned **no matches**; JSX pages only show incidental “gpu” / “migrate” strings. — [frontend](file:///Volumes/Storage/tt/axiom/frontend)
- `Nodes.jsx` shows `allocatableCpu`, `allocatableMemory`, `allocatablePods` — not GPU fields. — [Nodes.jsx](file:///Volumes/Storage/tt/axiom/frontend/src/pages/Nodes.jsx)
- `PodDetail.jsx` has no GPU field usage. — [PodDetail.jsx](file:///Volumes/Storage/tt/axiom/frontend/src/pages/PodDetail.jsx)
- `NodeDetail.jsx` taint form placeholder is `"gpu"` (UI copy only). — [NodeDetail.jsx](file:///Volumes/Storage/tt/axiom/frontend/src/pages/NodeDetail.jsx)
- `App.js` routes `/nodes`, `/pods`, `/ops/:tab` — no `/accelerators` route. — [App.js](file:///Volumes/Storage/tt/axiom/frontend/src/App.js)
- Ops tabs include Cost and Agents; OpenCost “where it goes” shows `cpu · mem · storage · net` only — **does not render `otherUsd`**. — [Ops.jsx](file:///Volumes/Storage/tt/axiom/frontend/src/pages/Ops.jsx)

### Inferences
- Even without a new Accelerators page, Nodes/PodDetail could surface existing JSON fields once list/detail APIs already return them from kubernetes.go.
- A full Ops → Accelerators tab would need the missing GET route first.

### Gaps
- Locales / nav / design-system copy for Accelerators not surveyed beyond absence of GPU strings.
- Dist build (`frontend/dist/`) is dirty/untracked; not treated as source of truth.

---

## How does cost handle gpuCost / otherUsd?

### Takeaway
OpenCost’s `gpuCost` (+ adjustment) is ingested server-side but **folded into `otherUsd`** with load balancer, shared, and external costs — there is **no first-class `gpuUsd`** field, and the Cost UI does not show `otherUsd` at all.

### Cited Findings
- OpenCost allocation struct unmarshals `gpuCost` / `gpuCostAdjustment`. — [cost_opencost.go](file:///Volumes/Storage/tt/axiom/internal/platform/cost_opencost.go)
- `openCostRowFrom` initially sums GPU+LB+shared+external into a local `other`, then sets `r.Other = r.Total - (CPU+Memory+Storage+Network)` (residual of totalCost). Comment: OtherUSD is GPU, load balancer, shared and external. — [cost_opencost.go](file:///Volumes/Storage/tt/axiom/internal/platform/cost_opencost.go); [types.go](file:///Volumes/Storage/tt/axiom/internal/platform/types.go) (`CostBreakdown.OtherUSD`)
- API docs: row breakdown includes `otherUsd` (GPU, load balancer, shared, external and anything else in totalCost). — [API.md](file:///Volumes/Storage/tt/axiom/docs/API.md); [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md)
- Estimate path prices **CPU/memory requests only** (flat rates); no GPU request pricing without OpenCost. — [API.md](file:///Volumes/Storage/tt/axiom/docs/API.md); [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md)
- Ops Cost table breakdown string omits `otherUsd` (and therefore any visible GPU share). — [Ops.jsx](file:///Volumes/Storage/tt/axiom/frontend/src/pages/Ops.jsx)

### Inferences
- Splitting `gpuUsd` out of `otherUsd` would be a small API/type change on top of already-parsed `GPUCost` fields; today GPU spend is invisible in the console even when OpenCost reports it.
- Budgets compare total `currentUsd` from the active cost source — GPU is inside the total when OpenCost is up, but not attributed.

### Gaps
- Exact OpenCost demo fixture GPU amounts beyond test JSON (`"gpuCost": 0` in unit fixtures) not re-run here. — [cost_opencost_test.go](file:///Volumes/Storage/tt/axiom/internal/platform/cost_opencost_test.go)

---

## Rightsizing/Prometheus — any GPU metrics?

### Takeaway
**No.** Rightsizing and Prometheus hooks are CPU/memory (cAdvisor) only; apply explicitly rejects `"gpu"`; DCGM presence is detected for accelerator inventory status but never queried.

### Cited Findings
- PromQL builders: `promCPUP95Query` → `container_cpu_usage_seconds_total`; `promMemP99Query` → `container_memory_working_set_bytes`. No DCGM / `DCGM_*` / `nvidia_*` queries in `prometheus.go`. — [prometheus.go](file:///Volumes/Storage/tt/axiom/internal/platform/prometheus.go)
- Rightsizing notes tell operators to point Prometheus at cAdvisor CPU/memory metrics. — [cost_rightsizing.go](file:///Volumes/Storage/tt/axiom/internal/platform/cost_rightsizing.go)
- Apply validation: `resources must be a non-empty subset of ["cpu", "memory"]`; test case `"unknown resource"` sets `Resources: []string{"cpu", "gpu"}`. — [cost_rightsizing_apply.go](file:///Volumes/Storage/tt/axiom/internal/platform/cost_rightsizing_apply.go); [cost_rightsizing_apply_test.go](file:///Volumes/Storage/tt/axiom/internal/platform/cost_rightsizing_apply_test.go)
- Operator detection sets `DCGMExporter` from DaemonSet name/label heuristics only (inventory signal). — [nvidia_gpu_kubernetes.go](file:///Volumes/Storage/tt/axiom/internal/platform/nvidia_gpu_kubernetes.go)
- `GET /api/v1/metrics/history` is registered (Prometheus history charts on Pod/Node/Workload per ROADMAP) — surveyed prometheus builders remain CPU/mem-oriented; no GPU metric names found in that file. — [server.go](file:///Volumes/Storage/tt/axiom/internal/api/server.go); [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md); [prometheus.go](file:///Volumes/Storage/tt/axiom/internal/platform/prometheus.go)

### Inferences
- GPU rightsizing would be a new product surface (DCGM utilization → request/time-slice guidance), not an extension of current apply path.
- Inventory can already say “DCGMExporter: true” while charts/rightsizing ignore GPU telemetry — a console gap, not a cluster gap.

### Gaps
- Full contents of untracked `internal/api/metrics_history.go` not deep-read; absence of GPU strings in `prometheus.go` is the strong signal for rightsizing/history queries.

---

## ROADMAP / GAPS mentions + adjacent dirty-tree packs

### Takeaway
**Neither `docs/GAPS.md` nor `docs/ROADMAP.md` mention GPU, NVIDIA, accelerators, or DCGM.** Adjacent 0.14 / dirty-tree work (Envoy Gateway, agents, Alertmanager, MCP, metrics history) is further along than GPU and is the natural “next pack after GPU” competition for research prioritization.

### Cited Findings
- Ripgrep of `docs/GAPS.md` and `docs/ROADMAP.md` for GPU/NVIDIA/accelerator/gpu (case-insensitive) → **no hits**. Cost/OpenCost/Prometheus rightsizing are documented without GPU. — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md); [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md)
- ROADMAP “Shipped in 0.14.0” already lists agentic ops, Agent UI, MCP, Prometheus history charts, Alertmanager, upgrade risk. “Shipped in 0.4.0” lists Envoy Gateway via Gateway API. “Next” is MCP write tools / deeper upgrade planner / translations — **not GPU**. — [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md)
- Dirty / untracked adjacent surfaces (brief):
  - **NVIDIA (this pack):** `?? internal/platform/nvidia_gpu.go`, `?? internal/platform/nvidia_gpu_kubernetes.go`
  - **Alertmanager:** `?? internal/api/alertmanager.go` (+ related platform files); mux `s.registerAlertmanager(mux)` in server.go
  - **Agents:** many `?? internal/platform/agent_*.go`; Ops `AgentsTab`; agent-runs routes registered
  - **Envoy / Gateway:** `?? deploy/k8s/envoy-gateway-example.yaml`; `GET /api/v1/gateways` registered; ROADMAP north-south Envoy already marked shipped
  - **MCP / metrics history / upgrade risk:** untracked or modified API files; routes present for metrics/history and agent-runs
  — [git status](file:///Volumes/Storage/tt/axiom); [server.go](file:///Volumes/Storage/tt/axiom/internal/api/server.go); [Ops.jsx](file:///Volumes/Storage/tt/axiom/frontend/src/pages/Ops.jsx)

### Inferences
- GPU console work is **ahead of docs** (code exists, ROADMAP/GAPS silent) and **behind HTTP/UI** relative to agents/Alertmanager which already have API registration + Ops/Alerts UI hooks.
- After a GPU inventory API+UI pack, the cheapest related cost win is exposing `otherUsd` (or split `gpuUsd`) on Ops Cost; a later pack would be DCGM Prometheus charts.
- For “next pack after GPU,” research can prefer finishing/polishing Alertmanager+agents (already mux’d) or Envoy example/docs — unless product wants GPU visibility first because platform inventory is already written.

### Gaps
- Full completeness of agent/Alertmanager/Envoy implementations (tests, Helm values, i18n) was out of scope; only enough to flag them as competing dirty-tree packs.

---

## Summary matrix (for report writer)

| Surface | Status | Evidence |
|--------|--------|----------|
| Node/Pod GPU JSON fields | **Present** (k8s list + demo) | types.go, kubernetes.go, demo.go |
| `ListAccelerators` provider | **Present** (demo + k8s; untracked nvidia files) | provider.go, nvidia_gpu*.go |
| `GET /api/v1/accelerators` | **Missing** | no internal/api matches |
| Frontend Accelerators / GPU columns | **Missing** | Nodes/PodDetail/Ops/App |
| Cost `gpuCost` → API | **Folded into `otherUsd`** (no `gpuUsd`) | cost_opencost.go, types.go |
| Cost UI shows GPU/`otherUsd` | **Missing** | Ops.jsx breakdown |
| Estimate path GPU pricing | **Missing** | API.md / GAPS estimate = CPU+mem requests |
| Rightsizing GPU | **Rejected / N/A** | cost_rightsizing_apply.go |
| Prometheus/DCGM metrics | **Detect only, no queries** | nvidia_gpu_kubernetes.go vs prometheus.go |
| ROADMAP/GAPS GPU | **Absent** | docs |
| Partial NVIDIA in dirty tree | **Yes (`??` nvidia_gpu*.go)** | git status |
