# Events

## Purpose

Kubernetes warning/normal events stream for triage.

## When to use it

- Operate **Events** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/events`
- Nav: **Sidebar → Events**

## Operate from the console (UX)

1. Open `/events`.
2. Filter by type/namespace; open related object links when present.
3. **Empty / fail:** Quiet cluster is OK; missing events → API watch issue.
4. **Success:** Recent warnings explain Pending/Failed pods.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Pods](../workloads/pods.md)
- [Alerts](alerts.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
