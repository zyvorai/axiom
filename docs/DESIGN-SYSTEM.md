# Axiom Design System

`frontend/` is a React 19 + CRACO + Tailwind + shadcn/ui app wired to Axiom's real Go API. Its visual language is the **Netra** web design system (apple.com-derived) with a Meta-style presentation on the Overview: `#000` / `#f5f5f7` canvas, bordered 28px-radius cards, blue-topped metric tiles, pill buttons and uppercase badges, blue hero glows, and tight 600-weight headlines. Type is the system **SF Pro** stack; there are no web fonts. Theme toggle lives in the top bar and under **Settings → Appearance**.

The reference is `netra/web/src/styles.css`; token values here are copied from it (see also `zeus-os/docs/netra-design-system.md`). If Netra's tokens change, change them in `tokens.css` first.

Three files carry the system:

- **`frontend/src/styles/tokens.css`** — **the single source of truth.** Every colour, radius, motion curve, font stack and terminal/syntax colour is a CSS custom property here (light on `:root`, dark on `:root[data-theme="dark"]`). It also defines the short shell aliases (`--bg`, `--ink`, `--line`, `--blue` …) so the shell CSS reads the same values. Imported first from `src/index.js`.
- **`frontend/src/index.css`** — the Tailwind/shadcn bridge. shadcn needs HSL triplets (so `bg-primary/50` opacity modifiers work); they are *derived from the same values* as `tokens.css` and must be mirrored by hand when a colour changes. Dark `--border` is a solid colour, not alpha-in-HSL, so `border-border/50` works.
- **`frontend/src/axiom-shell.css`** — the component vocabulary consumed by the app shell and every page: `.bar`/`.side`/`.item`, `.head`/`.detail-head`, `.card`, `.pill`, `.tbl`, `.tile`/`.vm`, `.tabs`, `.stat`/`.stats`/`.meter`, `.btn*`, `.hero`/`.check`/`.notice`/`.section-h`, plus the shared layout helpers listed below. Plain CSS classes scoped under `.ax`, like the login page's `axiom-login.css`. **No literal colours** — everything comes from tokens.

## Tokens

```css
:root {                          /* light */
  --bg-page: #f5f5f7;   --bg-elevated: #ffffff;   --bg-elevated-2: #f5f5f7;
  --text-primary: #1d1d1f;  --text-secondary: #6e6e73;  --text-tertiary: #86868b;
  --border-hairline: #d2d2d7;  --border-strong: #c7c7cc;
  --apple-blue: #0071e3;   --apple-link: #0066cc;   --danger: #ff3b30;
  --radius-3xl: 28px;  --ease-apple: cubic-bezier(0.28, 0.11, 0.32, 1);
  /* … surfaces, accents, semantic bg/border, terminal + syntax colours … */
}
:root[data-theme="dark"] {       /* dark */
  --bg-page: #000;   --bg-elevated: #1d1d1f;   --bg-elevated-2: #2c2c2e;
  --text-primary: #f5f5f7;   --border-hairline: rgba(255,255,255,.16);
  --apple-link: #2997ff;   --danger: #ff453a;
}
```

The blue **fill** stays `#0071e3` in both themes (white label); only the *link/text* blue changes (`#0066cc` light, `#2997ff` dark). Small text on light surfaces uses the darker text-safe accents (`--green`, `--amber`, `--orange`, `--red`) instead of the fill accents.

Radii: `xs 6 · sm 10 · md 12 · base 14 · lg 16 · xl 18 · 2xl 24 · 3xl 28 · pill 980`. Cards use 3xl, entity tiles 2xl, metric tiles xl, inputs md, buttons/badges pill. Tailwind's `rounded-*` scale is mapped onto these.

Type is the system stack (`--font-display` body/UI, `--font-hero` headlines; `-apple-system, BlinkMacSystemFont, "SF Pro Text/Display", "Segoe UI", sans-serif`, `font-synthesis: none`). Mono is `ui-monospace, SFMono-Regular, "SF Mono", Menlo`, with self-hosted JetBrains Mono Variable as a non-Apple fallback (imported in `src/index.js`). Inter is not loaded.

### Using colours in code

Pages and components must not contain hex literals or raw Tailwind palette classes — `npm run lint:design` (`frontend/scripts/lint-design.sh`) fails on them. Use:

