# Axiom frontend

React 19 + [CRACO](https://craco.js.org/) (Create React App, un-ejected) + Tailwind CSS + [shadcn/ui](https://ui.shadcn.com/) (Radix primitives). This is the embedded web UI for [Zyvor Axiom](../README.md) — see the root README and [`docs/DESIGN-SYSTEM.md`](../docs/DESIGN-SYSTEM.md) for the page list and design tokens.

## Develop against a running backend

```bash
# from the repo root
make demo &                 # Go backend on :5090 (AXIOM_MODE=demo, demo/demo)

# in this directory
npm install --legacy-peer-deps
npm start                   # dev server on :3000, proxies /api to :5090 (see "proxy" in package.json)
```

`--legacy-peer-deps` is needed because a couple of transitive peer-dependency ranges are stricter than what's actually compatible here (this mirrors what `scripts/sync-ui.sh` runs).

## Build

```bash
npm run build
```

Output goes to `build/`. This repo doesn't consume it directly from here — `scripts/sync-ui.sh` (invoked by `make ui` / `make build` / the Dockerfile) runs this build and copies `build/` into `../internal/ui/dist/`, which the Go binary embeds. Never edit `internal/ui/dist/` directly; it's a generated, gitignored artifact.

## Structure

```text
src/
  App.js              route table
  pages/*.jsx           one file per route
  components/           shared shell (Sidebar, TopBar, AppShell, ViewerBanner), PodTerminal.jsx (xterm.js)
  components/ui/        shadcn/ui primitives (button, dialog, table, tabs, ...)
  lib/
    api.js               axios instance, log streaming, withAccessToken() for WS/plain-link auth
    auth.js               AuthProvider (local Basic-auth + OIDC/SAML)
    cluster.js             real cluster-switcher state (GET/POST /api/v1/clusters*)
    theme.js                dark/light toggle
  index.css              design tokens + "Liquid Glass" utility classes
```

## Real backend, not mocks

Every page calls the real Go API under `/api/v1/*` (see [`docs/API.md`](../docs/API.md)) — there is no mock data layer. Two backend capabilities are conditional:

- The **interactive Pod terminal** (`PodTerminal.jsx`) and the Pod **YAML/Details tab**'s rich per-container spec only work against the `kubernetes`-tagged Go build (a real cluster). Check `GET /api/v1/realtime/capabilities`'s `interactiveExec` before assuming the WebSocket terminal is available — `make demo` (the stdlib-only build) doesn't register that route at all, and the frontend falls back to a one-shot exec panel.
- The **Atlas** and **PacketWolf** pages/panels degrade to an honest "not connected" state when `AXIOM_ATLAS_URL` / `AXIOM_PACKETWOLF_URL` aren't configured, rather than showing fake data.
