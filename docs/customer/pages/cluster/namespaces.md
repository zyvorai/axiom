# Namespaces

## Purpose

Namespace inventory — create and delete tenancy boundaries. Click a namespace to inspect pods, workloads, and services running inside it.

## When to use it

- Operate **Namespaces** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/namespaces`
- Detail: `/namespaces/{name}`
- Nav: **Sidebar → Namespaces**

## Operate from the console (UX)

1. Open `/namespaces`.
2. Click a namespace card to open its detail page (pods / workloads / services tabs). Rows link through to pod and workload detail.
3. **Create namespace** → name → Create; Delete from the trash icon on a card (confirm).
4. **Empty / fail:** 403 → viewer; delete blocked if resources remain.
5. **Success:** Namespace appears in filters across Workloads/Pods; detail lists match `kubectl -n {name} get pods,deploy,svc`.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Workloads](../workloads/workloads.md)
- [Pods](../workloads/pods.md)
- [Quotas & Limits](../config/quotas.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
