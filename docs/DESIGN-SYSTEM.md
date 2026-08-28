# Axiom Design System (Kube-cockpit / Liquid Glass)

`frontend/` **is** the kube-cockpit reference source (React 19 + CRACO + Tailwind + shadcn/ui), copied in directly and wired to Axiom's real Go API — not a hand-rolled reimplementation. Dark liquid-glass surfaces, coral brand accent (`#ff7550`), Inter + JetBrains Mono, 16px card radii.

## Tokens

Defined in `frontend/src/index.css` as CSS custom properties (HSL-based, shadcn convention):

```css
:root {
  --background: 20 10% 3%;
  --foreground: 30 20% 96%;
  --brand-hex: #ff7550;
  --glass-bg: 255 255 255 / 0.04;
  --glass-border: 255 255 255 / 0.10;
  --grid-color: 255 255 255 / 0.05;
  --k-ok: 128 45% 62%;
  --k-warn: 22 92% 62%;
  --k-err: 4 90% 65%;
  --k-info: 205 65% 72%;
}
```

Fonts load from Google Fonts (see `frontend/public/index.html`'s preconnect + stylesheet link) — this is a real network dependency, not self-hosted.

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
| Nodes | `/nodes` | Cordon/uncordon/drain |
| Workloads | `/workloads` | Full action menu + Run workload wizard |
| Pods | `/pods` | Exec + evict per row |
| Platform Intents | `/platform-intents` | Read-only, populated by the Intent launch flow |
| Networking · Cilium | `/networking` | Service graph + live PacketWolf panel |
| Services | `/services` | |
| Namespaces | `/namespaces` | Create/delete |
| Virtual Machines | `/vms` | KubeVirt lifecycle + data-protection tabs |
| Storage | `/storage` | PVCs grouped by StorageClass |
| Atlas | `/atlas` | Live when `AXIOM_ATLAS_URL` is configured |
| Pod Logs | `/logs` | Accepts `?pod=namespace/name` |
| Events | `/events` | |
| Secrets | `/secrets` | + External Secrets section |
| Backups | `/backups` | VolumeSnapshots + Velero, create/restore |
| Alerts | `/alerts` | |
| Ops | `/ops/:tab` | Tabbed: Helm/GitOps/Agents/Identity/Audit/Access/Cost |
| Settings | `/settings` | OIDC/Atlas/Postgres/Probe cards |

Not in the sidebar but routable: `/workloads/:ns/:kind/:name` (workload detail), `/pods/:ns/:name` (pod detail).

## Auth

The login screen (`frontend/src/pages/Login.jsx`) shows three SSO-styled buttons (Google/GitHub/Okta icons, matching the reference 1:1 visually) **all pointing at the same real OIDC login redirect** — Axiom has one generic OIDC provider slot, not per-vendor integrations, so the buttons are a visual choice, not three distinct backends. Below them, a local username/password form is the real functioning sign-in path (`POST` is actually just HTTP Basic auth stored client-side, verified against `GET /api/v1/session`).

- `GET /api/v1/auth/providers` drives everything: `oidc.enabled`/`saml.enabled` show/hide the SSO buttons and a SAML link; `lab.operator_login: true` + `lab.hint` shows a "fill demo / demo" one-click helper under the local form
- Clicking an SSO button when OIDC isn't configured shows a toast pointing at Settings instead of doing nothing
- `AXIOM_LDAP_ENABLED`/`AXIOM_LDAP_DOMAIN`/`AXIOM_LDAP_URL` are surfaced in `GET /api/v1/auth/providers`'s `ldap` block, but there is no actual LDAP bind/authentication implementation behind them yet (`internal/auth` only implements Basic, OIDC and SAML-cookie checks) — a known gap, not a login option today

## Shell

- Fixed ambient backdrop (grid + orbs)
- Glass sidebar: brand, cluster switcher, nav (scrollable — 19 items), theme pill toggle
- Glass topbar: breadcrumbs, ⌘K search, Connect kubeconfig (shows local dev instructions), user menu
- Cluster switcher is **cosmetic**: Axiom connects to one Kubernetes cluster per instance; the switcher lists `session.regions` (a synthetic multi-region breakdown of that one cluster, see `internal/platform.DefaultRegions`) and does not change which cluster API calls hit

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
- **Validate YAML** → `POST /api/v1/validate`, informational only — Axiom has no raw-manifest-apply endpoint, so this tab validates syntax/schema and says so; it does not launch anything

## Pod detail

`frontend/src/pages/PodDetail.jsx`, using the real pod spec from `GET /api/v1/resources/{ns}/pod/{name}` (kubernetes build) with a graceful fallback to the simpler pod-list fields in demo mode or on error:

- Details: containers (resources, env, volume mounts, liveness/readiness probe summaries), init containers, volumes, tolerations, node selector, service account, conditions, owner reference
- Terminal: real xterm.js + WebSocket PTY when `GET /api/v1/realtime/capabilities` reports `interactiveExec: true`; otherwise a one-shot exec panel with an explanation why
- YAML: raw manifest
- Ports: "Open" buttons proxy into the pod's container port via `GET /api/v1/pods/{ns}/{name}/window/{port}/`

## Helm ops

Rollback opens a revision input inline in the Ops → Helm row, then `POST .../rollback` with `{ "revision": N }`. Uninstall is a single click per row.

## PacketWolf (Networking page)

Status chip from `GET /api/v1/packetwolf/status`. When connected, the flows panel polls `GET /api/v1/packetwolf/proxy/api/v1/flows?limit=25` every 8s, rendering a table with a defensive raw-JSON fallback if the shape doesn't match the expected flow-record fields.

## Atlas page

`GET /api/v1/atlas` drives an adaptive table view (field names aren't hardcoded — Atlas's REST shapes vary per resource type and evolve independently of this repo) for backends/pools/volumes/buckets/policies/storage classes, plus a capacity summary and a "Create volume" dialog (`POST /api/v1/atlas/volumes`). Shows an honest "not connected" state with a Settings link when `AXIOM_ATLAS_URL` isn't configured.

## Appearance

Default is dark liquid glass. Theme toggle in sidebar footer switches dark/light (`html.dark` / `html.light`).