| Need | Use |
|------|-----|
| Text / surfaces / borders | shadcn tokens (`text-foreground`, `text-muted-foreground`, `bg-card`, `border-border`) or shell text ramp (`text-ink`, `text-ink-2`, `text-ink-3`, `border-line`) |
| Accent text | `text-tone-blue\|link\|cyan\|green\|amber\|orange\|red\|purple\|teal` |
| Tinted alert surfaces | `bg-soft-danger\|warning\|success\|info` with `border-soft-*-border` |
| Terminals / code panes (dark in both themes) | `bg-term-bg`, `text-term-plain\|dim\|key\|str\|punct`; xterm.js reads the same tokens via `lib/terminalTheme.js` |
| One-off dynamic colour | a CSS variable, e.g. `style={{"--c": "var(--green)"}}` |

Tone/soft/term colours are plain `var()` colours, so they don't take a `/opacity` modifier; use the `soft-*` surfaces for tints.

**Buttons**: `.btn-fill`/shadcn `default`/`brand` are the same solid blue pill (white label, brightness +6% on hover, blue glow on hover in dark, `scale(.98)` on press). `.btn-line`/`outline`/`secondary`/`ghost` stay neutral; `.btn-danger`/`destructive` and `.btn-success` are the only other hues. Disabled buttons use a **solid** disabled fill (`--disabled-bg`/`--disabled-text`), never an opacity fade. Focus is a 2px `rgba(0,113,227,.45)` outline.

## App shell vocabulary (`axiom-shell.css`)

