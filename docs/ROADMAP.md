# Roadmap

Where Axiom stands against other Kubernetes consoles, what shipped in 0.4.0, and
what is deliberately left for later. Competitor capabilities come from public
product documentation and vendor pages (checked November 2026); vendor claims
are theirs, not ours.

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

- **Fleet:** a cross-cluster overview and real cluster health probing (registered clusters currently always report Healthy).
- **Autoscaling & cost:** VPA/KEDA views, HPA beyond CPU, cost budgets and rightsizing, historical Prometheus data.
- **Identity:** TOTP/MFA for local accounts, SAML `InResponseTo` tracking, session revocation.
- **GitOps & Helm:** create Argo/Flux applications, a Helm chart catalog.
- **Reach:** internationalisation, a separate origin for the pod proxy.

## Not planned

- **Browser TCP port-forward.** A browser cannot open a local listener, so Axiom keeps the HTTP proxy and offers a copyable `kubectl port-forward` command instead.
- **ClusterRole write UI.** Deliberate: RBAC changes stay in your GitOps flow.
