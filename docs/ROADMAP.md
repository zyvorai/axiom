# Roadmap

Where Axiom stands against other Kubernetes consoles, what shipped in 0.4.0 to 0.10.0, and
what is deliberately left for later. Competitor capabilities come from public
product documentation and vendor pages (checked November 2026); vendor claims
are theirs, not ours.

## Shipped, unreleased

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Node maintenance | Drain wizard on Node detail: server-side read-only drain preview (`GET /api/v1/nodes/{name}/drain-preview`: Pods to evict by namespace/owner, ignored DaemonSet/mirror/kube-system Pods, emptyDir, bare Pods, PDB-blocked, single-replica, volume-pinned, approximate reschedule headroom, blocked/caution/ok verdict), then typed node-name confirmation (server-enforced) and cordon + drain with progress, uncordon afterwards | Lens/OpenLens drain, Rancher node drain |
| Idle resources | Read-only idle/zombie finder on Ops → Cost (`GET /api/v1/cost/idle`): unmounted or unbound PVCs with a monthly storage estimate, empty namespaces, LoadBalancer Services with no endpoints, old finished Jobs, scaled-to-zero Deployments, and advisory unreferenced ConfigMaps/Secrets. Heuristics with a confidence level; nothing is deleted | Kubecost abandoned-resources, Robusta KRR/popeye-style sanitizers |
| Progressive delivery | Read-only Argo Rollouts (strategy, step/weight, replicas, phase, pause reason, latest AnalysisRun, stable vs canary revision and images) and Flagger Canaries (phase, canaryWeight, failedChecks, lastTransitionTime) on Ops → GitOps (`GET /api/v1/rollouts`). Promote/abort are not offered | Argo Rollouts dashboard, Flagger/Weave GitOps |
| Autoscaling | Read-only Karpenter view: NodePools and NodeClaims (instance type, spot/on-demand, zone, phase) on the Autoscaling page | Headlamp Karpenter plugin |

## Shipped in 0.13.2

| Area | What you get | Comparable to |
|------|--------------|---------------|
| License | Zyvor Production License v1.0: source-available, free for non-production use, commercial license for production | — |
| Website | Refreshed GitHub Pages site with the full 0.13 feature set, framed screenshots and a new social card | — |
| Docs | Plans & licensing, the real-world verification checklist and the design system are published alongside the guides | — |

## Shipped in 0.13.1

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Languages | Role names, AI provider names, terminal titles and the remaining labels are translated in all eight languages | — |

## Shipped in 0.13.0 (experimental)

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Billing | Ops → Cost compares the actual AWS / Azure / GCP invoice for this and last month with the allocated cost (OpenCost or the estimate); admin-configured with write-only credentials, fixed provider hosts only. Experimental: fixtures + STS session token / NextPageToken; live harness `e2e/scripts/validate-billing-live.sh` | Kubecost cloud-bill reconciliation, Vantage |

## Shipped in 0.12.1

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Pod proxy | A pod window stays on the cluster it was opened on, whichever replica serves it or the tab is switched to (links from 0.12.0 must be reopened) | — |

## Shipped in 0.12.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Pod proxy | Tickets and sessions are signed and valid on every replica; no session affinity needed | — |

## Shipped in 0.11.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Helm | Private OCI registries: write-only registry logins, per-repository credentials, plain-http registries when explicitly allowed | Harbor/GHCR private charts in Rancher, Portainer |
| Autoscaling | Edit VPAs and KEDA ScaledObjects in place after previewing the exact change | Lens, Rancher |
| Fleet | Health history survives restarts and is shared across replicas (ConfigMap-backed) | — |
| Pod proxy | Per-session hostnames (`https://{id}.pods.example.com`) so each pod window has its own origin | Rancher/OpenShift console proxies |
| Languages | Português, Italiano, 日本語 and 简体中文 in addition to English, Deutsch, Español and Français | — |

## Shipped in 0.10.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Languages | The console in English, German, Spanish and French: a language switcher on the login page, in the user menu and in Settings, locale-aware numbers, dates and currency, and lint that keeps every language complete | Rancher, Headlamp, OpenShift console localisation |
| LDAP two-factor | TOTP second factor for directory accounts (stored in a dedicated Secret), covered by `AXIOM_REQUIRE_MFA`, with admin listing and reset | Rancher/Portainer MFA for external auth |
| OCI Helm registries | Browse and install charts from OCI registries (ghcr.io, Docker Hub and others) with default values and README; digests verified, same SSRF guard as index repositories | Helm 3 OCI, Artifact Hub |
| GitOps edit and credentials | Edit an Argo CD or Flux application's managed fields after a structured preview, refusing stale edits; write-only repository credentials for Argo CD and Flux | Argo CD app edit, Flux secretRef management |
| VPA and KEDA creation | Typed, validated creation and deletion of VerticalPodAutoscalers and KEDA ScaledObjects with a YAML preview; trigger credentials only by TriggerAuthentication name | Lens/Rancher autoscaler forms |
| Actual cost | Real allocation cost from OpenCost instead of request-based estimates, with idle cost and a per-namespace breakdown; budgets follow the same source | OpenCost, Kubecost |
| Fleet history | Per-cluster uptime, a state timeline and incidents over 1 h, 6 h or 24 h | Rancher cluster history |
| Pod proxy isolation | Optional second listener serves pod web UIs from a separate origin through single-use links, so pod content can no longer script the console | Argo CD/Grafana proxy isolation guidance |

