# Node detail

## Purpose

Single-node conditions, capacity, taints, and related workloads.

## When to use it

- Operate **Node detail** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/nodes/:name`
- Nav: **Nodes → row**

## Operate from the console (UX)

1. Open a node from `/nodes`.
2. Review conditions and capacity; add/remove taints as needed.
3. Correlate NotReady with Events and Pods.
4. **Empty / fail:** 404 → stale name; permission errors → ClusterRole.
5. **Success:** Taint/condition changes reflect after refresh.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Nodes](nodes.md)
- [Events](../observe/events.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
