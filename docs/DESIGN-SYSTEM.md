# Axiom Design System (Kube-cockpit / Liquid Glass)

`frontend/` **is** the kube-cockpit reference source (React 19 + CRACO + Tailwind + shadcn/ui), copied in directly and wired to Axiom's real Go API — not a hand-rolled reimplementation. Dark liquid-glass surfaces, coral→sky-blue brand gradient (`#ff7550` → `#6ab6f0`), Inter + JetBrains Mono (with `-apple-system`/`BlinkMacSystemFont` ahead of Inter in the stack, so Mac/iOS/Safari render real San Francisco), 20px card radii.

## Tokens

Defined in `frontend/src/index.css` as CSS custom properties (HSL-based, shadcn convention), registered as Tailwind color/radius tokens in `frontend/tailwind.config.js`:

```css
:root {
  --background: 20 10% 3%;
  --foreground: 30 20% 96%;
  --radius: 1.25rem;
  --brand-hex: #ff7550;
  --glass-bg: 255 255 255 / 0.04;
  --glass-border: 255 255 255 / 0.10;
  --grid-color: 255 255 255 / 0.05;
  --k-ok: 128 45% 62%;
  --k-warn: 22 92% 62%;
  --k-err: 4 90% 65%;
  --k-info: 205 65% 72%;
  --chart-1..5: /* five distinct series hues for future chart work */
}
```

