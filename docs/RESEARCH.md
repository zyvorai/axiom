# Research: where Axiom sits in the Kubernetes control-plane field

Date: 2026-11-23. Method: fetched pages were read with a page fetch tool and a summarising model; every claim that comes from such a page carries its URL. Anything not read from a fetched page is marked **[U]** (unverified: from memory or from the repo's earlier research notes, not re-checked here). Several vendor sites could not be reached from the research host (Rancher's docs host did not resolve), so Rancher, Lens, Portainer, OpenShift, Kubermatic and Argo CD are **[U]** below. Product claims are the vendors' own.

Hard boundaries respected (from `docs/ROADMAP.md` "Not planned", `docs/GAPS.md`, `SECURITY.md`): no ClusterRole write UI, no browser TCP port-forward, no Run:ai/KAI fairshare or model-catalog MLOps, no Metal3-first or multi-runtime provisioning, no SOC2/pen-test SKU, read-only features never write, nothing here may exceed Axiom's ServiceAccount RBAC allowlist, no database is required, and gaps must be stated honestly (`docs/GAPS.md`). Earlier research already covers GPU and general console features (`research_notes/`, `reports/`); this document looks only for what is still missing.

## What the field does

| Product | What it does | Source |
|---|---|---|
| Cluster API | Declarative, Kubernetes-style APIs for the whole lifecycle of clusters (create, scale, upgrade, delete) and the machines, load balancers and networks beneath them | https://cluster-api.sigs.k8s.io/introduction |
| Crossplane | Platform teams define a custom API (for example `App`) that app teams fill in; one request creates and keeps in sync the underlying Deployment and Service | https://docs.crossplane.io/latest/get-started/get-started-with-composition/ |
| Backstage | A software catalog: who owns each service plus descriptive metadata, kept as YAML in source control and read by Backstage | https://backstage.io/docs/features/software-catalog/ |
| Harvester | Hyperconverged infrastructure on Kubernetes: VM management (clone, live migration, backup/restore to NFS, S3 or NAS), distributed block storage, VLAN networking, a web UI | https://docs.harvesterhci.io/v1.5/ |
| Velero | Backups with cron schedules and a TTL (default 30 days); phases `Completed`, `Failed`, `PartiallyFailed`; the page it documents says nothing about verifying that a backup can be restored | https://velero.io/docs/main/how-velero-works/ |
| Pluto (Fairwinds) | Finds deprecated and removed apiVersions in manifests, Helm charts and Helm releases running in a cluster, labelled `DEPRECATED` or `REMOVED`; cross-cluster tracking needs the paid Fairwinds Insights | https://github.com/FairwindsOps/pluto |
| kubent (kube-no-trouble) | Reads the live cluster, local manifests, Helm v3 release manifests and `last-applied-configuration` annotations; groups findings by target Kubernetes version | https://github.com/doitintl/kube-no-trouble |
| Kubernetes PodDisruptionBudgets | `minAvailable` / `maxUnavailable` as a number or percentage; `maxUnavailable: 0` or `minAvailable` equal to the replica count blocks voluntary disruption, so a drain waits; with one replica `minAvailable: 1` blocks drains entirely | https://kubernetes.io/docs/tasks/run-application/configure-pdb/ |
| Eviction API | Obeys PDBs: `429` when a budget blocks the eviction, `500` when several PDBs select one Pod; a broken application can leave evictions stuck | https://kubernetes.io/docs/concepts/scheduling-eviction/api-eviction/ |
| Rancher | Multi-cluster management UI, Fleet GitOps, a Backups operator, cluster provisioning (RKE2/K3s), Harvester integration | **[U]** |
| Portainer | Container/Kubernetes management UI with RBAC and edge agents | **[U]** |
| OpenShift console | Console bundled with OpenShift, with Lightspeed assistant | **[U]** (see `reports/` for the earlier write-up) |
| Lens / Headlamp / Kubermatic / Argo CD multi-cluster | Desktop or web consoles, a multi-cluster platform and multi-cluster GitOps | **[U]** |
| Kubecost / OpenCost | Cost allocation and rightsizing; Axiom already integrates OpenCost | **[U]** (already in Axiom) |

