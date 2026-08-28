# Axiom Design System (Kube-cockpit / Liquid Glass)

Axiom’s console follows the kube-cockpit multi-cluster visual language from `/Users/ssahani/axiom`: dark liquid-glass surfaces, coral brand accent, Inter + JetBrains Mono, 16px card radii.

Reference implementation: [frontend/src/index.css](https://github.com/ssahani/axiom) (React preview) — ported to vanilla JS in `web/`.

## Tokens

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

Typography is **self-hosted** under `web/fonts/` (Inter, JetBrains Mono) — no Google CDN at runtime.

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
| `.status-dot` | Health indicator (ok/warn/err/info) |

## Navigation

**Primary (kube-cockpit order):**

| UI | Route / page |
|----|----------------|
| Overview | Cluster hero, metric rings, sparklines |
| Nodes | Hosts |
| Workloads | Deployments / StatefulSets |
| Networking · Cilium | Network map |
| Virtual Machines | KubeVirt VMs |
| Storage | Volumes / PVCs |
| Pod Logs | Dedicated log stream page |
| Events | Activity stream |

**System (retained):** Pods, Secrets, Backups, Alerts, Ops hub, Settings

## Auth

SSO-first login UI (Google / GitHub / Okta labels) wired to real providers via `GET /api/v1/auth/providers`:

| Button | Provider key |
|--------|----------------|
| Google | OIDC |
| GitHub | OIDC |
| Okta | SAML |

- When SSO is **not** configured, `GET /api/v1/auth/providers` returns `lab.operator_login: true` and the **demo / demo** username/password form is shown by default (no hidden easter egg)
- SSO buttons are hidden until OIDC/SAML/LDAP is enabled
- Enabled providers redirect to `login_url` after 900ms pending animation
- With SSO on, use “Sign in with username & password” for operator break-glass access

## Shell

- Fixed ambient backdrop (grid + orbs)
- Glass sidebar: brand, cluster switcher, nav, theme pill toggle
- Glass topbar: breadcrumbs, ⌘K search, Connect kubeconfig, user menu
- Cluster switcher adapts single-cluster `/api/v1/overview` + `/api/v1/regions` data

## Run workload

Launch modal modes (unchanged): Intent, Form, YAML → `POST /api/v1/workloads`

## Appearance

Default is dark liquid glass. Theme toggle in sidebar footer switches dark/light (`html.dark` / `html.light`). Legacy palette swatches remain in Settings.
