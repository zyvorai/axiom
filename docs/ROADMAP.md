# Roadmap

Where Axiom stands against other Kubernetes consoles, what shipped in 0.4.0 to 0.10.0, and
what is deliberately left for later. Competitor capabilities come from public
product documentation and vendor pages (checked November 2026); vendor claims
are theirs, not ours.

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
| Troubleshooting | Timeline of events, audit entries, alerts and rollouts; "What changed" per workload; cluster health check with suggested fixes; multi-pod log view with regex, download and previous-container; ephemeral debug containers; opt-in admin-only node shell | Komodor timeline, k8sgpt analyzers |
| Security | Read-only RBAC viewer ("who can…", per subject), ServiceAccounts, NetworkPolicy analysis, Trivy-operator and PolicyReport (Kyverno) views | Rancher/NeuVector, Lens Security Center |
| Auth hardening | Per-account login throttling, single-use WebSocket/SSE tickets instead of credentials in URLs, OIDC ID-token signature and nonce verification, audit JSON export | — |
| AI (opt-in) | Bring-your-own-LLM "Explain with AI" for Pods, Warning events and health findings; redacted payload preview; never sends Secret or ConfigMap values | Lens Prism, Komodor Klaudia, Rancher Liz, OpenShift Lightspeed |

## Next

Deliberately left for later, with the reason:

- **Billing:** per-cloud invoice integrations (OpenCost covers allocation, not your actual bill).
- **Registry credentials:** private OCI chart registries need a credential store and token exchange with secrets.
- **Editing VPA and KEDA objects in place:** today delete and recreate.
- **Languages:** more locales, and a few technical screens still show English (see docs/GAPS.md for the exact coverage); translations have not been reviewed by native speakers.
- **Fleet history across restarts:** history is kept in memory per replica; a durable store would need a time-series backend.
- **Pod proxy:** per-session origins are available (host mode, `https://{id}.pods.example.com`); remaining: proxy tickets and sessions shared across replicas (today session affinity is needed).

## Not planned

- **Browser TCP port-forward.** A browser cannot open a local listener, so Axiom keeps the HTTP proxy and offers a copyable `kubectl port-forward` command instead.
- **ClusterRole write UI.** Deliberate: RBAC changes stay in your GitOps flow.
