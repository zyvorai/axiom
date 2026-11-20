# Getting Started

## Purpose

Live setup checklist — integration status and first tasks after sign-in.

## When to use it

- Operate **Getting Started** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/getting-started`
- Nav: **Sidebar → Getting Started**

## Operate from the console (UX)

1. Open `/getting-started` after first login.
2. Walk the checklist; items reflect live Atlas/OIDC/PacketWolf/Haven status.
3. Jump from incomplete items into Settings → Integrations or the related center.
4. **Empty / fail:** Checklist stale → refresh; viewer role can read but cannot complete mutating setup.
5. **Success:** Critical integrations green; you can run a workload or open Overview confidently.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Overview](overview.md)
- [Settings](../admin/settings.md)
- [Workloads](../workloads/workloads.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
