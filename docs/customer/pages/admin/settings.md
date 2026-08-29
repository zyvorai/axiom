# Settings

## Purpose

Integrations, clusters, theme, and console configuration.

## When to use it

- Operate **Settings** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/settings`
- Nav: **Sidebar → Settings**

## Operate from the console (UX)

1. Open `/settings`.
2. Integrations: Atlas URL/token → Save → Test connection; OIDC/SAML; PacketWolf.
3. Clusters: review activated cluster label (`AXIOM_CLUSTER_NAME`).
4. Theme toggle via shell control.
5. **Empty / fail:** Test connection failed → network/token; OIDC redirect URI must be `https://<host>:30590/api/v1/auth/oidc/callback`.
6. **Success:** Integration cards healthy; Atlas/Networking panels populate.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Atlas](../storage/atlas.md)
- [Users](users.md)
- [Getting Started](../console/getting-started.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
