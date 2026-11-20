# Storage

## Purpose

PVC / storage class intelligence for the connected cluster.

## When to use it

- Operate **Storage** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/storage`
- Nav: **Sidebar → Storage**

## Operate from the console (UX)

1. Open `/storage`.
2. Review PVCs/StorageClasses; note bound vs pending.
3. Cross-link to Atlas for intent-backed provisioning.
4. **Empty / fail:** Pending PVCs → check provisioner / Atlas.
5. **Success:** Bound volumes listed for workloads.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Atlas](atlas.md)
- [Backups](../protect/backups.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
