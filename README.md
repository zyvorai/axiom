# Zyvor Axiom

Zyvor Axiom is a Go-based private-cloud control plane for **k3s and Kubernetes**. It presents applications first, while still giving operators direct access to Pods, logs, an interactive terminal, events, nodes, scaling, restart, stop/start, eviction and drain workflows.

This project intentionally contains **no Metal3 workload support**.

## UX

The web UI is a React 19 + Tailwind + shadcn/ui single-page app, styled in the Netra design language (apple.com-derived: `#000`/`#f5f5f7` canvas, bordered rounded cards, pill buttons and badges, light and dark themes, with a Meta-style presentation on the Overview — see [docs/DESIGN-SYSTEM.md](docs/DESIGN-SYSTEM.md)), built with CRACO and embedded inside the Go binary. Source lives in `frontend/`; `scripts/sync-ui.sh` builds it and copies the output into `internal/ui/dist/` (a generated, gitignored build artifact — never edit `internal/ui/dist/` directly).

Pages:

- **Getting Started** — live setup checklist (SSO, Atlas, PacketWolf, Postgres, probe) driven by real `/api/v1/integrations` + `/api/v1/auth/providers` status
- **Overview** — cluster hero, metric rings, utilization sparklines, alerts, activity
- **Nodes** — capacity, kubelet status, cordon/uncordon/drain; each node links to a detail page (live CPU/memory history, taint add/remove)
- **Workloads** — Deployments/StatefulSets/DaemonSets/CronJobs, with a full action menu (start/stop/restart/scale/seal/unseal/delete), a rollout dialog (history + pause/resume/restart/undo), bulk multi-select restart/delete, and a "Run workload" wizard (Form / natural-language Intent / YAML validate-and-apply)
- **Workload detail** (`/workloads/:ns/:kind/:name`) — labels/annotations, an editable raw-manifest YAML tab (Save applies via server-side apply, kubernetes build only), owned Pods, related events
- **Pods** — list with per-row exec and evict, plus bulk multi-select evict
- **Pod detail** (`/pods/:ns/:name`) — full container specs (resources, env, mounts, probes), volumes, tolerations, owner, conditions, an editable raw-manifest YAML tab (Save applies via server-side apply), live per-pod/per-container CPU & memory graphs (Metrics tab, rolling window from `metrics.k8s.io`), an automatic scheduling-diagnosis banner ("why is this pod not running") whenever the pod isn't cleanly Running/Succeeded, a **multi-tab interactive PTY terminal** (open several concurrent shell sessions, each its own real WebSocket connection — falls back to one-shot exec in demo mode), and per-port "Open" buttons that proxy straight into the pod's container port from the browser
- **Platform Intents** — workloads placed via the Intent launch flow
- **Networking · Cilium** — service graph, a live PacketWolf (eBPF/Cilium flow) panel when `AXIOM_PACKETWOLF_URL` is configured, and full NetworkPolicy list/delete + "Create via YAML"
- **Command palette (⌘K)** — jumps to any top-level route and live-searches real pods/workloads/namespaces by name, not just a static page list
- **Timeline** — events, audit entries, alerts and rollouts in one filterable feed; the same data as a "What changed" tab on every workload
- **Health check** — rule-based findings (crash loops, OOM kills, Services without endpoints, Pending volumes, broken Ingress backends, HPAs that cannot scale) with a suggested fix each
- **Security** — Trivy-operator vulnerability and config-audit reports and PolicyReport (Kyverno) results per workload, read-only
- **Access review** (Ops → Access) — read-only RBAC viewer: "who can delete Secrets here?" and per-subject effective permissions (admin/operator only)
- **Resource graph, diff and shortcuts** — a relationship graph per workload, a diff review before any Apply, a global namespace selector, saved filter views, keyboard shortcuts (`?`) and an in-app notification inbox
- **Explain with AI** (opt-in) — bring your own OpenAI-compatible, Anthropic or Ollama endpoint; the payload is redacted and previewed first and Secret values are never sent
- **Fleet** — search workloads, services and namespaces across every cluster; every registered cluster at a glance: reachable / degraded / unreachable (probed in the background), latency, nodes ready, pods running, warnings and CPU/memory, with one-click switching; the cluster chip in the top bar carries the active cluster's state
- **Services**, **Namespaces** (with create/delete)
- **Virtual Machines** — KubeVirt lifecycle (start/stop/migrate), real console info (`virtctl vnc` command — Axiom has no embedded browser VNC client), and Snapshots/Clones/Exports/Restores tabs
- **Storage** — PVCs grouped by StorageClass, plus StorageClasses and cluster-scoped PersistentVolumes as first-class objects
- **Atlas** — live storage control-plane view (backends/pools/volumes/buckets/policies) when Atlas is connected, with a "Create volume" flow
- **Pod Logs** — chunked-HTTP live tail with container selection, deep-linkable from Pod detail
- **Jobs & CronJobs** — CronJob run-now/suspend-resume/delete, Job status/completions/duration, separate from the Workloads table
- **ConfigMaps** — full CRUD alongside Secrets
- **Quotas & Limits** — ResourceQuotas (hard/used), LimitRanges, PodDisruptionBudgets per namespace, and cluster-wide PriorityClasses, read-only
- **Ingresses** — hosts, class, TLS, address — OpenShift-Route-equivalent view, plus a Gateway API section (GatewayClass/Gateway/HTTPRoute) when that CRD group is installed
- **Autoscaling** — HorizontalPodAutoscalers (every metric with current/target and the autoscaler's own state; create on CPU and memory), plus read-only VPA recommendations and KEDA ScaledObjects
- **Custom Resources** — a generic browser for any installed CustomResourceDefinition: pick a CRD by group/version/kind, browse its instances, open one to view its raw manifest — works for CRDs Axiom has no dedicated integration for
- **Events**, **Secrets** (+ External Secrets, create/delete), **Backups** (VolumeSnapshots + Velero, create/restore), **Alerts**
- **Ops hub** — tabbed Helm (browse a chart catalog or install from a repo URL, rollback/uninstall existing releases) / GitOps (per-row sync trigger for Argo/Flux) / Agents / Identity / Audit (client-side search + CSV export) / Access (RBAC — create/delete namespaced Roles and RoleBindings) / Webhooks (admission webhook visibility) / Cost (by namespace and by workload, per-namespace budgets, and rightsizing recommendations)
- **Helm catalog and GitOps creation** (Ops → Helm / GitOps) — browse approved chart repositories and install with default values pre-filled; create Argo CD or Flux applications from a form after previewing the exact YAML
- **Users** — local accounts (admin/operator/viewer roles) layered on top of the env-configured operator account and any SSO login; two-factor (TOTP) status, admin reset and sign-out-everywhere per account
- **Account security** (user menu) — enrol two-factor sign-in with any authenticator app, keep one-time recovery codes, sign out of every device
- **Languages** — English, Deutsch, Español, Français, Português, Italiano, 日本語 and 简体中文 (user menu, login page or Settings → Appearance; defaults to the browser language). Kubernetes terms and server messages stay in English; see `frontend/src/locales/README.md` to add strings or a language
- **Settings** — OIDC, Atlas, Postgres, Probe integration cards with save + test-connection, plus Slack/webhook alert notifications

Real auth throughout: local Basic-auth sign-in form plus SSO buttons that redirect to OIDC/SAML when configured (`GET /api/v1/auth/providers`), with a lab-mode "fill demo/demo" hint when no SSO is set up. SAML completes at `/api/v1/auth/saml/acs` (pin the IdP cert with `AXIOM_SAML_IDP_CERT`, or set `AXIOM_SAML_ALLOW_UNSIGNED=true` for lab). LDAP binds through the same username/password form when `AXIOM_LDAP_URL` is set. Local accounts support `admin` / `operator` / `viewer` — viewers are read-only at the auth middleware; operators can mutate day-2 resources but not users, integrations, or the cluster registry.

Multi-cluster switching is real, not cosmetic: the sidebar cluster switcher is backed by `GET /api/v1/clusters` / `POST /api/v1/clusters/{name}/activate`. A production deployment running in-cluster via ServiceAccount (the normal case) honestly reports exactly one cluster; demo mode ships three independent demo clusters so the switcher has something real to exercise.

## Backend

The demo build has no Kubernetes/client-go dependency, so it stays reproducible in restricted environments — it does pull in a few small non-Kubernetes packages shared with the production build (`golang.org/x/crypto/bcrypt`, `jackc/pgx`, `gorilla/websocket`).

Production Kubernetes support is kept behind the `kubernetes` build tag and uses:

- `client-go`
- Kubernetes Deployments, StatefulSets, DaemonSets
- Kubernetes Pod log API
- Kubernetes exec/SPDY API — both one-shot (`ExecCommand`) and a real interactive PTY over WebSocket (`ExecWS`)
- Eviction API for Pod disruption
- Kubernetes Nodes API for cordon/drain
- Kubernetes Events API
- `autoscaling/v2` HorizontalPodAutoscalers, `policy/v1` PodDisruptionBudgets, `networking.k8s.io/v1` NetworkPolicies
- Server-side apply (`types.ApplyPatchType`, `FieldManager: "zyvor-axiom"`) for generic manifest apply — reuses the same YAML/JSON parse and GVK→GVR discovery as manifest validation
- `metrics.k8s.io` raw REST access for per-pod/per-container **and per-node** CPU & memory (same `AbsPath` pattern, sharing one in-memory ring buffer keyed by `"node:<name>"` vs. `"<namespace>/<name>"`)
- Dynamic client + `apiextensions.k8s.io` CustomResourceDefinition listing for the generic CRD browser — works for any installed CRD, not just the ones Axiom has a dedicated integration for
- Dynamic client for Gateway API (`gateway.networking.k8s.io` GatewayClass/Gateway/HTTPRoute) — tolerates the CRDs being absent, the same way GitOps tolerates no Argo/Flux install
- `AdmissionregistrationV1` for read-only ValidatingWebhookConfiguration/MutatingWebhookConfiguration visibility
- `StorageV1`/cluster-scoped `PersistentVolumes` for StorageClass and PV listing
- Node taint add/remove (`Nodes().Update`, same get-mutate-Update shape as cordon/drain)
- Pod scheduling diagnosis: aggregates the pod's `PodScheduled` condition, matching warning Events, and container waiting reasons into one plain-English "why is this pod not running" answer — no new API dependency, just a smarter read of data already fetched elsewhere
- Namespaced RBAC `Role`/`RoleBinding` create/delete (deliberately **not** `ClusterRole`/`ClusterRoleBinding` — see Security below)
- GitOps force-sync: patches Flux's `reconcile.fluxcd.io/requestedAt` annotation or Argo's top-level `operation.sync` field via a JSON merge patch — the same mechanism `flux reconcile`/`argocd app sync` use
- Helm chart install: launches a `batchv1.Job` running `alpine/helm` with `helm install --repo <url>` (same Job-based mechanism as the existing uninstall/rollback), values YAML mounted from a ConfigMap — no shell interpolation of user input anywhere, so no command-injection surface regardless of the release/chart name or repo URL
- `SchedulingV1` PriorityClasses (read-only, cluster-scoped)
- Per-workload cost breakdown: the existing per-pod CPU/memory-request cost loop gained a second accumulator keyed by owner reference, with ReplicaSet owners resolved to their Deployment name (stripping the pod-template-hash suffix) so pods roll up to "checkout" rather than the ReplicaSet's generated "checkout-7f7fc496b4" — no extra API calls

### API surface

See [`docs/API.md`](docs/API.md) for the full reference (100+ routes). Core groups:

- **Cluster:** health, self-metrics (`/api/v1/metrics`, Prometheus text format), session, overview, regions, utilization, events, real cluster list/switch
- **Workloads:** CRUD, scale, stop/start, restart, seal/unseal, rollout, Still previews
- **Jobs/CronJobs:** list, delete, trigger (run-now), suspend/resume
- **Config:** ConfigMaps (CRUD), ResourceQuotas, LimitRanges, Ingresses, PodDisruptionBudgets
- **Pods:** list, logs, one-shot exec, interactive terminal (WebSocket), evict, window proxy, per-pod metrics
- **Nodes:** list, detail, cordon, uncordon, drain, per-node metrics, taint add/remove
- **Autoscaling:** HorizontalPodAutoscaler list/create/delete (CPU and memory targets), read-only VPA and KEDA views
- **Network:** namespaces, services, topology, PacketWolf status/proxy, NetworkPolicy list/delete, Gateway API (GatewayClass/Gateway/HTTPRoute)
- **Apply:** generic server-side apply for any manifest (kubernetes build only)
- **Custom resources:** list installed CRDs, list/get instances of any CRD by group/version/plural
- **Storage objects:** StorageClasses, cluster-scoped PersistentVolumes
- **Admission:** ValidatingWebhookConfiguration/MutatingWebhookConfiguration visibility
- **Diagnostics:** per-pod scheduling diagnosis ("why is this pod not running")
- **VMs:** KubeVirt lifecycle, snapshots, clones, exports, restores
- **Ops:** Helm (list/install/rollback/uninstall), GitOps (list/sync), secrets, backups, Velero (list + schedule create), alerts, audit, RBAC (list + namespaced Role/RoleBinding create/delete), cost (by namespace and by workload)
- **Scheduling policy:** PriorityClasses (read-only)
- **Platform:** Atlas, Haven identity, agent runs, integrations, intent placement
- **Notifications:** Slack/webhook config + test, automatic Warning-event dispatch
- **Users:** local account CRUD (admin/viewer)
- **Auth:** providers, OIDC/SAML login, logout

## Run the tested local demo

```bash
go test ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...
make build   # builds the frontend, then the Go binary

AXIOM_MODE=demo \
AXIOM_USERNAME=demo \
AXIOM_PASSWORD=demo \
AXIOM_ADDR=:5090 \
./bin/zyvor-axiom
```

Open `http://localhost:5090` — the login form appears automatically in lab mode. Sign in with **`demo` / `demo`**.

Remote k3s lab deploys use the same credentials unless you override `AXIOM_USERNAME` / `AXIOM_PASSWORD`. See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Production Kubernetes build

The production module manifest is separate so the dependency-free demo build can be compiled without downloading Kubernetes modules.

```bash
go mod download -modfile=go.production.mod
./scripts/sync-ui.sh   # builds frontend/ and refreshes internal/ui/dist/
go build -modfile=go.production.mod -tags kubernetes -o bin/zyvor-axiom-kubernetes ./cmd/zyvor-axiom
```

Run using in-cluster credentials:

```bash
AXIOM_MODE=kubernetes \
AXIOM_USERNAME=demo \
AXIOM_PASSWORD='replace-me' \
AXIOM_ADDR=:5090 \
./bin/zyvor-axiom-kubernetes
```

Or outside the cluster:

```bash
KUBECONFIG=$HOME/.kube/config AXIOM_MODE=kubernetes ./bin/zyvor-axiom-kubernetes
```

Only the `kubernetes`-tagged build registers the interactive terminal WebSocket route and Pod `resources` support; the demo build's Pod detail page falls back to one-shot exec automatically (via `GET /api/v1/realtime/capabilities`).

## Build the frontend

The Go binary embeds `internal/ui/dist`, which is a **generated build artifact** (gitignored) — never commit changes to it directly.

```bash
./scripts/sync-ui.sh
```

This runs `npm install && npx craco build` inside `frontend/` and copies the output into `internal/ui/dist/`, then rebuild the Go binary. Requires Node 20+ and npm locally (not needed if you only build via Docker — the multi-stage `Dockerfile` has its own `node:20-slim` build stage).

For local frontend development against a running backend:

```bash
make demo &                 # backend on :5090
cd frontend && npm start    # dev server on :3000, proxies /api to :5090 (see package.json "proxy")
```

## Workload semantics

Axiom treats workload lifecycle semantically rather than pretending a Pod is a VM:

- **Stop Deployment/StatefulSet**: save desired replicas in `axiom.zyvor.dev/last-replicas`, then scale to zero.
- **Start**: restore the saved replica count.
- **Restart**: patch the Pod template using `axiom.zyvor.dev/restarted-at`.
- **Scale**: update the native workload replica count.
- **Evict Pod**: use Kubernetes Eviction API in production.
- **Drain Node**: skip DaemonSet and `kube-system` Pods and evict ordinary workloads.

## Security

Lab deploys use HTTP Basic auth (`AXIOM_USERNAME` / `AXIOM_PASSWORD`, default `demo` / `demo`) over HTTPS with a self-signed certificate. Production deployments should:

- Replace the lab cert with a CA-issued TLS certificate
- Enable OIDC/SAML via Settings → Integrations or environment variables
- Place Axiom behind an identity-aware proxy when using LDAP (`AXIOM_LDAP_ENABLED`)
- Grant the ServiceAccount only the RBAC verbs in `deploy/k8s/rbac.yaml` and narrow namespaces further where possible — this is a deliberate per-group allowlist, not a wildcard, so the generic CRD browser can *list CRD definitions* for any installed CRD but can only *read instances* of the specific groups granted here; browsing an ungranted CRD's instances returns the real Kubernetes RBAC-forbidden error rather than a misleading empty list
- Axiom's own RBAC UI (Ops → Access) can create/delete namespaced `Role`/`RoleBinding` objects but deliberately cannot touch `ClusterRole`/`ClusterRoleBinding` — a UI path to create cluster-scoped roles would be a materially bigger privilege-escalation surface than a namespace-scoped one, and every other write feature in this app is already namespace-scoped
- The Helm "Install chart" feature runs `helm install` as a Job under Axiom's own ServiceAccount — a chart can only create resources that ServiceAccount is itself permitted to create, so it inherits whatever RBAC boundary is configured here rather than escalating beyond it
- Prefer `viewer`-role local accounts (Settings → Users) for anyone who only needs read access, instead of sharing the single admin operator credential

A browser can't attach an `Authorization` header to a plain link or WebSocket, which the pod-window proxy and interactive terminal both need for local Basic-auth sessions. Axiom accepts a `?access_token=<base64 user:pass>` query parameter for exactly this case (`internal/auth/basic.go`); OIDC/SAML sessions ride their cookie and need nothing extra.

### Users and roles

Beyond the single env-configured operator account (always `admin`), Settings → Users manages local accounts with bcrypt-hashed passwords, persisted in a `zyvor-axiom-users` Secret in production (in-memory in demo). Each account is `admin` or `viewer`, with create/edit-role/reset-password/delete all available from the Users page:

- `viewer` accounts can read every page and every `GET` endpoint; the UI shows a persistent banner so a viewer always knows they're read-only, not just after a failed click
- Any mutating request (`POST`/`PUT`/`PATCH`/`DELETE`) from a `viewer` is rejected with `403` at the auth middleware (`internal/auth/basic.go`), before it reaches any handler — enforcement is centralized, not per-page
- A `viewer` can optionally be restricted to specific namespaces (blank = every namespace, the default). Restricted namespace access is checked in the same middleware, before dispatch — via the `?namespace=` query param on list endpoints or the path segment on single-resource GETs (e.g. `/api/v1/secrets/{namespace}/{name}`). Omitting the namespace, or passing `all`, is treated as a cross-namespace request and blocked. Only `viewer` accounts can be namespace-restricted — promoting one to `admin` clears the restriction, since an admin needs cross-namespace write access anyway.
- OIDC logins can carry a role too: set `AXIOM_OIDC_ADMIN_GROUP`/`AXIOM_OIDC_VIEWER_GROUP` (and optionally `AXIOM_OIDC_GROUPS_CLAIM`, default `groups`) to map the ID token's group membership claim to a role. Leave both unset and `AXIOM_OIDC_DEFAULT_ROLE` applies (default `admin`). SAML ACS mints the same session cookie and uses the same group → role mapping when attribute groups are present.

The audit log (`GET /api/v1/audit`) persists across restarts in production — a `zyvor-axiom-audit` ConfigMap holds the last 200 entries — rather than the pure in-memory ring it used to be.

### Multi-cluster

`internal/platform.Provider` is always a `*SwitchableProvider` that delegates every call to whichever cluster is currently active *for the caller* (`internal/platform/switchable_provider_gen.go` — one generated delegator method per `Provider` interface method, so no handler code needs to know clusters exist). `GET /api/v1/clusters` lists registered clusters; `POST /api/v1/clusters/{name}/activate` switches the active one for the calling browser tab/script.

"Active" is scoped per caller, not one global switch: the frontend sends an `X-Axiom-Session` header (a random ID minted once per browser tab), so two tabs — or two different users — can each have a different cluster active without stomping on each other. A request with no session header (e.g. a scripted `curl`) falls back to a per-authenticated-username default instead. See `docs/ARCHITECTURE.md`'s Multi-cluster section for the exact plumbing.

- **Kubernetes mode, in-cluster (the normal deploy)**: exactly one cluster is registered, since a ServiceAccount token has no notion of "other contexts". Name it with `AXIOM_CLUSTER_NAME` instead of the generic `in-cluster` default.
- **Kubernetes mode, kubeconfig with multiple contexts**: one provider is registered per context, and switching is real.
- **Demo mode**: three independent demo clusters are registered so the switcher has real (if fake) behavior to exercise without a live kubeconfig.

## Storage: Atlas

Axiom's Atlas page (and `/api/v1/atlas*`) talks to [Atlas](../atlas), Zyvor's separate storage control-plane service, when `AXIOM_ATLAS_URL` (and optionally `AXIOM_ATLAS_TOKEN`) is configured — via Settings → Integrations, or automatically when deploying with `./scripts/deploy-remote.sh <host> <user> --with-atlas` (see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)). Atlas is entirely optional; without it the Atlas page shows an honest "not connected" state.

