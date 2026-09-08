# Axiom Design System

`frontend/` is a React 19 + CRACO + Tailwind + shadcn/ui app wired to Axiom's real Go API, restyled to a flat, dark **apple.com** visual language: a plain black `#000` canvas (dark) / white `#fff` canvas (light), `#1d1d1f` (dark) / `#f5f5f7` (light) panel surfaces, hairline borders, pill-shaped buttons and status badges — no glass panels, no ambient gradient washes, no glow rings. Theme toggle lives in the top bar and under **Settings → Appearance**. Typography is apple.com **SF Pro** everywhere (`-apple-system` / `SF Pro Text` / `SF Pro Display`; mono prefers `SF Mono`).

Two stylesheets carry the system:

- **`frontend/src/index.css`** — the shared Tailwind/shadcn design tokens (CSS custom properties, HSL-based, registered as Tailwind color/radius tokens in `frontend/tailwind.config.js`). Both light and dark now use the *same* apple.com-derived palette (previously light was Apple.com but dark was a mismatched GitHub-Primer-dark palette — that split is gone).
- **`frontend/src/axiom-shell.css`** — the actual component vocabulary consumed by the app shell and every page: `.bar`/`.side`/`.item` (top bar/sidebar), `.head`/`.detail-head` (page/detail headers), `.card` (flat surface), `.pill` (status dot + label), `.tbl` (table), `.tile`/`.vm` (entity/VM card), `.tabs` (underline tabs), `.stat`/`.stats`/`.meter` (metric tiles), `.btn`/`.btn-fill`/`.btn-line`/`.btn-danger` (pill buttons), `.hero`/`.check`/`.notice`/`.section-h` (Overview/Getting-started patterns). Plain CSS classes, not Tailwind utility soup — same pattern as the login page's `axiom-login.css`.

## Tokens

```css
:root { /* Apple light */
  --background: 240 5% 96%;     /* #f5f5f7 */
  --foreground: 240 6% 10%;     /* #1d1d1f */
  --card: 240 5% 96%;
  --primary: 211 100% 45%;      /* Action Blue #0071e3 */
  --brand-hex: #0071e3;
  /* … */
}
.dark { /* Flat apple.com dark */
  --background: 0 0% 0%;        /* #000 */
  --foreground: 240 9% 96%;     /* #f5f5f7 */
  --card: 240 3% 12%;           /* #1d1d1f */
  --border: 0 0% 100% / 0.1;    /* hairline */
  --muted-foreground: 240 2% 65%;
  --primary: 209 100% 58%;      /* system blue #2997ff */
  --brand-hex: #2997ff;
  /* … */
}
```

Spacing/type/motion tokens are shared between themes.

UI type is the apple.com **SF Pro** system stack (`--font-display` for body/UI, `--font-hero` for headlines). Mono prefers **SF Mono**, with JetBrains Mono Variable as a non-Apple fallback (still imported in `index.css`). Inter is no longer loaded.

### Brand accent & the i17 palette

`index.css` also defines a fuller accent palette under the legacy `--i17-cosmic/deep-blue/sky-blue/lavender/sage/mist/white/silver/gold` variable names (kept because dozens of files already reference `i17-*`/`brand-*` Tailwind classes — retinting the CSS variable values in place, rather than renaming every call site, is what makes a palette-wide change a token-only edit almost everywhere). `--i17-cosmic` is the primary brand accent (≡ `--brand-hex`, system blue in both themes), `--i17-sage` is the emerald "healthy" accent, `--i17-lavender` a violet accent, `--i17-gold` the amber warning hue.

**Buttons**: `.btn-fill`/shadcn `default`/`brand` are the same solid blue pill fill (white label, inset highlight, darken on hover). `.btn-line`/`outline`/`secondary`/`ghost` stay neutral (flat, hairline or no border); `.btn-danger`/`destructive` is the only non-blue hue.

