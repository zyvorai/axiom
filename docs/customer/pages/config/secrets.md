# Secrets

## Purpose

Secret inventory — create typed secrets, add keys, delete.

## When to use it

- Operate **Secrets** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/secrets`
- Detail: `/secrets/{namespace}/{name}`
- Nav: **Sidebar → Secrets**

## Operate from the console (UX)

1. Open `/secrets`.
2. Click a name to open detail (keys masked). Use **Reveal values** only when needed.
3. **Create secret** — type + keys → Create; treat values as sensitive in UI.
4. **Empty / fail:** ClusterRole must allow secrets write in scope used by Users/Integrations.
5. **Success:** Secret available to workloads; delete cleans up.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [ConfigMaps](configmaps.md)
- [Users](../admin/users.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
