# Backing up and restoring Axiom's own state

This covers Axiom's *own* configuration — not the cluster it manages
(your workloads, PVCs, etc. are covered by your existing backup story,
e.g. Velero, which Axiom's Backups page already surfaces).

If Axiom's namespace is lost, the Deployment/Service/RBAC come back with
one `helm install` or `kubectl apply`. What doesn't come back on its own
is anything an operator configured *through Axiom's own UI* after that —
local accounts, integration settings, and the audit trail.

## What's stored, and where

| What | Resource | Kind | Notes |
|---|---|---|---|
| Local accounts (Settings → Users) | `zyvor-axiom-users` | Secret | bcrypt-hashed passwords |
| Integration settings (Settings → Integrations: OIDC, Atlas, Postgres, probe) | `zyvor-axiom-integrations` | ConfigMap + Secret | non-secret fields in the ConfigMap, tokens/passwords in the Secret — same name, both kinds |
| Audit log (Ops → Audit) | `zyvor-axiom-audit` | ConfigMap | only the most recent 200 entries — forward to your SIEM for anything longer-lived, see `docs/DEPLOYMENT.md` |
| Operator credential | `zyvor-axiom-auth` | Secret | normally reproducible from your Helm `values.yaml`/deploy command, not runtime state — back up only if it was changed outside that |
| TLS certificate | `zyvor-axiom-tls` | Secret | self-signed certs regenerate automatically (and the Helm chart reuses the existing one across upgrades); a CA-issued cert should already be tracked in your own secret management, not just here |

## Back up

```bash
./scripts/backup-axiom-state.sh > axiom-state-backup.yaml
```

This is a thin wrapper around `kubectl get -o yaml` for the resources
above — run it on whatever cadence matches how often these settings
change (there's no live data loss risk from running it too often; it's
a read-only snapshot). Store the output like you would any other
credential-bearing backup — it contains bcrypt hashes and any
integration tokens configured through Settings.

## Restore

```bash
./scripts/restore-axiom-state.sh axiom-state-backup.yaml
```

This `kubectl apply`s the snapshot back into the target namespace. Restore
into a namespace that already has Axiom's Deployment running — restoring
state alone into an empty namespace won't bring the application up by
itself; install the chart first (see `docs/QUICKSTART.md`), then restore
state, then restart the Deployment so it picks up the restored ConfigMaps/
Secrets:

```bash
kubectl -n axiom-system rollout restart deploy/zyvor-axiom
```

## What this does *not* cover

- The cluster Axiom manages (your workloads, PVCs, VolumeSnapshots) —
  that's your existing backup tooling's job, which Axiom's own Backups
  page already surfaces (Velero schedules, VolumeSnapshots).
- Anything configured purely via environment variables on the Deployment
  (e.g. `AXIOM_OIDC_ISSUER` set at install time rather than through
  Settings → Integrations) — that lives in your Helm `values.yaml` or
  deployment manifest, which should already be under your own version
  control.
