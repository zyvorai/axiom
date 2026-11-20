# Axiom — Customer Documentation

**Axiom** is a private in-house cloud console on **k3s and Kubernetes** — applications, VMs, storage, identity, and networking in one console UI.

| You want to… | Open |
|--------------|------|
| Install and sign in | [Getting Started](getting-started.md) |
| Learn the shell | [Using the Dashboard](using-the-dashboard.md) |
| Screen-by-screen UX | [Page-by-page guides](pages/README.md) |
| Look up any route | [Complete page index](PAGE_INDEX.md) |
| Deploy / auth / ports | [Admin basics](admin-basics.md) |
| Multi-page recipes | [Common workflows](workflows.md) |

## Printable PDFs

```bash
set -a; source scripts/customer-docs/product.env; set +a
node scripts/customer-docs/build-customer-pdfs.mjs
```

Output lands in [`pdf/`](pdf/).

## Product at a glance

```text
  UI / API   →  https://<host>:30590/   (NodePort HTTPS; AXIOM_NODE_PORT to override)
  Local demo →  http://<host>:5090      (AXIOM_MODE=demo)
  Health     →  GET /api/v1/health
  Lab login  →  demo / demo when SSO is off
```

Never publish lab IPs — use `<host>` everywhere.
