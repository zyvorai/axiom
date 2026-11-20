# ConfigMaps

## Purpose

ConfigMap inventory — create, edit keys, delete.

## When to use it

- Operate **ConfigMaps** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/configmaps`
- Detail: `/configmaps/{namespace}/{name}`
- Nav: **Sidebar → ConfigMaps**

## Operate from the console (UX)

1. Open `/configmaps`.
2. Click a name to view keys and values on the detail page.
3. **Create ConfigMap** — add keys → Create; delete retired maps from the list.
4. **Empty / fail:** 403 on mutate → viewer role.
5. **Success:** ConfigMap listed and mountable by workloads.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Secrets](secrets.md)
- [Workloads](../workloads/workloads.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
