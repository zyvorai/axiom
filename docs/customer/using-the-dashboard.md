# Using the Dashboard

Axiom is a Go API with an embedded React SPA. Production talks to k3s/Kubernetes; demo mode uses deterministic fixtures.

## Shell chrome

| Element | Job |
|---------|-----|
| Sidebar | NAV_ITEMS — Getting Started through Settings |
| Cluster switcher | `GET/POST /api/v1/clusters…/activate` |
| Command palette | ⌘K / Ctrl+K — same route list as the sidebar |
| Top bar / user menu | Session, theme, sign out |
| Toasts | Mutation results (create/scale/delete) |

## Operate tips

1. Start on **Overview**, then **Getting Started** for integration gaps.
2. Prefer semantic workload actions (Start/Stop/Restart/Scale/Seal) over raw kubectl for day-2.
3. Pod detail terminal needs a valid session (WebSocket may use `?access_token=` for Basic auth).
4. Deterministic navigation calls the router — do not send “open Workloads” through an AI helper.
5. Never paste lab IPs into runbooks — use `<host>`.

## Related

- [Getting Started](getting-started.md)
- [Page index](PAGE_INDEX.md)
- [Workflows](workflows.md)
