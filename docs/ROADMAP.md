# Roadmap

Where Axiom stands against other Kubernetes consoles, what shipped in 0.4.0 to 0.7.0, and
what is deliberately left for later. Competitor capabilities come from public
product documentation and vendor pages (checked November 2026); vendor claims
are theirs, not ours.

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

- **Fleet:** health history and alerting on a cluster going down (today state changes are audited and shown, not notified), and cross-cluster workload search.
- **Autoscaling & cost:** VPA/KEDA views, HPA beyond CPU, cost budgets and rightsizing, historical Prometheus data.
- **Identity:** MFA for LDAP accounts, QR-code enrolment and a require-MFA policy, shared revocation across replicas.
- **GitOps & Helm:** delete and edit applications, Argo repository credentials, an OCI chart catalog.
- **Reach:** internationalisation, a separate origin for the pod proxy.

## Not planned

- **Browser TCP port-forward.** A browser cannot open a local listener, so Axiom keeps the HTTP proxy and offers a copyable `kubectl port-forward` command instead.
- **ClusterRole write UI.** Deliberate: RBAC changes stay in your GitOps flow.
