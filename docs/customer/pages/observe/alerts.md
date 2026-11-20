# Alerts

## Purpose

Alert inventory and webhook-driven notifications.

## When to use it

- Operate **Alerts** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/alerts`
- Nav: **Sidebar → Alerts**

## Operate from the console (UX)

1. Open `/alerts`.
2. Review firing/open alerts; configure webhooks under Settings when available.
3. **Empty / fail:** No alerts → healthy or integrations not wired.
4. **Success:** Alert rows match monitoring rules / webhooks.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Events](events.md)
- [Settings](../admin/settings.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