## What Axiom already covers (so it is not proposed again)

Fleet health, history and search; Helm catalog (including OCI), history and rollback; Argo CD / Flux create, edit, delete; Velero backup, schedule and restore wizard; cost, budgets, rightsizing (including in place), idle resources; node drain preview and wizard; image vulnerabilities; change correlation; cert-manager and External Secrets health; Gateway API; accelerators; agents with human approval; MCP tools; an upgrade pre-check limited to ingress-nginx and kube-proxy `ipvs` (CHANGELOG 0.15.0).

## What it lacked (gaps found)

1. **No answer to "which namespaces have no recent backup?"** The Backups page lists backups, schedules and restores; Velero's own docs describe creating, scheduling and expiring backups but nothing that reports coverage gaps.
2. **Drain previews do not warn about the PodDisruptionBudgets that already exist** across the cluster before anyone starts a drain: a PDB that can never be satisfied, or replicated workloads with no PDB at all. (The per-node preview reads `disruptionsAllowed` for Pods on one node.)
3. **The upgrade pre-check knew two risks.** Removed and deprecated API versions are the classic upgrade breaker, and Pluto and kubent exist because of it; Axiom had no equivalent.

## Ranked ideas

| Rank | Idea | Why it fits Axiom | Status |
|---|---|---|---|
| 1 | **Deprecated and removed API check** | Cheap, pure, safe (reads text the caller supplies), directly serves the upgrade story Axiom already has; Pluto and kubent prove demand | Built (`POST /api/v1/manifests/deprecations`) |
| 2 | **Backup coverage report** | Axiom already reads Velero objects, PVCs and workloads; turns a list into a risk answer; read-only | Built (`GET /api/v1/backups/coverage`) |
| 3 | **Disruption readiness audit** | Reads PDBs Axiom already lists; prevents the stalled-drain failure the Eviction API documents | Built (`GET /api/v1/disruption/audit`) |
| 4 | Scan Helm release manifests and live objects for deprecated APIs (kubent's collectors) | Axiom already reads `sh.helm.release.v1.*` Secrets for history and diff; would turn rank 1 from "paste" into "scan the cluster" | Not built: needs Helm release manifest decoding on a code path that has only run against fakes, and live objects hide the original version (Pluto's own caveat: the API server converts versions). Next candidate |
| 5 | Restore drill: periodically restore a backup into a scratch namespace and report the result | Closes the "backup is not proof" gap Velero's docs leave open | Not built: writes to the cluster, needs RBAC the chart does not grant and a live cluster to validate |
| 6 | Ownership and required-label hygiene (Backstage-style catalog metadata from namespace labels) | Fits apps-first view | Not built: Axiom would be inventing a catalog schema; better to read an existing convention if users ask |
| 7 | Cluster lifecycle (Cluster API-style create/upgrade clusters) | Rancher, Kubermatic, Harvester do it **[U]** | Rejected: provisioning is the stated non-goal (Metal3-first / multi-runtime), and Axiom's identity is operating clusters that exist |
| 8 | Crossplane-style composition / self-service platform API | Backstage and Crossplane lead here | Rejected: large new surface; GitOps and the Run wizard already cover the app-team path |
| 9 | HCI (VMs, distributed block storage, bare metal) like Harvester | KubeVirt is already supported | Rejected: Axiom manages KubeVirt, it does not become an HCI distribution |

## Caveats

- Rank 1 compares only `apiVersion` and `kind` against a static table written from the Kubernetes deprecation guide (https://kubernetes.io/docs/reference/using-api/deprecation-guide/, read only up to v1.32 removals; the page was truncated for the research tool). The v1 `Endpoints` deprecation in 1.33 is included from memory of the 1.33 release **[U]**: the release-blog URL tried returned 404 and was not read. Re-check the table against the guide when a new Kubernetes minor ships.
- Ranks 2 and 3 are heuristics (see `docs/GAPS.md`).
- Percentage handling in PDB values follows the PDB page above: only `100%` and `maxUnavailable` 0 are treated as unsatisfiable; other percentages round at runtime and are not simulated.