## App shell vocabulary (`axiom-shell.css`)

| Class | Purpose |
|-------|---------|
| `.bar` | Sticky top bar (brand, cluster pill, breadcrumbs, search, alerts, theme toggle, user menu) |
| `.side` / `.side.min` | Sidebar, expanded / collapsed-to-icons; `.side.open` is the mobile slide-in state |
| `.item` | A sidebar nav link; `[aria-current="page"]` drives the active state |
| `.card` | Flat panel surface (`GlassCard` renders this) |
| `.pill` | Status dot + label (`StatusBadge` renders this); `.pill.soft` is a neutral outline variant |
| `.tbl` | A real `<table>` — hover-row highlight and column typography built in |
| `.tile` / `.vm` | An entity card (name/status header, a spec grid, an actions row) — used for Nodes/Namespaces/VM grids |
| `.tabs` | Flat underline tabs, keyed off `[aria-selected="true"]` (works with Radix `Tabs`) |
| `.head` / `.detail-head` | Page header / resource-detail header (`PageHeader` / `DetailHeader`) |
| `.hero` / `.notice` / `.section-h` | Overview's marketing-style hero, setup banner, section intro |
| `.check` | Getting Started's numbered checklist row |
| `.form-section` / `.form-row` | Settings/Users form grouping |
| `.status-dot` | Health indicator (ok/warn/err/info), still used by a few legacy call sites alongside `.pill` |

Buttons are pill-shaped (`border-radius: 980px`/`16px`) with a solid Action Blue fill + inset highlight + darken-on-hover on `.btn-fill`, focus-visible outlined via the `--focus-ring` token.

## Navigation

`frontend/src/components/Sidebar.jsx` groups `constants/navItems.js`'s routes into sections (**Core**, **Cluster**, **Network**, **Storage**, **Config**, **Observe**, **Admin**, plus an **Extensions** group for dynamically-registered plugins) using `.grp`/`.grp-h` (collapsible per group) and `.item` rows, with live workload/pod/alert counts next to the relevant items. Expanded `248px`, collapsed `64px` icon rail with hover tooltips (`.tip`); collapse choice persists in `localStorage` (`axiom:sidebar-collapsed`) and is shared with the top bar's toggle button. Below `900px` viewport width the sidebar becomes a `.side.open` fixed overlay with a backdrop, triggered by a `PanelLeft` button in the top bar.

Sidebar order:

| UI | Route | Notes |
|----|-------|-------|
| Getting Started | `/getting-started` | Live setup checklist |
| Overview | `/overview` | apple.com-style marketing home: hero, capacity rings, inventory, utilization; observe links only |
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

The login screen (`frontend/src/pages/Login.jsx` + `axiom-login.css`, via the shared `PremiumLoginShell` component) is an Apple Store–style two-chapter page: a full-bleed hero chapter (Zyvor mark, **AXIOM** wordmark, **Sign in to Axiom.**, a tagline, four domain pills, a single **Get started** CTA that scrolls down) and a "parchment" sign-in chapter below it holding a two-step flow modeled on Apple ID's pattern.

- **Step 1 (identity)**: a username field with a **Continue** action. When SSO is configured, plain text links — **Sign in with SSO** (OIDC) and/or **Sign in with SAML** — sit under the password form instead of a row of per-provider icon buttons; Axiom has one generic OIDC provider slot and one SAML provider slot, not per-vendor integrations.
- **Step 2 (password)**: a back chip showing the chosen username, the password field, a **Remember me on this device** checkbox (persists only the username to `localStorage`, never the password), and a **Sign in to Axiom** submit. Submitting is the real functioning sign-in path (`POST` is actually just HTTP Basic auth stored client-side, verified against `GET /api/v1/session`); the two-step split is purely client-side UI state — both values still travel together in one `signInLocal(username, password)` call. A failed attempt shows an inline red error banner (not a toast) and clears the password field.

