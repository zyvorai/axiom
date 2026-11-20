# Admin basics

## Endpoints

| Service | URL |
|---------|-----|
| Axiom UI | `https://<host>:30590/` |
| API health | `curl -k https://<host>:30590/api/v1/health` |
| Session | `curl -k -u demo:demo https://<host>:30590/api/v1/session` |

Namespace: `axiom-system`. Cluster deploys are **HTTPS only**.

If **30590** is allocated, redeploy with `AXIOM_NODE_PORT=30591` (or another free port).

## Ports

| Service | Port | Notes |
|---------|------|------|
| Axiom UI | `30590` | NodePort HTTPS (self-signed lab cert OK) |
| In-cluster | `443` | Service → container `5090` |
| Local demo | `5090` | HTTP OK for `AXIOM_MODE=demo` only |

Port-forward:

```bash
kubectl -n axiom-system port-forward svc/zyvor-axiom 5090:443
curl -k https://localhost:5090/api/v1/health
```

## Secrets and env

| Name | Purpose |
|------|---------|
| `zyvor-axiom-auth` | Operator username/password |
| `zyvor-axiom-tls` | TLS cert/key |
| `zyvor-axiom-users` | Local users JSON (production) |
| `zyvor-axiom-integrations` | Integration config |

| Env | Purpose |
|-----|---------|
| `AXIOM_MODE` | `demo` or `kubernetes` |
| `AXIOM_USERNAME` / `AXIOM_PASSWORD` | Operator Basic auth |
| `AXIOM_TLS_CERT` / `AXIOM_TLS_KEY` | Mounted TLS paths |
| `AXIOM_CLUSTER_NAME` | In-cluster label (default `in-cluster`) |
| `AXIOM_ATLAS_URL` / `AXIOM_ATLAS_TOKEN` | Atlas |
| `AXIOM_PACKETWOLF_URL` / `AXIOM_PACKETWOLF_API_KEY` | PacketWolf |
| `AXIOM_NODE_PORT` | Override NodePort |
| `AXIOM_OIDC_*` | OIDC groups/claims |
| `AXIOM_SESSION_SECRET` | Stable sessions across restarts |
| `AXIOM_TRUSTED_PROXY_HEADER` | Real client IP behind proxies |

Lab login **`demo` / `demo`** when SSO is off.

## Auth model

- Public: `GET /api/v1/health`
- Protected: Basic, OIDC/SAML cookie, or API key
- Viewer: GET allowed; mutating verbs **403**
- WebSocket / port proxy: `?access_token=<base64 user:pass>` when needed

OIDC: register a **public** client (Authorization Code + PKCE). Redirect URI:

`https://<host>:30590/api/v1/auth/oidc/callback`

## Operate from the console (admin)

1. **Settings → Integrations** — Save + Test Atlas/OIDC/PacketWolf.
2. **Users** — create admin/viewer accounts.
3. **Ops** — RBAC roles/bindings; Identity when Haven is present.
4. After restart, re-login; confirm Overview metrics.

## Related

- [Getting Started](getting-started.md)
- [Settings](pages/admin/settings.md)
- [Users](pages/admin/users.md)
