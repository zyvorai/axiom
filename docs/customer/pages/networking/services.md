# Services

## Purpose

Kubernetes Service inventory and endpoints overview.

## When to use it

- Operate **Services** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/services`
- Nav: **Sidebar → Services**

## Operate from the console (UX)

1. Open `/services`; filter by namespace.
2. Open a row for ports/selectors; correlate with Ingresses and Pods.
3. **Empty / fail:** No services → deploy a workload first.
4. **Success:** ClusterIP/NodePort/LB entries listed.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Ingresses](ingresses.md)
- [Networking · Cilium](networking.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
