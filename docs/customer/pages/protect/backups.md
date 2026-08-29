# Backups

## Purpose

Volume snapshots, Velero backups, and schedules.

## When to use it

- Operate **Backups** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/backups`
- Nav: **Sidebar → Backups**

## Operate from the console (UX)

1. Open `/backups`.
2. **Create snapshot** for a PVC; **Restore** into a new PVC name when offered.
3. Velero: create schedules / list backups when Velero is installed.
4. **Empty / fail:** Snapshot class missing → install CSI snapshotter; Velero empty → install Velero.
5. **Success:** Snapshot/backup row appears; restore binds a PVC.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Storage](../storage/storage.md)
- [Atlas](../storage/atlas.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
