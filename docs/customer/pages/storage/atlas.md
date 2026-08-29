# Atlas

## Purpose

Atlas storage control-plane panel — backends, pools, volumes, buckets, policies.

## When to use it

- Operate **Atlas** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/atlas`
- Nav: **Sidebar → Atlas**

## Operate from the console (UX)

1. Wire Atlas under **Settings → Integrations** (URL + token) or deploy with `--with-atlas`.
2. Open `/atlas` — backends/pools should load when connected.
3. **Create volume** for a smoke provision; review buckets/policies tables.
4. **Empty / fail:** Connection failed → Test connection in Settings; token/RBAC.
5. **Success:** Volume appears; Storage page reflects new PVC if bound into k8s.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Storage](storage.md)
- [Settings](../admin/settings.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
