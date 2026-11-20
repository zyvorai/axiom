# Competitor Kubernetes / private-cloud console GPU & accelerator features (late 2025–2026)

Research date: 2026-11-12. Focus: actionable console UX a k8s-native private cloud (Axiom / zyvor-axiom) could add without becoming an MLOps platform. Prefer primary vendor docs; blog posts used only when they document product UI shipped with a version.

---

## What GPU inventory, MIG, time-slicing, and operator-health views do consoles expose?

### Takeaway
Table-stakes inventory is capacity/allocatable/allocated for `nvidia.com/*` plus node readiness; OpenShift and Portainer go further with operator/device-plugin detection and DCGM overview tiles, while Lens/Headlamp differentiate on DRA device inventory (`ResourceSlice` / claims) rather than classic MIG/time-slice config UIs (those remain ClusterPolicy/ConfigMap YAML in NVIDIA docs).

### Cited Findings
- OpenShift + NVIDIA console plugin (`console-plugin-nvidia-gpu`) adds GPU panels to **Home > Overview** cluster utilization: available GPU count, per-GPU power (W), and encoder/decoder utilization percent — [NVIDIA GPU Operator OpenShift dashboard docs](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/openshift/enable-gpu-op-dashboard.html) (docs for gpu-operator 23.6.0; Helm chart from `rh-ecosystem-edge/console-plugin-nvidia-gpu`).
- OpenShift Observe dashboards can host the NVIDIA DCGM Exporter Grafana JSON via ConfigMap labels `console.openshift.io/dashboard=true` (Admin) / `odc-dashboard=true` (Developer); default graphs include temperature, power, SM clocks, GPU utilization %, framebuffer memory, tensor-core utilization — [NVIDIA OpenShift GPU monitoring dashboard](https://docs.nvidia.com/datacenter/cloud-native/openshift/latest/enable-gpu-monitoring-dashboard.html).
- MIG on OpenShift is configured via GPU Operator **ClusterPolicy** in the console Operators UI (apply MIG profiles such as `3x 2g.10gb`); docs show Installed Operators → NVIDIA GPU Operator → ClusterPolicy tab with `State:ready`, not a dedicated MIG inventory explorer — [MIG Support in OpenShift](https://docs.nvidia.com/datacenter/cloud-native/openshift/latest/mig-ocp.html).
- Time-slicing on OpenShift is ConfigMap + ClusterPolicy (`devicePlugin.config`) driven; replicas multiply advertised `nvidia.com/gpu` (example: Tesla-T4 → 8 replicas); GFD may append `-SHARED` to `nvidia.com/gpu.product`; docs verify via `oc get node ... capacity` / labels, not a first-class time-slice console editor — [Time-slicing NVIDIA GPUs in OpenShift](https://docs.nvidia.com/datacenter/cloud-native/openshift/latest/time-slicing-gpus-in-openshift.html).
- GPU Feature Discovery labels that any console can surface as inventory columns include `nvidia.com/gpu.product`, `.count`, `.memory`, `.family`, `.replicas`, `.sharing-strategy`, CUDA driver/runtime versions, `nvidia.com/mig.strategy` / `.capable`, and MIG-overridden product names — [NVIDIA k8s-device-plugin GFD README](https://github.com/NVIDIA/k8s-device-plugin/blob/main/docs/gpu-feature-discovery/README.md); label catalog also documented in [kubectl-nv labels.md](https://github.com/NVIDIA/kubectl-nv/blob/main/docs/labels.md).
- Portainer Business Edition (documented in “What's new” / GPU page; “Last updated 1 month ago” relative to fetch ≈ late 2025–2026) shows a dedicated **GPU** view with three tables: (1) per-node Ready/Degraded/Not Available + capacity/allocatable/allocated/available by `nvidia.com/*`, GPU Operator detected + Device plugin detected badges; (2) GPU Nodes; (3) GPU Workloads (pods with GPU requests, owner, node, status, scheduling issues). Cluster Details embeds a GPU summary with “View GPU details”; Dashboard tile counts GPUs — [Portainer GPU docs](https://docs.portainer.io/user/kubernetes/gpu); [Cluster Details](https://docs.portainer.io/user/kubernetes/cluster/details); [What's new 2.45](https://docs.portainer.io/whats-new); Terraform datasource mirrors fields — [portainer_kubernetes_gpu](https://library.tf/providers/portainer/portainer/latest/docs/data-sources/kubernetes_gpu).
- Lens K8S IDE **Device Resources** (auto-shown when `resource.k8s.io/v1` is served, Kubernetes **1.34+**) lists ResourceClaims, ResourceClaimTemplates, DeviceClasses, ResourceSlices; pod details show Resource Claims; node details show Devices (driver, pool, attributes, holding claim) — [Lens Device Resources](https://docs.lenshq.io/k8slens/using-lens/device-resources/).
- Headlamp **0.46.0** adds alpha DRA **Devices** section (1.34+): DeviceClasses (CEL selectors, driver settings, extended resource names), ResourceClaims (waiting/allocated/in use + devices/node), ResourceClaimTemplates — [Headlamp 0.46.0 release notes](https://github.com/kubernetes-sigs/headlamp/releases).
- Kubernetes core DRA graduated GA in **v1.34** (blog dated **2025-09-01**); `resource.k8s.io/v1` enabled by default; prioritized device lists and continuing DRA feature stream — [Kubernetes v1.34 DRA blog](https://kubernetes.io/blog/2025/09/01/kubernetes-v1-34-dra-updates/).
- NVIDIA DRA GPU driver docs describe inventory via ResourceSlices/pools for full GPU, MIG, VFIO; consumable capacity for shared full-GPU/MIG; time-slicing/MPS as in-claim strategies — [DRA Driver for NVIDIA GPUs – GPU workloads](https://dra-driver-nvidia-gpu.sigs.k8s.io/docs/guides/gpu-allocation/).
- Rancher itself does not advertise a first-party GPU inventory console in official product docs found in this pass; SUSE/Rancher staff guidance is “install NVIDIA GPU Operator; RKE/k3s are CNCF-conformant so upstream k8s GPU docs apply,” with optional Prometheus scrape of DCGM — [rancher/rancher#40692](https://github.com/rancher/rancher/issues/40692) (2023 thread; still the clearest public Rancher GPU stance). NVIDIA telemetry docs state DCGM Exporter ships by default with GPU Operator — [About GPU Telemetry](https://docs.nvidia.com/datacenter/cloud-native/gpu-telemetry/latest/).
- Community Headlamp plugins go beyond core console scope: Intel GPU plugin (node/device-plugin/allocation/pods + power via node-exporter); Inference Hub (GPU capacity + MIG/time-slicing/GPU Operator visibility as part of LLM serving UX); KAITO (GPU node provisioning + model deploy) — [headlamp-intel-gpu-plugin](https://github.com/privilegedescalation/headlamp-intel-gpu-plugin); [Inference Hub Artifact Hub](https://artifacthub.io/packages/headlamp/batuhancikrikci-headlamp-plugins/inference-hub); [Headlamp KAITO](https://kaito-project.github.io/headlamp-kaito/).

### Inferences
- For Axiom, the highest-leverage “inventory” package mirrored by Portainer + GFD labels is: resource-name breakdown (`nvidia.com/gpu`, MIG resource names, `.shared`), per-node capacity vs allocated, Operator/device-plugin health badges, and GFD product/MIG/sharing labels — without needing a MIG partition editor.
- DRA ResourceSlice/Claim views (Lens/Headlamp) are emerging table-stakes for 1.34+ clusters and differentiate from 2024-era “just show nvidia.com/gpu integers” consoles.
- True MIG/time-slice *configuration* UX is rare in general consoles; NVIDIA/OpenShift still push YAML ClusterPolicy. Surface **state** (strategy, replicas, SHARED product suffix, mig.config.state) rather than rebuild the operator configurator.

### Gaps
- No primary Rancher UI screenshot/docs page found (2026) documenting a native GPU inventory page comparable to Portainer’s; absence may mean “none,” but could also be undocumentated App Catalog + Grafana only.
- Exact OpenShift ClusterPolicy console fields for MIG (beyond “apply profile”) not fully enumerated in the fetched MIG page beyond operator-tab readiness.
- Portainer docs do not detail whether MIG resource names appear as separate `nvidia.com/mig-*` rows vs only `nvidia.com/gpu` (they say `nvidia.com/*`).

---

## What scheduling / quota / fair-share / GPU sharing UX exists?

### Takeaway
General cluster consoles mostly expose Kubernetes ResourceQuota for `requests.nvidia.com/gpu` and (newer) DRA claim state; hierarchical GPU quotas, over-quota weights, time-based fairshare, fractional GPU annotations, and preemption are concentrated in Run:ai / KAI Scheduler / Volcano / Domino hardware tiers — i.e., scheduler platforms, not generic consoles.

### Cited Findings
- OpenShift documents namespace ResourceQuota for extended resources using `requests.nvidia.com/gpu` (overcommit disallowed for extended resources); console: project **Quota** page shows hard limits / used; CLI examples enforce Forbidden when exceeding — [OCP 4.21 quotas chapter](https://docs.redhat.com/en/documentation/openshift_container_platform/4.21/html/scalability_and_performance/compute-resource-quotas); [OCP 4.18 Building applications quotas](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/building_applications/quotas) (“navigate in the web console to the project’s Quota page”).
- NVIDIA Run:ai Scheduler: per project/department **deserved quota** (GPUs/CPUs/memory per node pool), **Max GPU Device Allocation** (limit) in UI, fairshare = quota + over-quota share, preemption of over-quota preemptible workloads when rebalancing; **time-based fairshare** (Run:ai v2.24 / KAI) adjustable per node pool in UI (historical usage weight default 1.0, window default 7 days) — [Run:ai concepts](https://run-ai-docs.nvidia.com/self-hosted/platform-management/runai-scheduler/scheduling/concepts-and-principles); [Node Pools (SaaS)](https://run-ai-docs.nvidia.com/saas/platform-management/aiinitiatives/resources/node-pools); [NVIDIA blog on time-based fairshare](https://developer.nvidia.com/blog/ensuring-balanced-gpu-allocation-in-kubernetes-clusters-with-time-based-fairshare/).
- Run:ai also schedules/presents DRA claims (direct cluster YAML or via Run:ai UI/API/CLI) and accounts them in quota — same concepts doc.
- Open-source **KAI Scheduler** (powers Run:ai): hierarchical Queues (`scheduling.run.ai/v2`) with GPU quota / overQuotaWeight / limit; only leaf queues schedule; workloads labeled `kai.scheduler/queue` + `schedulerName: kai-scheduler` — [KAI queues README](https://github.com/NVIDIA/KAI-Scheduler/blob/main/docs/queues/README.md); [scheduling deep dive](https://github.com/NVIDIA/KAI-Scheduler/blob/main/docs/scheduling-deep-dive/README.md).
- KAI GPU sharing modes: Disabled, NonMemoryEnforced (default), HamiCore, NvFractions; user annotations `gpu-fraction`, `gpu-memory`, NvFractions memory request/limit; admission rejects mixing fractional + whole `nvidia.com/gpu` — [KAI gpu-sharing README](https://github.com/NVIDIA/KAI-Scheduler/blob/main/docs/gpu-sharing/README.md) (search/synth from GitHub; raw blob fetch returned empty in this session).
- Volcano: Queue with `deserved` / capability / weight (proportion or capacity plugins); DRF for multi-resource fairness including GPU; **fairshare** plugin (2026 design) tracks decayed historical resource-seconds per namespace within a queue (`halfLifeMinutes`, `targetQueues`, `resourceKey` e.g. `nvidia.com/gpu`) — orders pending jobs, optional enqueue gate, **does not preempt** running jobs; GPU memory sharing via `volcano.sh/gpu-memory` deprecated since v1.9 in favor of HAMI VGPU — [Volcano fairshare design](https://github.com/volcano-sh/volcano/blob/master/docs/design/fairshare.md); [Queue concepts v1.16](https://volcano.sh/docs/v1.16.0/concepts/queue/); [GPU sharing user guide](https://github.com/volcano-sh/volcano/blob/master/docs/user-guide/how_to_use_gpu_sharing.md); [DRF design](https://github.com/volcano-sh/volcano/blob/master/docs/design/drf.md).
- Domino: admin **Hardware Tiers** set GPU count, cents/minute, node pool; advanced settings allow custom GPU resource names including MIG (`nvidia.com/mig-1g.5gb`, etc.); users pick tiers at launch — not a raw k8s scheduler UI — [Create a hardware tier](https://docs.dominodatalab.com/en/6.2/admin_guide/c3aaf3/create-a-hardware-tier/); [Advanced hardware tier settings](https://docs.domino.ai/cloud/admin/infrastructure-and-compute/manage-compute-resources/hardware-tiers/advanced-hardware-tier-settings).
- Portainer GPU Workloads table surfaces **scheduling issues** for GPU-requesting pods (actionable console UX without owning a custom scheduler) — [Portainer GPU](https://docs.portainer.io/user/kubernetes/gpu).
- Headlamp 0.46 adds CompositePodGroup views for hierarchical gang scheduling failure diagnosis — adjacent to Volcano/batch scheduling visibility — [Headlamp 0.46 release](https://github.com/kubernetes-sigs/headlamp/releases).
- Kubecost requires `renameByDefault=true` so shared GPUs advertise `nvidia.com/gpu.shared` (else cost ambiguity); replica count × physical GPUs = advertised shared capacity — [IBM Kubecost NVIDIA GPU monitoring](https://www.ibm.com/docs/en/kubecost/self-hosted/2.x?topic=configuration-nvidia-gpu-monitoring-configurations).

### Inferences
- Axiom-appropriate scheduling UX: (1) ResourceQuota/LimitRange for `nvidia.com/gpu` and MIG names with used/hard; (2) pending GPU pods + reason (FailedScheduling / insufficient nvidia.com/gpu); (3) optional read-only views of Volcano Queues / KAI Queues / DRA claims if CRDs present — **not** implementing fairshare itself.
- Fair-share / over-quota / preemption / fractional GPU admission belong in “integrate or deep-link to KAI/Volcano/Run:ai,” not in a private-cloud console MVP.
- Differentiator vs Portainer: show **sharing mode clarity** (exclusive vs `.shared` vs MIG resource names) and which namespaces hold which slices — still inventory/quota, not ML job queues.

### Gaps
- Lens/Headlamp docs reviewed do not describe ResourceQuota GPU-specific UX beyond generic k8s objects.
- No primary Portainer doc for creating GPU quotas in-UI (only inventory/workloads).
- Domino FinOps vs Control Center cost UX is ML-platform-specific; limited transfer to Axiom except “cents per GPU-minute” as optional chargeback metadata.

---

## What utilization/metrics (DCGM, Prometheus) and cost attribution for GPUs do they show?

### Takeaway
Industry standard path is GPU Operator → DCGM Exporter → Prometheus → Grafana/console dashboards; Kubecost/OpenCost add usage-based GPU cost and idle/efficiency when `DCGM_FI_PROF_GR_ENGINE_ACTIVE` is present, with explicit holes for MIG and time-sliced shared containers.

### Cited Findings
- NVIDIA positions DCGM Exporter as the recommended Kubernetes GPU telemetry path; deployed by default with GPU Operator; scrapable by Prometheus; official Grafana dashboard ID **12239** — [About GPU Telemetry](https://docs.nvidia.com/datacenter/cloud-native/gpu-telemetry/latest/); [dcgm-exporter Helm site](https://nvidia.github.io/dcgm-exporter/).
- OpenShift embeds DCGM dashboard in Observe (metrics listed above) plus overview power/encoder tiles via console plugin — [OpenShift DCGM dashboard](https://docs.nvidia.com/datacenter/cloud-native/openshift/latest/enable-gpu-monitoring-dashboard.html); [GPU Operator dashboard](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/openshift/enable-gpu-op-dashboard.html).
- Kubecost (IBM docs, Self Hosted 2.x): requires self-managed DCGM Exporter (managed cloud DCGM unsupported); utilization metric **`DCGM_FI_PROF_GR_ENGINE_ACTIVE`** (GPU efficiency UI only when non-zero); validates also `DCGM_FI_DEV_POWER_USAGE`; **MIG unsupported** (under evaluation); time-slicing/MPS: **GPU cost shows zero** for shared containers because NVIDIA does not expose per-container util via DCGM for shared GPUs — Enterprise Savings card can still flag sharing — [Kubecost NVIDIA GPU monitoring configurations](https://www.ibm.com/docs/en/kubecost/self-hosted/2.x?topic=configuration-nvidia-gpu-monitoring-configurations).
- Kubecost 2.4 (Apptio blog): Efficiency Report adds **GPU Workload Idle** cost for requested-but-unused GPU; allocated GPU cost based on **usage**, not mere presence; OpenCost gained GPU utilization ~1.111–1.112 — [Kubecost GPU monitoring blog](https://www.apptio.com/blog/gpu-monitoring/).
- OpenCost PRs: reintroduced `gpuRequestAverage` / `gpuUsageAverage`; later swapped util metric to `DCGM_FI_PROF_GR_ENGINE_ACTIVE` for precision **and MIG support** in OpenCost’s calculation path — [opencost#2731](https://github.com/opencost/opencost/pull/2731); [opencost#2853](https://github.com/opencost/opencost/pull/2853). **Conflict note:** IBM Kubecost product docs still say MIG unsupported; OpenCost PR claims MIG support via the profiler metric — treat product vs OSS differently.
- Domino: Prometheus + Grafana; platform monitoring includes GPU utilization (memory + compute load on GPU execution nodes); Control Center / FinOps for spend by project/user/hardware tier (admin-set cents per minute) — [Platform monitoring](https://docs.domino.ai/cloud/admin/security-and-compliance/platform-monitoring); [Standard dashboards](https://docs.domino.ai/6.3/admin/operations/monitoring/grafana/dashboards); [Control Center metrics](https://docs.domino.ai/cloud/admin/operations/control-center/view-metrics-in-cc).
- NVIDIA Base Command Manager (HPC/cluster manager, not a k8s console): DCGM-based device metrics (temp, power, util, memory, ECC); job-centric `job_gpu_utilization` / `job_gpu_wasted` for allocated-but-idle GPUs; reporting for chronic underutilizers — [BCM monitoring](https://docs.nvidia.com/base-command-manager/product-manuals/admin/monitoring); [Job accounting](https://docs.nvidia.com/base-command-manager/product-manuals/admin/monitoringaccounting); [NVIDIA developer BCM overview](https://developer.nvidia.com/bright-cluster-manager).
- Community Grafana dashboard “NVIDIA GPU Operator & DCGM Observability” (ID 25617) claims coverage of DCGM metric families + GPU Operator health with workload attribution labels (`exported_namespace`, `exported_pod`, etc.) — [Grafana 25617](https://grafana.com/grafana/dashboards/25617-nvidia-gpu-operator-dcgm-observability/) (community-published; verify against operator version).

### Inferences
- Axiom should treat DCGM→Prometheus as the metrics backbone and ship: (1) cluster overview util/power/memory; (2) per-node and per-GPU drilldown; (3) optional OpenCost/Kubecost integration for namespace/workload GPU $ and idle; (4) honest empty-states when sharing/MIG metrics are missing.
- “GPU wasted while allocated” (BCM `job_gpu_wasted` / Kubecost idle) is a strong private-cloud differentiator without MLOps — maps cleanly to pod request vs DCGM util.
- Cost attribution to namespace/label is console-appropriate; FinOps budgets, cloud bill reconciliation, and project-level cents/minute catalogs are optional or outsource-to-OpenCost.

### Gaps
- No Lens official doc found for built-in DCGM charts (likely external Prometheus only).
- Headlamp core DRA docs do not claim DCGM panels; metrics appear only in third-party plugins.
- Precise Kubecost UI screens for GPU columns beyond Efficiency Report not fetched from IBM UI guides in this pass.
- OpenCost vs Kubecost MIG support contradiction unresolved without reading current OpenCost release notes end-to-end.

---

## What is clearly out of scope for a cluster console vs an ML platform?

### Takeaway
Cluster consoles own inventory, operator health, extended-resource quota, DRA claim visibility, DCGM utilization, and cost/idle signals; ML/HPC platforms own experiment/job UX, model catalogs, hardware-tier abstraction, hierarchical fairshare with preemption, fractional GPU admission policies, and job-accounting by user/initiative.

### Cited Findings
- Domino abstracts GPUs behind Hardware Tiers, workspaces/jobs/Model APIs, model execution logs, and Control Center/FinOps — [hardware tiers](https://docs.dominodatalab.com/en/6.2/admin_guide/c3aaf3/create-a-hardware-tier/); [platform monitoring](https://docs.domino.ai/cloud/admin/security-and-compliance/platform-monitoring).
- Run:ai/KAI center on projects/departments/node pools, deserved vs over-quota, preemption, time-based fairshare, fractional GPU annotations — [Run:ai concepts](https://run-ai-docs.nvidia.com/self-hosted/platform-management/runai-scheduler/scheduling/concepts-and-principles); [KAI queues](https://github.com/NVIDIA/KAI-Scheduler/blob/main/docs/queues/README.md).
- Base Command Manager emphasizes HPC workload managers, job metrics, and user accounting — not Kubernetes Admin console patterns — [BCM job accounting](https://docs.nvidia.com/base-command-manager/product-manuals/admin/monitoringaccounting).
- Headlamp Inference Hub / KAITO plugins explicitly add model catalogs, deploy wizards, playgrounds, GPU node auto-provisioning — illustrating the MLOps boundary when bolted onto a k8s UI — [Inference Hub](https://artifacthub.io/packages/headlamp/batuhancikrikci-headlamp-plugins/inference-hub); [KAITO](https://kaito-project.github.io/headlamp-kaito/).
- Portainer/Lens/OpenShift overview features stay at cluster/node/pod/operator/metrics layers without model lifecycle — [Portainer GPU](https://docs.portainer.io/user/kubernetes/gpu); [Lens DRA](https://docs.lenshq.io/k8slens/using-lens/device-resources/); [OpenShift GPU overview plugin](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/openshift/enable-gpu-op-dashboard.html).

### Inferences
**In scope for Axiom (k3s→prod private-cloud console):**
- GPU/accelerator inventory from node capacity + GFD labels + DRA ResourceSlices
- GPU Operator / device-plugin / DCGM DaemonSet health
- MIG/time-slice/sharing **status** (not full partition designer)
- Namespace ResourceQuota for GPU/MIG resources
- Pending/unschedulable GPU workloads with reasons
- DCGM utilization, power, memory, temp; idle vs requested
- Cost/efficiency via OpenCost-compatible attribution
- Optional read-only CRD browsers for Queue/ClusterPolicy/ResourceClaim

**Out of scope (defer to ML platform / specialized scheduler):**
- Model registries, fine-tune/serve wizards, playgrounds
- Project/department hierarchical fairshare with preemption policy editors
- Hardware-tier catalogs and per-run cents billing product
- Experiment tracking, dataset lineage, notebook UX
- Automatic GPU node provisioning tied to model size (KAITO-style)
- Full BCM-style HPC job accounting across Slurm+k8s
- Training NVIDIA GPUs / CUDA workload authoring itself

### Gaps
- Boundary cases (e.g., “show Volcano queue depth” vs “configure deserved GPU”) need product judgment; docs alone do not draw a hard line.
- Multi-cluster GPU fleet views appear in ML platforms more than Portainer/Lens; sparse primary docs for multi-cluster GPU inventory in general consoles.

---

## Which features differentiate vs common table-stakes? (prioritized for Axiom)

### Takeaway
Table-stakes are counting `nvidia.com/gpu` and linking to Grafana; differentiation in 2025–2026 is DRA-native device views, operator/sharing-mode health, scheduling-issue lists, usage-based GPU cost/idle, and honest MIG/shared-metric caveats — still below Run:ai-class fairshare.

### Cited Findings
- **Table-stakes (common / expected):** node GPU capacity integers; install GPU Operator; scrape DCGM; Grafana 12239; ResourceQuota `requests.nvidia.com/gpu` — [GPU Telemetry](https://docs.nvidia.com/datacenter/cloud-native/gpu-telemetry/latest/); [OCP GPU quota](https://docs.redhat.com/en/documentation/openshift_container_platform/4.21/html/scalability_and_performance/compute-resource-quotas).
- **Strong console differentiators already shipping:**
  1. Portainer-style **GPU Workloads + scheduling issues + Operator/device-plugin badges** — [Portainer GPU](https://docs.portainer.io/user/kubernetes/gpu).
  2. OpenShift **Overview GPU tiles** (count, power, encoder) without leaving Home — [console-plugin-nvidia-gpu](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/openshift/enable-gpu-op-dashboard.html).
  3. Lens/Headlamp **DRA Device Resources** (Claims, Classes, Slices on pods/nodes) post-1.34 GA — [Lens](https://docs.lenshq.io/k8slens/using-lens/device-resources/); [Headlamp 0.46](https://github.com/kubernetes-sigs/headlamp/releases); [K8s 1.34 DRA](https://kubernetes.io/blog/2025/09/01/kubernetes-v1-34-dra-updates/).
  4. Kubecost/OpenCost **GPU efficiency / idle $** from `DCGM_FI_PROF_GR_ENGINE_ACTIVE` — [Kubecost docs](https://www.ibm.com/docs/en/kubecost/self-hosted/2.x?topic=configuration-nvidia-gpu-monitoring-configurations); [Apptio 2.4 blog](https://www.apptio.com/blog/gpu-monitoring/).
  5. GFD label-rich inventory (product, MIG strategy, replicas, SHARED) — [GFD README](https://github.com/NVIDIA/k8s-device-plugin/blob/main/docs/gpu-feature-discovery/README.md).
- **Platform-tier differentiators (usually out of scope, optional integrate):** Run:ai time-based fairshare UI; KAI fractional GPU modes; Volcano fairshare plugin; Domino hardware tiers; BCM job_gpu_wasted user reports — sources above.

### Inferences
Recommended Axiom priority stack (dense):
1. **GPU fleet page** — Portainer-like tables + GFD columns + Operator/DCGM health.
2. **Sharing clarity** — exclusive / `nvidia.com/gpu.shared` / MIG resource names; warn when util metrics unavailable.
3. **DRA browser** — ResourceSlice inventory + claim lifecycle on 1.34+.
4. **Quota & pressure** — namespace GPU quota used/hard; unschedulable GPU pods.
5. **Utilization & waste** — DCGM overview + requested-vs-used idle signal; optional OpenCost.
6. **CRD awareness** — detect ClusterPolicy / KAI Queue / Volcano Queue and deep-link or read-only summarize — do not reimplement schedulers.

### Gaps
- Competitive feature matrix by license tier (Portainer BE-only GPU view) may constrain “table-stakes” perception for OSS consoles.
- No single vendor doc ranks “must-have GPU console features”; prioritization above is synthesized from what ships in primary docs as of this research date.
`)