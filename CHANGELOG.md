# Changelog

All notable changes to Zyvor Axiom are documented here.

## [Unreleased]

### Added — Node maintenance wizard
- `GET /api/v1/nodes/{name}/drain-preview`: read-only, server-computed preview of a drain (Pods to evict grouped by namespace/owner; ignored DaemonSet, mirror and kube-system Pods; emptyDir Pods needing delete-emptydir-data; bare Pods that would be lost; Pods blocked by a PodDisruptionBudget with `disruptionsAllowed == 0`; single-replica downtime; Pods whose PVC is pinned to the node by PV node affinity; approximate reschedule headroom on other Ready schedulable Nodes) with a blocked/caution/ok verdict. Pure logic in `internal/platform/drain_preview.go`; uses existing list calls, so no new RBAC; denied for namespace-restricted accounts.
- Node detail: Drain now opens a wizard (Preview, typed node name, cordon then drain with progress, uncordon). The Nodes list Drain button links to it.
- Behaviour change: `POST /api/v1/nodes/{name}/drain` now requires `?confirm=<node name>` (`400` otherwise). Authz (operator/admin) and audit (`node.drain`) are unchanged. Strings in all 8 locales. Not yet run against a live cluster.

### Added — In-place pod resize
- `POST /api/v1/cost/rightsizing/apply` accepts `inPlace:true` (opt-in, default unchanged): the same recommendation is written to each running pod of the workload through the `pods/resize` subresource (Kubernetes >= 1.33 beta, 1.35 stable) with no restart and no rollout; the pod template is not changed. Dry run supported (server-side `dryRun=All`), operator/admin only, audited as `cost.rightsizing_apply_inplace`. Limits are never lowered or added; a Guaranteed pod whose request would drop is skipped (a resize cannot change QoS class). A cluster without the subresource gets `501` with a clear message. RBAC: `pods/resize` `patch` (k8s + Helm). Pod detail shows PodResizePending / PodResizeInProgress and legacy `status.resize`; demo supported. Strings in all 8 locales.

### Added — Image vulnerability and SBOM view
- `GET /api/v1/security/vulnerabilities?namespace=&severity=` and an Images and SBOM section on Security: per-image rollup (critical/high/medium/low, fixable, workloads using the image), top CVEs with `fixedIn`, per-image fix suggestions (lowest fixed version per package) and SBOM component counts with a license summary, from Trivy Operator VulnerabilityReports / SbomReports / ClusterSbomReports. Read-only; ClusterRole gains get/list/watch on `sbomreports` and `clustersbomreports` (k8s + Helm). Strings in all 8 locales.
### Added — change correlation
- `GET /api/v1/changes?namespace=&workload=&at=&window=` and MCP tool `recent_changes`: ordered recent changes (GitOps syncs, Helm upgrades, rollouts, Secret/ConfigMap updates, node cordon/drain/NotReady, HPA scaling, Axiom audit) in a lookback window (default 60 min), each with source, time, actor and a relevance score (same workload/namespace and closer in time rank higher). Pure ranking in `internal/platform/changes.go`; no new RBAC. 'Recent changes' panel on Workload detail (Changes tab) and Timeline. Strings in all 8 locales.