## Alert notifications

Settings → Notifications configures a Slack incoming-webhook URL and/or a generic alert-webhook URL, each with a one-shot "Test" button (`POST /api/v1/integrations/slack/test`, `.../webhook/test`). When "Forward Warning events automatically" is on, a background dispatcher (`platform.RunAlertDispatcher`) polls `ListAlerts` every 60s and pushes new cluster Warning events to whichever channels are configured — this is genuine outbound delivery (a real Slack message / a real HTTP POST), not a UI-only toggle.

## Testing

- `go test ./cmd/... ./internal/...` — demo build. `go test -modfile=go.production.mod -tags kubernetes ./internal/...` — the real Kubernetes provider against fake clientsets (about 60 write operations: objects stored, 404 / 409 / 400 / 429 mapping, cross-namespace isolation). Fake clientsets don't reproduce API-server validation, admission, server-side apply or real drain semantics.
- `cd frontend && npm run lint` — design, a11y and JS lint.
- `e2e/` — Playwright: smoke, the Nikola workload walkthrough, **functional specs that submit every create / edit / delete / action flow** (and sign in through OIDC / SAML / LDAP mock IdPs), and opt-in visual regression. Runs in Chromium, Firefox and WebKit with `BROWSERS=all`. Details and what a pass does and does not prove: [`e2e/README.md`](e2e/README.md).

