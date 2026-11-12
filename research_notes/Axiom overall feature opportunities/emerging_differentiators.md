# Emerging differentiators for a k8s-native private-cloud console (late 2026)

Research date: 2026-11-12. Scope: console UX + API ideas Axiom can implement on top of Kubernetes APIs / CRD detection. Excludes Metal3-first, multi-runtime migration paths, browser TCP port-forward, ClusterRole write UI, and Run:ai fairshare / MLOps catalogs. Builds on prior local reports under `reports/`.

---

## What uncommon but high-signal console feats are rare in Rancher / OpenShift / Lens / Portainer?

### Takeaway
Competitors converge on inventory CRUD, YAML editors, Prometheus tiles, and (for OpenShift) plugin-heavy networking/security; they under-serve containment theatre, operator-presence memory, fleet twins, data-plane-from-tenant-pod truth, weird operational findings, and ProposeOnly remediation narratives—surfaces Axiom’s Still/Seal/PacketWolf/fleet seams already point at.

### Cited Findings
- Most general consoles emphasize inventory, terminals, Helm, and metrics; Portainer marketing itself notes that even Rancher/Spectro/Portainer evaluations lacked a unified fleet-wide application view out of the box until Portainer’s fleet management was called out as a differentiator — [Portainer evaluation writeup](https://medium.com/@portainerio/why-we-won-inside-an-industry-giants-evaluation-of-container-management-platforms-d1d380d67372).
- Lens differentiates as a local IDE (multi-context, terminal, extensions, DRA Device Resources on `resource.k8s.io/v1`) rather than as a private-cloud ops theatre with containment vocabulary or presence-aware blast radius — [Lens Device Resources](https://docs.lenshq.io/k8slens/using-lens/device-resources/); [Mirantis 2025 tools roundup](https://www.mirantis.com/blog/the-best-kubernetes-management-tools-of-2025/).
- OpenShift ships rich plugins (e.g. Connectivity Link / Gateway API console views with TLSPolicy via cert-manager; NVIDIA DCGM overview tiles) but as a full platform stack, not a thin k3s→production console with Still/Seal-style containment UX — [Connectivity Link GA](https://developers.redhat.com/articles/2025/01/23/red-hat-connectivity-link-now-generally-available); [OpenShift GPU dashboard](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/openshift/enable-gpu-op-dashboard.html).
- Portainer Business documents a dedicated GPU inventory page (capacity/allocatable/allocated, Operator/device-plugin detection, GPU workloads) but not sealed-twin fleet search, operator-presence timelines, or PacketWolf-style flow verification after isolate — [Portainer GPU docs](https://docs.portainer.io/user/kubernetes/gpu).
- Community GUI pain points emphasize cluster switching, restricted-user RBAC, and Helm upgrade friction—not containment ladders, twin diffs, or “who poked it” presence merge — [r/kubernetes GUI thread](https://www.reddit.com/r/kubernetes/comments/1evv6nn/what_features_do_you_miss_in_k8s_gui_tools/).
- Prior Axiom research packs (Still blast radius, sealed twin search, who-poked-it Timeline, tenant-pod reachability, finalizer ghosts, cert “will break at”, secret age hygiene, HPA thrash, Ingress-with-zero-endpoints) are explicitly framed as rare vs inventory/CRUD consoles — local report [Axiom weird k8s feats.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20weird%20k8s%20feats.md).
- AIOps/remediation products emerging in 2026 (Kubernaut, Kentinel, KubeManta, KubeAstra) converge on approval-gated agent remediations and RCA chat—adjacent to Axiom ProposeOnly agents but not shipped as first-class identity inside Rancher/Lens/Portainer base consoles — [Kubernaut](https://kubernaut.ai/); [Kentinel](https://github.com/emreoztoprak/kentinel); [KubeManta](https://kubemanta.com/); [KubeAstra](https://kubeastra.io/).

### Inferences
- **High-signal uncommon feats (concrete bullets competitors under-serve):**
  - **Live operator blast radius** before seal/sleep/end: count open terminals/pod windows on target objects.
  - **Containment ladder UX** (hush → sleep → seal → end) with preview of surviving volumes and post-seal flow receipt.
  - **Sealed / intent twin search + side-by-side twin diff** across fleet clusters for same workload name.
  - **“Who poked it” Timeline**: merge console presence into cluster event stream.
  - **Tenant-pod reachability matrix**: probe from chosen pods to Atlas/IdP/URL (data-plane truth, not console→service ping).
  - **Weird findings feed**: finalizer ghosts, mutable-tag vs running digest, HPA thrash, Service+Ingress with 0 ready endpoints, node clock skew hints.
  - **Cert/secret hygiene deadlines** as Timeline/Alert first-class objects (not only Grafana).
  - **Upgrade-risk code → ProposeOnly prep pack** (e.g. `pdb-blocks-drain` → cordon/scale allowlist).
  - **Outage narrative composer**: fleet incident window × Timeline × AgentRun plans.
- Table-stakes gaps Axiom still owes (Portainer/OpenShift/Lens class, not unique): accelerators API/UI, DCGM tiles, DRA browser — already documented in GPU report; closing them removes competitive miss but is not the differentiator story — local [Axiom GPU console additions.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20GPU%20console%20additions.md).

### Gaps
- No systematic 2026 feature matrix scraped from current Rancher Manager / OpenShift Console / Lens Pro / Portainer BE release notes for every CRD listed below; vendor contrast here is selective (docs + prior Axiom reports), not exhaustive product-audit evidence that “feature X is absent from all four.”

---

## What CNCF / SIG projects are maturing that deserve first-class console UX?

### Takeaway
Late-2026 maturity clusters around Gateway API Standard-channel depth, DRA GA, Kyverno graduation, cert-manager graduation, storage mutability (VolumeAttributesClass GA) + CSI snapshot/clone/group-snapshot, Velero CSI path, OpenCost idle/allocation APIs, vCluster multi-tenancy, and Sigstore/SLSA verification at admission—each maps to detect-CRDs → inventory/status UX → limited ProposeOnly actions, not full vendor control planes.

### Cited Findings

#### Networking — Gateway API + Envoy / conformant controllers
- Gateway API v1.4 (Oct 2025) moved BackendTLSPolicy, `supportedFeatures` on GatewayClass status, and named route rules to Standard; experimental Mesh / Default Gateways / externalAuth — [Kubernetes blog Gateway API 1.4](https://kubernetes.io/blog/2025/11/06/gateway-api-v1-4/).
- Gateway API v1.5 (Feb 2026) promoted ListenerSet, TLSRoute, HTTPRoute CORS, client cert validation, certificate selection for TLS origination, and ReferenceGrant to Standard — [Kubernetes blog Gateway API 1.5](https://kubernetes.io/blog/2026/04/21/gateway-api-v1-5/).
- Gateway API v1.6 RC graduated TCPRoute and UDPRoute to GA (`v1`) — [gateway-api v1.6.0-rc.1](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.6.0-rc.1).
- Conformant implementations include Envoy Gateway, Cilium, Istio, kgateway, NGINX Gateway Fabric, Traefik, among others — [Gateway API implementations list](https://gateway-api.sigs.k8s.io/docs/implementations/list/).
- cert-manager can mint TLS for Gateway listeners via annotations (`cert-manager.io/issuer` / `cluster-issuer`); ListenerSet support noted in newer cert-manager betas — [cert-manager Gateway usage](https://cert-manager.io/docs/usage/gateway/).
- OpenShift Connectivity Link adds console-integrated Gateway + policy (TLS/auth/rate-limit) views—shows the UX bar Axiom can match in a thinner form — [Connectivity Link GA](https://developers.redhat.com/articles/2025/01/23/red-hat-connectivity-link-now-generally-available).

**Console UX candidates:** Gateway / HTTPRoute / GRPCRoute / TLSRoute / TCPRoute / UDPRoute topology graph; ListenerSet self-service vs Gateway edit; BackendTLSPolicy status; ReferenceGrant cross-ns edges; GatewayClass `supportedFeatures` capability matrix; “route has parents Accepted=False” finding; Envoy Gateway status/conditions deep-link when CRDs present (detect, don’t install).

#### Devices — DRA
- Core DRA APIs graduated GA in Kubernetes 1.34 (`resource.k8s.io/v1` enabled by default) — [Kubernetes v1.34 DRA blog](https://kubernetes.io/blog/2025/09/01/kubernetes-v1-34-dra-updates/).
- Lens and Headlamp already ship Device Resources / ResourceClaims browsers — [Lens Device Resources](https://docs.lenshq.io/k8slens/using-lens/device-resources/); [Headlamp Resource Claims PR](https://github.com/kubernetes-sigs/headlamp/pull/7820).

**Console UX candidates:** DeviceClass / ResourceSlice / ResourceClaim inventory; claim state on Pod/Node detail; “pending claim / no matching slice” finding; NVIDIA `GPUCluster` vs `ClusterPolicy` detection (read-only).

#### Policy — Kyverno + native VAP/MAP
- Kyverno graduated CNCF March 2026; declarative policy-as-code with CEL adoption aligning to Kubernetes admission direction — [CNCF Kyverno graduation](https://www.cncf.io/announcements/2026/03/24/cloud-native-computing-foundation-announces-kyvernos-graduation/); [Kyverno CNCF project page](https://www.cncf.io/projects/kyverno/).
- ValidatingAdmissionPolicy is GA since Kubernetes 1.30 (CEL in-process); Lens added VAP/MAP browser UX in 2026 — [VAP GA blog](https://kubernetes.io/blog/2024/04/24/validating-admission-policy-ga/); [Lens Admission Policies](https://docs.lenshq.io/k8slens/using-lens/config/admission-policies/); [Lens release note via issue](https://github.com/lensapp/lens/issues/8296).

**Console UX candidates:** Policy coverage heatmap (which namespaces/kinds have Enforce vs Audit); recent deny/warn events joined to Timeline; Kyverno PolicyReport findings; VAP/MAP list with CEL syntax highlight (read-first; write behind ProposeOnly if ever); “unsigned image would be denied here” dry-run against live policy set.

#### Certs & secrets hygiene — cert-manager + External Secrets
- cert-manager graduated CNCF Sep/Nov 2024; cited ~500M downloads/month and high production attach rate in graduation materials — [CNCF cert-manager graduation](https://www.cncf.io/announcements/2024/11/12/cloud-native-computing-foundation-announces-cert-manager-graduation/); [cert-manager announcement](https://cert-manager.io/announcements/2024/11/12/cert-manager-graduation/).
- External Secrets Operator remains CNCF Sandbox (accepted 2022); incubation application closed/deferred amid maintainer health work; readiness tracker still open into 2026 — [ESO CNCF page](https://www.cncf.io/projects/external-secrets/); [TOC incubation issue #1486](https://github.com/cncf/toc/issues/1486); [Incubation readiness #5207](https://github.com/external-secrets/external-secrets/issues/5207).
- ClusterSecretStore is the cluster-scoped backend gateway ExternalSecrets reference — [ClusterSecretStore API](https://external-secrets.io/latest/api/clustersecretstore/).

**Console UX candidates:** Certificate `notAfter` deadline board + Timeline; Issuer/ClusterIssuer Ready false; Gateway+cert-manager annotation coverage; ExternalSecret sync lag / Ready=False without showing values; Secret age heatmap (metadata only); “Secret not backed by ExternalSecret” drift finding.

#### Storage — CSI snapshot / clone / VAC / Velero
- VolumeAttributesClass for volume modification GA in Kubernetes 1.34 — [VAC GA blog](https://kubernetes.io/blog/2025/09/08/kubernetes-v1-34-volume-attributes-class/); [VAC concepts](https://kubernetes.io/docs/concepts/storage/volume-attributes-classes/).
- CSI PVC cloning via `dataSource` (same-namespace) is stable CSI feature — [CSI Volume Cloning](https://kubernetes.io/docs/concepts/storage/volume-pvc-datasource/).
- VolumeGroupSnapshot moved to beta (K8s 1.32 blog) for multi-PVC atomic snapshots — [Volume Group Snapshot beta](https://kubernetes.io/blog/2024/12/18/kubernetes-1-32-volume-group-snapshot-beta/).
- Velero CSI path creates VolumeSnapshot via labeled VolumeSnapshotClass (`velero.io/csi-volumesnapshot-class`) and restores PVC with DataSource → snapshot — [Velero CSI docs](https://velero.io/docs/main/csi/); [Velero restore reference](https://velero.io/docs/main/restore-reference/).

**Console UX candidates:** SnapshotClass inventory + default/Velero labels; VolumeSnapshot readyToUse/age; one-click “clone PVC” ProposeOnly (same-ns); VAC class catalog + PVC modify status Infeasible; VolumeGroupSnapshot membership view; Velero Backup/Restore status timeline joined to Atlas volumes / Still “volumes survive” preview.

#### Multi-tenancy — vCluster / Hierarchy / Quota
- CNCF blog (Sep 2025) positions vCluster as virtual control planes for CRD/team isolation while syncing pods into a host namespace; ResourceQuota configurable on host namespace — [CNCF vCluster blog](https://www.cncf.io/blog/2025/09/23/solving-kubernetes-multi-tenancy-challenges-with-vcluster/); [vCluster ResourceQuota](https://www.vcluster.com/docs/vcluster/configure/vcluster-yaml/policies/resource-quota); [vCluster architecture](https://www.vcluster.com/docs/vcluster/introduction/architecture/).
- Hierarchical Namespace Controller (HNC) HierarchicalResourceQuota aggregates budgets across subnamespaces (project retired under kubernetes-sigs but concepts still referenced) — [HNC concepts](https://github.com/kubernetes-retired/hierarchical-namespaces/blob/master/docs/user-guide/concepts.md).

**Console UX candidates:** Detect vCluster instances (host ns + syncer pods); tenant map host-ns ↔ virtual kubeconfig status; ResourceQuota / LimitRange pressure tiles; HRQ tree if CRDs present; “tenant pod can reach X” matrix scoped per tenant.

#### Cost — OpenCost
- OpenCost Allocation API supports `includeIdle`, `shareIdle`, `idleByNode` for private-cloud showback; on-prem uses custom pricing JSON (Cloud Costs API not for on-prem) — [OpenCost API docs](https://github.com/opencost/opencost-website/blob/main/docs/integrations/api.md); [on-prem config](https://github.com/opencost/opencost-website/blob/main/docs/configuration/on-prem.mdx); [OpenCost specification idle model](https://github.com/opencost/opencost-website/blob/main/docs/specification.md).
- Axiom already ingests OpenCost `gpuCost` into `otherUsd` invisibly per prior GPU report — local [Axiom GPU console additions.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20GPU%20console%20additions.md).

**Console UX candidates:** Idle-vs-allocated by node/namespace; GPU waste card (requested vs DCGM util); shareIdle toggle for chargeback narratives; never claim cloud CUR accuracy on bare metal without custom prices.

#### Supply chain — SLSA / cosign / Sigstore
- Kubernetes documents cosign verification of signed release images; SIG Release aims at Sigstore/SLSA for artifacts — [Verify signed artifacts](https://kubernetes.io/docs/tasks/administer-cluster/verify-signed-artifacts/); [KEP-3031 signing](https://www.kubernetes.dev/resources/keps/3031/).
- OWASP DevSecOps guidance: verify signatures + SLSA provenance at admission via Sigstore policy-controller or Kyverno; cosign `verify-attestation --type slsaprovenance` — [OWASP Artifact Signing and Provenance](https://owasp.github.io/DevSecOpsGuideline/2-Process/2-3-Build/2-3-6-Supply-Chain-Security/2-3-6-2-Artifact-Signing-and-Provenance/).
- Example in-cluster SLSA enforcement via Sigstore policy-controller + namespace include label — [s3cme README](https://github.com/mchmarny/s3cme/blob/main/README.md).

**Console UX candidates:** Workload “signature / provenance” badge (signed / attested / unknown) via registry lookup or admission policy dry-run; fleet filter `unsigned:`; mutable-tag vs digest mismatch finding; policy coverage for Sigstore/Kyverno image verify rules—not a full cosign CI product.

#### Virtualization — KubeVirt
- KubeVirt-Manager / VirtFoundry / Kubermatic Virtualization compete on VNC/serial, VM CRUD, snapshots—often separate UIs from the container console — [KubeVirt Summit 2025 talk notes](https://confpad.io/conferences/kubevirt-summit-2025/leveling-up-kubevirt-manager-new-features-better-ux-and-real-world-usability); [VirtFoundry](https://github.com/virtfoundry/core); [Kubermatic Virtualization](https://www.kubermatic.com/products/kubermatic-virtualization/).

**Console UX candidates (Axiom-fit):** Detect VirtualMachine / VirtualMachineInstance; VM↔PVC↔Network twin with container workloads; live-migration status; Still/Seal applicability story for VMs (if/when modeled); avoid building a second hypervisor product.

### Inferences
- Prefer **detect → status → findings → ProposeOnly** over install wizards for every CRD.
- Highest maturity × buyer leverage: Gateway API Standard features, cert-manager deadlines, DRA inventory, Kyverno/VAP policy coverage, CSI snapshot/clone + Velero status, OpenCost idle, vCluster detection.
- ESO is widely adopted but CNCF maturity/governance is messy—still worth console status UX; do not bet product story on graduation timing.

### Gaps
- Exact production attach rates for VolumeGroupSnapshot and private Envoy Gateway vs Istio Gateway API in on-prem fleets were not quantified from primary surveys in this pass.
- OPA/Gatekeeper vs Kyverno share-of-console in Rancher was not re-audited against latest Rancher Manager UI screenshots.

---

## What “no one thinks of” feats fit Axiom’s Still / seal / fleet-twin / who-poked-it identity?

### Takeaway
Axiom’s identity differentiators are ops-theatre compositions—containment + presence + fleet twins + data-plane verification + ProposeOnly narrative—not another CRD browser. Prior local packs A–E remain the uniqueness core; CNCF maturity should be absorbed as *evidence attached to those narratives*, not as competing product pillars.

### Cited Findings
- Local Pack A (containment theatre): live blast radius from presence counters; Still ladder preview; PacketWolf seal receipt (flows before→after); Network Guard on workload row; Atlas volumes in Still preview — [Axiom weird k8s feats.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20weird%20k8s%20feats.md).
- Local Pack B (fleet ghosts & twins): `sealed:` fleet search; twin diff panel; outage narrative across fleet history × Timeline × AgentRuns — same source.
- Local Pack C (presence & memory): who-poked-it Timeline; Place Intent ↔ Platform Intent drift; Health finding → Seal ProposeOnly; upgrade-risk codes → prep actions — same source.
- Local Pack D (data-plane truth): tenant-pod reachability; MCP tools wrapping Still/fleet/guard semantics rather than raw kubectl — same source.
- Local Pack E (weird findings): finalizers, tag float, HPA thrash, empty endpoints behind Ingress, cert deadlines, secret age, clock skew, GPU wasted-FLOPS — same source.
- Emerging AIOps consoles emphasize approval gates, RCA cards, and remediation workflows—validating ProposeOnly + narrative composition as a market pattern, while Axiom can bind those patterns to Still/Seal/fleet twin nouns competitors lack — [Kubernaut](https://kubernaut.ai/); [Kentinel](https://github.com/emreoztoprak/kentinel).

### Inferences
**“No one thinks of” feats that specifically fit Axiom (concrete):**
1. **Seal with witnesses** — before Seal, list humans with open windows/terminals; after Seal, auto-attach PacketWolf flow delta + Timeline card “sealed by X while Y was attached.”
2. **Fleet sealed-twin radar** — cross-cluster search `sealed:` / intent annotations; open twin-diff (image, replicas, seal state, cert expiry, signature badge).
3. **Containment-aware upgrade narrative** — upgrade-risk codes that recommend seal/hush before drain; PDB/blockers as prep steps with ProposeOnly.
4. **Intent drift detective** — NL Place Intent vs Platform Intent CR vs live replicas/Gateway routes.
5. **Data-plane still works after seal?** — post-seal reachability matrix (should fail to noisy peers; should pass to Atlas if allowed).
6. **Findings that propose Seal** — HPA thrash / empty endpoints / unsigned image / cert dying → ProposeOnly `seal_workload` with blast preview.
7. **MCP as Axiom semantics** — external agents call hush/seal/fleet-twin/guard, not generic kubectl—productizes identity outside the UI.
8. **AI ops without MLOps** — continuous review + approval-gated remediations using existing AgentRun/ProposeOnly; map findings to Still/Seal/cordon allowlist; no model catalogs, notebooks, or Run:ai queues.

### Gaps
- In-repo code confirmation of which Pack A–E hooks are already wired vs GAPS was taken from prior report claims, not re-validated against current `zyvor-axiom` tree in this research pass.

---

## Rank candidates by leverage (private-cloud buyer), effort, and Axiom fit

### Takeaway
Ship identity-max packs (Still/presence/twins/weird findings) and table-stakes closes (GPU/DRA/Gateway/cert hygiene) before deep CRD editors; rank below uses High/Med/Low for buyer leverage, engineering effort, and Axiom-fit (Still/seal/fleet-twin/who-poked-it alignment). Exclusions applied as requested.

### Cited Findings
- Ranking inputs synthesize: local weird-feats ship order and GPU tiering — [Axiom weird k8s feats.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20weird%20k8s%20feats.md); [Axiom GPU console additions.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20GPU%20console%20additions.md); Gateway/DRA/Kyverno/cert-manager/VAC/Velero/OpenCost/vCluster/Sigstore sources cited in prior sections; competitor contrast from Portainer/OpenShift/Lens docs above.

### Inferences

#### Ranked candidate table (concrete features)

| Rank | Candidate (concrete console feat) | Buyer leverage | Effort | Axiom fit | Notes |
|------|-----------------------------------|----------------|--------|-----------|-------|
| 1 | **Still ladder + live blast radius + Atlas volumes in preview** | High | Med | Max | Unique; builds on existing seams |
| 2 | **Who-poked-it Timeline + presence counters on seal** | High | Med | Max | Ops theatre competitors lack |
| 3 | **PacketWolf seal receipt + Network Guard on workload row** | High | Med | Max | Verify isolate, don’t only create NetworkPolicy |
| 4 | **Weird findings feed (E1–E4 + cert/secret deadlines)** | High | Med–Low | High | Cheap signal density for platform teams |
| 5 | **Sealed twin search + twin diff + outage narrative** | High | Med–High | Max | Fleet private-cloud buyer magnet |
| 6 | **GPU accelerators vertical slice + gpuUsd + idle/DCGM** | High | Low–Med | High | Table-stakes miss; code partially exists |
| 7 | **Cert-manager Certificate deadline board + Gateway TLS status** | High | Low–Med | High | Graduated, universal private-cloud pain |
| 8 | **Gateway API topology + Accepted=False findings (+ Envoy GW detect)** | High | Med | High | Standard channel deep enough for UX |
| 9 | **Tenant-pod reachability matrix** | High | Med | Max | Data-plane truth |
| 10 | **Health/finding → Seal ProposeOnly + upgrade-risk prep packs** | High | Med | Max | AIOps-shaped without Run:ai |
| 11 | **CSI VolumeSnapshot / clone ProposeOnly + Velero Backup status** | High | Med | High | Private-cloud storage ops |
| 12 | **OpenCost idle-by-node + shareIdle storytelling** | Med–High | Low | High | Already partially ingested |
| 13 | **DRA DeviceClass/Claim/Slice browser (read-only)** | Med–High | Med | Med–High | Lens/Headlamp parity; pairs with GPU |
| 14 | **Kyverno PolicyReport + VAP coverage heatmap (read-first)** | Med–High | Med | Med–High | Graduated; bind to unsigned-image findings |
| 15 | **Supply-chain badges (cosign/SLSA attest / mutable tag)** | Med–High | Med–High | High | Fits seal/deny narrative; registry access needed |
| 16 | **ExternalSecret / ClusterSecretStore Ready + secret age hygiene** | Med | Low–Med | High | Values never shown |
| 17 | **vCluster / quota / HRQ tenant map (detect + pressure)** | Med | Med | Med–High | Multi-tenant private cloud |
| 18 | **VolumeAttributesClass catalog + modify Infeasible status** | Med | Low–Med | Med | GA storage mutability |
| 19 | **KubeVirt VM inventory twin with PVC/net (detect, limited actions)** | Med | Med–High | Med | Only if buyers run VMs on same clusters |
| 20 | **MCP tools for Still/fleet/guard** | Med | Med | Max | Ecosystem/agents leverage |
| 21 | **VolumeGroupSnapshot membership UX** | Med–Low | Med | Med | Beta adoption uneven |
| 22 | **Read-only KAI/Volcano queue depth if CRDs exist** | Low | Low | Low | Boundary; not fairshare editors |

#### Explicitly excluded (per constraints / prior reports)
- Metal3-first bare-metal provisioning stories; multi-runtime migration path catalogs.
- Browser TCP port-forward; ClusterRole write UI.
- Run:ai / KAI fairshare editors, model catalogs, notebook MLOps, Inference Hub/KAITO-as-product.
- Full chaos studio; autonomous mute/seal of production without ProposeOnly.

#### Suggested sequencing (buyer value × effort × fit)
1. **Identity core:** Still ladder + blast radius + who-poked-it + PacketWolf receipt + weird findings + cert deadlines.
2. **Fleet twins + ProposeOnly seal/upgrade narrative + reachability.**
3. **Table-stakes close:** accelerators API/UI, OpenCost gpu/idle, Gateway topology, DRA detect.
4. **Policy/supply-chain/storage CRD status layers** (Kyverno/VAP, cosign badges, snapshots/Velero, ESO Ready).
5. **Tenancy + KubeVirt detect** as optional packs when CRDs present.
6. **MCP Still/fleet/guard** once verbs are stable.

### Gaps
- Effort estimates are relative (not person-weeks from a scoped design doc).
- Buyer leverage is inferred for ops/platform private-cloud teams; no primary customer interviews in this pass.
- Competitive “absence” claims for rare feats rely on pattern evidence and prior Axiom research rather than a full four-vendor UI audit checklist.

---

## Source index (web)

- [Gateway API 1.4](https://kubernetes.io/blog/2025/11/06/gateway-api-v1-4/)
- [Gateway API 1.5](https://kubernetes.io/blog/2026/04/21/gateway-api-v1-5/)
- [Gateway API v1.6.0-rc.1](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.6.0-rc.1)
- [Gateway API implementations](https://gateway-api.sigs.k8s.io/docs/implementations/list/)
- [DRA GA (1.34)](https://kubernetes.io/blog/2025/09/01/kubernetes-v1-34-dra-updates/)
- [Lens Device Resources](https://docs.lenshq.io/k8slens/using-lens/device-resources/)
- [Kyverno graduation](https://www.cncf.io/announcements/2026/03/24/cloud-native-computing-foundation-announces-kyvernos-graduation/)
- [cert-manager graduation](https://www.cncf.io/announcements/2024/11/12/cloud-native-computing-foundation-announces-cert-manager-graduation/)
- [VAP GA](https://kubernetes.io/blog/2024/04/24/validating-admission-policy-ga/)
- [Lens Admission Policies](https://docs.lenshq.io/k8slens/using-lens/config/admission-policies/)
- [VAC GA](https://kubernetes.io/blog/2025/09/08/kubernetes-v1-34-volume-attributes-class/)
- [CSI Volume Cloning](https://kubernetes.io/docs/concepts/storage/volume-pvc-datasource/)
- [Volume Group Snapshot beta](https://kubernetes.io/blog/2024/12/18/kubernetes-1-32-volume-group-snapshot-beta/)
- [Velero CSI](https://velero.io/docs/main/csi/)
- [CNCF vCluster blog](https://www.cncf.io/blog/2025/09/23/solving-kubernetes-multi-tenancy-challenges-with-vcluster/)
- [OpenCost Allocation API](https://github.com/opencost/opencost-website/blob/main/docs/integrations/api.md)
- [Verify signed K8s artifacts](https://kubernetes.io/docs/tasks/administer-cluster/verify-signed-artifacts/)
- [OWASP signing & provenance](https://owasp.github.io/DevSecOpsGuideline/2-Process/2-3-Build/2-3-6-Supply-Chain-Security/2-3-6-2-Artifact-Signing-and-Provenance/)
- [Connectivity Link / OpenShift Gateway console](https://developers.redhat.com/articles/2025/01/23/red-hat-connectivity-link-now-generally-available)
- [Portainer GPU](https://docs.portainer.io/user/kubernetes/gpu)
- [ESO CNCF](https://www.cncf.io/projects/external-secrets/)
- Local: `reports/Axiom weird k8s feats.md`, `reports/Axiom GPU console additions.md`