- `GET /api/v1/auth/providers` drives everything: `oidc.enabled`/`saml.enabled` show/hide the two SSO links; `lab.operator_login: true` + `lab.hint` shows a "Lab mode — Fill {hint}" inline action under the identity-step card
- `AXIOM_LDAP_ENABLED`/`AXIOM_LDAP_DOMAIN`/`AXIOM_LDAP_URL` are surfaced in `GET /api/v1/auth/providers`'s `ldap` block and are now backed by a real LDAP simple-bind implementation (`internal/auth/ldap.go`) — the login form's username/password fields double as the LDAP form when `AXIOM_LDAP_URL` is set, there's no separate LDAP UI
- An OIDC or SAML login's role (admin vs. viewer) is resolved server-side from the group claim/attribute when `AXIOM_OIDC_ADMIN_GROUP`/`AXIOM_OIDC_VIEWER_GROUP` are configured — the frontend has no visibility into this beyond the `role` it gets back from `GET /api/v1/session`, same as a local account

## Shell

`AppShell.jsx` renders `.ax` → `.frame` (`[Sidebar, main]`) under a sticky `.bar` top bar — no ambient backdrop, no glass chrome.

- Sidebar (`.side`): a `.cluster` pill at top (when a cluster is active), collapsible `.grp` groups of `.item` nav links with live workload/pod/alert counts, a collapse toggle at the bottom. Collapses to a `64px` icon rail (`.side.min`) with hover tooltips, or slides in as a mobile overlay (`.side.open`) below `900px`.
- Top bar (`.bar`, `48px`, backdrop-blurred): a `PanelLeft` sidebar toggle, the Zyvor mark + **Axiom** wordmark (links to `/overview`), the cluster pill, `.crumbs` breadcrumbs, then on the right — a `.search` chip that opens the command palette (⌘K), a **Connect** pill, an alerts bell (red dot when there are unresolved alerts), an in-bar theme toggle, and the user avatar menu.
- `Breadcrumbs.jsx` now renders as `.crumbs` inside the top bar itself (not in the content area) — `Clusters / {cluster} / {page}`, with a mono resource-name segment on detail pages. **Self-hides on `/overview`** so the product home reads as a marketing first viewport.
- Cluster switcher (`frontend/src/components/ClusterSwitcher.jsx`, state in `frontend/src/lib/cluster.js`) is **real**: it's backed by `GET /api/v1/clusters` and `POST /api/v1/clusters/{name}/activate`. Selecting a cluster switches which one every API call hits, then reloads the page. A single-cluster deployment (the normal in-cluster case) renders the switcher as a static, non-interactive `.cluster` pill instead of a dropdown — there's nothing to switch to, so it doesn't pretend otherwise. Demo mode registers three clusters, so the dropdown and switching are both fully exercisable without a live kubeconfig.

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

Two themes via the top-bar toggle or **Settings → Appearance** (`lib/theme.js`, `localStorage` key `kubecloud.theme` — unchanged name for backward compatibility with existing saved preferences). Setting a theme both toggles Tailwind's `dark` class on `<html>` and sets `<html data-theme="dark|light">`, the latter driving `axiom-shell.css`'s flat palette:

- **Light** — apple.com (`#fff` / `#f5f5f7` panels / Action Blue `#0071e3`)
- **Dark** — flat apple.com dark (`#000` / `#1d1d1f` panels / system blue `#2997ff`) — no longer GitHub-Primer-derived

Default is dark. ⌘K → "Toggle light/dark mode" also switches.

## Page headers

