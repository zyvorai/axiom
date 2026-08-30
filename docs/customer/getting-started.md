# Getting Started with Axiom

## Prerequisites

| Requirement | Notes |
|-------------|--------|
| k3s / Kubernetes | Or `AXIOM_MODE=demo` for a local deterministic lab |
| Browser | Accept self-signed HTTPS on lab NodePort **30590** |
| Credentials | Lab `demo` / `demo`, operator Secret, local Users, or SSO |

## 1. Open the console

1. Browse to `https://<host>:30590/` (or your ingress). Accept the certificate warning once if needed.
2. Probe health: `curl -k https://<host>:30590/api/v1/health`.
3. If NodePort **30590** is taken, redeploy with `AXIOM_NODE_PORT=30591` (or another free port).

### Lab deploy (remote)

```bash
./scripts/deploy-remote.sh <host> <user>
./scripts/deploy-remote.sh <host> <user> --quick          # skip image rebuild
./scripts/deploy-remote.sh <host> <user> --with-atlas     # optional Atlas on same host
```

### Local demo (no cluster)

Follow product QUICKSTART for `AXIOM_MODE=demo` on port **5090**.

## 2. Sign in

| Account | Credentials | When |
|---------|-------------|------|
| Lab demo | `demo` / `demo` | SSO not configured |
| Operator | `AXIOM_USERNAME` / `AXIOM_PASSWORD` | Secret `zyvor-axiom-auth` |
| Local user | Settings → Users | `admin` or `viewer` |
| SSO | OIDC/SAML | Settings → Integrations |

Use **Lab mode — fill demo** on the login screen's username step when SSO is off — it fills both fields and jumps straight to the password step.

## 3. Orient yourself (UX)

1. **Overview** — cluster pulse and shortcuts.
2. **Getting Started** — live integration checklist.
3. Sidebar + command palette (⌘K) — Workloads, Pods, Networking, Atlas, Ops, Settings.
4. Cluster switcher — real multi-cluster activate; demo mode ships three clusters.
5. Viewer role can read everything but mutating APIs return **403**.

## 4. First useful work

1. **Workloads → Run workload** (Form / Intent / YAML).
2. Open **Pods** / **Pod detail** — logs and terminal.
3. **Settings → Integrations** — wire Atlas or PacketWolf if needed.

## Next steps

- [Using the Dashboard](using-the-dashboard.md)
- [Admin basics](admin-basics.md)
- [Page guides](pages/README.md)
- [Workflows](workflows.md)