### Added — Idle resources finder
- `GET /api/v1/cost/idle?namespace=&minAgeDays=7` and an Idle resources section on Ops → Cost: unbound PVCs and PVCs no Pod mounts (with a monthly estimate at a flat storage rate, default $0.10/GiB, `AXIOM_COST_STORAGE_RATE`), empty namespaces (never `default`, `kube-*` or Axiom's own), LoadBalancer Services with no ready endpoints, finished Jobs, scaled-to-zero Deployments, and low-confidence unreferenced ConfigMaps/Secrets (names only, values never read). Read-only: no write endpoint and nothing is deleted; each finding carries a confidence and suggested action. Uses existing list calls, so no RBAC change; namespace-restricted accounts get `403`. Strings in all eight languages
### Added — Velero restore wizard
- Backups page: Velero Restores list and a wizard (pick Backup, included/excluded namespaces and resources, namespace mapping, restorePVs, existingResourcePolicy none|update) with a server-side dry-run preview (backup summary, namespace collisions, effect on existing objects, warnings) and a typed confirmation before the Restore CR is created.
- `GET /api/v1/velero/restores`, `POST /api/v1/velero/restores/preview`, `POST /api/v1/velero/restores` (admin-only, audited by name, confirmation enforced server-side). ClusterRole (k8s + Helm) gains create on `restores.velero.io`. Strings in all 8 locales. Not yet exercised against a live cluster.

### Added — Karpenter view
- Autoscaling → Karpenter tab: read-only NodePools (node class, consolidation, limits, nodes, ready) and NodeClaims (instance type, capacity type, zone, phase) from `GET /api/v1/karpenter`; shows a not-installed state when `karpenter.sh` is absent. Translated in all eight languages
### Added — Gateway API deep view
- `GET /api/v1/gateway-api` and a Gateway API section on Networking: Gateways (class, listeners, addresses, Accepted/Programmed), HTTPRoute and GRPCRoute (hostnames, parentRefs linked to their Gateway, backendRefs, Accepted/ResolvedRefs) and ReferenceGrants. Read-only; the ClusterRole gains get/list/watch on `grpcroutes` and `referencegrants`. Strings in all 8 locales.
### Added
- cert-manager read-only view: `GET /api/v1/certificates` (Certificates with expiry heat bucket, Issuers/ClusterIssuers, failing CertificateRequests/Orders); Secrets page Certificates section; ClusterRole get/list on cert-manager.io (k8s + Helm). No write actions.

## [0.14.1] - 2026-11-12

### Added — deepen pack (GPU/DRA, GitOps write, cost/bill)
- DRA browser: ResourceClaim phase / allocated devices / bound pod on `GET /api/v1/dra`; Ops Status + Detail columns.
- DCGM overview tiles: framebuffer, temperature, power, XID on `GET /api/v1/accelerators` metrics (first-hit PromQL families).
- GitOps list enrichment: `health`, `sync`, `revision`, `suspended`, `canSync`/`canEdit`; ApplicationSet sync returns 422; Ops hides Sync/Edit for AppSets.
- GitOps suspend/resume: `POST /api/v1/gitops/{kind}/{ns}/{name}/suspend` (Flux `spec.suspend`, Argo automated sync); admin-only, RV-guarded.
- Cost estimate prices `nvidia.com/gpu` (+ MIG) at `gpuRatePerGpu` (default 100 USD/mo, `AXIOM_COST_GPU_RATE`); OpenCost status fields on `GET /cost`; invoice top-N AWS SERVICE breakdown (experimental).

### Fixed
- Health total-failure path: cert-manager Certificate LIST errors surface as analyzer errors; fake dynamic client registers CertificateList.
- Deploy remints mTLS admin client cert when it no longer verifies against the current CA.

## [0.14.0] - 2026-11-12

### Docs
- ROADMAP / GAPS / VERIFICATION / QUICKSTART / API synced; marketing site hero: New in 0.14.

### Added — Velero CRD / billing live / Helm RBAC
- Velero Backup/Schedule create validated against upstream v1.15.2 CRD OpenAPI (`testdata/velero/`); optional `storageLocation` on backups; `e2e/scripts/validate-velero.sh` + enterprise-writes path when CRDs exist.
- Cloud invoice AWS: `NextPageToken` pagination, STS session token (`billingAwsSessionToken`), live harness `e2e/scripts/validate-billing-live.sh` / `TestAWSBillingLive`.
- Helm ClusterRole: ServiceAccounts, PDBs (write), Leases, EndpointSlices, ControllerRevisions — so ordinary chart installs succeed under Axiom's SA.

### Added — residual GAPS pack
- Pod exec + node shell idle timeout (10 m websocket ping/pong), same window as pod-proxy.
- Non-demo mode refuses default `demo`/`demo` unless `AXIOM_ALLOW_DEFAULT_CREDS=true` (Helm `auth.allowDefaultCreds`).
- Shared login throttle across replicas (`zyvor-axiom-login-throttle`).
- CSI PVC clone: `sourcePVC` on `POST /api/v1/volumes` + ProposeOnly agent action `clone_pvc`.
- Policy coverage: `GET /api/v1/security/policy-coverage` (PolicyReport rollup by policy/rule).

### Added — ROADMAP Next + GAPS closed
- MCP HITL sandbox: `mint_sandbox` / `list_sandboxes` (allowlisted grants; no ClusterRole minting).
- Deepened upgrade-risk (version skew, cert/clock, GPU operator) folded into `AssessUpgradeRisk`.
- GitOps lists Argo ApplicationSets read-only; demo includes `tenant-apps`.
- OCI cloud token refresh: `POST /api/v1/helm/registry-credentials/cloud-token` (ecr/gar/acr via local CLIs).
- ValidatingAdmissionPolicy browser: `GET /api/v1/admission-policies`.
- Shared WebSocket/SSE ticket store (`zyvor-axiom-stream-tickets`).
- Seal NetworkPolicies named `axiom-seal-{kind}-{name}`; blast radius uses exact ownership (no substring).
- Users: unknown roles rejected; session reports Operator + `auth=saml`; Atlas token clear; unknown cluster DELETE → 404.

### Added — Overall opportunities pack (HA + theatre deepen + MCP HITL)
- Multi-replica notify leadership via coordination Lease (`zyvor-axiom-notify`); Slack/webhook dispatch retries once; non-leaders stay silent.
- Audit ConfigMap Save merges rings across replicas (union by time/action/target/user/detail) instead of last-writer-wins.
- Secret list prefers PartialObjectMetadata (no `.data` download) with Limit/Continue pagination fallback.
- Shared SAML AuthnRequest store (`zyvor-axiom-saml-authn`) so login→ACS works without sticky sessions.
- Twin-diff enrich: image / ready / desired / sealed / signed badge on fleet search hits.
- Outage narrative API `GET /api/v1/fleet/outage-narrative` + Fleet UI (who-poked-it + agent runs).
- Seal receipts record flowsBefore/After/delta and presence witnesses.
- Findings → Seal ProposeOnly in deterministic agent planner; upgrade-risk prep actions expanded.
- DRA browser `GET /api/v1/dra` + Ops Accelerators panel; OpenCost queries `shareIdle`/`includeIdle`.
- MCP HITL writes: `propose_seal`, `approve_agent_run`, `deny_agent_run`, plus `list_dra` / `outage_narrative`.

### Added — NVIDIA Accelerators + weird k8s feats
- `GET /api/v1/accelerators`: GPU nodes/pods, Operator/device-plugin/DCGM, GFD sharing/MIG labels, GPU ResourceQuotas, FailedScheduling issues, optional DRA counts and DCGM util snapshot.
- Ops → Accelerators tab; GPU columns on Nodes / Node detail / Pod detail; Cost shows `gpuUsd` + `otherUsd`.
- Node metrics history can include DCGM GPU util (classic + `_RATIO` families).
- Still ladder execute: `POST …/still` (hush/seal/sleep/end) with live terminal/window blast preview; Seal writes PacketWolf flow receipt to audit/Timeline; Workloads menu offers hush/seal/sleep/end.
- Health analyzers: finalizer ghosts, mutable image tags, HPA thrash, Ingress with zero ready endpoints.
- Fleet search `sealed:` finds network-isolated twins; twin-diff panel when the same object exists on ≥2 clusters; agent action `seal_workload`; MCP tools `list_accelerators`, `still_preview`, `fleet_search`, `network_guard`.
- Timeline labels operator presence (`pod.terminal` / `pod.window` / exec) as “who poked it”.
- Workload detail Network Guard (flows + Seal/Unseal); Settings reachability matrix (`POST /integrations/probe/matrix`).
- GPU rightsizing: `waste[]` on accelerators report + `POST /api/v1/accelerators/rightsizing/apply` (nvidia.com/gpu); Ops → Accelerators apply/dry-run.
- Health hygiene: cert expiry, secret age, node clock-skew findings.
- Upgrade-risk agent runs enriched with allowlisted prep actions (`EnrichUpgradeRiskPlan`).
- Intent drift: `GET /api/v1/platform-intents/drift` + Platform Intents UI panel.
- Fleet incident history stitches an outage narrative tip (Timeline / Agents).

### Added — Envoy Gateway (Gateway API)
- Helm `expose.type=Gateway` creates Gateway + HTTPRoute for Envoy Gateway (`gatewayClassName` default `eg`).
- Example manifest `deploy/k8s/envoy-gateway-example.yaml`; QUICKSTART prefers Envoy over Ingress NGINX.
- `GET /api/v1/gateways` includes `controller`, `provider` (envoy/nginx/…), and Gateway `addresses`; Ingresses page highlights Envoy and Ingress NGINX retirement.

### Changed — login, auth defaults, mTLS
- Login page: fuller night-ops mesh background (ember/cyan aurora + grid).
- Remote deploy defaults to `admin` / `Admin@321` (existing Secret still reused).
- HTTPS mutual TLS by default: `AXIOM_TLS_CLIENT_CA` + `AXIOM_TLS_MTLS=require`; deploy mints `.run/mtls/admin.p12` (password `axiom`).

### Added — competitive ops pack
- Alertmanager firing alerts + silence CRUD (`/api/v1/alertmanager/*`) with Run agent from Alerts.
- Upgrade-risk lite on Health check (`GET /api/v1/upgrade-risk`, ProposeOnly agent run).
- MCP tools: `list_nodes`, `list_workloads`, `upgrade_risk`, `list_firing_alerts`.
- Agent actions: `cordon_node`, `uncordon_node`, `apply_rightsizing`.
- Prometheus history charts on Node and Workload detail (`kind=node|workload|pod`).
- Fleet search: Pods (name/image/labels); Atlas API token field in Settings; terminal idle disconnect (10m).

### Added — agentic ops (Lightspeed-class)
- Built-in agent planner: investigate (diagnosis, events, health) → structured ProposeOnly plan → human approve/deny → allowlisted apply (`restart_pod`, `delete_pod`, `scale_workload`, `rollout_restart`, `noop`). Uses BYO LLM when AI is enabled; otherwise a deterministic plan from CrashLoop/ImagePull/scale signals.
- Dual-path storage: AgentRun CRD when present (Axiom-owned annotation); ConfigMap `zyvor-axiom-agent-runs` when the CRD is missing so Agents works on plain k3s.
- Ops → Agents create dialog, plan/actions/executed detail; **Run agent** from Pod detail, health findings and Warning events.
- Optional auto runs from Warning/CrashLoop (Settings → AI → Auto agent runs, off by default).
- Approve/Deny refuse when the run is not pending. Axiom-owned approve executes (respecting dry-run) and audits `agentrun.execute`.

### Added — MCP
- Authenticated JSON-RPC MCP at `POST /api/v1/mcp` (`GET /api/v1/mcp`, `/api/v1/mcp/tools`): `list_pods`, `list_events`, `diagnose_pod`, `cluster_health`, `list_agent_runs`, `create_agent_run`, `get_workload`.

### Added — competitive GAPS batch
- Prometheus history charts on Pod detail (`GET /api/v1/metrics/history`, needs `AXIOM_PROMETHEUS_URL`).
- Embedded noVNC iframe for VMs when the VNC websocket proxy path is ready.
- Clear Slack / alert webhook from Settings (`slackWebhookUrlClear` / `alertWebhookUrlClear`).

## [0.13.2] - 2026-11-05

### Changed — Website
- The GitHub Pages site is refreshed: new hero and framed product shots, Fleet, GitOps and Autoscaling & cost sections, languages and license sections, WebP screenshots, sitemap and structured data, and a new social card (`scripts/site-shots.mjs`, `scripts/site-og.mjs` regenerate them).

### Changed — License
- Axiom is now licensed under the **Zyvor Production License v1.0** (`LicenseRef-Zyvor-Production-1.0`): source-available, free for non-production use, with production use requiring a commercial license from Zyvor AI Labs Private Limited. This replaces the previous "all rights reserved" notice. Added `NOTICE` and `LICENSES/LicenseRef-Zyvor-Production-1.0.txt`.

## [0.13.1] - 2026-11-04

### Changed — Languages
- The last English-only labels are translated in all eight languages: role names in the Users table, user menu and two-factor gate (display only; stored and API role values are unchanged), AI provider names and the "(rate limited)" suffix, terminal window titles (`exec — …`, `logs — …`, `receipt`, the default `Terminal`), the network-policy `default-deny ingress/egress` badges and Ingress/Egress columns, the ConfigMap/Secret `KEY`/`value` and cluster-label placeholders. English wording is unchanged.

## [0.13.0] - 2026-11-04

### Added — cloud bill reconciliation (experimental)
- Ops → Cost shows the actual cloud invoice for the previous and the current month next to the allocated cost (OpenCost or the request estimate, prorated from the current run-rate) and the unallocated difference, with an *Experimental* badge; hidden until configured. `GET /api/v1/cost/invoice` returns `{provider, currency, months:[{month, invoiceTotal, allocated, unallocated, asOf, partial}], experimental:true, error?}` and is readable like budgets (every role except namespace-restricted accounts).
- Providers, each a small standard-library client: AWS Cost Explorer `GetCostAndUsage` (UnblendedCost, monthly, hand-written SigV4), Azure Cost Management query (ActualCost, subscription scope, client-credentials token) and GCP Cloud Billing through a fixed parameterised BigQuery query over the billing export table (service-account JWT; strictly validated table name). Optional single tag / label filter.
- Settings → Cloud bill reconciliation (admin only): provider, ids, region / table and tag filter in the integrations ConfigMap; the AWS key pair, Azure client secret and GCP service-account key only in the integrations Secret, write-only (`…Set` flags). `POST /api/v1/integrations/billing/test` makes one uncached call.
- Outbound requests only over HTTPS to each provider's fixed API hosts (no configurable URL, no proxy, no redirects, the Helm catalog's public-address dialer), 20 s / 4 MiB per request, results cached for one hour (failures five minutes); errors carry only the provider's error code. Demo mode returns deterministic canned data.
- **Experimental:** the clients are tested only against httptest fixtures and have not been verified against real AWS, Azure or Google Cloud accounts (see docs/GAPS.md).

## [0.12.1] - 2026-11-04

