#!/usr/bin/env bash
# Restore Axiom's own runtime state from a snapshot taken by
# scripts/backup-axiom-state.sh — see docs/BACKUP-RESTORE.md.
#
# Usage:
#   ./scripts/restore-axiom-state.sh <backup-file.yaml> [namespace]
#
# Restore into a namespace where Axiom's Deployment already exists (install
# the chart first — see docs/QUICKSTART.md), then restart it to pick up the
# restored ConfigMaps/Secrets:
#   kubectl -n axiom-system rollout restart deploy/zyvor-axiom
set -euo pipefail

BACKUP_FILE="${1:?Usage: $0 <backup-file.yaml> [namespace]}"
NAMESPACE="${2:-${NAMESPACE:-axiom-system}}"

[ -f "$BACKUP_FILE" ] || { echo "Backup file not found: $BACKUP_FILE" >&2; exit 1; }

kubectl -n "$NAMESPACE" apply -f "$BACKUP_FILE"

echo "Restored. Now restart Axiom so it picks up the restored state:" >&2
echo "  kubectl -n $NAMESPACE rollout restart deploy/zyvor-axiom" >&2
