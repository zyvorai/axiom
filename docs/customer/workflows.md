# Common workflows

Each recipe assumes you can reach `https://<host>:30590/` and have signed in.

## Verify a fresh deploy

1. Deploy: `./scripts/deploy-remote.sh <host> <user>` ([Getting Started](getting-started.md)).
2. `curl -k https://<host>:30590/api/v1/health`.
3. Sign in (`demo` / `demo` when SSO is off).
4. [Overview](pages/console/overview.md) → metrics load.
5. [Getting Started](pages/console/getting-started.md) checklist.
6. [Nodes](pages/cluster/nodes.md) → Ready nodes.
7. [Settings](pages/admin/settings.md) → integrations.

If NodePort conflicts: `AXIOM_NODE_PORT=30592 ./scripts/deploy-remote.sh <host> <user> --quick`.

## Connect Atlas

```bash
./scripts/deploy-remote.sh <host> <user> --with-atlas
```

Or Settings → Integrations → Atlas → Save → Test → [Atlas](pages/storage/atlas.md) → **Create volume**.

## Connect PacketWolf

Set `AXIOM_PACKETWOLF_URL` (and key) → [Networking](pages/networking/networking.md) live flow panel → seal a workload to confirm isolation.

## Run and scale a workload

1. [Workloads](pages/workloads/workloads.md) → **Run workload** (Form / Intent / YAML).
2. Open workload detail → owned Pods.
3. Scale / Restart from the row menu.
4. [Pod detail](pages/workloads/pods-namespace-name.md) → Logs / Terminal.

## Backup a PVC

1. [Backups](pages/protect/backups.md) → **Create snapshot**.
2. Restore to a new PVC name when rehearsing DR.

## Related

- [Using the Dashboard](using-the-dashboard.md)
- [Admin basics](admin-basics.md)