### Fixed — pod proxy cluster
- A pod proxy session now stays on the cluster its pod was opened on. `window-link` records the registered name of the cluster the pod was resolved on (the calling tab's active cluster) in the ticket and the session (signed into the token and cookie with `AXIOM_SESSION_SECRET`, in the in-memory grant otherwise), and the proxy listener looks the pod up on exactly that cluster on every request. Switching clusters in the console tab, a replica's own default and another replica's selection no longer re-target an open session to a same-named pod elsewhere. A ticket or session for a cluster that has since been unregistered, or that the serving replica does not know, is refused with `410` (and logged). Namespace restriction and revocation checks are unchanged. Signed pod proxy links and sessions issued by 0.12.0 do not record their cluster and are refused (`410`) after the upgrade; open the port again.

## [0.12.0] - 2026-11-04

### Changed — pod proxy across replicas
- The pod proxy origin no longer needs session affinity. With `AXIOM_SESSION_SECRET` set (Helm `auth.sessionSecret`, already required for `replicaCount>1`), pod proxy tickets and session cookies are HMAC-signed tokens under a key derived from the session secret for this purpose only, so a `window-link` minted on one replica can be redeemed on another and the session cookie works on every replica, in path and host mode. Tickets stay single use across replicas through the new `zyvor-axiom-pod-proxy-redeemed` ConfigMap (redeemed ticket ids only, pruned after the 60-second ticket lifetime, conflict-safe updates; covered by the existing ConfigMap permissions) plus each replica's memory; if the ConfigMap cannot be read or written the redeeming replica's memory alone applies and the error is logged. The 10-minute sliding expiry is kept by re-issuing the cookie with a later signed deadline (at most once a minute) and the 1-hour maximum is counted from the signed creation time. Revocation, the role check and a new namespace-restriction check run on every request as before; the per-user link and session caps are now counted per replica. In host mode a replica that did not create a session answers a request without a valid cookie with an empty `404`. Without a session secret (single replica, random per-process key) tickets and sessions stay in memory exactly as before. The startup log reports `shared=true|false`. See `docs/GAPS.md` for the residual limits

## [0.11.0] - 2026-11-04

### Added — Languages
- Português, Italiano, 日本語 and 简体中文; the registry-login, autoscaling-edit and fleet-history screens are translated in every language.

### Added — VPA & KEDA edit
- Edit VerticalPodAutoscalers and KEDA ScaledObjects in place instead of deleting and recreating them: `GET /api/v1/vpas/{namespace}/{name}/edit` and `GET /api/v1/scaledobjects/{namespace}/{name}/edit` return the current values in the create body's typed shape plus `resourceVersion`, `editable`, `reasons` and `readOnly`; `POST …/edit/preview` returns a structured diff of the managed fields (`changes[{object,field,from,to}]`, the object's YAML after the change, warnings, `noop`); `PUT …/{namespace}/{name}` applies only the managed fields (VPA update mode and container policies; ScaledObject min/max replicas, polling interval, cooldown and triggers) as a JSON Patch that first tests `metadata.resourceVersion`, so annotations, finalizers, status and unknown spec keys are untouched and a concurrent change is a `409`. Validation is shared with create; namespace, name and target are immutable (`400`). Objects the typed model cannot represent (`advanced`, `fallback`, other trigger types or settings, ClusterTriggerAuthentication, container policy `mode`/`controlledValues`, `recommenders`, …) are `editable:false` with reasons, and credential-looking trigger settings are never returned. Roles and namespaces as for create (GET is a namespaced read); audited as `autoscale.vpa_edit|keda_edit` with changed field names only. The ClusterRole gains `patch` on VPAs and ScaledObjects

### Added — OCI registry credentials
- Private OCI chart registries. `GET`/`POST /api/v1/helm/registry-credentials` and `DELETE /api/v1/helm/registry-credentials/{name}` store write-only `{"name","registry","username","password"}` credentials (`registry` is `host[:port]` only; password or access token at most 4 KiB; body at most 16 KiB, decode errors quote nothing). Admin only for all three, the listing included, which shows `name`, `registry`, `username`, `age` and `inUse` (the repositories that reference it) and never the password. `409` on an existing name (never overwritten), `409` when deleting a credential a repository still uses; no update (rotate = delete + recreate). All credentials live in one Secret, `zyvor-axiom-helm-registry-creds` (key `credentials.json`, conflict-safe updates; memory in demo). Audited as `helm.registry_credential_create` / `helm.registry_credential_delete` with name and registry only
- `POST /api/v1/helm/repos` accepts `credentialsSecret` for OCI repositories: the credential must exist and be for the repository's host. Browsing answers a `Basic` challenge from the registry host, and a bearer challenge by requesting the token with the credential — only from the registry host or a realm on the same registrable domain; any other realm is refused instead of being sent the password, and redirects never carry it. Tokens are cached in memory for their `expires_in` (60 s default, 5 min cap). A registry that needs credentials now says "this registry needs credentials: add them under Helm → Registries". Repository rows gain `credentialsSecret` (the name), `authenticated` and `plainHttp`
- OCI installs and upgrades use the credentials of the stored repository their `ociRef` belongs to (registry host + longest path prefix): the Helm Job mounts a short-lived Secret with a Docker-style `config.json`, owned by the Job, and sets `HELM_REGISTRY_CONFIG`; the password is never in the Job's args or env or in the audit entry (detail `<ociRef> registry-credential=<name>`). A repository added over plain http (with `AXIOM_HELM_ALLOW_INSECURE_REPOS`) is recorded as `plainHttp` and installed with `--plain-http`, so plain-http registries can now be installed from. Index installs and anonymous OCI installs are unchanged
- `helm.upgrade` audit entries for OCI charts now carry the `ociRef` (they recorded an empty chart before)

