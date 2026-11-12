#!/usr/bin/env bash
# Validates Axiom Velero create/list against REAL Velero CRDs on a cluster.
# Skips cleanly when backups.velero.io is not installed.
#
# Usage (on a host with kubectl + Axiom reachable):
#   AXIOM_BASE=https://localhost:30590 AXIOM_USER=admin AXIOM_PASS=... ./e2e/scripts/validate-velero.sh
set -u
BASE="${AXIOM_BASE:-https://localhost:30590}"
AUTH="${AXIOM_USER:-}:${AXIOM_PASS:-}"
NS="${VELERO_NS:-velero}"
PASS=0
FAIL=0
SKIP=0

req() {
  local method=$1 path=$2 data=${3:-}
  if [ -n "$data" ]; then
    curl -sk ${AUTH:+-u "$AUTH"} -X "$method" -H 'Content-Type: application/json' -d "$data" -w '\n%{http_code}' "$BASE$path"
  else
    curl -sk ${AUTH:+-u "$AUTH"} -X "$method" -w '\n%{http_code}' "$BASE$path"
  fi
}

check() {
  local label=$1 expect_re=$2 resp=$3
  local code body
  code=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | sed '$d')
  if [[ "$code" =~ $expect_re ]]; then
    echo "PASS  $label  (HTTP $code)"
    PASS=$((PASS+1))
  else
    echo "FAIL  $label  (HTTP $code) body=$body"
    FAIL=$((FAIL+1))
  fi
}

if ! kubectl get crd backups.velero.io >/dev/null 2>&1; then
  echo "SKIP  backups.velero.io CRD not installed — install Velero first"
  SKIP=$((SKIP+1))
  echo "===== SUMMARY: $PASS passed, $FAIL failed, $SKIP skipped ====="
  exit 0
fi

NAME="axiom-e2e-$(date +%s)"
echo "== Velero Backup create/list against live CRDs (ns=$NS name=$NAME) =="
r=$(req POST /api/v1/velero/backups "{\"namespace\":\"$NS\",\"name\":\"$NAME\",\"includedNamespaces\":[\"default\"],\"ttl\":\"24h0m0s\",\"storageLocation\":\"default\"}")
check "create backup" '^(200|201)$' "$r"
r=$(req GET "/api/v1/velero/backups?namespace=$NS")
check "list backups" '^200$' "$r"
echo "$r" | sed '$d' | grep -q "$NAME" && echo "PASS  backup visible in list" && PASS=$((PASS+1)) || { echo "FAIL  backup NOT visible"; FAIL=$((FAIL+1)); }

# Confirm the object the API wrote is accepted by the live CRD (kubectl get succeeds).
if kubectl -n "$NS" get backup.velero.io "$NAME" >/dev/null 2>&1; then
  echo "PASS  kubectl get backup.velero.io/$NAME"
  PASS=$((PASS+1))
else
  echo "FAIL  kubectl cannot get backup.velero.io/$NAME"
  FAIL=$((FAIL+1))
fi

SCHED="axiom-e2e-sched-$(date +%s)"
r=$(req POST /api/v1/velero/schedules "{\"namespace\":\"$NS\",\"name\":\"$SCHED\",\"schedule\":\"0 3 * * *\",\"ttl\":\"72h\"}")
check "create schedule" '^(200|201)$' "$r"
if kubectl -n "$NS" get schedule.velero.io "$SCHED" >/dev/null 2>&1; then
  echo "PASS  kubectl get schedule.velero.io/$SCHED"
  PASS=$((PASS+1))
  kubectl -n "$NS" delete schedule.velero.io "$SCHED" --wait=false >/dev/null 2>&1 || true
else
  echo "FAIL  kubectl cannot get schedule.velero.io/$SCHED"
  FAIL=$((FAIL+1))
fi

kubectl -n "$NS" delete backup.velero.io "$NAME" --wait=false >/dev/null 2>&1 || true

echo
echo "===== SUMMARY: $PASS passed, $FAIL failed, $SKIP skipped ====="
[ "$FAIL" -eq 0 ]
