# Overview

## Purpose

Product home for the activated cluster — marketing-style hero, capacity, inventory, and utilization, with links into observe pages.

## When to use it

- Operate **Overview** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/overview`
- Nav: **Top bar brand (Axiom) or Sidebar → Overview (default after login)**

## Operate from the console (UX)

1. Land on `/overview` after sign-in (or click Overview / the Axiom wordmark).
2. Read the hero (cluster name, health line, CTAs), then scroll capacity rings, inventory counts, and utilization charts.
3. Use **Stay informed** links for Alerts, Events, and Logs — Overview does not dump alert/activity lists on the home screen.
4. Use the sidebar cluster switcher when multiple clusters are configured (demo mode ships three).
5. Jump elsewhere via sidebar or ⌘K (compact Search in the top bar).
6. **Empty / fail:** Metrics blank → API unreachable or wrong activated cluster; re-login.
7. **Success:** Hero + capacity/inventory match `kubectl` reality (or demo fixtures).

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Nodes](../cluster/nodes.md)
- [Workloads](../workloads/workloads.md)
- [Alerts](../observe/alerts.md)
- [Events](../observe/events.md)
- [Logs](../observe/logs.md)
- [Getting Started](getting-started.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
