# Axiom overall feature opportunities — shipped vs residual gaps

Research date: 2026-11-12. Scope: inventory of console capabilities from local repo docs/routes vs residual opportunity areas **excluding** already-closed 0.14.x pack items (NVIDIA accelerators, Still theatre, weird findings, sealed search, intent drift, upgrade prep, GPU rightsizing, cert/secret/clock hygiene) except where “deepen” is noted. Product identity: `zyvor-axiom` / `axiom-system` / `AXIOM_*`; k8s-native private cloud (not Aether/Metal3-first).

## What major surfaces already exist (workloads, fleet, cost, agents, MCP, GitOps, Helm, VMs, security, health, Still, accelerators)?

### Takeaway
Axiom already ships a broad private-cloud console: core k8s inventory/ops, multi-cluster fleet, Helm+GitOps, KubeVirt VMs, cost/autoscaling, security/health/timeline, auth/MFA, plus a 0.14 pack of agents/MCP/Alertmanager/metrics and the Unreleased 0.14.x theatre (Still, accelerators, weird findings, sealed search, intent drift, upgrade prep).

### Cited Findings
- Frontend routes cover overview, nodes/workloads/pods, platform-intents, networking/services/ingresses, configmaps/secrets/quotas, autoscaling, CRDs/plugins, namespaces, VMs, storage/Atlas, logs/events/timeline, fleet, health-check, jobs, backups, alerts, Ops (`/ops/:tab`), security, settings, users — [App.js](file:///Volumes/Storage/tt/axiom/frontend/src/App.js).
- Ops tabs explicitly include Helm, GitOps, Agents, Identity (Haven), Audit, Access (RBAC), Webhooks, Accelerators, Cost — [Ops.jsx](file:///Volumes/Storage/tt/axiom/frontend/src/pages/Ops.jsx) (`TABS` keys: `helm`, `gitops`, `agents`, `identity`, `audit`, `access`, `webhooks`, `accelerators`, `cost`).
- Core API mux registers workloads/pods/nodes/VMs/volumes, Still preview/execute + seal/unseal, network guard, PacketWolf, fleet-adjacent clusters, Helm install/upgrade/releases, GitOps sync, Velero backups/schedules, agent-runs, platform-intents (+ drift), accelerators (+ GPU rightsizing apply), Atlas, Haven, AI explain, users/integrations — [server.go](file:///Volumes/Storage/tt/axiom/internal/api/server.go).
- Additional mux files: fleet (`GET/POST /fleet*`), MCP (`/api/v1/mcp`), Alertmanager silence CRUD, upgrade-risk, cost budgets/rightsizing/invoice — [fleet.go](file:///Volumes/Storage/tt/axiom/internal/api/fleet.go); [mcp.go](file:///Volumes/Storage/tt/axiom/internal/api/mcp.go); [alertmanager.go](file:///Volumes/Storage/tt/axiom/internal/api/alertmanager.go); [upgrade_risk.go](file:///Volumes/Storage/tt/axiom/internal/api/upgrade_risk.go); [cost_budgets.go](file:///Volumes/Storage/tt/axiom/internal/api/cost_budgets.go).
- ROADMAP tables credit shipped surfaces across 0.4–0.14: UX/diff/graph/timeline/health/security/AI (0.4), MFA/sessions/SAML (0.5), fleet health page (0.6), Helm catalog + GitOps create (0.7), HPA/VPA/KEDA + budgets/rightsizing (0.8), notifications/fleet search/rightsizing apply/GitOps delete (0.9), OCI Helm/GitOps edit/OpenCost/pod proxy (0.10–0.12), experimental cloud invoice (0.13), agentic ops + MCP + metrics history + Alertmanager + upgrade-risk lite + VM noVNC (0.14.0), and Unreleased 0.14.x accelerators/Still/weird findings/sealed search/intent drift/upgrade prep/GPU rightsizing/hygiene — [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md).
- CHANGELOG Unreleased lists concrete 0.14.x APIs: `GET /api/v1/accelerators`, Still `POST …/still`, health analyzers (finalizers/tags/HPA thrash/empty Ingress backends/cert/secret/clock), `sealed:` fleet search, `GET /api/v1/platform-intents/drift`, GPU rightsizing apply, upgrade-risk enrichment, MCP tools `list_accelerators`/`still_preview`/`fleet_search`/`network_guard` — [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md).
- API.md documents workloads Still ladder, seal/unseal, network guard; fleet health/history/search; Helm catalog/OCI/private registry creds; GitOps create/edit/delete/credentials; OpenCost + budgets + rightsizing apply + experimental invoice; VMs including VNC path — [API.md](file:///Volumes/Storage/tt/axiom/docs/API.md).
- Security page + API: read-only RBAC viewer, NetworkPolicy analysis, Trivy/PolicyReport views (closed in GAPS; shipped 0.4) — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md); [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md).
- Health: `GET /api/v1/overview`, HealthCheck page, upgrade-risk lite, Timeline/Events/Alerts — [App.js](file:///Volumes/Storage/tt/axiom/frontend/src/App.js); [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md).
- Adjacent private-cloud surfaces beyond the named list: place-intent NL launch (`POST /intent/parse|place`), Platform Intents page, Atlas storage control plane, Haven identity planes, Envoy Gateway (`GET /gateways`, Helm `expose.type=Gateway`), ephemeral debug / node shell (0.4), pod proxy isolation — [server.go](file:///Volumes/Storage/tt/axiom/internal/api/server.go); [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md); [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md).

### Inferences
- The product is past “inventory console”: GitOps/Helm/cost/fleet/agents/MCP/Still form a differentiated private-cloud ops layer; remaining value is less greenfield pages and more reliability, depth of typed models, and enterprise validation.
- Ops is the gravity well for “advanced” features (Helm/GitOps/Agents/Accelerators/Cost); day-2 k8s CRUD lives as first-class routes.

### Gaps
- Did not exhaustively inventory every Security.jsx / HealthCheck.jsx sub-panel vs each analyzer implementation (docs + CHANGELOG are the authority used here).
- Did not map sidebar nav labels vs every route (App.js routes used as surface of truth).

## What does GAPS.md / ROADMAP Next still call out?

### Takeaway
ROADMAP Next is thin and agent/MCP-centric; GAPS.md is the living residual catalog—mostly HA/reliability, typed-model ceilings (GitOps/VPA/KEDA/OCI), experimental cost verification, and deliberate non-goals—rather than missing primary console pages.

### Cited Findings
- **ROADMAP Next** (only three bullets): MCP write tools / ephemeral RBAC sandbox for agent execution; deeper agentic cluster-upgrade planner beyond lite assessor; translations reviewed by native speakers (some technical screens still English per GAPS) — [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md).
- **ROADMAP Not planned**: browser TCP port-forward; ClusterRole write UI (GitOps stays source of truth) — [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md).
- **GAPS “Still thinner than a full private-cloud suite”**: ClusterRole write UI (deliberate); SOC2 / pen-test packaging; MCP ephemeral execution sandbox / deeper Lightspeed-style upgrade planner; Run:ai / KAI fairshare editors and model-catalog MLOps (out of scope); Velero on-demand backup only fake-clientset tested; Helm installs limited to Axiom ClusterRole permissions — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **GAPS known security limitations**: SSO MFA not in-console; SAML AuthnRequests + stream tickets in-memory per replica (affinity); in-origin pod window still allows `?access_token=` when separate proxy off; session revocation eventually consistent; pod proxy residual session/cap/affinity issues; LDAP `ldap://` plaintext; default `demo`/`demo` credentials not refused — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **GAPS known platform limitations**: OCI no cloud-identity registry auth (ECR/GCR/ACR beyond pasted short-lived token); GitOps typed-only (no ApplicationSets, multi-source, valuesFrom, Kustomize patches); VPA/KEDA typed ceilings + no ScaledJobs / TriggerAuthentication create; OpenCost single-cluster + estimate fallback; cloud bill experimental/unverified; rightsizing never auto-applied + Prometheus only home cluster; fleet probe/history multi-replica quirks; notifications best-effort; audit last-writer-wins multi-replica; alert dispatch no leader election; blast pod name substring + Atlas volumes note; ListSecrets reads data / no list pagination; terminal idle (partially addressed in Unreleased CHANGELOG); seal NetworkPolicy name collisions — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **GAPS functional-test leftovers**: `POST /users` unknown role → admin; Atlas token clear-from-UI (Slack clear shipped); `/session` role label / SAML `auth:"oidc"` quirks; `DELETE /clusters/{unknown}` → 400 not 404 — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **GAPS UI**: eight languages complete for page literals; by-design untranslated k8s terms/API text; no RTL; money always USD — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md). Contradicts ROADMAP Next “a few technical screens still show English” unless that means API-returned English — compare [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md) vs [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Closed list in GAPS already includes large prior gaps (SAML hardening, LDAP, fleet, OCI private creds, GitOps CRUD, OpenCost, MFA, etc.) — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).

### Inferences
- Explicit product “Next” is deepen agents/MCP/upgrade—not new inventory domains.
- Most GAPS residual items are hardening or typed-model expansion of surfaces that already exist in UI/API.

### Gaps
- ROADMAP Next does not mention HA notification/audit fixes even though GAPS ranks them as known limitations—prioritization between docs is unclear.
- Native-speaker translation review status is aspirational; no evidence of completion tracking beyond `lint:i18n`.

## What thin or half-built areas appear in code/docs (pagination, audit multi-replica, notifications reliability, Velero real-cluster validation, OCI cloud identity, GitOps ApplicationSets, etc.)?

### Takeaway
Thin/half-built areas concentrate on multi-replica correctness, notification delivery, list scalability, GitOps/autoscaler typed ceilings, OCI cloud identity, experimental billing, and Velero real-cluster proof—not missing Ops tabs.

### Cited Findings
- **Pagination / large-cluster scale**: “no list call is paginated (`Limit`/`Continue`)”; `ListSecrets` still reads every Secret with data — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **Audit multi-replica**: each replica keeps its own ring; `Save` is unconditional update → last writer wins — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **Notifications reliability**: best effort, one attempt, no retry/queue; debounce/state in memory (restart forgets); each replica notifies independently; budget watcher only default active cluster; duplicate fleet notifications per replica — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **Alert dispatch**: no leader election; multi-replica duplicate Slack posts; flapping pod re-fires every minute — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **Velero**: `POST /api/v1/velero/backups` unit-tested against fake clientset only; no real CRDs in validation environment — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md). API exists for list/create backups + schedules — [server.go](file:///Volumes/Storage/tt/axiom/internal/api/server.go); [API.md](file:///Volumes/Storage/tt/axiom/docs/API.md).
- **OCI cloud identity**: username/password or pasted token only; no AWS ECR / GCR workload identity / Azure AD ACR flows; unrelated token realms refused; catalog caps (50 charts, 500 tags, newest Chart.yaml only) — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **GitOps ApplicationSets / advanced Argo/Flux**: no multi-source Applications, ApplicationSets, Helm `valuesFrom`/`valuesObject`, Kustomize patches/postBuild/`chartRef`; advanced objects shown read-only — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md); [API.md](file:///Volumes/Storage/tt/axiom/docs/API.md).
- **VPA/KEDA half-model**: advanced fields read-only; ScaledJobs not shown; TriggerAuthentications name-only (create via apply); HPA create CPU/memory Utilization only — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **Cost thin edges**: without OpenCost = request estimates; OpenCost home-cluster only; invoice experimental (fixtures only; no CUR/EA/managed identity/session tokens; monthly totals only) — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md); [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md) 0.13.0.
- **Rightsizing thin edges**: Jobs/CronJobs/bare pods not assessed; VPA targets unused; registered fleets use snapshot; never auto-apply — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **Fleet thin edges**: point-in-time overview probe; history ≤24h; multi-replica history merge-at-startup only; search live substring, rate-limited, limited kinds (Pods added in Unreleased competitive pack) — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md); [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md).
- **Blast / Still residual**: GAPS still claims Atlas volume list can never populate (`WorkloadVolumes` not in Provider) and pod match by name substring — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md). **Conflict**: code now has `WorkloadVolumes` on Demo and Kubernetes providers and blast uses `VolumeLister` — [window_kubernetes.go](file:///Volumes/Storage/tt/axiom/internal/platform/window_kubernetes.go); [demo.go](file:///Volumes/Storage/tt/axiom/internal/platform/demo.go); [blast.go](file:///Volumes/Storage/tt/axiom/internal/platform/blast.go). Reports still cite the gap as A5 — [Axiom weird k8s feats.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20weird%20k8s%20feats.md).
- **Seal naming**: NetworkPolicy names omit workload kind → Deployment/StatefulSet same-name collide — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **Auth HA thin**: SAML outstanding requests + ws-tickets per-replica memory — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **MCP / agents depth**: MCP write tools + ephemeral RBAC sandbox and deeper upgrade planner still Next; 0.14 ships ProposeOnly allowlist + read/create_agent_run MCP — [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md); [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md).
- **VM console docs lag**: API.md still says “no embedded noVNC UI yet”; CHANGELOG 0.14.0 and frontend locales ship embedded console — [API.md](file:///Volumes/Storage/tt/axiom/docs/API.md); [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md); [en/pages.json](file:///Volumes/Storage/tt/axiom/frontend/src/locales/en/pages.json).
- **GPU report age**: `reports/Axiom GPU console additions.md` still describes accelerators as unregistered / no UI; ROADMAP Unreleased + server.go + Ops Accelerators tab show that pack as shipped in tree — contradicted by [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md); [server.go](file:///Volumes/Storage/tt/axiom/internal/api/server.go); [Ops.jsx](file:///Volumes/Storage/tt/axiom/frontend/src/pages/Ops.jsx). Prefer CHANGELOG/ROADMAP/code over that report for current state.
- **Helm permission ceiling**: chart install fails if ClusterRole lacks kinds (e.g. ServiceAccounts) — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **Terminal idle**: GAPS said no idle timeout; Unreleased CHANGELOG adds 10m terminal idle disconnect — prefer [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md) over stale GAPS line.

### Inferences
- Highest residual risk for production HA installs is notification/alert leader election + audit merge + SAML/ws-ticket affinity—not missing feature pages.
- GitOps/Helm/OCI “half-built” means typed happy-path is solid; enterprise edge cases need either YAML apply path (already exists) or typed expansion.
- Several docs/reports lag the tree; opportunity planning should treat GAPS platform bullets as primary residual list and verify Atlas/WorkloadVolumes and terminal idle against code before reopening them as work.

### Gaps
- No live multi-replica soak evidence in this research pass (docs-only for HA behavior).
- Exact MCP tool allowlist beyond CHANGELOG/API snippets not fully enumerated from `mcp.go` tool table.
- Whether Atlas volumes now populate in Still preview end-to-end was not UI-verified (code suggests Provider gap closed; GAPS not updated).

## Which gaps are polish/hardening vs net-new feats?

### Takeaway
Most residual GAPS items are polish/hardening or deepen-existing; true net-new (beyond 0.14 pack and ROADMAP Next) is sparse—ApplicationSets-class GitOps, cloud-identity OCI, real bill verification, SOC2 packaging, and optional scheduler deep-links—while deliberate non-goals (port-forward, ClusterRole write, Run:ai) should stay out.

### Cited Findings

**Polish / hardening (existing surface, correctness/reliability/scale)**
- Multi-replica: audit last-writer-wins; alert/notification leader election; fleet history replica merge quirks; SAML AuthnRequest + stream-ticket shared store; notification retry/queue + persisted debounce — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Scale: list `Limit`/`Continue` pagination; ListSecrets metadata-only list path — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Blast precision: exact pod matching; seal policy names include kind — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Pod proxy residual: revoke pod sessions on console logout; per-user caps globalized; ConfigMap redeem failure modes — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Session/API quirks: `/session` SAML `auth` label; unknown cluster DELETE status; unknown role fail-closed — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Velero real-cluster e2e validation of already-shipped endpoints — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Cloud invoice: real-account verification of experimental clients — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md); [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md).
- Native-speaker i18n review / RTL (if desired) — [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md); [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Docs hygiene: refresh GAPS WorkloadVolumes + terminal idle; API.md noVNC wording; GPU report stale — see conflicts above.

**Deepen existing (not greenfield pages; expand typed/agent depth)**
- MCP write tools + ephemeral RBAC sandbox — [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md).
- Deeper agentic upgrade planner beyond lite + EnrichUpgradeRiskPlan — [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md); [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md).
- GitOps typed model: ApplicationSets, multi-source, valuesFrom, Kustomize patches; credential update (not delete+recreate); GitHub App / cloud creds — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- VPA/KEDA/HPA typed expansion; ScaledJob; TriggerAuthentication create UI — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- OCI cloud-identity registry auth; broader catalog enumeration — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Cost: per-namespace invoice attribution; IAM roles / managed identity; multi-cluster OpenCost; rightsizing for Jobs + VPA-aware — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Fleet search index / more kinds; longer durable history — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- **Deepen only (exclude from “add next” as new themes)**: NVIDIA DRA browser / more DCGM series; Still Atlas volume UX polish; weird-finding coverage expansion; sealed twin UX; intent-drift actions; upgrade-prep action catalog; GPU rightsizing confidence — already credited in 0.14.x Unreleased — [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md); [CHANGELOG.md](file:///Volumes/Storage/tt/axiom/CHANGELOG.md); prior research [Axiom GPU console additions.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20GPU%20console%20additions.md); [Axiom weird k8s feats.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20weird%20k8s%20feats.md).

**Net-new / packaging (or suite-level) residual**
- SOC2 / pen-test packaging — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Helm ClusterRole expansion policy / install-as-user modes (product decision) — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).
- Optional read-only KAI/Volcano queue summaries if CRDs present (GPU report P2; still out of fairshare editors) — [Axiom GPU console additions.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20GPU%20console%20additions.md); [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md).

**Deliberate non-goals (do not treat as opportunities)**
- Browser TCP port-forward; ClusterRole write UI — [ROADMAP.md](file:///Volumes/Storage/tt/axiom/docs/ROADMAP.md).
- Run:ai / KAI fairshare editors, model catalogs, MLOps — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md); [Axiom GPU console additions.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20GPU%20console%20additions.md).
- Full chaos studio — [Axiom weird k8s feats.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20weird%20k8s%20feats.md).
- Auto-apply rightsizing / mute production without ProposeOnly — [GAPS.md](file:///Volumes/Storage/tt/axiom/docs/GAPS.md); [Axiom weird k8s feats.md](file:///Volumes/Storage/tt/axiom/reports/Axiom%20weird%20k8s%20feats.md).

### Inferences
- Best “overall feature opportunities” after 0.14 pack: (1) HA notification/audit/SAML ticket correctness, (2) agent/MCP execution sandbox + upgrade planner (ROADMAP Next), (3) GitOps/OCI typed enterprise depth, (4) cost invoice real-world hardening, (5) list pagination + Velero e2e—rather than inventing new top-level domains.
- Excluding the 0.14 pack from “add next” correctly removes the highest-uniqueness weird/Still/GPU themes already credited Unreleased; residual uniqueness then shifts to MCP sandbox + private-cloud reliability.

### Gaps
- No competitor gap analysis refreshed in this pass beyond ROADMAP’s existing comparable-to column.
- Commercial packaging (SOC2) content/requirements not researched beyond the GAPS bullet.
- Prioritization among polish items (which HA gap hurts customers first) is not evidenced in repo beyond documentation order.