Fonts load from Google Fonts (see `frontend/public/index.html`'s preconnect + stylesheet link) — this is a real network dependency, not self-hosted; it's the fallback behind the system-font stack above, not the primary source.

### Brand accent & the i17 palette

`index.css` also defines a fuller "iPhone 17 palette" (`--i17-cosmic/deep-blue/sky-blue/lavender/sage/mist/white/silver/gold`, each with distinct light/dark values) — this is the actual source of the brand gradient (`--i17-cosmic` ≡ `--brand-hex`, `--i17-sky-blue` is the gradient's blue end) plus extended accent hues used for metric-ring/sparkline/chart coloring (`Overview.jsx`, `MetricRing.jsx`, `Sparkline.jsx`) instead of ad hoc per-page hex literals. Exposed as Tailwind tokens (`brand-cosmic`, `brand-sky`, `i17-lavender`, etc.) and as a `brand` variant on `components/ui/button.jsx` (`bg-gradient-to-r from-brand-cosmic to-brand-sky`).

**Convention**: the brand gradient (`variant="brand"`) is reserved for the single most prominent action on a page or dialog (the one thing a user came there to do — "Run workload," "Create X," "Install," "Save"). Every other button — secondary actions, row-level operations, less-frequent dialog confirms — uses the plain `default` or `outline` variant instead. This is deliberate: the gradient is a rare, confident accent, not a default button style.

## Glass utilities

| Class | Purpose |
|-------|---------|
| `.glass` | Frosted panel with blur + border |
| `.glass-inset` | Inset variant (cluster switcher, inputs) |
| `.specular` | Gradient border highlight |
| `.grid-bg` | Background grid overlay |
| `.aurora-a/b/c` | Login hero orbs |
| `.orb` | Shell ambient orbs |
| `.fade-up` | Entry animation |
| `.status-dot` | Health indicator (ok/warn/err/info), driven by `<StatusBadge>` |

## Navigation

Sidebar order (`frontend/src/components/Sidebar.jsx`):

| UI | Route | Notes |
|----|-------|-------|
| Getting Started | `/getting-started` | Live setup checklist |
| Overview | `/overview` | Cluster hero, metric rings, sparklines, alerts, activity |
| Nodes | `/nodes` | Cordon/uncordon/drain; each card links to `/nodes/:name` for metrics + taint management |
| Workloads | `/workloads` | Full action menu + Run workload wizard + bulk restart/delete |
| Jobs & CronJobs | `/jobs` | Run-now/suspend-resume/delete CronJobs, delete Jobs |
| Pods | `/pods` | Exec + evict per row, bulk evict |
| Platform Intents | `/platform-intents` | Read-only, populated by the Intent launch flow |
| Networking · Cilium | `/networking` | Service graph + live PacketWolf panel |
| Services | `/services` | |
| Ingresses | `/ingresses` | Hosts, class, TLS, address — OpenShift-Route equivalent, plus a Gateway API section |
| Namespaces | `/namespaces` | Create/delete |
| Virtual Machines | `/vms` | KubeVirt lifecycle + data-protection tabs |
| Storage | `/storage` | PVCs grouped by StorageClass |
| Atlas | `/atlas` | Live when `AXIOM_ATLAS_URL` is configured |
| Pod Logs | `/logs` | Accepts `?pod=namespace/name` |
| Events | `/events` | |
| ConfigMaps | `/configmaps` | Create/delete, same pattern as Secrets |
| Secrets | `/secrets` | + External Secrets section |
| Quotas & Limits | `/quotas` | ResourceQuotas (hard/used) + LimitRanges + PodDisruptionBudgets + PriorityClasses, read-only |
| Autoscaling | `/autoscaling` | HorizontalPodAutoscaler list/create/delete |
| Custom Resources | `/custom-resources` | Generic browser for any installed CRD |
| Backups | `/backups` | VolumeSnapshots + Velero, create/restore + Velero schedule create |
| Alerts | `/alerts` | |
| Ops | `/ops/:tab` | Tabbed: Helm/GitOps/Agents/Identity/Audit/Access/Webhooks/Cost |
| Users | `/users` | Local account CRUD, admin/viewer role badge |
| Settings | `/settings` | OIDC/Atlas/Postgres/Probe/Notifications cards |

Not in the sidebar but routable: `/workloads/:ns/:kind/:name` (workload detail), `/pods/:ns/:name` (pod detail), `/nodes/:name` (node detail).

## Auth

The login screen (`frontend/src/pages/Login.jsx`) shows three SSO-styled buttons (Google/GitHub/Okta icons, matching the reference 1:1 visually) **all pointing at the same real OIDC login redirect** — Axiom has one generic OIDC provider slot, not per-vendor integrations, so the buttons are a visual choice, not three distinct backends. Below them, a local username/password form is the real functioning sign-in path (`POST` is actually just HTTP Basic auth stored client-side, verified against `GET /api/v1/session`).

- `GET /api/v1/auth/providers` drives everything: `oidc.enabled`/`saml.enabled` show/hide the SSO buttons and a SAML link; `lab.operator_login: true` + `lab.hint` shows a "fill demo / demo" one-click helper under the local form
- Clicking an SSO button when OIDC isn't configured shows a toast pointing at Settings instead of doing nothing
- `AXIOM_LDAP_ENABLED`/`AXIOM_LDAP_DOMAIN`/`AXIOM_LDAP_URL` are surfaced in `GET /api/v1/auth/providers`'s `ldap` block, but there is no actual LDAP bind/authentication implementation behind them yet (`internal/auth` only implements Basic, OIDC and SAML-cookie checks) — a known gap, not a login option today
- An OIDC login's role (admin vs. viewer) is resolved server-side from the ID token's group claim when `AXIOM_OIDC_ADMIN_GROUP`/`AXIOM_OIDC_VIEWER_GROUP` are configured — the frontend has no visibility into this beyond the `role` it gets back from `GET /api/v1/session`, same as a local account

## Shell

- Fixed ambient backdrop (grid + orbs)
- Glass sidebar: brand, cluster switcher, nav (scrollable — 26 items), theme pill toggle
- Glass topbar: breadcrumbs, ⌘K search, Connect kubeconfig (shows local dev instructions), user menu
- Cluster switcher (`frontend/src/components/ClusterSwitcher.jsx`, state in `frontend/src/lib/cluster.js`) is **real**: it's backed by `GET /api/v1/clusters` and `POST /api/v1/clusters/{name}/activate`. Selecting a cluster switches which one every API call hits, then reloads the page. A single-cluster deployment (the normal in-cluster case) renders the switcher as a static, non-interactive label instead of a dropdown — there's nothing to switch to, so it doesn't pretend otherwise. Demo mode registers three clusters, so the dropdown and switching are both fully exercisable without a live kubeconfig.

## Command palette

`frontend/src/components/CommandPalette.jsx` — ⌘K opens a `cmdk`-based dialog with a "Navigate" group listing all 26 top-level routes (from the shared `frontend/src/constants/navItems.js`, the same list `Sidebar.jsx` renders, so the two can't drift apart again), plus live-searched "Pods"/"Workloads"/"Namespaces" groups fetched once per palette session from the existing list endpoints (`/pods`, `/workloads`, `/namespaces`) and filtered by `cmdk`'s built-in fuzzy match as you type — not a static page list. Selecting a pod/workload jumps straight to its detail route; selecting a namespace jumps to the Namespaces page (there's no per-namespace detail route to link to more precisely).

## Node detail

`NodeDetail.jsx` (route `/nodes/:name`) — Details/Metrics/Taints tabs, plus the same Cordon/Drain buttons as the Nodes list:

- Details: allocatable CPU/memory/pods, OS image, kernel, container runtime, conditions
- Metrics: two `Sparkline`s (CPU/memory, shared component with Pod detail and Overview) from `GET /api/v1/nodes/{name}/metrics`, labeled as a live rolling window, not historical Prometheus data
- Taints: current taints as badges with a delete button each, plus an "Add taint" form (key/value/effect, effect is a shadcn `Select` of `NoSchedule`/`PreferNoSchedule`/`NoExecute`)

## Pod scheduling diagnosis

`PodDetail.jsx` fetches `GET /api/v1/pods/{ns}/{name}/diagnosis` on load and renders an amber banner (not scheduled) or a neutral banner (scheduled but not yet `Running`/`Succeeded`) automatically above the stat cards — no tab to click, since this is the first thing an operator wants to see when a pod isn't healthy. Nothing renders when the pod is running normally.

## Workload actions

| Action | API |
|--------|-----|
| Start/Stop/Restart | `POST .../start` \| `.../stop` \| `.../restart` |
| Scale | `POST .../scale {"replicas": N}` (dialog) |
| Seal / Unseal | `POST .../seal` \| `.../unseal` |
| Rollout | `GET .../rollout` (dialog: status + history) → `POST .../rollout/{pause\|resume\|restart\|undo}` |
| Delete | `DELETE /workloads/{ns}/{kind}/{name}` |

## Run workload wizard

Three tabs, all real (`frontend/src/pages/Workloads.jsx`'s `LaunchWizard`):

- **Form** → `POST /api/v1/workloads` (direct `LaunchRequest`)
- **Intent** → `POST /api/v1/intent/parse` to preview a receipt, `POST /api/v1/intent/place` to actually launch
- **Validate YAML** → `POST /api/v1/validate` for a dry-run schema/syntax check, then an "Apply" button (enabled only after a successful validation) that opens the shared `ApplyManifestDialog` component and hits `POST /api/v1/apply` for a real server-side apply (kubernetes build only; demo mode shows the honest "not available" message)

## Pod detail

`frontend/src/pages/PodDetail.jsx`, using the real pod spec from `GET /api/v1/resources/{ns}/pod/{name}` (kubernetes build) with a graceful fallback to the simpler pod-list fields in demo mode or on error:

- Details: containers (resources, env, volume mounts, liveness/readiness probe summaries), init containers, volumes, tolerations, node selector, service account, conditions, owner reference
- Terminal: multi-tab — a nested tab bar lets you open several concurrent shell sessions (a "+" button adds one, each closable), every tab mounting its own real xterm.js + WebSocket PTY instance (`interactiveExec: true` from `GET /api/v1/realtime/capabilities`), each fully independent; otherwise a one-shot exec panel with an explanation why
- YAML: an editable `<textarea>` (not just a read-only `<pre>`) with a Save button that POSTs the edited text to `POST /api/v1/apply` — a real server-side apply, not a dry run
- Metrics: two `Sparkline`s (CPU/memory, shared component with Overview.jsx) from `GET /api/v1/pods/{ns}/{name}/metrics`, plus a per-container breakdown table; labeled as a live rolling window, not historical Prometheus data
- Ports: "Open" buttons proxy into the pod's container port via `GET /api/v1/pods/{ns}/{name}/window/{port}/`

`WorkloadDetail.jsx`'s YAML tab follows the identical editable-textarea-plus-Save pattern against the same `/api/v1/apply` endpoint.

## Bulk actions

`Pods.jsx` and `Workloads.jsx` both add a checkbox column (shadcn `Checkbox`, header checkbox selects/clears all currently-filtered rows) and a "N selected" toolbar that appears next to the page title once at least one row is checked. No new backend endpoints — bulk evict (Pods) and bulk restart/delete (Workloads) fire `Promise.allSettled` over the existing single-resource endpoints and report how many of N succeeded.

## Autoscaling page

`HPA.jsx` — a single-section list (target workload, min–max replicas, current replicas, current/target CPU%, age, delete) plus a "Create autoscaler" dialog whose target-workload picker is populated from the same `GET /api/v1/workloads` list used elsewhere, so there's no duplicated workload-listing logic.

## Custom Resources page

`CustomResources.jsx` — a two-column layout: a searchable list of installed CRDs on the left (from `GET /api/v1/crds`), and the selected CRD's instances on the right via **`AutoTable`, reused verbatim from Atlas.jsx** (extracted into `frontend/src/components/AutoTable.jsx` for this purpose). Clicking a row opens a dialog with the instance's raw JSON manifest. Because RBAC for CRD instances is an intentional per-group allowlist (see [ARCHITECTURE.md](ARCHITECTURE.md#generic-apply-and-the-crd-browser)), a `403` from the backend renders as "Permission denied: ..." in place of the row list, not a misleading "no instances" empty state.

## Helm ops

Rollback opens a revision input inline in the Ops → Helm row, then `POST .../rollback` with `{ "revision": N }`. Uninstall is a single click per row. An "Install chart" button opens a dialog (namespace, release name, repo URL, chart name, optional version and values YAML) backed by `POST /api/v1/helm/install`, with an inline note that the install runs under Axiom's own permissions — the chart can only create resources Axiom itself is allowed to.

## GitOps sync

`GitOpsTab` adds a per-row Sync icon button (`RotateCcw`) next to each Application/Kustomization/HelmRelease, calling `POST /api/v1/gitops/{kind}/{namespace}/{name}/sync` and refreshing the list on success.

## RBAC (Ops → Access)

`RbacTab` shows four sections (Cluster roles, Cluster role bindings, Roles, Role bindings) in the same two-column grid as before, but only the namespaced Roles/RoleBindings sections get a "New role"/"New binding" button and a per-row delete icon — Cluster roles/bindings stay read-only, matching the backend's deliberate security boundary (see [ARCHITECTURE.md](ARCHITECTURE.md#rbac-write-boundary)). The create dialogs take rules/subjects as a small JSON textarea (pre-filled with a valid example) rather than a policy-rule form builder, mirroring the generic-apply textarea pattern used elsewhere.

## Cost (Ops → Cost)

`CostTab` shows the total/CPU-rate/memory-rate summary cards, then two `Rows` sections: "By namespace" (unchanged) and a new "By workload" table (workload name, namespace · kind, cores, GiB, monthly USD) sourced from the same `GET /api/v1/cost` response's `workloads` field.

## Audit (Ops → Audit)

`AuditTab` gained a search input (filters the already-fetched entries client-side by action/target/detail/user substring — the whole dataset is always ≤200 entries in one response, so no new backend query params) and an "Export CSV" button that builds a CSV blob from the currently-filtered entries and triggers a browser download (`axiom-audit-YYYY-MM-DD.csv`) — no new endpoint, since the server has no more data than what's already on screen.

## Admission webhooks (Ops → Webhooks)

A new read-only Ops tab: two `GlassCard` sections (Validating/Mutating), each row showing the webhook name, its owning configuration, matched rules, and failure policy (amber when `Fail`).

## Storage page

`Storage.jsx` gained two `AutoTable` sections below the existing PVC list — StorageClasses (provisioner, reclaim policy, binding mode, default flag) and PersistentVolumes (capacity, access modes, claim, storage class) — reusing the same `AutoTable` component as Atlas and the CRD browser, since the page's own header already claimed to cover both and previously only delivered PVCs.

## PacketWolf (Networking page)

Status chip from `GET /api/v1/packetwolf/status`. When connected, the flows panel polls `GET /api/v1/packetwolf/proxy/api/v1/flows?limit=25` every 8s, rendering a table with a defensive raw-JSON fallback if the shape doesn't match the expected flow-record fields.

## NetworkPolicies (Networking page)

A `NetworkPolicyPanel` below the PacketWolf panel: a list (name, namespace, policy types, pod selector rendered as a label-selector string or "all pods", delete) from `GET /api/v1/networkpolicies`, and a "Create via YAML" button that opens the shared `ApplyManifestDialog` pre-filled with a minimal deny-by-default NetworkPolicy template. There's no dedicated ingress/egress rule-builder form — creation always goes through the generic apply endpoint, since a bespoke rule builder would either be half-built or duplicate a full YAML editor.

## Atlas page

`GET /api/v1/atlas` drives an adaptive table view (field names aren't hardcoded — Atlas's REST shapes vary per resource type and evolve independently of this repo) for backends/pools/volumes/buckets/policies/storage classes, plus a capacity summary and a "Create volume" dialog (`POST /api/v1/atlas/volumes`). Shows an honest "not connected" state with a Settings link when `AXIOM_ATLAS_URL` isn't configured.

## Users page

`frontend/src/pages/Users.jsx` — a table (username, role badge, namespace badges or "All namespaces", created-at, edit + delete) plus "Add user" and "Edit user" dialogs (username — create only, password, a shadcn `Select` for admin/viewer, and a comma-separated namespace-restriction field shown only when role is viewer). Backed by `GET/POST/PATCH/DELETE /api/v1/users`; `PATCH` accepts role/password/namespaces independently so an existing account can be edited in place. A viewer-restricted account shows its allowed namespaces as badges in the table; an unrestricted viewer shows "All namespaces".

`frontend/src/components/ViewerBanner.jsx` renders a small persistent strip under the TopBar (`AppShell.jsx`) whenever the signed-in account's role is Viewer — sourced from `useAuth()`'s `isViewer`, itself derived from `GET /api/v1/session`'s `role` field. It's the only role-aware UI chrome; individual pages don't hide or disable buttons based on role, relying on the backend's 403 instead.

## Notifications (Settings)

A card in `frontend/src/pages/Settings.jsx` alongside OIDC/Atlas/Postgres/Probe: two password-masked URL fields (Slack webhook, generic alert webhook — masked because a webhook URL is itself a bearer credential), a "Forward Warning events automatically" toggle, a Save button, and two independent Test buttons (one per channel) that call `POST /api/v1/integrations/slack/test` / `.../webhook/test` and toast the real result.

## Jobs & CronJobs page

`frontend/src/pages/Jobs.jsx` — two stacked sections (same `Section` row-grid pattern as Backups): CronJobs (schedule, Running/Paused status derived from `suspend`, last-scheduled time, a run-now icon button, a shadcn `Switch` for suspend/resume, delete) and Jobs (status badge — `Complete`/`Failed`/`Running`/`Pending` — completions, duration, delete). No create dialog; Jobs come from CronJob triggers or external tooling.

## ConfigMaps / Quotas & Limits / Ingresses pages

- **ConfigMaps** (`ConfigMaps.jsx`) mirrors Secrets.jsx's CRUD pattern exactly: a searchable table, key badges per row, a "Create ConfigMap" dialog with dynamic key/value rows.
- **Quotas & Limits** (`Quotas.jsx`) reuses Backups.jsx's `Section` component for four read-only lists: ResourceQuotas (Hard/Used badge groups), LimitRanges (Container/Pod/PVC type badges with default/request/max/min inline), PodDisruptionBudgets (min/max-available, current/desired healthy, expected pod count), and PriorityClasses (value, a "default" badge for the cluster's `globalDefault` class, description).
- **Ingresses** (`Ingresses.jsx`) is a plain searchable table (name, namespace, class, host badges, address, a lock/unlock icon for TLS, age) — no create/delete, since Ingress objects are typically managed by application manifests rather than click-ops.

## Appearance

Default is dark liquid glass. Theme toggle in sidebar footer switches dark/light (`html.dark` / `html.light`).