## Continuous integration

`.github/workflows/ci.yml` runs on every push/PR to `main`: builds the frontend (required before any Go step, since `internal/ui/embed.go`'s `//go:embed dist/*` needs `internal/ui/dist/` populated), then `go vet`/`go test`/`go build` for both the demo and `-tags kubernetes` builds, then sanity-checks `deploy/k8s/*.yaml` parses as valid YAML.

## Detailed documentation

- [`docs/QUICKSTART.md`](docs/QUICKSTART.md) — install via Helm in a few minutes (no Go/Node toolchain needed)
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — runtime and provider design
- [`docs/API.md`](docs/API.md) — complete HTTP API reference
- [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) — k3s/Kubernetes deployment and hardening
- [`docs/DESIGN-SYSTEM.md`](docs/DESIGN-SYSTEM.md) — Axiom's flat apple.com-dark UX system
- [`docs/RENAMING.md`](docs/RENAMING.md) — full Aether → Axiom identity map
- [`docs/BACKUP-RESTORE.md`](docs/BACKUP-RESTORE.md) — backing up Axiom's own runtime state (local accounts, integration settings, audit log)
- [`docs/COMPLIANCE.md`](docs/COMPLIANCE.md) — current security/compliance posture
- [`CHANGELOG.md`](CHANGELOG.md) — release history
- [`SECURITY.md`](SECURITY.md) — vulnerability disclosure policy
- [`LICENSE`](LICENSE) — Zyvor Production License v1.0 (source-available; free for non-production use, production use needs a commercial license)
- `legal/` — draft EULA/DPA templates for counsel review (not for customer use as-is)

## License

Axiom is source-available under the **[Zyvor Production License v1.0](LICENSE)** (SPDX `LicenseRef-Zyvor-Production-1.0`, also in [LICENSES/](LICENSES/LicenseRef-Zyvor-Production-1.0.txt); see [NOTICE](NOTICE)).

- **Free** for evaluation, development, testing, proofs of concept, research, education and all other non-production use.
- **Production use needs a commercial license** from Zyvor AI Labs Private Limited: production clusters, customer workloads, SaaS, managed services, OEM and redistribution. See [docs/SUBSCRIPTION-MODEL.md](docs/SUBSCRIPTION-MODEL.md); talk to us at [zyvor.dev](https://zyvor.dev).

## Project layout

```text
cmd/zyvor-axiom/          server entrypoint
internal/api/              HTTP API and realtime hooks
internal/auth/             Basic-auth reference middleware
internal/config/           environment configuration
internal/platform/         provider model, demo + Kubernetes implementations
internal/ui/               embed.go + generated dist/ (gitignored build output)
frontend/                  React UX source (CRACO/Tailwind/shadcn) — edit here
scripts/                   development and deploy helpers
deploy/k8s/                Kubernetes deployment and RBAC
```
