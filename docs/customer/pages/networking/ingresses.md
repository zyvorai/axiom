# Ingresses

## Purpose

Ingress and Gateway API exposure inventory.

## When to use it

- Operate **Ingresses** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/ingresses`
- Nav: **Sidebar → Ingresses**

## Operate from the console (UX)

1. Open `/ingresses`.
2. Review host/path rules and Gateway API section when present.
3. **Empty / fail:** No ingresses → install ingress controller / apply manifests.
4. **Success:** Hosts listed match customer DNS expectations.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Services](services.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