## Shipped in 0.9.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Notifications | A cluster going unreachable or degraded, and a namespace budget reaching warning or over, are announced through the Slack/webhook alert destinations already configured, debounced and audited, with the Timeline showing them | Rancher/Komodor cluster alerts, Kubecost budget alerts |
| Cross-cluster search | "Where is `payments` running?" across every registered cluster, skipping unreachable ones, with one-click switching to the cluster that has it | Rancher global search, Lens catalog search |
| Rightsizing apply | Review the exact request changes (CPU and/or memory), then apply them with a stale-view guard; limits are never lowered; the server recomputes the recommendation | Goldilocks/Kubecost "apply" |
| Require-MFA policy | `AXIOM_REQUIRE_MFA` (admin / operator / all) holds covered local accounts at an enrolment screen until they have a second factor; QR enrolment | Rancher/Portainer enforced 2FA |
| Shared revocation | Logouts and session revocations reach every replica within seconds, written conflict-safely to the shared ConfigMap | — |
| GitOps delete | Delete an Argo CD or Flux application with the consequence spelled out (finalizer, prune, Helm uninstall), a typed confirmation when it removes resources, and optional removal of an unshared source | Argo CD delete dialog |

## Shipped in 0.8.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Autoscaling beyond CPU | HPAs show every metric (CPU, memory, Pods, Object, External) with current and target plus what the autoscaler reports, and can be created on CPU and memory with a scale-down window; read-only VPA recommendations and KEDA ScaledObjects | Lens/Headlamp HPA views, Rancher workload autoscaling |
| Cost budgets | Per-namespace monthly budgets against the estimated cost with on-track / near / over state | OpenCost budgets, Kubecost alerts |
| In-place pod resize | Opt-in `inPlace` rightsizing apply via the pods `resize` subresource (no restart); resize status on pod detail | Kubernetes 1.35 in-place resize (stable) |
| Rightsizing | Request-vs-usage recommendations with headroom, from Prometheus history (`AXIOM_PROMETHEUS_URL`) or, without it, a clearly labelled low-confidence snapshot; never applied automatically | Kubecost/Goldilocks recommendations |

## Shipped in 0.7.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Helm chart catalog | Administrator-approved chart repositories, search, versions, default values and README, installing through the existing Helm Job; fetches are guarded against reaching private, loopback or metadata addresses | Rancher Apps & Marketplace, Portainer app templates, Lens Helm charts |
| GitOps application creation | Create an Argo CD Application, a Flux Git source + Kustomization or a Flux Helm source + HelmRelease from typed fields, after previewing the exact YAML; admin only, no credentials in URLs, rolled back if only part of it could be created | Argo CD "New app", Rancher Fleet, Devtron |

## Shipped in 0.6.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Fleet health | Every registered cluster probed in the background and reported healthy / degraded / unreachable with latency and the last error; state changes audited | Rancher cluster states, Portainer environment status |
| Fleet page | All clusters at a glance — nodes ready, pods running, warnings, CPU/memory, since-when — with a roll-up and one-click switching; the top-bar chip shows the active cluster's state | Rancher cluster list, Lens catalog |

## Shipped in 0.5.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Two-factor sign-in | TOTP for local accounts and the built-in admin, enrolled from the user menu, with recovery codes, replay protection and throttling; admins can see and reset it | Rancher/Portainer local-auth MFA, Argo CD with Dex |
| Session control | Logout that really ends the session, "sign out everywhere", automatic revocation on role/password/namespace changes and deletion, live role tracking for local sessions, configurable session lifetime | Headlamp/OpenShift console token revocation |
| SAML hardening | `InResponseTo` tracking (single-use, bound to the browser that started the sign-in), `SubjectConfirmationData` Recipient and expiry checks; IdP-initiated sign-in is opt-in | — |

