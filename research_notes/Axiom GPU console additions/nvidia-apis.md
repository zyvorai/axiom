# NVIDIA GPU Operator / Device Plugin / MIG / Time-Slicing / DCGM / DRA — Console-Detectable Kubernetes Signals (2025–2026)

Research date: 2026-11-12. Scope: read-only inventory/observability signals a cluster console (Axiom) can detect with a service account; write/install actions called out as optional admin.

## Which ClusterPolicy / GPUFeatureDiscovery / NFD labels are stable to surface?

### Takeaway
Prefer **GFD `nvidia.com/*` inventory labels** and **Operator `nvidia.com/gpu.deploy.*` / MIG state labels** for console inventory; treat NFD `feature.node.kubernetes.io/*` and PCI vendor hints as presence detectors only, not product identity. Prefer newer `*.driver-version.*` / `*.runtime-version.*` labels over deprecated split driver/runtime labels.

### Cited Findings
- GFD publishes a documented catalog of node labels including `nvidia.com/gpu.product`, `nvidia.com/gpu.count`, `nvidia.com/gpu.memory` (MiB), `nvidia.com/gpu.family`, `nvidia.com/gpu.compute.major` / `.minor`, `nvidia.com/gpu.replicas`, `nvidia.com/gpu.mode`, `nvidia.com/gpu.clique`, `nvidia.com/gfd.timestamp`, plus CUDA driver/runtime version labels; older `nvidia.com/cuda.driver.major|minor|rev` and `nvidia.com/cuda.runtime.major|minor` are marked **Deprecated** in favor of `nvidia.com/cuda.driver-version.{major,minor,revision,full}` and `nvidia.com/cuda.runtime-version.{major,minor,full}` — [GFD README (NVIDIA/k8s-device-plugin)](https://github.com/NVIDIA/k8s-device-plugin/blob/main/docs/gpu-feature-discovery/README.md)
- GFD e2e expected label patterns also include `nvidia.com/gpu.sharing-strategy` values `[none|mps|time-slicing]`, `nvidia.com/mig.capable` `[true|false]`, and `nvidia.com/mps.capable` — [gpu-feature-discovery_test.go](https://github.com/NVIDIA/k8s-device-plugin/blob/8d8e5eda/tests/e2e/gpu-feature-discovery_test.go)
- Device plugin label catalog documents `nvidia.com/gpu.sharing-strategy` (`none` | `mps` | `time-slicing`) — [NVIDIA/k8s-device-plugin README](https://github.com/NVIDIA/k8s-device-plugin)
- GPU Operator MIG install example shows stable operational labels on nodes: `nvidia.com/gpu.present`, `nvidia.com/gpu.product`, `nvidia.com/gpu.replicas`, `nvidia.com/gpu.sharing-strategy`, `nvidia.com/mig.capable`, `nvidia.com/mig.config`, `nvidia.com/mig.config.state`, `nvidia.com/mig.strategy`, `nvidia.com/mps.capable` — [GPU Operator with MIG](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-operator-mig.html)
- NFD presence probe used by GPU Operator install docs: check for any node label key starting with `feature.node.kubernetes.io` — [Installing the NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/getting-started.html)
- `kubectl-nv` label map treats `nvidia.com/gpu.present` and `feature.node.kubernetes.io/pci-10de.present` as **hint only, never membership**; inventory fields map from GFD product/count/replicas/sharing-strategy/clique/driver/CUDA/MIG labels and Operator `nvidia.com/mig.config`, `nvidia.com/gpu.deploy.*`, `nvidia.com/gpu-driver-upgrade-state` — [NVIDIA/kubectl-nv docs/labels.md](https://github.com/NVIDIA/kubectl-nv/blob/main/docs/labels.md)
- ClusterPolicy CRD (`apiVersion` `nvidia.com/v1`, kind `ClusterPolicy`) exposes component specs including `driver`, `toolkit`, `devicePlugin`, `dcgmExporter`, `dcgm`, `gfd`, `mig`, `migManager`, `validator`, etc.; GFD defaults to enabled when `spec.gfd.enabled` is unset — [clusterpolicy_types.go](https://github.com/NVIDIA/gpu-operator/blob/d5750f2f/api/nvidia/v1/clusterpolicy_types.go)
- GPU Operator print column / status field is `.status.state` — [clusterpolicy_types.go](https://github.com/NVIDIA/gpu-operator/blob/d5750f2f/api/nvidia/v1/clusterpolicy_types.go)
- Per-component DaemonSet selectors use `nvidia.com/gpu.deploy.<component>=true` (replacing a single global `nvidia.com/gpu.present=true` selector) — [GPU Operator Release Notes](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/release-notes.html)
- GFD README still states the tool should be considered **beta until v1.0.0** and may break API before then (with a deprecation policy) — [GFD README](https://github.com/NVIDIA/k8s-device-plugin/blob/main/docs/gpu-feature-discovery/README.md)

### Inferences
- **v1 console read path (no write):** list Nodes; surface GFD product/count/memory/family/compute/driver/CUDA/replicas/sharing-strategy/mig.*; detect Operator stack via CRD `clusterpolicies.nvidia.com` + CR `cluster-policy` + DaemonSets in `gpu-operator` (or vendor NS) selected by `nvidia.com/gpu.deploy.*`.
- Prefer `nvidia.com/cuda.driver-version.full` and `nvidia.com/cuda.runtime-version.full` when present; fall back to deprecated major/minor/(rev) only for older clusters.
- Do **not** treat `nvidia.com/gpu.present` or NFD PCI `10de` alone as “GPU ready for scheduling”; require allocatable extended resources and/or healthy device-plugin pods.
- Changing ClusterPolicy / node MIG labels / time-slicing ConfigMaps is an **optional admin write**, not needed for inventory display.

### Gaps
- No single NVIDIA “stable API version” guarantee for GFD labels beyond the documented catalog and deprecation notes; GFD remains labeled beta until v1.0.0.
- Exact enum of every `nvidia.com/gpu.deploy.*` key varies by Operator version/features (sandbox, kata, nvsm, node-status-exporter, client); console should discover present keys rather than hard-code a closed set.
- Whether Axiom’s default SA can read ClusterPolicy may depend on cluster RBAC; CRD discovery via `apiextensions` also needs appropriate list/get permissions.

## How are MIG profiles exposed as resources (`nvidia.com/mig-*`)?

### Takeaway
Under **mixed** MIG strategy, each profile is an extended resource `nvidia.com/mig-<slice>g.<memory>gb` (e.g. `nvidia.com/mig-1g.10gb`); under **single**, MIG instances are advertised as `nvidia.com/gpu` and product labels carry `-MIG-<profile>` (or `-MIG-INVALID` on misconfig). Desired vs applied config is visible via `nvidia.com/mig.config` and `nvidia.com/mig.config.state` without writing.

### Cited Findings
- Device plugin `MIG_STRATEGY`: `none | single | mixed` (default `none`); mixed exposes resources of form `nvidia.com/mig-<slice_count>g.<memory_size>gb` — [NVIDIA/k8s-device-plugin](https://github.com/NVIDIA/k8s-device-plugin)
- GPU Operator MIG docs: `single` → each MIG device as `nvidia.com/gpu` (all GPUs MIG, same profile, or GFD sets product to `…-MIG-INVALID`); `mixed` → per-profile `nvidia.com/mig-1g.10gb` etc., full GPUs with MIG disabled remain `nvidia.com/gpu` — [GPU Operator with MIG](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-operator-mig.html)
- Mixed GFD labels use `MIG_TYPE=mig-<slice>g.<mem>.gb` (e.g. `mig-3g.20gb`) with keys `nvidia.com/<MIG_TYPE>.count|memory|multiprocessors|slices.*|engines.*` plus `nvidia.com/mig.strategy=mixed` — [GFD README](https://github.com/NVIDIA/k8s-device-plugin/blob/main/docs/gpu-feature-discovery/README.md)
- Single strategy overrides `nvidia.com/gpu.product` (e.g. `A100-SXM4-40GB-MIG-1g.5gb`), `gpu.count`, `gpu.memory`, and adds `gpu.slices.gi/ci`, engines, multiprocessors — [GFD README](https://github.com/NVIDIA/k8s-device-plugin/blob/main/docs/gpu-feature-discovery/README.md)
- Applying MIG is done by labeling `nvidia.com/mig.config=<profile>` (default `all-disabled`); MIG Manager sets `nvidia.com/mig.config.state` to `pending` (or `rebooting` with `WITH_REBOOT`), then `success` — [GPU Operator with MIG](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-operator-mig.html)
- From GPU Operator v26.3.0, MIG Manager can generate per-node ConfigMaps named `<node>-mig-config` with profiles (`all-disabled`, `all-enabled`, `all-1g.10gb`, `all-balanced`, etc.); older drivers may fall back to static `default-mig-parted-config` — [GPU Operator with MIG](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-operator-mig.html)
- Time-sliceable MIG resources (device plugin docs) on A100 40GB include `nvidia.com/mig-1g.5gb`, `mig-2g.10gb`, `mig-3g.20gb`, `mig-7g.40gb`; A100 80GB uses `1g.10gb` / `2g.20gb` / `3g.40gb` / `7g.80gb` — [NVIDIA/k8s-device-plugin](https://github.com/NVIDIA/k8s-device-plugin)
- With NVIDIA DRA, MIG is requested via `ResourceClaim` against DeviceClass `mig.nvidia.com` with attributes such as profile / parent UUID — not via `nvidia.com/mig-*` extended resources; EKS docs explicitly contrast device-plugin strategies vs DRA — [Amazon EKS MIG guide](https://docs.aws.amazon.com/eks/latest/userguide/device-management-nvidia-mig.html); [DRA GPU allocation](https://dra-driver-nvidia-gpu.sigs.k8s.io/docs/concepts/gpu-allocation/)

### Inferences
- **Read-only UI:** scan Node `status.allocatable` / `capacity` for keys matching `nvidia.com/gpu` and `nvidia.com/mig-*`; cross-check GFD MIG labels and `mig.config` / `mig.config.state`.
- Profile string in resource name uses `.` not `+` (e.g. `mig-1g.10gb`).
- Setting `nvidia.com/mig.config` or patching `ClusterPolicy.spec.mig.strategy` **evicts GPU clients** and is privileged write — optional admin action, not v1 inventory.

### Gaps
- Exact advertised profile set is GPU-model-specific; console should enumerate discovered capacity keys rather than assume a fixed A100/H100 table.
- Failed MIG apply may leave `mig.config.state=failed` (seen in Operator troubleshooting/issues); official docs emphasize `pending` / `rebooting` / `success` more than a full state machine table.
- DRA path does not use `nvidia.com/mig-*` extended resources; dual-stack display logic needed if both models appear in the wild (they should not coexist under Operator: ClusterPolicy vs GPUCluster exclusivity).

## Time-slicing ConfigMaps / replicas — what can UI show?

### Takeaway
UI can show **effective sharing** from node labels (`gpu.sharing-strategy=time-slicing`, `gpu.replicas`, product `-SHARED` suffix) and inflated `nvidia.com/gpu` (or `.shared`) allocatable counts; ConfigMap content and ClusterPolicy `devicePlugin.config` explain configured replica factor. Creating/patching those objects is admin write.

### Cited Findings
- Time-slicing ConfigMap schema: `sharing.timeSlicing.resources[].name` + `replicas`; flags `renameByDefault`, `failRequestsGreaterThanOne`, `flags.migStrategy` — [Time-Slicing GPUs in Kubernetes](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-sharing.html)
- With `renameByDefault: false` (default), resource name stays `nvidia.com/gpu` but GFD product label is suffixed `-SHARED` (e.g. `Tesla-T4-SHARED`); with `true`, resource becomes `nvidia.com/gpu.shared` — [GPU Operator time-slicing docs](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-sharing.html)
- `replicas` is scheduling oversubscription (slots per physical GPU), **not** memory/fault isolation — [GPU Operator time-slicing docs](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-sharing.html); [Crusoe how-to](https://support.crusoecloud.com/hc/en-us/articles/55445905068571-How-To-Enable-GPU-Time-Slicing-on-Crusoe-Managed-Kubernetes)
- Operator wiring: ConfigMap in Operator namespace + patch `ClusterPolicy` `spec.devicePlugin.config.name` / `.default` (config key, e.g. `any`) — [GPU Operator time-slicing docs](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-sharing.html)
- GFD `nvidia.com/gpu.replicas` equals physical count unless sharing multiplies by replicas; `nvidia.com/gpu.sharing-strategy` becomes `time-slicing` — [GFD README](https://github.com/NVIDIA/k8s-device-plugin/blob/main/docs/gpu-feature-discovery/README.md); [k8s-device-plugin README](https://github.com/NVIDIA/k8s-device-plugin)
- Time-slicing also applies to mixed MIG resource names (`nvidia.com/mig-*`) — [k8s-device-plugin README](https://github.com/NVIDIA/k8s-device-plugin); OpenShift examples list per-MIG replica maps — [Time-slicing on OpenShift](https://docs.nvidia.com/datacenter/cloud-native/openshift/latest/time-slicing-gpus-in-openshift.html)
- ConfigMap changes may require device-plugin DaemonSet rollout restart to pick up — [gpu-operator#1144](https://github.com/NVIDIA/gpu-operator/issues/1144)

### Inferences
- **Read-only display:** for each GPU node show `sharing-strategy`, `replicas`, physical `gpu.count`, allocatable GPU (or `.shared` / mig) counts, and whether product ends in `-SHARED`.
- If SA can read ConfigMaps in `gpu-operator` / `nvidia-gpu-operator`, show named configs and `replicas` from `devicePlugin.config` reference; namespace name varies by install.
- Do not imply isolation; UI copy should state time-slicing shares compute without memory/fault isolation (contrast MIG).

### Gaps
- No standard ConfigMap name; examples use `time-slicing-config`, `time-slicing-config-all`, `device-plugin-config`.
- Exact physical vs replica math for UI (`allocatable / replicas`) is reliable only when `gpu.count` and `gpu.replicas` agree with capacity; edge cases during plugin restart need handling.
- MPS sharing is a separate strategy (`mps`); not fully inventoried here beyond label enum.

## DCGM-Exporter metric names for utilization, memory, temp, ECC that pair with Prometheus?

### Takeaway
Prometheus scrapes DCGM-Exporter `/metrics` (commonly port **9400**). **Legacy (widely deployed) names** use `DCGM_FI_DEV_GPU_UTIL`, `DCGM_FI_DEV_FB_USED`/`_FREE`, `DCGM_FI_DEV_GPU_TEMP`, etc.; **current default-counters.csv (main branch, 2026)** renamed many fields to unit-suffixed forms (`_UTIL_RATIO`, `_BYTES`, `_CELSIUS`, `_WATTS`). ECC counters exist but are **commented out by default** — enable via custom metrics ConfigMap (admin).

### Cited Findings
- DCGM-Exporter exposes GPU metrics at HTTP `/metrics` for Prometheus; example scrape `curl localhost:9400/metrics` shows gauges with labels `gpu`, `UUID`, etc. — [DCGM Exporter docs](https://docs.nvidia.com/datacenter/cloud-native/gpu-telemetry/latest/dcgm-exporter.html)
- Documented / Helm template counters (classic naming): utilization `DCGM_FI_DEV_GPU_UTIL`, `DCGM_FI_DEV_MEM_COPY_UTIL`; memory `DCGM_FI_DEV_FB_USED`, `DCGM_FI_DEV_FB_FREE` (MiB); temp `DCGM_FI_DEV_GPU_TEMP`, `DCGM_FI_DEV_MEMORY_TEMP`; power `DCGM_FI_DEV_POWER_USAGE`; XID `DCGM_FI_DEV_XID_ERRORS`; ECC fields present but commented: `DCGM_FI_DEV_ECC_{SBE,DBE}_{VOL,AGG}_TOTAL` — [metrics-configmap.yaml](https://github.com/NVIDIA/dcgm-exporter/blob/main/deployment/templates/metrics-configmap.yaml); also mirrored in older `dcp-metrics-included.csv` style lists cited by secondary guides
- Current `etc/default-counters.csv` and `etc/dcp-metrics-included.csv` on main use renamed metrics: `DCGM_FI_DEV_GPU_UTIL_RATIO`, `DCGM_FI_DEV_MEMORY_UTIL_RATIO`, `DCGM_FI_DEV_GPU_TEMP_CELSIUS`, `DCGM_FI_DEV_MEMORY_TEMP_CELSIUS`, `DCGM_FI_DEV_FB_{CAPACITY,FREE,USED,RESERVED}_BYTES`, `DCGM_FI_DEV_BOARD_POWER_WATTS`, `DCGM_FI_DEV_XID_ERROR`, remapped rows `DCGM_FI_DEV_ROW_REMAP_*`; ECC still commented — [default-counters.csv](https://raw.githubusercontent.com/NVIDIA/dcgm-exporter/main/etc/default-counters.csv)
- DCGM Exporter publishes metrics for full GPUs and individual MIG devices — [DCGM Exporter docs](https://docs.nvidia.com/datacenter/cloud-native/gpu-telemetry/latest/dcgm-exporter.html)
- GPU Operator `dcgmExporter.enablePodLabels` / `enablePodUID` add pod dimensions; require ClusterRole grants; `enablePodUID` on GPUCluster also enables DRA attribution / ResourceSlice read — [Installing the NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/getting-started.html)
- Secondary ops guidance: ECC and throttle-violation counters oft off by default; volatile ECC resets on driver reload — [Rackworks DCGM guide](https://rackworks.io/guides/gpu-health-monitoring-with-dcgm)

### Inferences
- Console Prometheus queries should **detect which name family is present** (`DCGM_FI_DEV_GPU_UTIL` vs `DCGM_FI_DEV_GPU_UTIL_RATIO`, MiB vs bytes, `XID_ERRORS` vs `XID_ERROR`) rather than hard-coding one generation.
- Default pairing for dashboards (if classic exporter): util %, FB used/free MiB, GPU/memory temp °C, power W, XID gauge; treat ECC as optional after admin enables counters.
- Read path: Service/Pod scrape or existing Prometheus; no Operator install required if exporter already deployed. Installing/changing metrics ConfigMap is optional admin.

### Gaps
- Which metric name set ships with GPU Operator **v26.7.2**’s pinned dcgm-exporter image was not verified against that image’s embedded CSV in this pass—only upstream repo `main` + docs samples.
- Exact default ServiceMonitor labels / namespace for Operator-managed exporter vary by chart values; not exhaustively listed here.
- Mapping MIG instance labels in Prometheus series (`gpu` / `UUID` / MIG identifiers) needs sample series from a MIG node for UI join keys.

## DRA ResourceClaim / DeviceClass for NVIDIA — readiness for console support?

### Takeaway
DRA is **production-installable via GPU Operator `GPUCluster` (v26.7+)** as an alternative to device-plugin `ClusterPolicy`, requiring Kubernetes **≥1.34.2**, driver **≥580**, CDI runtime, and **mutual exclusion** with ClusterPolicy. Console can **read** DeviceClasses / ResourceSlices / ResourceClaims today; full feature parity (dynamic MIG, health, sharing) still has **alpha** gates. Treat DRA console support as **detect + inventory first**, not as default path for ClusterPolicy fleets.

### Cited Findings
- DeviceClasses: `gpu.nvidia.com` (full GPU), `mig.nvidia.com` (MIG slice), `vfio.gpu.nvidia.com` (VFIO; needs Alpha `PassthroughSupport`) — [DRA GPU allocation](https://dra-driver-nvidia-gpu.sigs.k8s.io/docs/concepts/gpu-allocation/)
- Workloads request devices via `ResourceClaim` / `ResourceClaimTemplate` with CEL selectors on attributes (e.g. `productName`, `memory` capacity); pods reference claims under `spec.resourceClaims` — [Request full GPUs](https://dra-driver-nvidia-gpu.sigs.k8s.io/docs/guides/gpu-allocation/allocating-gpus/); [Kubernetes DRA task](https://kubernetes.io/docs/tasks/configure-pod-container/assign-resources/allocate-devices-dra/)
- Kubelet plugin publishes per-node `ResourceSlice` pools under driver `gpu.nvidia.com`; MIG-enabled GPUs advertise slices (`type: mig`) instead of full GPU; inventory is snapshot at plugin start — [DRA GPU allocation](https://dra-driver-nvidia-gpu.sigs.k8s.io/docs/concepts/gpu-allocation/)
- GPU Operator DRA install: singleton `GPUCluster` named `gpu-cluster`; Helm `gpuCluster.deployCR=true` and `clusterPolicy.deployCR=false`; manages DRA driver (v0.5.1 in docs), ComputeDomains (default on), DRA validator, DCGM Exporter (default on); driver via `NVIDIADriver` or preinstalled — [DRA Driver for NVIDIA GPUs (GPU Operator)](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/dra-intro-install.html)
- Prerequisites: K8s v1.34.2+ with `resource.k8s.io` DeviceClass API; NVIDIA driver 580+; CDI-capable runtime; **no ClusterPolicy** in cluster; in-place ClusterPolicy→GPUCluster migration **not supported** — [DRA Driver for NVIDIA GPUs (GPU Operator)](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/dra-intro-install.html)
- Maturity: full GPU + existing MIG allocation GA; ComputeDomains GA; Alpha (default off): `ConsumableShares`, `DynamicMIG`, `MPSSupport`, `NVMLDeviceHealthCheck`, `PassthroughSupport`, `TimeSlicingSettings`, `DeviceMetadata` — [DRA Driver for NVIDIA GPUs (GPU Operator)](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/dra-intro-install.html)
- Managed `gpu.nvidia.com` DeviceClass does **not** set `spec.extendedResourceName` to `nvidia.com/gpu`; extended-resource compatibility needs `DRAExtendedResource` (default on in K8s ≥1.36) and/or a separate DeviceClass — [DRA Driver for NVIDIA GPUs (GPU Operator)](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/dra-intro-install.html)
- Opaque configs use CRDs under `resource.nvidia.com` (e.g. `GpuConfig` with time-slicing strategy) — [DRA API reference](https://dra-driver-nvidia-gpu.sigs.k8s.io/docs/reference/api/)
- Upstream driver repo: kubernetes-sigs/dra-driver-nvidia-gpu for K8s 1.32+ — [GitHub](https://github.com/kubernetes-sigs/dra-driver-nvidia-gpu)

### Inferences
- **v1 read-only console:** if CRDs exist, list `deviceclasses`, `resourceslices`, `resourceclaims` / templates; show DeviceClass names, claim allocation status, and ResourceSlice device counts/attributes. Detect mode via presence of `GPUCluster` vs `ClusterPolicy`.
- Do **not** assume DRA on typical ClusterPolicy clusters in 2025–2026; device plugin remains the common path.
- Installing GPUCluster / DRA driver / enabling alpha feature gates is **optional admin** (and may require a new cluster if ClusterPolicy already exists).

### Gaps
- Authoritative ResourceSlice attribute schema URL returned 404 during fetch (`…/resourceslice-device-attributes/`); attribute list should be pulled from current DRA driver docs “View available GPU resources” before implementing attribute columns.
- Behavior of dual extended-resource + DRA claiming on K8s 1.36 was documented at a high level only; console UX for mixed claim styles needs product decision.
- Exact RBAC verbs for listing cluster-scoped ResourceSlices/DeviceClasses for a console SA not specified in NVIDIA docs (standard Kubernetes RBAC).

## Operator health checklist: device-plugin, toolkit, driver, dcgm, MIG manager statuses

### Takeaway
Canonical aggregate signal is **`ClusterPolicy` `status.state == ready`** (or **`GPUCluster` status ready** on DRA stacks), but it is **insufficient alone** because disabled components still reconcile as ready. Pair with DaemonSet/pod readiness for `nvidia-device-plugin-daemonset`, `nvidia-container-toolkit-daemonset`, `nvidia-driver-daemonset` (or `NVIDIADriver`), `nvidia-dcgm` / `nvidia-dcgm-exporter`, `nvidia-mig-manager`, `gpu-feature-discovery`, and `nvidia-operator-validator`, plus node `mig.config.state` and allocatable GPU resources.

### Cited Findings
- NVIDIA aicr health-check recipe asserts: CRD `clusterpolicies.nvidia.com` Established; Deployment `gpu-operator` availableReplicas > 0; ClusterPolicy `cluster-policy` `status.state: ready`; no pods in Pending/Failed/Unknown or CrashLoop/ImagePull waiting in `gpu-operator` namespace — [health-check.yaml](https://github.com/NVIDIA/aicr/blob/main/recipes/checks/gpu-operator/health-check.yaml)
- Same recipe notes validator DaemonSet runs sequential init containers (driver, toolkit, cuda, plugin) before Running; ClusterPolicy ready is the post-reconcile signal those passed — [health-check.yaml](https://github.com/NVIDIA/aicr/blob/main/recipes/checks/gpu-operator/health-check.yaml)
- Typical Operator pods on MIG-capable nodes include: `gpu-feature-discovery`, `nvidia-container-toolkit-daemonset`, `nvidia-cuda-validator` (Completed), `nvidia-dcgm-exporter`, `nvidia-device-plugin-daemonset`, `nvidia-driver-daemonset`, `nvidia-mig-manager`, `nvidia-operator-validator`, NFD workers — [GPU Operator with MIG](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-operator-mig.html)
- DaemonSets select nodes via `nvidia.com/gpu.deploy.<component>=true` (examples: `device-plugin`, `gpu-feature-discovery`, `dcgm`, `dcgm-exporter`, `mig-manager`, `driver`, `container-toolkit`, `operator-validator`) — [Release Notes](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/release-notes.html); [MIG troubleshooting issues](https://github.com/NVIDIA/gpu-operator/issues/646)
- Preinstalled drivers: Operator may label `nvidia.com/gpu.deploy.driver=pre-installed` and skip driver DaemonSet — [Release Notes](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/release-notes.html)
- Device plugin marks GPUs unhealthy on XidCriticalError and removes them from Allocatable; check `nvidia-device-plugin-daemonset` logs — [Troubleshooting the NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/troubleshooting.html)
- Intentional `spec.devicePlugin.enabled=false` can leave ClusterPolicy `status.state=ready` while device-plugin DaemonSet is gone — operational pitfall documented in field troubleshooting — [EKS GPU troubleshooting write-up](https://sirzzang.github.io/kubernetes/Kubernetes-EKS-GPU-TroubleShooting-03-01-GPU-Pod-Pending/)
- MIG health labels: desired `nvidia.com/mig.config`, applied state `nvidia.com/mig.config.state` ∈ {`pending`, `rebooting`, `success`, and failure paths}; product may show `-MIG-INVALID` — [GPU Operator with MIG](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-operator-mig.html)
- GPUCluster path: `kubectl get gpucluster gpu-cluster` STATUS `ready`; driver readiness via `NVIDIADriver` CR, not ClusterPolicy driver DaemonSet; DRA validator uses ResourceClaim — [DRA Driver for NVIDIA GPUs](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/dra-intro-install.html); [OpenShift DRA install](https://docs.nvidia.com/datacenter/cloud-native/openshift/26.7/dra-gpu-ocp.html)
- Disable all operands on a node: `nvidia.com/gpu.deploy.operands=false`; disable driver only: `nvidia.com/gpu.deploy.driver=false` — [Installing the NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/getting-started.html)

### Inferences
- **Read-only checklist for Axiom v1:**
  1. CRDs: `clusterpolicies.nvidia.com` and/or `gpuclusters.nvidia.com` (+ `nvidiadrivers` if DRA).
  2. CR status: `cluster-policy` / `gpu-cluster` → `ready`.
  3. Per GPU node: `gpu.present` / NFD PCI hint; `gpu.deploy.*` expected true for enabled components.
  4. DaemonSet desired/ready scheduled count vs nodes with matching deploy labels.
  5. Pod phases for toolkit, device-plugin, driver (or NVIDIADriver), dcgm-exporter, mig-manager, GFD, validator.
  6. Node allocatable `nvidia.com/gpu` and/or `nvidia.com/mig-*` (device plugin) **or** ResourceSlice presence (DRA).
  7. `mig.config.state` and product not `*-MIG-INVALID`.
  8. Optional: Prometheus DCGM XID / temp / ECC once scraper configured.
- Installing/upgrading Operator, patching ClusterPolicy, labeling MIG profiles, or driver upgrades are **optional admin writes**.

### Gaps
- Full machine-readable `ClusterPolicy.status.conditions` schema / per-component condition types were not exhaustively extracted from the CRD OpenAPI in this pass (secondary DeepWiki mentions conditions; primary health recipes key off `status.state`).
- Namespace is conventionally `gpu-operator` but OpenShift/vendor installs use `nvidia-gpu-operator` — console should search by label/CRD, not only by namespace name.
- Validator pod “Completed” vs “Running” semantics differ by Operator version; UI should accept both healthy terminal states for one-shot validators.
