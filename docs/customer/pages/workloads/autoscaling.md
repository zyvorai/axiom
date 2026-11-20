# Autoscaling

## Purpose

Horizontal Pod Autoscalers — create and delete.

## When to use it

- Operate **Autoscaling** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/autoscaling`
- Nav: **Sidebar → Autoscaling**

## Operate from the console (UX)

1. Open `/autoscaling`.
2. **Create autoscaler** — pick workload + min/max/CPU target → Create.
3. Delete retired HPAs from the table.
4. **Empty / fail:** metrics-server missing → HPA cannot compute.
5. **Success:** HPA listed; replicas move under load.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Workloads](workloads.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