## Shipped in 0.4.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| UX | Global namespace selector, URL-synced and saved filters, keyboard shortcuts (`?` for help), system theme, in-app notification inbox | Headlamp, k9s, Lens |
| Change safety | Review a YAML diff (dry-run server-side apply) before Apply, with a warning when another tool (Argo CD, Flux, Helm) owns fields | Argo CD diff, Lens |
| Understanding | Resource relationship graph per workload (owner chain, Services, Ingress, HPA, volumes) | Octant, Headlamp map, k9s xray |
| North-south | Envoy Gateway via Gateway API (`expose.type=Gateway`, Ingresses UI provider=envoy) | Ingress NGINX → Envoy migration |
| Troubleshooting | Timeline of events, audit entries, alerts and rollouts; "What changed" per workload; cluster health check with suggested fixes; multi-pod log view with regex, download and previous-container; ephemeral debug containers; opt-in admin-only node shell | Komodor timeline, k8sgpt analyzers |
| Security | Read-only RBAC viewer ("who can…", per subject), ServiceAccounts, NetworkPolicy analysis, Trivy-operator and PolicyReport (Kyverno) views, per-image vulnerability rollup and SBOM component/license summary | Rancher/NeuVector, Lens Security Center |
| Troubleshooting | Timeline of events, audit entries, alerts and rollouts; "What changed" per workload; ranked "Recent changes" correlation (GitOps, Helm, rollouts, config, nodes, HPA, audit) around a workload and time; cluster health check with suggested fixes; multi-pod log view with regex, download and previous-container; ephemeral debug containers; opt-in admin-only node shell | Komodor timeline, k8sgpt analyzers |
| Security | Read-only RBAC viewer ("who can…", per subject), ServiceAccounts, NetworkPolicy analysis, Trivy-operator and PolicyReport (Kyverno) views | Rancher/NeuVector, Lens Security Center |
| Auth hardening | Per-account login throttling, single-use WebSocket/SSE tickets instead of credentials in URLs, OIDC ID-token signature and nonce verification, audit JSON export | — |
| AI (opt-in) | Bring-your-own-LLM "Explain with AI" for Pods, Warning events and health findings; redacted payload preview; never sends Secret or ConfigMap values | Lens Prism, Komodor Klaudia, Rancher Liz, OpenShift Lightspeed |

## Shipped in 0.14.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Accelerators | NVIDIA GPU inventory, Operator/DCGM health, quotas, scheduling issues, waste hints + GPU rightsizing apply, Ops Accelerators + GPU cost | Portainer GPU, OpenShift DCGM overview |
| Still theatre | Blast preview with live terminals/windows; hush/seal/sleep/end; PacketWolf seal receipt | — (Axiom-native) |
| Weird findings | Finalizer ghosts, image-tag float, HPA thrash, Ingress empty backends, cert expiry / secret age / clock skew | Radar/k8sgpt-class, rare in consoles |
| Sealed search | Fleet `sealed:` cross-cluster isolated twins | — |
| Intent drift | Place-intent vs Platform Intent CR vs ready/desired | — |
| Upgrade prep | Upgrade-risk findings → allowlisted ProposeOnly actions | Lightspeed update-risk (lite+) |

## Shipped in 0.14.0

| Area | What you get | Comparable to |
|------|--------------|---------------|
| Agentic ops | Built-in investigate → ProposeOnly plan → approve/deny → allowlisted apply; ConfigMap store when AgentRun CRD is absent; CR path when present | OpenShift Lightspeed agentic runs (TP) |
| Agent UI | Create run in Ops → Agents; Run agent from Pod, health findings and Warning events; plan/actions/executed detail | OpenShift Agentic Runs UI |
| Auto runs | Optional Warning/CrashLoop → ProposeOnly run (Settings → AI, off by default) | Lightspeed alert-triggered investigation |
| MCP | Authenticated JSON-RPC MCP at `/api/v1/mcp` (list/diagnose/health/create_agent_run + nodes/workloads/alerts/upgrade_risk) | OpenShift MCP gateway (read-only + ProposeOnly) |
| Metrics | Prometheus history charts on Pod, Node, Workload detail (`AXIOM_PROMETHEUS_URL`) | OpenShift/Lens charts |
| Alertmanager | Firing alerts + silence CRUD (`AXIOM_ALERTMANAGER_URL`); Run agent from Alerts | OpenShift alerting / silences |
| Upgrade risk | Lite readiness report on Health check + ProposeOnly agent run | Lightspeed update-risk (lite) |
| VM console | Embedded noVNC against the existing VNC websocket proxy | OpenShift/KubeVirt console |
| Notifications | Clear Slack / alert webhook URLs from Settings | — |

## Shipped in 0.14.x (opportunities pack)

