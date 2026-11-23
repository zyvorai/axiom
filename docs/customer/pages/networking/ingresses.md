# Ingresses

## Purpose

Ingress (legacy) and Gateway API exposure inventory. **Setura** is the preferred north-south path (Gateway API + Setura) as Ingress NGINX retires.

## When to use it

- Prefer **Gateway API / Setura** for new exposures (HTTP, gRPC, TLS, TCP and UDP routes on a built-in Go data plane)
- Use legacy **Ingress** rows only while migrating off Ingress NGINX
- Prefer **Overview** after login if you are unsure where to start

## How to get there

- Route: `/ingresses`
- Nav: **Sidebar → Ingresses**

## Operate from the console (UX)

1. Open `/ingresses`.
2. Check the **Setura** banner (detected / not installed).
3. Review **Gateway API** (GatewayClass / Gateway / HTTPRoute) and legacy Ingress tables.
4. **Empty Gateway API:** install Setura (`setura gateway install`) and apply `deploy/k8s/setura-gateway-example.yaml` (or Helm `expose.type=Gateway`).
5. **Success:** Setura provider shows on GatewayClass/Gateway; hosts match DNS.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Services](services.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
