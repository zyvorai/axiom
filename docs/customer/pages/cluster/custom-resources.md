# Custom Resources

## Purpose

Browse CRD kinds and list custom resource instances.

## When to use it

- Operate **Custom Resources** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/custom-resources`
- Nav: **Sidebar → Custom Resources**

## Operate from the console (UX)

1. Open `/custom-resources`.
2. Select a kind; inspect instance table.
3. **Empty / fail:** No CRDs → expected on minimal clusters; install operators as needed.
4. **Success:** Instances for installed operators appear.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Ops hub](../admin/ops.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