| Area | What you get | Comparable to |
|------|--------------|---------------|
| HA notify | Lease-backed notify leadership + one retry | Rancher/Portainer multi-replica ops |
| Audit merge | ConfigMap union across replicas | — |
| Secret scale | Metadata-only list + pagination | Serious console scale hygiene |
| SAML HA | Shared AuthnRequest store | Affinity-free ACS |
| Twin diff | Image/replicas/seal/sign columns | Portainer fleet views |
| Outage narrative | Incident × who-poked-it × agent runs | Komodor-class storytelling |
| MCP HITL | propose_seal / approve / deny | Lightspeed HITL writes |
| DRA browser | DeviceClass/Slice/Claim inventory | Lens Device Resources |
| Gateway API deep view | Gateways (class, listeners, addresses, Accepted/Programmed), HTTPRoute/GRPCRoute (hostnames, parentRefs, backendRefs, Accepted/ResolvedRefs) and ReferenceGrants on Networking | Lens/Headlamp Gateway API views |
| cert-manager view | Certificates expiry heat, Issuers, failing CertificateRequests/Orders (read-only, `/certificates`) | Lens cert-manager panels |

## Shipped in 0.14.x (ROADMAP Next closed)

| Area | What you get | Comparable to |
|------|--------------|---------------|
| MCP HITL sandbox | `mint_sandbox` / `list_sandboxes` — short-lived allowlisted grants (does **not** mint ClusterRoles) | Lightspeed HITL without CR authoring |
| Upgrade planner | Deepened pre-check (version skew, cert/clock, GPU) + enriched ProposeOnly actions | Lightspeed update-risk |
| ApplicationSets | Listed read-only with Applications / Flux | OpenShift GitOps status |
| OCI cloud identity | `POST /helm/registry-credentials/cloud-token` (ECR/GAR/ACR via local CLIs) | Cloud registry login helpers |
| VAP browser | `GET /admission-policies` | Lens Admission Policies |
| Stream tickets HA | Shared ConfigMap for WebSocket/SSE tickets | Affinity-free streams |
| Seal / blast / users / clusters / Atlas | Kind-scoped Seal NPs, exact blast match, fail-closed roles, SAML auth label, DELETE 404, Atlas token clear | — |
| Terminal idle | Pod exec + node shell idle out at 10 m (ping/pong), matching pod-proxy | — |
| Default-creds harden | Non-demo refuses `demo`/`demo` unless `AXIOM_ALLOW_DEFAULT_CREDS` | — |
| Login throttle HA | Shared ConfigMap `zyvor-axiom-login-throttle` | — |
| CSI clone ProposeOnly | `sourcePVC` on volumes + agent action `clone_pvc` | — |
| Policy coverage | `GET /security/policy-coverage` (PolicyReport rollup) | Kyverno / Gatekeeper reports |
| Velero restore wizard | Backups page: pick a Backup, namespaces/resources, mapping, restorePVs, existingResourcePolicy, server-side dry-run preview, typed confirm; `/velero/restores` list/preview/create (admin) | Creating a real Restore not yet exercised against a live cluster |
| Velero CRD fidelity | Create Backup/Schedule validated against upstream v1.15.2 OpenAPI; live script when CRDs exist | — |
| Helm install RBAC | ClusterRole covers ServiceAccounts, PDBs, Leases, EndpointSlices, ControllerRevisions | Ordinary chart installs under Axiom's SA |

## Shipped deepen pack (post-0.14.0)

| Area | What you get |
|------|--------------|
| DRA depth | Claim phase, devices, bound pod on `/dra` |
| DCGM tiles | FB / temp / power / XID on Accelerators |
| GitOps list + suspend | Health/sync/revision columns; AppSet sync blocked; Flux/Argo suspend resume |
| Cost estimate GPU | Flat GPU rate on estimate path + OpenCost status badge |
| Invoice services | Experimental AWS SERVICE top-N on `/cost/invoice` |

## Next

_None._ Remaining items are deliberate non-goals or packaging (see Not planned / docs/GAPS.md).

## Not planned

- **Browser TCP port-forward.** A browser cannot open a local listener, so Axiom keeps the HTTP proxy and offers a copyable `kubectl port-forward` command instead.
- **ClusterRole write UI.** Deliberate: RBAC changes stay in your GitOps flow. MCP sandbox grants allowlisted actions only.
- **Run:ai / KAI fairshare editors and model-catalog MLOps.** Out of scope.
- **Metal3-first / multi-runtime portability.** Axiom is k8s-native private cloud, not Aether.
- **SOC2 / pen-test packaging** as a product SKU (attest the shipped surface yourself).
- **Native-speaker translation review** is continuous polish (eight locales already complete).