### Added — Durable fleet history
- Fleet health history now survives restarts. Every `AXIOM_FLEET_HISTORY_PERSIST_INTERVAL` (default `5m`, minimum `1m`, `off` disables; Helm `auth.fleetHistoryPersistInterval`) and on graceful shutdown, each replica saves the last 24 h of its history at 5-minute resolution (at most 288 points per cluster, with probe and failed-probe counts and incident reasons) to its own key `history-<POD_NAME or hostname>` of the `zyvor-axiom-fleet-history` ConfigMap (gzip'd JSON, whole object capped at 900 KiB by dropping the oldest buckets of the largest cluster). At startup every replica's key is merged into memory before the first probe; clusters no longer registered are dropped and keys of replicas silent for 24 h are deleted. Save errors are logged at most every 10 minutes and never stop the console
- `GET /api/v1/fleet/history` gains top-level `durable` (a persistent store is configured and the last save succeeded within 3 intervals) and `restoredFrom` (when the restored snapshot was saved, or `null`); restored samples carry `restored: true`, and each cluster lists restart `gaps`. A restart gap is never counted as downtime and never creates or extends an incident
- The Helm chart and `deploy/k8s` manifest set `POD_NAME` from the downward API (used as the replica id)

### Security — per-session pod origins
- The pod proxy origin can give every pod session its own hostname, and so its own origin: set `AXIOM_POD_PROXY_ORIGIN` with `{id}` as the leftmost host label (`https://{id}.pods.example.com`, or `http://{id}.localhost:5091` in a lab). `window-link` then returns `https://<sid>.pods.example.com/p/<ticket>/` with a fresh 128-bit session id per link and a ticket bound to it (a ticket presented on another session's host is refused and burnt); after redeeming, the session lives at the root of its host behind a host-only cookie (`__Host-axiom_pod_proxy` on https), so pods opened through the proxy can no longer read each other's `localStorage`, IndexedDB or cookies, and pod UIs with absolute paths (`/static/…`, `/api/…`) work. The listener routes on the `Host` header and answers an empty `404` to any other host or an unknown/expired session. All existing hardening (revocation per request, 10 min sliding / 1 h max, per-user caps, credential stripping, CSP, service-worker refusal, `Set-Cookie` stripping, limits) applies unchanged. Startup refuses a template whose base domain is, contains or sits inside the console's host. Needs wildcard DNS and a wildcard certificate: new optional `AXIOM_POD_PROXY_TLS_CERT` / `AXIOM_POD_PROXY_TLS_KEY` (Helm `podProxy.tlsSecret`, an existing `kubernetes.io/tls` Secret) let the listener serve one separate from the console's. `GET /api/v1/realtime/capabilities` adds `podProxyMode` (`path` or `host`). The fixed-origin form is unchanged

## [0.10.0] - 2026-11-04

### Added — Languages
- The console is available in English, Deutsch, Español and Français (i18next + react-i18next). The language follows the saved choice (`localStorage` `axiom.lang`), else the browser language, else English, and can be changed from the user menu, the login page and Settings → Appearance (`data-testid="language-select"`); `<html lang>` follows it. English is bundled and is the fallback for any missing string; the other languages load on demand as separate chunks
- Translated: the app shell (sidebar, top bar, command palette, shortcuts, user menu, banners, error pages), sign-in, account security and the two-factor gate, Users, and every page and its dialogs, toasts and empty states. Kubernetes kinds and terms, YAML, `kubectl` commands and text returned by the API stay as they are. Numbers, dollar amounts and dates are formatted for the selected language
- `npm run lint:i18n` (run by `npm run lint`) fails on missing or extra keys, mismatched placeholders and incomplete plurals, prints each language's completion and warns about UI text that is still hard-coded; `frontend/src/locales/README.md` explains how to add a string, a namespace or a language, and `frontend/src/locales/GLOSSARY.md` keeps terminology consistent

### Security — pod proxy origin
- Pod web UIs ("Open" on a pod port) can now be served from a separate origin. Set `AXIOM_POD_PROXY_ADDR` (e.g. `:5091`) and `AXIOM_POD_PROXY_ORIGIN` (e.g. `https://pods.example.com`; Helm `podProxy.addr` / `podProxy.origin`, which also adds a `pod-proxy` Service port) to run a second listener for pod content. The console mints a link with `POST /api/v1/pods/{namespace}/{name}/window-link` (operator/admin, namespace-checked like the window route) carrying a single-use 60-second ticket; redeeming it on the proxy origin sets a path-scoped HttpOnly session cookie there (10 minutes sliding, 1 hour max) that is checked against the console's revocations on every request. The proxy listener only serves `/p/<ticket>/` and `/s/<sid>/…`, never accepts the console's cookie or Basic/Bearer/API-key credentials, has no CORS, and forbids framing. With it on, the in-origin `/window/{port}/` route answers `410`, so no console credential goes in a URL for it any more. `GET /api/v1/realtime/capabilities` reports `podProxy`. Off by default; a missing half or a proxy origin equal to the console's is logged and leaves the old behaviour in place
### Added — LDAP MFA
- Two-factor sign-in for LDAP directory accounts. A person signed in with an LDAP bind can enrol a TOTP authenticator from Account security (`POST /api/v1/auth/mfa/setup` re-checks the directory password with a bind); afterwards the directory password alone is refused over Basic (`401` + `X-Axiom-MFA: required`) and `POST /api/v1/auth/login` collects the code, with the same replay protection, single-use recovery codes and per-IP/per-account throttling as local accounts. The password is verified before anything about the second factor is revealed. Sessions for directory accounts carry `Issuer: "ldap"` and the directory-mapped role (a group change applies at the next sign-in) and `GET /api/v1/session` reports `auth: "ldap"`. A local account with the same username takes precedence. Records live in the new `zyvor-axiom-external-mfa` Secret (key `records.json`, conflict-safe updates; memory in demo)
- `AXIOM_REQUIRE_MFA` now also covers directory accounts by their mapped role: an unenrolled covered LDAP user gets the `mfaEnrollmentRequired` hold, and `GET /session` reports `mfa: {required, enrolled}` for them. SSO sessions and API keys remain unaffected
- `DELETE /api/v1/users/{username}/mfa` also resets a directory account's second factor (and revokes its sessions) when no local account has that name; `404` only when neither exists. New `GET /api/v1/users/external-mfa` (admin only) lists directory accounts with a second factor: `[{username, enabled, updatedAt}]`, never secret material. MFA audit entries for directory accounts carry `detail: "ldap"`
### Added — VPA & KEDA create
- Create and delete VerticalPodAutoscalers and KEDA ScaledObjects from typed, validated requests, with a pure preview of the rendered YAML: `POST /api/v1/vpas/preview`, `POST /api/v1/vpas`, `DELETE /api/v1/vpas/{namespace}/{name}`, and the same three under `/api/v1/scaledobjects`. Raw YAML is never accepted. VPAs default to `updateMode: Off` (recommend only) with per-container cpu/memory bounds; ScaledObject triggers come from a closed allow-list (cpu, memory, cron, prometheus, kafka, redis, rabbitmq, aws-sqs-queue) with per-type setting keys, credentials refused in settings and accepted only as `authenticationRef` to an existing TriggerAuthentication, and KEDA's scale-to-zero rule enforced. Warnings for restarting update modes and, on create, for an HPA already scaling the same workload. `422` without the CRD, `409` on exists. `GET /api/v1/triggerauthentications` lists TriggerAuthentication names for the form. Operator and admin; namespace-restricted operators only in their own namespaces. Audited as `autoscale.vpa_create|vpa_delete|keda_create|keda_delete`. The ClusterRole gains `create`/`delete` on VPAs and ScaledObjects and `get`/`list` on TriggerAuthentications

### Fixed
- `POST /api/v1/hpas` now checks the body's namespace against a namespace-restricted account (previously only a `?namespace=` query parameter was checked by the guard)
### Added — GitOps edit & credentials
- Edit a GitOps application in place: `GET /api/v1/gitops/{kind}/{namespace}/{name}/edit` returns the creation form's typed fields read from the live Argo CD Application / Flux Kustomization + GitRepository / HelmRelease + HelmRepository (with `resourceVersion`, `editable`, `reasons`, `readOnly`); `POST …/edit/preview` returns a structured diff of the managed fields (`changes[{object,field,from,to}]`, the resulting YAML, the creation warnings, `noop`); `PUT /api/v1/gitops/{kind}/{namespace}/{name}` applies only those fields as a JSON Patch guarded by a `resourceVersion` test (`409` "the application changed since you opened it"), leaving labels, annotations, finalizers, status and unknown spec fields untouched. Same validation as creation; identity fields and the git-path/Helm-chart source type are immutable; objects using features the typed model cannot represent are `editable:false`; Flux source fields change only when the source is Axiom's and unshared. Admin only (the GET: admin, operator, viewer); audited as `gitops.edit` with the changed field names
- Write-only GitOps repository credentials: `GET`/`POST /api/v1/gitops/credentials` and `DELETE /api/v1/gitops/credentials/{engine}/{namespace}/{name}` create Argo CD repository Secrets (`argocd.argoproj.io/secret-type: repository`) and Flux `secretRef` Secrets (basic, token, ssh with known hosts) after validating the URL, the auth fields and the SSH key; `409` on an existing name (never overwritten), delete only for Secrets Axiom labelled, no update (rotate = delete + recreate). Admin only for all three, the listing included; the listing shows only Argo-labelled and Axiom-labelled Secrets and never any data; secret values never appear in responses, errors, logs or the audit (`gitops.credential_create` / `gitops.credential_delete` with names and auth type only). No RBAC change: the ClusterRole already covers Secrets
### Added — OCI Helm catalog
- Browse and install Helm charts from OCI registries. `POST /api/v1/helm/repos` accepts `{"name","url":"oci://host/path","charts":[…]}`: charts are listed explicitly (most hosted registries do not offer `/v2/_catalog`, which is used when `charts` is omitted), versions are the tags that parse as semver, metadata comes from the Helm config blob of the newest release, and values/README are read from the chart layer, so a catalog entry is built without downloading the chart. Anonymous registries only, with the standard bearer-token challenge answered by an anonymous token; `credentialsSecret` is refused for now. The add verifies the registry and every listed chart. Repository rows and chart entries gain `kind` (`index`/`oci`), chart entries gain `ref` (`oci://host/path/chart`), and `POST /api/v1/helm/install` / `/helm/upgrade` accept `ociRef`, which runs `helm install <release> oci://…` (no `--repo`, literal args, validated against flags, userinfo and whitespace). The SSRF rules, caps, cache and the 20-repository limit are shared with index repositories and cover the token realm and every blob redirect; blob digests are verified while streaming and a mismatch is refused. `AXIOM_HELM_ALLOW_PRIVATE_REPOS` / `AXIOM_HELM_ALLOW_INSECURE_REPOS` apply to OCI registries too
### Added — OpenCost & fleet history
- Actual cost from OpenCost: with `AXIOM_OPENCOST_URL` (+ optional `AXIOM_OPENCOST_TOKEN`; Helm `cost.opencostUrl` / `cost.opencostToken`) `GET /api/v1/cost` reads OpenCost's `/allocation/compute` (aggregated by namespace and controller) and reports actual cost scaled to a 30-day month: `source: "opencost"`, `window` (`?window=7d|14d|30d`, default 30d, anything else `400`), per-row `cpuUsd` / `memoryUsd` / `storageUsd` / `networkUsd` / `otherUsd`, `idleMonthlyUsd`, `unallocatedMonthlyUsd` and a `note`; `cpuCores` / `memoryGi` stay the requests. OpenCost failures fall back to the request estimate (`source: "estimate"`, HTTP 200, the reason in `note`, never the URL). Answers are cached 60 s with a single flight; the client has the Prometheus client's limits (10 s, 8 MiB, no proxy, same-host redirects). Budgets (`GET /api/v1/cost/budgets`, the budget watcher) compare against the active source and report it as `source`. Rightsizing savings keep the request rates. Demo: `AXIOM_DEMO_OPENCOST=true` serves fixed OpenCost-shaped actuals
- Fleet health history: every probe now also appends a sample (state after the two-failure rule, latency, node / pod / warning counts, CPU / memory) to an in-memory ring per cluster (24 h, at most 3,000 samples, dropped on unregister). `GET /api/v1/fleet/history?cluster=&window=1h|6h|24h` returns per-cluster `uptimePercent` (time-weighted, gaps capped at two probe intervals), `checks`, `failedChecks`, `currentState`, `since`, `incidents` (non-healthy runs, short healthy gaps merged, `to: null` while ongoing) and up to 240 `samples` (bucketed: worst state wins, averages for latency / CPU / memory). Same access as `/fleet`. History is per replica and lost on restart. Demo: `AXIOM_DEMO_FLEET_HISTORY_SEED=true` backfills 24 h with one incident

## [0.9.1] - 2026-11-04

### Fixed
- Fleet health no longer flaps on a real cluster. A cluster that was reachable now needs two consecutive failed probes before it is declared unreachable (one slow overview call used to flip it and, with notifications on, announce an outage that cleared a minute later), and warning events alone no longer mark a cluster degraded (the Kubernetes provider's "Attention" status means "there are warnings"; only unready nodes or an explicitly unhealthy status do). Found on the dev cluster, where it had logged repeated degraded/recovered pairs

## [0.9.0] - 2026-11-04

### Added — QR enrolment and the enrolment gate
- Two-factor enrolment now shows a QR code to scan (rendered in the browser; the secret never leaves the page), with the typed setup key as a fallback. When `AXIOM_REQUIRE_MFA` covers an account that has not enrolled, the console shows only an enrolment screen until it has

### Added — Fleet search
- Cross-cluster search (`GET /api/v1/fleet/search?q=&kind=&limit=`): finds workloads, services and namespaces whose name or namespace (or workload image) contains the query across every registered cluster, each through its own provider, up to 8 clusters at once with a 5 s timeout each. Unreachable clusters are skipped and reported, a failing cluster is reported without failing the search, and results are ranked exact name, prefix, contains, then namespace/image matches with `total`/`truncated`. Same access as `/fleet` (namespace-restricted accounts get `403`); rate-limited to 20 searches per user per 10 s (`429` with `Retry-After`)
### Added — Notifications
- Cluster-down and budget notifications through the existing Slack / generic alert webhook (same channels and the same `notifyOnWarnings` switch as warning events, no new settings). The fleet prober announces a cluster becoming `unreachable` (with the probe error) or `degraded` (with the reason) and its recovery (with how long it was down); a new budget watcher (`AXIOM_BUDGET_CHECK_INTERVAL`, default `5m`, minimum `1m`, `off` disables; Helm `auth.budgetCheckInterval`) evaluates every cost budget with the same code as `GET /api/v1/cost/budgets` and, on ok→warning, →over and back to ok, records `cost.budget_warning` / `cost.budget_over` / `cost.budget_ok` audit entries (user `budget-watcher`, detail `$X of $Y (Z%)`) and notifies. The same state is not re-announced for a cluster/namespace within 10 minutes; a recovery or back-to-ok message always follows an announced problem; nothing is announced for conditions found at startup. In the Timeline, `fleet.cluster_unreachable` / `fleet.cluster_degraded` / `cost.budget_warning` / `cost.budget_over` are warnings and `cost.budget_*` entries carry their namespace. Delivery is best effort (one attempt, failures logged with the webhook URL redacted) and per replica
### Added — Identity
- Require-MFA policy: `AXIOM_REQUIRE_MFA` = `off` (default) | `admin` | `operator` | `all` (Helm `auth.requireMFA`) makes a second factor mandatory for local accounts in those roles. A covered account that has not enrolled can still sign in but every request except reading its session, enrolling and signing out gets `403` `{"error":"…","mfaEnrollmentRequired":true}`, enforced centrally in the auth guard. SSO/LDAP accounts and API keys are unaffected; the built-in admin is exempt (with a start-up warning) unless `AXIOM_TOTP_SECRET` is set. `GET /api/v1/session` gains `mfa: {required, enrolled}`
- Shared session revocation across replicas: each replica re-reads the `zyvor-axiom-revocations` ConfigMap every `AXIOM_REVOCATION_REFRESH` (default 15s, minimum 5s, `off` disables; Helm `auth.revocationRefresh`) and merges it, and writes are read-modify-write with resourceVersion conflict retries, so a logout on one replica applies everywhere within the interval and concurrent revocations are never lost
### Added — Rightsizing apply
- Apply a rightsizing recommendation for one container: `POST /api/v1/cost/rightsizing/apply` with `{namespace, kind, workload, container, resources:["cpu","memory"], window, dryRun, observed}`. The server recomputes the recommendation (same pipeline and window as `GET /api/v1/cost/rightsizing`; the client never sends target numbers), and `dryRun:true` returns the exact `changes` without writing. A real apply strategic-merge patches the workload's pod template requests (field manager `zyvor-axiom`), which triggers the normal rolling update; limits are never lowered or added, and an existing limit below the new request is raised to it and reported. Refused with `409` when the container is already right-sized or when `observed` no longer matches the live requests (stale view), `404` for an unknown workload/container. Operator and admin only (viewers and namespace-restricted accounts get `403`); audited as `cost.rightsizing_apply`
### Added — GitOps delete
- Delete GitOps applications safely: `GET /api/v1/gitops/{kind}/{namespace}/{name}/delete-preview` reports whether deleting an Argo CD `Application` (cascades iff it carries `resources-finalizer.argocd.argoproj.io` or a `/…` variant), a Flux `Kustomization` (iff `spec.prune`) or a Flux `HelmRelease` (uninstalls unless `spec.suspend`) also removes what it deployed, with a plain-English consequence, the Flux source and which other objects share it, and whether Axiom created it. `DELETE /api/v1/gitops/{kind}/{namespace}/{name}` recomputes that at delete time and requires `confirm=<name>` (exact) when it cascades; `removeSource=true` also deletes the Flux source, only when unshared and created by Axiom (`409` otherwise). Admin-only (operators keep sync; viewers and operators can read the preview), audited as `gitops.delete`. Objects created by Axiom now carry `app.kubernetes.io/managed-by: zyvor-axiom`. The ClusterRole gains `delete` on Argo CD applications and Flux kustomizations/helmreleases

## [0.8.0] - 2026-11-04

### Added — Autoscaling & cost
- Autoscaling beyond CPU: `GET /api/v1/hpas` now returns every `autoscaling/v2` metric (`metrics[]` with rendered target/current, e.g. memory, Pods, Object and External metrics), `desiredReplicas`, status `conditions`, `lastScaleTime` and `behavior`; `POST /api/v1/hpas` accepts `targetMemoryPercent` and `scaleDownStabilizationSeconds` alongside `targetCpuPercent`, with stricter validation (1-100 targets, `maxReplicas` <= 1000, DNS-1123 names, Deployment/StatefulSet/ReplicaSet targets). Read-only VerticalPodAutoscaler (`GET /api/v1/vpas`, recommendations per container) and KEDA ScaledObject (`GET /api/v1/scaledobjects`, triggers by type and name only — never metadata or authentication refs) views report `installed:false` when the CRD is absent. The ClusterRole gains read on `verticalpodautoscalers.autoscaling.k8s.io` and `scaledobjects.keda.sh`
- Cost budgets and rightsizing: administrators set per-namespace monthly budgets (`PUT`/`DELETE /api/v1/cost/budgets/{namespace}`, kept in ConfigMap `zyvor-axiom-cost-budgets`, audited as `cost.budget_set` / `cost.budget_removed`), and `GET /api/v1/cost/budgets` compares each with the estimate (`ok` / `warning` at `warnPercent`, default 80 / `over`) and lists the biggest unbudgeted namespaces. `GET /api/v1/cost/rightsizing?namespace=&window=7d` recommends per-container CPU/memory requests (usage + 20 % / 15 % headroom) with an over / under / ok verdict and the monthly saving at the `GET /cost` rates, from Prometheus p95/p99 history when `AXIOM_PROMETHEUS_URL` (+ optional `AXIOM_PROMETHEUS_TOKEN`; Helm `cost.prometheusUrl` / `cost.prometheusToken`) is set, else a low-confidence metrics.k8s.io snapshot. Recommendations are never applied. Both endpoints are cluster-wide, so namespace-restricted accounts get `403`

## [0.7.0] - 2026-11-04

### Added — GitOps & Helm
- Create GitOps applications from the console: `POST /api/v1/gitops` creates an Argo CD `Application`, a Flux `GitRepository` + `Kustomization`, or a Flux `HelmRepository` + `HelmRelease` from a typed request, and `POST /api/v1/gitops/preview` renders the exact YAML (with warnings such as "tracks a moving branch" or "automated prune on the whole repository") without touching the cluster. Objects are built field by field, never from client YAML; credentials in a repository URL are refused (use a Flux `secretRef` or Argo CD repository credentials). The Flux API version is picked from what the cluster serves (v1 or v1beta2 sources, v2/v2beta2/v2beta1 HelmReleases), a missing engine answers `422`, an existing object `409`, and a source created before a failure is rolled back. Admin-only (operators get `403`, like Helm install) and audited as `gitops.create`. The ClusterRole gains `create` on Argo/Flux applications and `create`/`delete` on Flux `gitrepositories`/`helmrepositories` (`delete` is used only for the rollback)
- Helm chart catalog: administrators add chart repositories (`POST /api/v1/helm/repos`, kept in ConfigMap `zyvor-axiom-helm-repos`), and every role except namespace-restricted accounts can search their charts (`GET /api/v1/helm/charts`), list a chart's versions and read its default `values.yaml` and README (`GET /api/v1/helm/charts/{repo}/{chart}[/values]`). Indexes are cached for 10 minutes and a failing repository keeps serving its last good copy. Fetches are SSRF-hardened: https only, public addresses only (checked at connect time, on every redirect and for chart archive URLs), with `AXIOM_HELM_ALLOW_PRIVATE_REPOS` / `AXIOM_HELM_ALLOW_INSECURE_REPOS` for in-cluster repositories. Adding and removing repositories is audited (`helm.repo_add`, `helm.repo_remove`)

## [0.6.0] - 2026-11-03

### Added — Fleet
- Real cluster health probing: every registered cluster is asked for its overview in the background (`AXIOM_FLEET_PROBE_INTERVAL`, default 30s; 8s timeout each, so one dead cluster never stalls the rest) and reported as healthy, degraded (nodes not ready) or unreachable, with latency, the last error and since when. Registered clusters used to be reported Healthy unconditionally
- A **Fleet** page (`/fleet`, `GET /api/v1/fleet`, `POST /api/v1/fleet/refresh`) showing every cluster's state, nodes ready, pods running, warnings and CPU/memory with a roll-up and one-click "Switch to"; Settings → Clusters shows the same state, and the top-bar cluster chip has a state dot for the active cluster
- State changes are written to the audit log (`fleet.cluster_unreachable`, `fleet.cluster_degraded`, `fleet.cluster_recovered`); a cluster registered from Settings is probed immediately
- Demo mode can simulate an outage (`AXIOM_DEMO_FLEET_DOWN`, `AXIOM_DEMO_FLEET_DEGRADED`) for testing

### Changed
- `GET /api/v1/clusters` now carries `state`, `latencyMs` and `error`; its `healthy` flag is false only for an unreachable cluster
- The fleet endpoints are cluster-wide, so namespace-restricted accounts get `403`

## [0.5.0] - 2026-11-03

### Added — Identity
- Two-factor sign-in (TOTP, RFC 6238) for local accounts: enrol from the user menu → **Account security** (confirm the password, add the setup key to an authenticator app, confirm a code, save ten one-time recovery codes). An account with MFA can no longer authenticate with HTTP Basic alone; the sign-in page asks for the code and `POST /api/v1/auth/login` returns a session. Codes are single-use per time step, recovery codes work once, and wrong codes are throttled like wrong passwords
- `AXIOM_TOTP_SECRET` adds the same second factor to the built-in admin account
- Server-side session revocation: logout now ends that session, "Sign out everywhere" (user menu, and per user on the Users page for admins) ends all of them, and role/namespace/password changes, deletion and MFA changes revoke the user's sessions automatically. Local sessions track the live account, so a role or namespace change applies immediately. Revocations persist in a ConfigMap
- `AXIOM_SESSION_TTL` sets the session lifetime (default 12h)
- Administrators can reset a locked-out person's two-factor and see who has it on the Users page

### Changed
- **Breaking for scripts that use Basic auth against an account that enables MFA:** use an API key or `POST /api/v1/auth/login` instead. Accounts that do not enable MFA are unaffected
- `/api/v1/session` reports `auth: "local"` for sessions from the local sign-in

### Security
- SAML: `GET /api/v1/auth/saml/login` now sends a real AuthnRequest with a random ID, tracked in a signed 5-minute `axiom_saml_req` cookie and a single-use in-memory set (per replica). The ACS requires the signed `InResponseTo` to match and consumes it, so a captured SAMLResponse can no longer be replayed within its validity window. `SubjectConfirmation` must be bearer, and `SubjectConfirmationData` `Recipient` must equal the ACS URL and `NotOnOrAfter` must not have passed. IdP-initiated (unsolicited) responses are now rejected unless `AXIOM_SAML_ALLOW_IDP_INITIATED=true` — **breaking** for setups that sign in from an IdP dashboard tile. With several replicas, SAML sign-in needs session affinity.

## [0.4.2] - 2026-11-03

### Fixed
- Access → "Who can…": the binding and its role were run together in the Via column ("ClusterRoleBinding/cluster-adminClusterRole/cluster-admin"); the role now sits on its own line.

## [0.4.1] - 2026-11-03

### Fixed
- End-to-end mock OIDC identity provider now issues RS256-signed ID tokens with a JWKS endpoint and the echoed nonce, and tests that unsigned, wrongly-signed and wrong-nonce tokens are rejected. Test-only change; no product behaviour differs from 0.4.0, but this is the first release whose tagged commit has a green CI.

## [0.4.0] - 2026-11-03

### Added — UX
- Global namespace selector in the top bar; filters and search on Pods, Workloads, Events, Jobs, Services, Ingresses, Storage and Custom Resources are kept in the URL (shareable, survive reload), with saved views per page
- Keyboard shortcuts (`/` search, `g` then `p`/`w`/`n`/`e`/`l` to navigate, `?` for the full list); a "System" theme option next to Light and Dark
- Apply and YAML edits now go through a diff review (`POST /api/v1/apply/diff`, dry-run server-side apply) that masks Secret values and warns when another tool (Argo CD, Flux, Helm) also manages the object
- Resource relationship graph (`GET /api/v1/graph/{namespace}/{kind}/{name}`) as a Graph tab on workloads; in-app notification inbox behind the top-bar bell
- "Explain with AI" also appears on health-check findings

### Added — security views
- Read-only RBAC viewer (`/api/v1/rbac/subjects`, `/rbac/who-can`), ServiceAccounts list, NetworkPolicy analysis (`/networking/policies/{namespace}/analysis`) and a Security page for Trivy-operator and PolicyReport results (empty state when the operators are absent)

### Added — troubleshooting
- **Change timeline**: `GET /api/v1/timeline` merges Kubernetes events, Axiom audit entries, alert warnings and Deployment rollout revisions into one newest-first feed (`namespace`, `object`, `since`, `limit` filters; namespace-restricted accounts must name an owned namespace). New Timeline page under Observe, a "What changed" tab on Workload detail, and an "Open timeline" link from Events.
- **Cluster health check**: `GET /api/v1/diagnose?namespace=` runs deterministic analyzers (crash-loop / `OOMKilled` / image-pull from the last termination state, Services without endpoints, Pending PVCs, Ingresses with a missing backend, HPAs that cannot scale, node pressure) and returns findings with a severity and a suggested fix. Shown on Overview and on a new Health check page. Works in demo and Kubernetes mode.
- **Workload logs**: `GET /api/v1/workloads/{namespace}/{kind}/{name}/logs` merges the logs of every pod and container (prefixed `[pod/container]`); the Logs page gained a Workload mode, a regex filter, a timestamps toggle, a previous-container toggle and Download.
- **Ephemeral debug containers**: `POST /api/v1/pods/{namespace}/{name}/debug` (operator+, audited `pod.debug`) adds a non-root, capability-dropped container (`AXIOM_DEBUG_IMAGE`, default `busybox`) through the `pods/ephemeralcontainers` subresource, with a terminal attached on the Pod page. RBAC: `pods/ephemeralcontainers` `update` in both `deploy/k8s/rbac.yaml` and the Helm chart.
- **Node shell** (off by default): `AXIOM_ENABLE_NODE_SHELL=true` enables an admin-only, audited (`node.shell`) root shell on a node through a short-lived privileged pod in a dedicated namespace that is deleted on disconnect or timeout; refused for operators, viewers and namespace-restricted accounts. It conflicts with the Pod Security `restricted` profile on its own namespace (documented in `docs/DEPLOYMENT.md`).
- **Port-forward helper**: "Copy kubectl port-forward" button next to each port's Open button on the Pod page (a browser cannot open a local listener, so TCP forwarding stays a kubectl job).
### Added
- Opt-in, bring-your-own-LLM "Explain with AI" (off by default): Settings → AI assistant (OpenAI-compatible, Anthropic or Ollama endpoint, model, write-only API key stored in the integrations Secret, redaction switch, connection test); an "Explain with AI" action on the pod diagnosis banner and on Warning events opens a right-hand drawer that states where the data will be sent and shows the exact redacted payload before anything leaves the cluster. Context is limited to the caller's namespace (events, diagnosis, container status, a short log tail), never includes Secret/ConfigMap data or env values, and is scrubbed of tokens, keys, JWTs, kubeconfig text, base64 blobs and (optionally) IPs/hostnames. Read-only (no tool calls), rate limited per user, audited as `ai.explain` (target only). New `POST /api/v1/ai/explain`, `GET /api/v1/ai/status`, `POST /api/v1/integrations/ai/test`. See `docs/DEPLOYMENT.md` (AI assistant and privacy).

### Security
- Login throttling is now also per attempted account name (on top of per client IP), keyed whether or not the account exists, with an injectable clock and a bounded table that does not evict locked entries; a valid login from another account can no longer reset the counter of an account being guessed (closes the interleaved-success bypass listed in `docs/GAPS.md`)
- Single-use, 30-second WebSocket/SSE/log-stream tickets (`POST /api/v1/ws-ticket`, `?ticket=`) replace credentials in the URL; the terminal now uses them. `?access_token=`, `?axiom_session=` and `?api_key=` keep working but are deprecated (`Deprecation` header + one-time server warning)
- OIDC ID tokens are now signature-verified against the provider's JWKS (RS/PS/ES algorithms only, `kid` required to match, cached with timeouts and a re-fetch floor) and bound to the browser by a `nonce`; `alg=none`, HMAC-with-public-key, wrong key, unknown `kid`, expired and nonce-mismatch tokens are rejected
- Helm install/upgrade values are stored in a Secret owned by the Job (garbage-collected with it) instead of a permanent plaintext ConfigMap; Helm Jobs honour the Axiom namespace, `AXIOM_SERVICE_ACCOUNT` and `AXIOM_HELM_IMAGE`, get DNS-safe names of at most 63 characters, and refuse release/chart/repo/version/namespace values that start with `-`
- One-shot exec output is capped at 1 MiB, interactive terminals at 64 concurrent sessions and 1 MiB frames
- The VM VNC proxy re-reads the rotating ServiceAccount token and has a handshake timeout
- Webhook/Slack errors and logs no longer contain the (secret) webhook URL
- Registering a cluster rejects kubeconfigs that use exec/auth-provider plugins, `tokenFile`, client-cert/CA file paths, impersonation or `proxy-url`, and bounds the request body
- OIDC: all IdP calls have a 10 s timeout; the ID token's `iss`, `aud` and `exp` are validated
- Local accounts are checked before LDAP, so local users' passwords are no longer sent to the directory on every request
- Login rate limiter keys on the rightmost (proxy-appended) trusted-header address, groups IPv6 by /64 and is size-bounded
- Metrics series are capped and methods/paths sanitised; request logs quote the path (no log forging)
- Outside demo mode `/auth/providers` no longer advertises `demo / demo`, the default username, the API-server URL or other cluster names; TLS 1.2 minimum; startup warning for default credentials

### Changed
- Full frontend redesign: the app shell (top bar, sidebar) and all 37 pages rebuilt around a flat, dark apple.com visual language — plain black `#000`/`#1d1d1f` (dark) or white/`#f5f5f7` (light) surfaces, hairline borders, pill buttons and status badges, no glass panels, no ambient gradient washes, no glow rings. Dark theme is no longer GitHub-Primer-derived; light and dark now share one palette source. New `frontend/src/axiom-shell.css` carries the component vocabulary (`.bar`/`.side`/`.card`/`.pill`/`.tbl`/`.tile`/`.vm`/`.tabs`/etc); `PageHeader`/`DetailHeader`/`StatusBadge`/`GlassCard`/`AutoTable` were re-skinned once and cascade the look everywhere. Sidebar gained live workload/pod/alert counts, collapsible groups, and a mobile slide-in drawer; top bar gained an in-bar theme toggle and an alerts bell. `MetricRing.jsx` was removed (Overview's capacity band now uses meter bars). Every API call, mutation, dialog, and `data-testid` was preserved — this is a markup/styling pass, not a behavior change. See `docs/DESIGN-SYSTEM.md`.
- Login page: SSO is now a plain "Sign in with SSO"/"Sign in with SAML" text link under the password form instead of per-provider (Google/GitHub/Okta) icon buttons; added a "Remember me on this device" checkbox (persists only the username) and an inline error banner on failed sign-in.
### Docs
- GitHub Pages: full marketing landing page with real product screenshots, and the docs rendered as a browsable site (`scripts/build-site.py`)


### Fixed
- `scripts/deploy-remote.sh` reuses the existing `zyvor-axiom-auth` Secret on the target instead of resetting it to demo/demo on every redeploy
- Log follow is no longer cut off at the API timeout; every client path (in-cluster, multi-context, remote clusters) now has the same timeout and a 50 QPS / 100 burst limit
- Persisted audit ring is trimmed (oldest first) to stay under the 1 MiB ConfigMap limit instead of silently failing to save
- Helm release listing selects `owner=helm` Secrets and decodes only each release's newest revision
- Utilization no longer invents 42.8%/60% when metrics-server is absent (reports `unavailable`) or pads short histories with a decorative curve, and its history is per cluster
- Deployment "undo" is a single conflict-safe update without the spurious annotation rollout and drops `pod-template-hash`
- Alerts ask the API server for Warning events (the 200-event cap used to apply before the filter, hiding warnings behind Normal events)
- `CodeEditor.jsx`'s editable manifest tabs (Pod/Workload detail YAML) now open straight into edit mode instead of requiring an extra "Edit" click first, matching their documented behavior as editable tabs.
- `DELETE /api/v1/vms/{namespace}/{name}` was rejected with a 403 in real (non-demo) deployments — the `zyvor-axiom` ClusterRole granted `get/list/watch/update/patch/create` on `kubevirt.io` VMs but not `delete`. Found by validating the enterprise write endpoints against a real cluster (`e2e/scripts/validate-enterprise-writes.sh`) rather than only the demo provider/fake clientset unit tests; VM create/list, Service/Ingress/ResourceQuota create+list+delete, fleet cluster register/activate/unregister, and Helm install/upgrade/uninstall all round-tripped correctly against the real cluster.

### Added
- Read-only RBAC viewer: `GET /api/v1/rbac/subjects`, `/rbac/subjects/{kind}/{name}` (effective cluster-wide and namespaced permissions, including implicit groups) and `/rbac/who-can?verb=&resource=&namespace=`, plus a read-only ServiceAccount list; Ops → Access gets "By subject" and "Who can…" views. Operator/admin only (viewers and namespace-restricted accounts get 403)
- NetworkPolicy analysis: `GET /api/v1/networking/policies/{namespace}/analysis` computes each pod's ingress/egress isolation and allowed peers/ports from the policies' selectors, flags pods not covered by any policy, and is shown on the Networking page
- Security page (`/security`): aggregates Trivy Operator VulnerabilityReports / ConfigAuditReports and `wgpolicyk8s.io` PolicyReports (Kyverno, Gatekeeper-compatible) with severity counts and per-workload drill-down, and an install hint when the CRDs are absent (`GET /api/v1/security/reports`). `zyvor-axiom` ClusterRole gains read access to ServiceAccounts and those CRD groups (Helm chart and `deploy/k8s/rbac.yaml`)
- Audit log can be exported as JSON as well as CSV (Ops → Audit)
- OIDC SSO validated end-to-end: minted a public (PKCE, no client secret) `axiom` client in a real Keycloak realm, wired `AXIOM_OIDC_ISSUER`/`AXIOM_OIDC_CLIENT_ID` via `PUT /api/v1/integrations`, and confirmed a full browser login mints a real Axiom session (`GET /api/v1/session` returns `"auth":"oidc"`). See `e2e/scripts/verify-sso.js`.
- LDAP group-based role mapping: `AXIOM_LDAP_ADMIN_GROUP`/`AXIOM_LDAP_VIEWER_GROUP` (plus `AXIOM_LDAP_GROUP_ATTR`, default `memberOf`) map a bound user's directory group membership to `admin`/`viewer`, matched by full DN or `cn`. Implemented as a minimal hand-rolled LDAP Search (base-scope, present filter), extending the existing hand-rolled bind — no new dependency. Only applies to the `AXIOM_LDAP_BASE_DN`-constructed bind form; both env vars unset (the default) keeps every bind at the previous blanket `operator` role, so this is backward compatible. See `docs/GAPS.md`.

### Added
- Enterprise gap closure pass: real SAML ACS sessions, LDAP bind login, operator role, fleet cluster register/unregister from kubeconfig, HA Helm guard (`sessionSecret` required when `replicaCount > 1`), audit ring default 5000, ConfigMap/Secret update, Velero on-demand backup, VM create/delete + VNC WebSocket proxy path, PVC/Service/Ingress/Quota writes, Helm upgrade. See `docs/GAPS.md`.
- List → detail drill-downs for ConfigMaps, Secrets, Services, VMs, Jobs, and CronJobs (plus namespace services tab links). Command palette, Alerts, and Events deep-link to known object kinds. Breadcrumbs cover multi-segment detail paths.

### Fixed
- Pod interactive terminal: WebSocket upgrade always failed with 500 ("closed" in the UI) because the metrics `statusRecorder` wrapper did not implement `http.Hijacker`/`http.Flusher`, so gorilla/websocket could not hijack the connection. One-shot exec kept working; only the Terminal tab was broken.

### Changed
- Appearance theme toggle and cluster Healthy/activate moved from the left rail menu to **Settings**; sidebar is a thin flat rail (no card chrome).
- Dark theme detail surfaces: colorful status chips, tinted metric tiles, syntax-highlighted manifests (Pod/Workload YAML + CRD/plugin JSON), and env/label chips. Secret-looking env values are masked in the Details tab.
- Typography: apple.com **SF Pro** stack sitewide (dropped Inter); headlines use SF Pro Display; mono prefers SF Mono.
- Surfaces: dropped card/box chrome sitewide — flat canvas with hairline dividers only (light and dark), apple.com-style underline tabs.
- Appearance: dark theme is **GitHub Primer dark** (`#0d1117` / `#161b22` / `#30363d` / accent `#2f81f7`), not charcoal. Light stays Apple.com. Theme toggle restored (default dark).
- Buttons / primary accent: dropped mist-blue, indigo, and the cosmic→sky gradient. `default` and `brand` are solid blue CTAs (Apple Action Blue in light, GitHub accent blue in dark).
- Namespaces: clicking a namespace opens `/namespaces/{name}` with pods, workloads, and services for that namespace (rows drill into pod/workload/service detail).

### Changed
- Overview: apple.com-style marketing home — centered hero (no stats strip), capacity rings, oversized inventory, utilization charts, and **Stay informed** links to Alerts / Events / Logs. Alert and activity log dumps no longer appear on `/overview`; breadcrumbs are hidden on that route.
- Shell: Zyvor mark + Axiom wordmark move to the top bar (left); compact Search chip (⌘K) sits with Connect and the user menu on the right. Brand mark removed from the sidebar (cluster switcher is the first chrome there). Header height tightened to `h-12`.
- Dark theme: soft charcoal (~14% lightness) instead of near-pure black so Overview and chrome do not read as a void.
- Login: Apple Account–style product gate above the card (Zyvor mark, **Axiom**, hero **Sign in to Axiom.**) so the destination product is unambiguous; password-step CTA matches.
- `MetricRing` gains optional `layout="stack"`; `Sparkline` accepts a `height` prop for Overview’s taller charts.

### Fixed
- Login page: two hardcoded orange hex literals in the brand-mark gradient survived the zeus-os design port (they were never wired to the `i17-cosmic` token, unlike the rest of the mark) — the sign-in page kept showing the old orange mark. Retinted to match.

### Changed
- Login page: sign-in is now a two-step flow matching Apple ID's pattern — enter your username first (step 1, alongside the SSO buttons), then a second screen shows an editable identity chip ("demo · Not you?") and just the password field. The underlying auth call is unchanged; this only changes how the two values are collected.
- Login page: replaced the two-column marketing layout (brand/stats panel + animated aurora glow) with a single minimal centered card on a flat background — closer to how appleid.apple.com's actual sign-in page looks.
- Design system: replaced the "Liquid Glass" iPhone-17 palette with zeus-os's design system and WebShell sidebar+content layout — new color tokens (soft-charcoal dark theme, indigo/plasma light theme, `success`/`warning`/`info`/`danger` semantics), self-hosted Inter/JetBrains Mono variable fonts, pill-shaped buttons, and a restructured shell: a full-width sticky header, a collapsible grouped sidebar (persisted across reloads), and breadcrumbs rendered in the content area. Every page kept working unchanged since they consume shared `PageHeader`/`DetailHeader`/`GlassCard`/`Button` primitives rather than owning their own styling.

### Fixed
- Demo mode (`AXIOM_MODE=demo`): ~17 mutating actions across node cordon/drain/taint, namespace create, VM lifecycle (start/stop/snapshot/clone/export/restore), rollout pause/resume/restart/undo, Helm install/uninstall/rollback, backup restore, GitOps sync, and CRD instance delete previously returned a success response without persisting the change — the paired list/get endpoint read from a static fixture or an untouched field, so the action silently reverted on the next page load. All now persist for the lifetime of the process.
- Demo mode: the workload detail page's Pods and Events tabs always showed empty, because `DemoProvider.GetResource` never populated them (unlike the real Kubernetes provider). Fixed to look up the workload's actual pods/events.
- Multi-cluster mode: `SwitchableProvider`'s active-cluster selection was a single value shared by every request — two browser tabs (or two users) switching clusters would silently stomp on each other's selection. Now scoped per browser tab via a session header (`X-Axiom-Session`, set once per tab and sent on every request), falling back to a per-username default for header-less API/script access. See `docs/ARCHITECTURE.md`.
- Real `KubernetesProvider.ListNodes` now reports node `status` as `"SchedulingDisabled"` when a node is cordoned — previously this was never set, so the Nodes page's Cordon/Uncordon button never flipped even though the underlying cordon succeeded.
- Every API error was reported to clients as a generic HTTP `500`, regardless of cause — including a real Kubernetes `403 Forbidden` from browsing a CRD outside the RBAC allowlist. Provider errors are now classified into the matching status (`403`/`404`/`409`/`400`/etc.) via `platform.HTTPStatus`, backed by real `apierrors` checks in the Kubernetes build and a message-phrasing heuristic in demo mode.
- Demo mode: an unbounded replica count passed to the scale or launch endpoints (e.g. `{"replicas": 999999999}`) crashed the whole server — `rebuildPodsLocked` allocated a `[]Pod` slice sized to the caller-supplied value, and a multi-gigabyte allocation triggers a fatal, unrecoverable Go runtime out-of-memory error that no HTTP handler can catch or recover from. `Scale` and `Launch` now reject replica counts above 1000 with a `400`, and `rebuildPodsLocked` itself clamps to that cap as defense in depth. Found via randomized/exploratory testing.
- ConfigMaps, Secrets, and Workloads tables: a single very long name (with no whitespace to break on) blew out that table's column widths, visually pushing or hiding the other columns. Long names are now truncated with an ellipsis and shown in full via a `title` tooltip.
- Demo mode: `GetResource` (the workload/pod detail page) built a synthetic manifest for literally any name, including one that was never created or was already deleted — so a URL to a nonexistent resource silently rendered a fake detail page instead of a not-found error. It now checks the resource actually exists first.
- Demo mode: rapid-clicking "Launch" before the button disabled sent multiple create requests, and `DemoProvider.Launch` had no existence check — every duplicate silently overwrote the prior workload and returned `201`, unlike a real cluster's `409 AlreadyExists`. Launch now rejects a name that's already taken. The same gap existed in `CreateConfigMap`, `CreateSecret`, `CreateHPA`, `CreateRole`, and `CreateRoleBinding` — all fixed the same way. Also fixed nine API handlers across these endpoints (`launch`, the Intent-tab launch path, `createNamespace`, `podWindow`, `createSecret`, `createConfigMap`, `createHPA`, `createRole`, `createRoleBinding`) that still reported a flat `400` for every provider error instead of using the status-code classifier added for the fix above.
- Demo mode: deleting an already-deleted (or never-existing) workload, RBAC role, or role binding silently reported success instead of a `404`, inconsistent with every other demo-mode delete method in the codebase (Secret, Job, CronJob, HPA, NetworkPolicy, Pod, CRD instance all already checked). Fixed to match that convention.
- Demo mode: the same duplicate-name gap also existed in Velero schedule create (silent overwrite) and Backup create (append-only storage, so a duplicate name produced two identical-looking rows instead). Both now reject a name already in use.
- Demo mode: a duplicate agent-run name was the worst case of this bug — `ApproveAgentRun`/`DenyAgentRun` look a run up by name and only ever act on the first match, so every run after the first with the same name got permanently stuck at "Pending" with no way to approve or deny it. `CreateAgentRun` now rejects a name already in use.
- Demo mode: closed out the same gap in VM snapshot/clone/export/restore create — all four accepted a caller-supplied name that collided with an existing one and silently overwrote it.
- Workloads and Pods bulk actions (Delete/Restart/Evict): both pages poll every 10s, so a checkbox selection could go stale — a selected row already gone by click time was silently dropped from the batch with no indication, so "Deleted 2 workloads" could mean 3 were selected. The toast now says so explicitly when it happens.
- User management: creating a duplicate username, or updating a role/password/namespace-restriction for a nonexistent user, reported a flat `400` instead of `409`/`404` — the same hardcoded-status-code gap as the earlier API fixes, just in `internal/api/users.go`.
- Audited every remaining always-`400` handler across `internal/api`: `approveAgentRun`, `denyAgentRun`, `triggerCronJob`, `restoreBackup`, `helmUninstall`, `helmRollback`, `helmInstall`, and `migrateVM` all now classify errors via `platform.HTTPStatus` instead of hardcoding `400`. Also found and fixed one more duplicate-name gap this turned up: `InstallHelmChart` silently overwrote a release name already in use instead of rejecting it like real Helm does.
- The Ops page's "Access" tab used the route segment `/ops/rbac`, not `/ops/access` — navigating to the URL matching its visible label silently rendered the Helm tab instead (Ops.jsx falls back to Helm for any unrecognized tab segment). Renamed the route key to `access` to match its label.

- Login rate limiting: the `Retry-After` header on a `429` was hardcoded to `60` regardless of the actual lockout, which ranges from 1 second (first offense past the grace count) up to a 5-minute cap under exponential backoff. A client honoring the header would wait far longer than necessary on a short lockout, or too briefly on a long one. Now reports the real remaining lockout time.

### Changed
- Design system: an Apple-adapted refinement of the existing "Liquid Glass" theme. Fixed the `body` font stack (it declared unlicensable "SF Pro Display"/"SF Pro Text" names that never resolved; now leads with `-apple-system`/`BlinkMacSystemFont`, the same system-font technique apple.com itself uses, so Mac/iOS/Safari render real San Francisco). Activated a full light/dark "iPhone 17" color palette that was defined in `index.css` but never wired to any component — it's now registered as Tailwind tokens and consolidated ~100 hardcoded hex literals across every page into it. Added a `brand` gradient variant to the shared `Button` component (defined once) and reserved it for the single most prominent action per page/dialog instead of every primary button carrying the same gradient. Softened card corner radii slightly (`--radius` 16px → 20px).
- UX pass inspired by apple.com and zyvor.dev:
  - **Confirmation dialogs before every destructive action.** Every delete/evict button in the app (18 call sites across 13 pages) previously fired immediately on click with no confirmation — a misclick permanently destroyed a real ConfigMap/Secret/Namespace/Workload. Added a shared `confirm()` dialog on the shadcn `AlertDialog` primitive (already installed, never used) and wired it into all of them, each naming the specific resource and consequence, with bulk actions naming the count.
  - A segmented (solid/striped/muted) progress bar — from zyvor.dev's migration-progress widget — in the workload Rollout dialog, replacing a bare stat grid with an at-a-glance available/updating/pending visualization.
  - The Run-workload wizard's Form/Intent/Validate-YAML tabs are now a numbered "01/02/03" stepper instead of a plain tab strip.
  - `PodDetail`'s "Logs" button (pure navigation, sitting next to the real "Evict" action) is now a plain text link, so the one destructive action per row reads unambiguously.
  - Workloads' empty-list message now distinguishes a genuinely empty cluster (icon + "Run your first workload" CTA) from filters just matching nothing (unchanged plain message).
  - Tactile motion polish: every button now gets a subtle press-down (`active:scale`) — previously only the Login page's SSO buttons had this. `GlassCard` now lifts slightly on hover (`.glass` already declared the transition, nothing ever triggered it). `Skeleton` loaders use a shimmer sweep instead of a flat opacity pulse.
  - Bigger, bolder page headers: a new shared `PageHeader` component (an eyebrow label above an oversized `text-4xl`/`5xl` title) generalizes the hero treatment that previously only `Overview` had, rolled out across all 26 other top-level pages in place of their old plain `text-3xl` heading. Every page's outer section spacing also grew from `space-y-4` to `space-y-6` for more breathing room, trading a little table density for a layout closer to apple.com's own product pages. Resource-detail pages (Pod/Node/Workload) got a matching `DetailHeader` component instead — same eyebrow/mono-name/meta-row identifier pattern they already used, scaled up (`text-3xl` → `text-4xl`) to match, with the same `space-y-6` spacing bump.

## [0.1.0] - 2026-08-29

First versioned release. Prior work was unversioned (`dev`-tagged images only).

### Added
- Tier 1 declarative CRD-view plugin architecture (`AxiomPlugin` custom resource, `deploy/k8s/plugins/`)
- Self-hosted enterprise packaging: a Helm chart (`deploy/helm/axiom/`)
- Self-observability: `GET /api/v1/metrics` (Prometheus text format) reporting Axiom's own request counts/latency and build info
- `SECURITY.md` vulnerability disclosure policy
- `docs/QUICKSTART.md` — a customer-facing install path that assumes only `helm`, `kubectl`, and a browser

### Notes
- Image tags before this release were the floating `dev` tag with no corresponding source tag — treat any `dev`-tagged image as pre-release and unsupported for upgrades.
