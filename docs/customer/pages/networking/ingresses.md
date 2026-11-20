# Ingresses

## Purpose

Ingress (legacy) and Gateway API exposure inventory. **Envoy Gateway** is the preferred north-south path (Gateway API + Envoy) as Ingress NGINX retires.

## When to use it

- Prefer **Gateway API / Envoy Gateway** for new exposures (gRPC/HTTP observability, HTTP/3-capable data plane)
- Use legacy **Ingress** rows only while migrating off Ingress NGINX
- Prefer **Overview** after login if you are unsure where to start

## How to get there

- Route: `/ingresses`
- Nav: **Sidebar → Ingresses**

## Operate from the console (UX)

1. Open `/ingresses`.
2. Check the **Envoy Gateway** banner (detected / not installed).
3. Review **Gateway API** (GatewayClass / Gateway / HTTPRoute) and legacy Ingress tables.
4. **Empty Gateway API:** install Envoy Gateway and apply `deploy/k8s/envoy-gateway-example.yaml` (or Helm `expose.type=Gateway`).
5. **Success:** Envoy provider shows on GatewayClass/Gateway; hosts match DNS.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Services](services.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
