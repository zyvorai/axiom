#!/usr/bin/env bash
# ============================================================================
# test-plugin-tier1.sh — Automated end-to-end test for the Tier 1 declarative
# plugin architecture (AxiomPlugin CRD), against a real deployed cluster.
#
# Applies deploy/k8s/plugins/axiomplugin-crd.example.yaml (CRD + example CR),
# creates a throwaway namespace/PVC/VolumeSnapshot to exercise the plugin's
# delete action end-to-end via Axiom's own API, asserts each step, and always
# cleans up afterward (even on failure) so it is safe to re-run.
#
# Usage:
#   ./scripts/test-plugin-tier1.sh <host> [user] [port]
#   ./scripts/test-plugin-tier1.sh 80.79.5.173 sus 30592
#
# Environment:
#   AXIOM_USERNAME / AXIOM_PASSWORD   default: demo / demo
# ============================================================================

set -euo pipefail

HOST="${1:?Usage: $0 <host> [user] [port]}"
SSH_USER="${2:-${DEPLOY_USER:-zyvorai}}"
NODE_PORT="${3:-30592}"
AXIOM_USERNAME="${AXIOM_USERNAME:-demo}"
AXIOM_PASSWORD="${AXIOM_PASSWORD:-demo}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CRD_MANIFEST="$REPO_DIR/deploy/k8s/plugins/axiomplugin-crd.example.yaml"
BASE_URL="https://${HOST}:${NODE_PORT}"

TEST_NS="axiom-plugin-test"
TEST_PVC="axiom-plugin-test-pvc"
TEST_SNAP="axiom-plugin-test-snap"

PASS=0
FAIL=0

ok()   { PASS=$((PASS+1)); echo "  ✓ $1"; }
bad()  { FAIL=$((FAIL+1)); echo "  ✗ $1"; }

_ssh() { ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${HOST}" "$@"; }
_api() { curl -sk -u "${AXIOM_USERNAME}:${AXIOM_PASSWORD}" "$@"; }

cleanup() {
  echo "── Cleanup ──"
  _ssh "kubectl delete namespace ${TEST_NS} --ignore-not-found --wait=false" || true
  _ssh "kubectl delete -f - --ignore-not-found <<'EOF'
$(cat "$CRD_MANIFEST")
EOF" || true
}
trap cleanup EXIT

echo "── Apply AxiomPlugin CRD + example CR ──"
_ssh "cat <<'EOF' | kubectl apply -f -
$(cat "$CRD_MANIFEST")
EOF"
_ssh "kubectl wait --for=condition=Established crd/axiomplugins.axiom.zyvor.dev --timeout=30s"
_ssh "cat <<'EOF' | kubectl apply -f -
$(cat "$CRD_MANIFEST")
EOF"

echo "── Create throwaway namespace/PVC/VolumeSnapshot ──"
_ssh "cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: ${TEST_NS}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${TEST_PVC}
  namespace: ${TEST_NS}
spec:
  accessModes: [\"ReadWriteOnce\"]
  storageClassName: zyvor-rbd-prod
  resources:
    requests:
      storage: 1Gi
EOF"
_ssh "kubectl -n ${TEST_NS} wait --for=jsonpath='{.status.phase}'=Bound pvc/${TEST_PVC} --timeout=30s"

_ssh "cat <<EOF | kubectl apply -f -
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: ${TEST_SNAP}
  namespace: ${TEST_NS}
spec:
  volumeSnapshotClassName: zyvor-rbd-snapclass
  source:
    persistentVolumeClaimName: ${TEST_PVC}
EOF"
_ssh "kubectl -n ${TEST_NS} wait --for=jsonpath='{.status.readyToUse}'=true vs/${TEST_SNAP} --timeout=60s"

echo "── Assertions ──"

plugins_json=$(_api "${BASE_URL}/api/v1/plugins")
if echo "$plugins_json" | grep -q '"volume-snapshots-viewer"'; then
  ok "GET /api/v1/plugins lists volume-snapshots-viewer"
else
  bad "GET /api/v1/plugins did not list volume-snapshots-viewer: $plugins_json"
fi

instances_json=$(_api "${BASE_URL}/api/v1/crds/snapshot.storage.k8s.io/v1/volumesnapshots?namespace=${TEST_NS}")
if echo "$instances_json" | grep -q "\"${TEST_SNAP}\""; then
  ok "GET .../volumesnapshots lists ${TEST_SNAP}"
else
  bad "GET .../volumesnapshots did not list ${TEST_SNAP}: $instances_json"
fi

delete_status=$(_api -o /dev/null -w '%{http_code}' -X DELETE "${BASE_URL}/api/v1/crds/snapshot.storage.k8s.io/v1/volumesnapshots/${TEST_SNAP}?namespace=${TEST_NS}")
if [ "$delete_status" = "200" ] || [ "$delete_status" = "204" ]; then
  ok "DELETE .../volumesnapshots/${TEST_SNAP} returned ${delete_status}"
else
  bad "DELETE .../volumesnapshots/${TEST_SNAP} returned ${delete_status}"
fi

sleep 2
after_json=$(_api "${BASE_URL}/api/v1/crds/snapshot.storage.k8s.io/v1/volumesnapshots?namespace=${TEST_NS}")
if echo "$after_json" | grep -q "\"${TEST_SNAP}\""; then
  bad "${TEST_SNAP} still present after delete: $after_json"
else
  ok "${TEST_SNAP} no longer present after delete"
fi

echo ""
echo "── Result: ${PASS} passed, ${FAIL} failed ──"
[ "$FAIL" -eq 0 ]