| Class | Purpose |
|-------|---------|
| `.bar` | Sticky top bar (brand, cluster pill, breadcrumbs, search, alerts, theme toggle, user menu) |
| `.side` / `.side.min` | Sidebar, expanded / collapsed-to-icons; `.side.open` is the mobile slide-in state |
| `.item` | A sidebar nav link; `[aria-current="page"]` drives the active state |
| `.card` | Bordered 28px-radius panel with a blue top hairline (`GlassCard` renders this). `.card.p-0` is an edge-to-edge table card (clipped to the radius) |
| `.pill` | Status dot + label (`StatusBadge` renders this); `.pill.soft` is a neutral outline variant |
| `.tbl` | A real `<table>` — hover-row highlight and column typography built in |
| `.tile` / `.vm` | An entity card (name/status header, a spec grid, an actions row) — used for Nodes/Namespaces/VM grids |
| `.tabs` | Flat underline tabs, keyed off `[aria-selected="true"]` (works with Radix `Tabs`) |
| `.head` / `.detail-head` | Page header / resource-detail header (`PageHeader` / `DetailHeader`) |
| `.hero` / `.notice` / `.section-h` | Overview's marketing-style hero, setup banner, section intro |
| `.check` | Getting Started's numbered checklist row |
| `.form-section` / `.form-row` | Settings/Users form grouping |
| `.status-dot` | Health indicator (ok/warn/err/info), still used by a few legacy call sites alongside `.pill` |
| `.panel-h` | Title row inside an edge-to-edge (`p-0`) card, above its table |
| `.rule-row` | Settings row between two hairlines |
| `.data-row` | Generic list row (used by `AutoTable`'s card list) |
| `.tile-facts` | Two-column key/value facts under a tile header |
| `.btn-success` / `.bare-input` | Green outline button / borderless input reset |
| `.eyebrow` | 12px uppercase blue label (page heads, detail heads) |
| `.stats.c3`, `.tile-specs.c2` | 3-column stats / 2-column tile specs modifiers; `.g3` is a responsive ≥300px auto-fill grid |
| `.page.rhythm`, `.hero-media`, `.stat.media`, `.card.media`, `.cta-cards` / `.cta-card` | Meta-style presentation, opt-in (used on the Overview): large rounded hero block, bolder section rhythm, larger gradient metric tiles, link cards |

Buttons are pills (`--radius-pill`), 36px by default (netra's compact tier), `.btn-lg` 44px, `.btn-sm` 30px.

## Navigation

`frontend/src/components/Sidebar.jsx` groups `constants/navItems.js`'s routes into sections (**Core**, **Cluster**, **Network**, **Storage**, **Config**, **Observe**, **Admin**, plus an **Extensions** group for dynamically-registered plugins) using `.grp`/`.grp-h` (collapsible per group) and `.item` rows, with live workload/pod/alert counts next to the relevant items. Expanded `248px`, collapsed `64px` icon rail with hover tooltips (`.tip`); collapse choice persists in `localStorage` (`axiom:sidebar-collapsed`) and is shared with the top bar's toggle button. Below `900px` viewport width the sidebar becomes a `.side.open` fixed overlay with a backdrop, triggered by a `PanelLeft` button in the top bar.

Sidebar order:

| UI | Route | Notes |
|----|-------|-------|
| Getting Started | `/getting-started` | Live setup checklist |
| Overview | `/overview` | Marketing-style home: rounded hero block, inventory, utilization, trend, and link cards; observe links only |
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

The login screen (`frontend/src/pages/Login.jsx` + `axiom-login.css`, via the shared `PremiumLoginShell` component) is a two-chapter page in the Netra login style, and it follows the app theme (light and dark): a hero chapter over the two-point hero glow (Zyvor mark, **AXIOM** wordmark, **Sign in to Axiom.**, a tagline, four domain pills, a single **Get started** CTA that scrolls down) and a sign-in chapter below it holding a two-step flow modeled on Apple ID's pattern, in a bordered 28px card.

- **Step 1 (identity)**: a username field with a **Continue** action. When SSO is configured, plain text links — **Sign in with SSO** (OIDC) and/or **Sign in with SAML** — sit under the password form instead of a row of per-provider icon buttons; Axiom has one generic OIDC provider slot and one SAML provider slot, not per-vendor integrations.
- **Step 2 (password)**: a back chip showing the chosen username, the password field, a **Remember me on this device** checkbox (persists only the username to `localStorage`, never the password), and a **Sign in to Axiom** submit. Submitting is the real functioning sign-in path (`POST` is actually just HTTP Basic auth stored client-side, verified against `GET /api/v1/session`); the two-step split is purely client-side UI state — both values still travel together in one `signInLocal(username, password)` call. A failed attempt shows an inline soft-danger error banner (not a toast) and clears the password field.

- `GET /api/v1/auth/providers` drives everything: `oidc.enabled`/`saml.enabled` show/hide the two SSO links; `lab.operator_login: true` + `lab.hint` shows a "Lab mode — Fill {hint}" inline action under the identity-step card
- `AXIOM_LDAP_ENABLED`/`AXIOM_LDAP_DOMAIN`/`AXIOM_LDAP_URL` are surfaced in `GET /api/v1/auth/providers`'s `ldap` block and are now backed by a real LDAP simple-bind implementation (`internal/auth/ldap.go`) — the login form's username/password fields double as the LDAP form when `AXIOM_LDAP_URL` is set, there's no separate LDAP UI
- An OIDC or SAML login's role (admin vs. viewer) is resolved server-side from the group claim/attribute when `AXIOM_OIDC_ADMIN_GROUP`/`AXIOM_OIDC_VIEWER_GROUP` are configured — the frontend has no visibility into this beyond the `role` it gets back from `GET /api/v1/session`, same as a local account

## Shell

`AppShell.jsx` renders `.ax` → `.frame` (`[Sidebar, main]`) under a sticky `.bar` top bar — no ambient backdrop, no glass chrome.

- Sidebar (`.side`): a `.cluster` pill at top (when a cluster is active), collapsible `.grp` groups of `.item` nav links with live workload/pod/alert counts, a collapse toggle at the bottom. Collapses to a `64px` icon rail (`.side.min`) with hover tooltips, or slides in as a mobile overlay (`.side.open`) below `900px`.
- Top bar (`.bar`, `48px`, backdrop-blurred): a `PanelLeft` sidebar toggle, the Zyvor mark + **Axiom** wordmark (links to `/overview`), the cluster pill, `.crumbs` breadcrumbs, then on the right — a `.search` chip that opens the command palette (⌘K), a **Connect** pill, an alerts bell (red dot when there are unresolved alerts), an in-bar theme toggle, and the user avatar menu. Below `760px` the bar is compact (wordmark text, ⌘K hint and Connect are hidden, the cluster pill is capped) so it fits a ~360px viewport — an overflowing bar makes mobile browsers widen the layout viewport for the whole page.
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

Two themes via the top-bar toggle or **Settings → Appearance** (`lib/theme.js`, `localStorage` key `axiom-theme`; a saved value under the old `kubecloud.theme` key is still read once). Setting a theme toggles Tailwind's `dark` class on `<html>` *and* sets `<html data-theme="dark|light">` — both are needed, one drives the shadcn tokens, the other the shell tokens. An inline script in `public/index.html` applies the saved theme (and the `theme-color` meta: `#000` dark, `#f5f5f7` light) before first paint, so there is no flash. Sonner toasts follow the same theme.

- **Light** — `#f5f5f7` canvas, `#fff` cards, `#1d1d1f` text, Action Blue `#0071e3`
- **Dark** — `#000` canvas, `#1d1d1f` cards, `#f5f5f7` text, link blue `#2997ff`

Default is dark. ⌘K → "Toggle light/dark mode" also switches.

## Page headers

Every top-level page uses a shared `PageHeader` component (`components/PageHeader.jsx`, rendering `.head`) in Netra's page-hero style: an optional blue **eyebrow**, a tight 600-weight title, an optional lede (max 640px), an optional right-aligned `action` slot for the page's primary button(s), and an optional `tools` slot for a filter/search/segmented-control row beneath the title, all over a soft radial glow. Resource-detail pages (Pod/Node/Workload/etc) use the sibling `DetailHeader` component (`.detail-head`) — a blue kind eyebrow, a large mono resource-name title, and a meta row. Both are consumed by every page specifically so a sitewide visual change touches only these shared components, not each individual page file.

## Redesign pass (app shell, all 37 pages)

The original flat apple.com-dark pass described below was later re-based on the Netra design language (single token source, bordered cards, blue-topped tiles, glows) — the per-page archetype mapping still holds, only the look changed.

The app was rebuilt end-to-end around the vocabulary above, mapped per page archetype rather than one template copied everywhere:

- **`Overview.jsx`** (the app's homepage) is a `.hero` composition: cluster name as the headline, a health pill + region line, dual CTAs, then hairline-separated `.stats` sections — **At a glance** (node/pod/workload/VM counts) and **Utilization** (CPU/memory/storage as `.stat` + `.meter` bars, replacing the old ring gauges) — plus a **Trend** section (the original CPU/memory `Sparkline` history, kept as a bonus beyond the reference layout) and **Stay informed** links to `/alerts`/`/events`/`/logs`. **Stay informed** is now a row of `.cta-card` link cards, and the page uses the opt-in Meta-style classes (`.page.rhythm`, `.hero-media`, `.stat.media`, `.card.media`). Breadcrumbs are suppressed on `/overview`.
- **`components/MetricRing.jsx`** was removed — Overview's capacity band now uses `.stat`/`.meter` bars instead of ring gauges, and nothing else referenced the component.
- **`components/FeatureTile.jsx`** — unchanged behavior, retinted via the token change; still used on PodDetail, NodeDetail, Atlas, Networking, and Ops' Cost tab.
- **`components/SpecTable.jsx`** — unchanged behavior, an apple.com "Tech Specs" style label/value table, now sitting inside `.card` sections instead of glass boxes.
- **Nodes/Namespaces** (`Nodes.jsx`/`Namespaces.jsx`) use a generic `.tile` entity card (name + status pill header, a spec grid, an actions row) — the same shape as the `.vm` card `VirtualMachines.jsx` uses for its VM grid (name/status header, vCPU/Memory/Node specs, Start-or-Stop/Migrate/Console actions).
- **List pages** (Workloads, Pods, Jobs, Secrets, ConfigMaps, Events, Services, Ingresses, CustomResources, PluginView, HPA, Quotas, Backups, Atlas, Alerts, PlatformIntents, Networking) use `.head`'s `tools` slot for search/filter controls and a real `<table class="tbl">` for their list — never a `<div>`-styled fake table, since `e2e/tests/nikola-ux.spec.js` locates Workloads/Pods rows via `page.locator("tr")`.
- **Detail pages** with tabs (PodDetail, WorkloadDetail, NodeDetail, NamespaceDetail) apply `.tabs` to their `TabsList`; tab names/roles are unchanged (PodDetail's Details/Terminal/YAML/Metrics/Events/Labels tabs are e2e-tested by exact name). Detail pages without tabs (ConfigMapDetail, SecretDetail, ServiceDetail, JobDetail, CronJobDetail, VMDetail) use `.detail-head` + stacked `.card` sections.
- **`components/AutoTable.jsx`** — the shared table renderer behind Atlas, CustomResources, PluginView, Ingresses, Storage now emits a real `.tbl` table when given explicit `columns`, or `.card`/`.stack` rows when auto-guessing columns for arbitrary/varying-shape objects.
- **Settings/Users** use `.form-section`/`.form-row` for grouped fields; **Ops** (the largest page — Helm/GitOps/Agents/Identity/Audit/Access/Webhooks/Cost) applies `.tabs` at the top level with `.tbl`/`.card`/`.stats` per tab's content shape. All dialogs and mutations across every page are unchanged — this was a markup/className pass, not a behavior change.
- **`CodeEditor.jsx`**: the editable manifest tabs (PodDetail/WorkloadDetail's YAML tab) now open straight into edit mode instead of requiring an extra "Edit" click first — read-only viewers (CustomResources, PluginView, Networking's raw-JSON fallback) are unaffected since they pass `editable={false}`.