Every top-level page uses a shared `PageHeader` component (`components/PageHeader.jsx`, rendering `.head`): a big title, an optional description line, an optional right-aligned `action` slot for the page's primary button(s), and an optional `tools` slot for a filter/search/segmented-control row beneath the title — no tracked-caps eyebrow label (apple.com's own pages never use one, so the prop was dropped). Resource-detail pages (Pod/Node/Workload/etc) use the sibling `DetailHeader` component instead (rendering `.detail-head`) — a small kind eyebrow, a big mono resource-name title, and a meta row, matching their identifier-header role rather than a page-title role. Both are consumed by every page specifically so a sitewide visual change touches only these shared components, not each individual page file.

## Flat redesign pass (app shell, all 37 pages)

The app was rebuilt end-to-end around the vocabulary above, mapped per page archetype rather than one template copied everywhere:

- **`Overview.jsx`** (the app's homepage) is a `.hero` composition: cluster name as the headline, a health pill + region line, dual CTAs, then hairline-separated `.stats` sections — **At a glance** (node/pod/workload/VM counts) and **Utilization** (CPU/memory/storage as `.stat` + `.meter` bars, replacing the old ring gauges) — plus a **Trend** section (the original CPU/memory `Sparkline` history, kept as a bonus beyond the reference layout) and **Stay informed** links to `/alerts`/`/events`/`/logs`. Breadcrumbs are suppressed on `/overview`.
- **`components/MetricRing.jsx`** was removed — Overview's capacity band now uses `.stat`/`.meter` bars instead of ring gauges, and nothing else referenced the component.
- **`components/FeatureTile.jsx`** — unchanged behavior, retinted via the token change; still used on PodDetail, NodeDetail, Atlas, Networking, and Ops' Cost tab.
- **`components/SpecTable.jsx`** — unchanged behavior, an apple.com "Tech Specs" style label/value table, now sitting inside `.card` sections instead of glass boxes.
- **Nodes/Namespaces** (`Nodes.jsx`/`Namespaces.jsx`) use a generic `.tile` entity card (name + status pill header, a spec grid, an actions row) — the same shape as the `.vm` card `VirtualMachines.jsx` uses for its VM grid (name/status header, vCPU/Memory/Node specs, Start-or-Stop/Migrate/Console actions).
- **List pages** (Workloads, Pods, Jobs, Secrets, ConfigMaps, Events, Services, Ingresses, CustomResources, PluginView, HPA, Quotas, Backups, Atlas, Alerts, PlatformIntents, Networking) use `.head`'s `tools` slot for search/filter controls and a real `<table class="tbl">` for their list — never a `<div>`-styled fake table, since `e2e/tests/nikola-ux.spec.js` locates Workloads/Pods rows via `page.locator("tr")`.
- **Detail pages** with tabs (PodDetail, WorkloadDetail, NodeDetail, NamespaceDetail) apply `.tabs` to their `TabsList`; tab names/roles are unchanged (PodDetail's Details/Terminal/YAML/Metrics/Events/Labels tabs are e2e-tested by exact name). Detail pages without tabs (ConfigMapDetail, SecretDetail, ServiceDetail, JobDetail, CronJobDetail, VMDetail) use `.detail-head` + stacked `.card` sections.
- **`components/AutoTable.jsx`** — the shared table renderer behind Atlas, CustomResources, PluginView, Ingresses, Storage now emits a real `.tbl` table when given explicit `columns`, or `.card`/`.stack` rows when auto-guessing columns for arbitrary/varying-shape objects.
- **Settings/Users** use `.form-section`/`.form-row` for grouped fields; **Ops** (the largest page — Helm/GitOps/Agents/Identity/Audit/Access/Webhooks/Cost) applies `.tabs` at the top level with `.tbl`/`.card`/`.stats` per tab's content shape. All dialogs and mutations across every page are unchanged — this was a markup/className pass, not a behavior change.
- **`CodeEditor.jsx`**: the editable manifest tabs (PodDetail/WorkloadDetail's YAML tab) now open straight into edit mode instead of requiring an extra "Edit" click first — read-only viewers (CustomResources, PluginView, Networking's raw-JSON fallback) are unaffected since they pass `editable={false}`.
