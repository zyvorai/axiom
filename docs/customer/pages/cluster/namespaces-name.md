# Namespace detail

## Purpose

Inspect what is running inside a single namespace — pods, workloads, and services — with drill-down into each resource.

## When to use it

- After opening **Namespaces**, click a card to see inventory for that tenancy boundary
- Prefer the cluster-wide Workloads / Pods / Services pages when comparing across namespaces

## How to get there

- Route: `/namespaces/{name}`
- From: **Sidebar → Namespaces** → click a namespace card
- Command palette: search the namespace name

## Operate from the console (UX)

1. Open `/namespaces/{name}`.
2. Use **Pods**, **Workloads**, and **Services** tabs.
3. Click a pod, workload, or service row to open its detail page.
4. **Empty / fail:** Empty tabs mean nothing is deployed in that namespace; 404 means the namespace was deleted.
5. **Success:** Counts and rows match `kubectl -n {name} get pods,deploy,svc`.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Namespaces](namespaces.md)
- [Workloads](../workloads/workloads.md)
- [Pods](../workloads/pods.md)
- [Services](../networking/services.md)
- [Page index](../../PAGE_INDEX.md)
