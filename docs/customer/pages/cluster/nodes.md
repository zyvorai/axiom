# Nodes

## Purpose

Kubernetes node inventory — Ready state, capacity, and entry into node detail.

## When to use it

- Operate **Nodes** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/nodes`
- Nav: **Sidebar → Nodes**

## Operate from the console (UX)

1. Open `/nodes` and wait for the table.
2. Filter/search; open a row → `/nodes/:name` for taints, conditions, capacity.
3. On detail: **Add taint** when draining or isolating capacity.
4. **Empty / fail:** No nodes → cluster API / RBAC; demo mode should still show fixtures.
5. **Success:** Ready nodes listed; detail matches cluster state.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Node detail](nodes-name.md)
- [Events](../observe/events.md)
- [Pods](../workloads/pods.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
