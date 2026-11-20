# postgres-lab (Axiom)

Minimal Postgres for **Settings → Integrations → Postgres → Test connection**.

## Deploy

```bash
./scripts/postgres-lab-up.sh                 # local kubectl
./scripts/postgres-lab-up.sh <host> <user>   # remote k3s via SSH
```

Creates Secret `postgres-lab-auth` in `axiom-system`, applies this manifest, waits for Ready.

Service DNS (from Axiom pod):

```text
postgres-lab.axiom-system.svc:5432
```

Settings:

| Field | Value |
|-------|--------|
| Host | `postgres-lab.axiom-system.svc` |
| Port | `5432` |
| Database | `axiom` |
| User | `axiom` |
| Password | from secret `postgres-lab-auth` / key `password` |
| SSL | disable |

## Smoke all integrations

```bash
export AXIOM_URL=https://axiom.example.com   # your Axiom URL (Ingress or NodePort)
export AXIOM_USER=admin
export AXIOM_PASS='…'
# Optional — set only what you want to configure/test:
# export ATLAS_URL=… ATLAS_CONSOLE_URL=… ATLAS_TOKEN=… PROBE_URL=…
# export OIDC_ISSUER=… OIDC_LOGIN_URL=…
# export LAB_HOST=… LAB_USER=…   # to fetch secrets via SSH kubectl
./scripts/smoke-integrations.sh
```

## Teardown

```bash
kubectl -n axiom-system delete deploy/postgres-lab svc/postgres-lab pvc/postgres-lab-data secret/postgres-lab-auth --ignore-not-found
```
