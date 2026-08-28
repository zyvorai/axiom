# Haven (identity plane)

Keycloak + CloudNativePG for Axiom identity. Configure URLs in **Settings → Integrations** (no ports baked into Axiom).

## Typical topology

| Component | Where |
|---|---|
| CNPG `platform-db` | `identity` ns |
| Keycloak | operator namespace (varies by install) |
| Access | Ingress / DNS preferred over NodePort |

Axiom UI: **Ops → Identity**. API: `GET /api/v1/haven/identity`.

### OIDC (Settings or env bootstrap)

```
AXIOM_OIDC_ISSUER=https://auth.example.com/realms/platform
AXIOM_OIDC_LOGIN_URL=https://auth.example.com/realms/platform/protocol/openid-connect/auth
AXIOM_OIDC_CLIENT_ID=axiom
```

Replace `auth.example.com` with your Ingress hostname. Include a port only if your IdP is published that way.

### Postgres (Settings)

```
Host:     platform-db-rw.identity.svc
Port:     5432
Database: platform
User:     platform
Password: (from CNPG app secret)
```
