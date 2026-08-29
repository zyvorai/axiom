# Overview

## Purpose

Cluster pulse — capacity metrics, activity, and shortcuts into day-2 centers.

## When to use it

- Operate **Overview** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/overview`
- Nav: **Sidebar → Overview (default after login)**

## Operate from the console (UX)

1. Land on `/overview` after sign-in (or click Overview).
2. Confirm cluster name/metrics and recent activity load.
3. Use the sidebar cluster switcher when multiple clusters are configured (demo mode ships three).
4. Jump to Nodes / Workloads / Alerts from shortcuts or ⌘K.
5. **Empty / fail:** Metrics blank → API unreachable or wrong activated cluster; re-login.
6. **Success:** Cluster headline + cards match `kubectl` reality (or demo fixtures).

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Nodes](../cluster/nodes.md)
- [Workloads](../workloads/workloads.md)
- [Alerts](../observe/alerts.md)
- [Getting Started](getting-started.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
