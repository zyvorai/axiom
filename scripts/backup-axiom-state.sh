#!/usr/bin/env bash
# Snapshot Axiom's own runtime state (local accounts, integration settings,
# audit log) as a single YAML stream on stdout — see docs/BACKUP-RESTORE.md.
#
# Usage:
#   ./scripts/backup-axiom-state.sh [namespace] > axiom-state-backup.yaml
#
# Default namespace: axiom-system (override via NAMESPACE env or arg).
set -euo pipefail

NAMESPACE="${1:-${NAMESPACE:-axiom-system}}"

RESOURCES=(
  "secret/zyvor-axiom-users"
  "configmap/zyvor-axiom-integrations"
  "secret/zyvor-axiom-integrations"
  "configmap/zyvor-axiom-audit"
)

for res in "${RESOURCES[@]}"; do
  if kubectl -n "$NAMESPACE" get "$res" >/dev/null 2>&1; then
    kubectl -n "$NAMESPACE" get "$res" -o yaml
    echo "---"
  else
    echo "# $res not found in namespace $NAMESPACE (never configured — nothing to back up)" >&2
  fi
done
