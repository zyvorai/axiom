# Namespaces

## Purpose

Namespace inventory — create and delete tenancy boundaries.

## When to use it

- Operate **Namespaces** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/namespaces`
- Nav: **Sidebar → Namespaces**

## Operate from the console (UX)

1. Open `/namespaces`.
2. **Create namespace** → name → Create; Delete from row actions (confirm).
3. **Empty / fail:** 403 → viewer; delete blocked if resources remain.
4. **Success:** Namespace appears in filters across Workloads/Pods.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Workloads](../workloads/workloads.md)
- [Quotas & Limits](../config/quotas.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
