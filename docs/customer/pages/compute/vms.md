# Virtual Machines

## Purpose

KubeVirt VM inventory and day-2 actions in the Axiom console.

## When to use it

- Operate **Virtual Machines** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/vms`
- Nav: **Sidebar → Virtual Machines**

## Operate from the console (UX)

1. Open `/vms`.
2. Filter and open VMs; use lifecycle actions the page exposes (start/stop when available).
3. Confirm KubeVirt is installed if the list stays empty on a real cluster.
4. **Empty / fail:** No VMs → install KubeVirt or use demo fixtures.
5. **Success:** VM phases update after power actions.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Storage](../storage/storage.md)
- [Overview](../console/overview.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
