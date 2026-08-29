# Quotas & Limits

## Purpose

ResourceQuotas, LimitRanges, PDBs, PriorityClasses.

## When to use it

- Operate **Quotas & Limits** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/quotas`
- Nav: **Sidebar → Quotas & Limits**

## Operate from the console (UX)

1. Open `/quotas`.
2. Review quota / limit range / PDB / priority class tables.
3. Adjust via YAML apply when the UI does not offer inline edit.
4. **Empty / fail:** No quotas → create for multi-tenant namespaces.
5. **Success:** Hard limits visible; workload create respects them.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Namespaces](../cluster/namespaces.md)
- [Workloads](../workloads/workloads.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
