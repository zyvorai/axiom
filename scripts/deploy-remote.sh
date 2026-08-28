#!/usr/bin/env bash
# ============================================================================
# deploy-remote.sh — Deploy Zyvor Axiom to a remote k3s host (HTTPS only)
# ============================================================================
#   1. Rsync repo to remote ~/.deployment/axiom
#   2. Generate/reuse self-signed TLS cert
#   3. Build zyvor-axiom with podman
#   4. Import image into k3s
#   5. Apply Kubernetes manifests + NodePort
#   6. Verify HTTPS health (curl -k)
#   7. --with-atlas: also deploy ../atlas to the same host (its own scripts/deploy-remote.sh),
#      mint an admin JWT from its bootstrap-admin-token, and PUT it into Axiom's own
#      Settings → Integrations (atlasUrl/atlasToken) so Atlas shows up wired on the Atlas page.
#
# Usage:
#   ./scripts/deploy-remote.sh <host> [user] [password]
#   ./scripts/deploy-remote.sh 80.79.5.173 zyvorai
#   ./scripts/deploy-remote.sh 80.79.5.173 zyvorai --quick        # skip image rebuild
#   ./scripts/deploy-remote.sh 80.79.5.173 zyvorai --skip-images  # manifests only
#   ./scripts/deploy-remote.sh 80.79.5.173 zyvorai --with-atlas   # also deploy + wire ../atlas
#   ./scripts/deploy-remote.sh 80.79.5.173 zyvorai --uninstall
#
# Environment:
#   DEPLOY_HOST / DEPLOY_USER / DEPLOY_PASS
#   DEPLOY_DIR                 absolute remote checkout (overrides layout)
#   DEPLOYMENTS_SUBDIR         default: .deployment
#   AXIOM_CHECKOUT             default: axiom
#   AXIOM_IMAGE_TAG            default: dev
#   AXIOM_NODE_PORT            default: 30590
#   ATLAS_REPO                 path to the atlas checkout (default: ../atlas next to this repo)
#   ATLAS_NODE_PORT            default: 30510 (must match ../atlas/deploy/k8s/atlas-gateway.yaml)
#   AXIOM_USERNAME             default: demo
#   AXIOM_PASSWORD             default: demo
#
# Remote deploy is HTTPS-only (self-signed). Plain HTTP is not supported.
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/deploy-remote-ui.sh
source "${SCRIPT_DIR}/lib/deploy-remote-ui.sh"
AXIOM_DEPLOY_START=${SECONDS}

QUICK_MODE=false
UNINSTALL_MODE=false
SKIP_IMAGES=false
SKIP_SYNC=false
DRY_RUN=false
WITH_ATLAS=false
POSITIONAL=()
for arg in "$@"; do
  case "$arg" in
    --quick)       QUICK_MODE=true; SKIP_IMAGES=true ;;
    --skip-images) SKIP_IMAGES=true ;;
    --skip-sync)   SKIP_SYNC=true ;;
    --uninstall)   UNINSTALL_MODE=true ;;
    --dry-run)     DRY_RUN=true ;;
    --with-atlas)  WITH_ATLAS=true ;;
    --help|-h)
      sed -n '2,32p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) POSITIONAL+=("$arg") ;;
  esac
done

HOST="${POSITIONAL[0]:-${DEPLOY_HOST:-}}"
USER="${POSITIONAL[1]:-${DEPLOY_USER:-zyvorai}}"
PASS="${POSITIONAL[2]:-${DEPLOY_PASS:-}}"

if [[ -z "$HOST" ]] && axiom_load_deploy_last "$(cd "$SCRIPT_DIR/.." && pwd)"; then
  info "Using last deploy target from .deploy-last: ${USER}@${HOST}"
fi

[ -n "$HOST" ] || error "Usage: $0 <host> [user] [password] [--quick|--skip-images|--uninstall]"

DEPLOYMENTS_SUBDIR="${DEPLOYMENTS_SUBDIR:-.deployment}"
AXIOM_CHECKOUT="${AXIOM_CHECKOUT:-axiom}"
AXIOM_IMAGE_TAG="${AXIOM_IMAGE_TAG:-dev}"
NODE_PORT="${AXIOM_NODE_PORT:-30590}"
AXIOM_USERNAME="${AXIOM_USERNAME:-demo}"
AXIOM_PASSWORD="${AXIOM_PASSWORD:-demo}"

REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
[ -f "$REPO_DIR/Dockerfile" ] || error "Not in axiom repo: $REPO_DIR"
[ -f "$REPO_DIR/deploy/k8s/deployment.yaml" ] || error "Missing deploy/k8s/deployment.yaml"

if [ -n "${DEPLOY_DIR:-}" ]; then
  REMOTE_DIR="$DEPLOY_DIR"
  RSYNC_DEST="${USER}@${HOST}:${DEPLOY_DIR}/"
  REMOTE_RM_TARGET="$(printf '%q' "$DEPLOY_DIR")"
else
  REMOTE_DIR="\$HOME/${DEPLOYMENTS_SUBDIR}/${AXIOM_CHECKOUT}"
  RSYNC_DEST="${USER}@${HOST}:${DEPLOYMENTS_SUBDIR}/${AXIOM_CHECKOUT}/"
  REMOTE_RM_TARGET="\"\$HOME/${DEPLOYMENTS_SUBDIR}/${AXIOM_CHECKOUT}\""
fi

REMOTE_SH_PREFIX="cd \"${REMOTE_DIR}\""

_SSH_KEEPALIVE_OPTS="-o ServerAliveInterval=15 -o ServerAliveCountMax=6"
_KUBECONFIG_PREFIX='if [ -r "$HOME/.kube/config" ]; then export KUBECONFIG="$HOME/.kube/config"; fi;'

_ssh() {
  if [ -n "$PASS" ]; then
    SSHPASS="$PASS" sshpass -e ssh -o StrictHostKeyChecking=accept-new ${_SSH_KEEPALIVE_OPTS} "${USER}@${HOST}" "$_KUBECONFIG_PREFIX" "$@"
  else
    ssh -o StrictHostKeyChecking=accept-new ${_SSH_KEEPALIVE_OPTS} "${USER}@${HOST}" "$_KUBECONFIG_PREFIX" "$@"
  fi
}

_rsync() {
  local ssh_cmd="ssh -o StrictHostKeyChecking=accept-new ${_SSH_KEEPALIVE_OPTS}"
  if [ -n "$PASS" ]; then
    SSHPASS="$PASS" rsync -az --delete \
      -e "sshpass -e $ssh_cmd" \
      --exclude '.git/' \
      --exclude 'bin/' \
      --exclude 'node_modules/' \
      "$@"
  else
    rsync -az --delete \
      -e "$ssh_cmd" \
      --exclude '.git/' \
      --exclude 'bin/' \
      --exclude 'node_modules/' \
      "$@"
  fi
}

if [ -n "$PASS" ] && ! command -v sshpass &>/dev/null; then
  error "sshpass required for password auth (brew install sshpass / dnf install sshpass)"
fi

axiom_ui_set_total 7
axiom_ui_banner "Zyvor Axiom Remote Deploy" "${USER}@${HOST}"
axiom_ui_kv "Checkout" "${DEPLOYMENTS_SUBDIR}/${AXIOM_CHECKOUT}"
axiom_ui_kv "Image tag" "${AXIOM_IMAGE_TAG}"
axiom_ui_kv "HTTPS" "https://${HOST}:${NODE_PORT} (self-signed)"
axiom_ui_kv "Mode" "$($UNINSTALL_MODE && echo uninstall || ($SKIP_IMAGES && echo skip-images || echo full))"

if $DRY_RUN; then
  warn "Dry run — no changes will be made"
  axiom_ui_panel "Would run" \
    "Rsync → ${REMOTE_DIR}" \
    "Generate/reuse self-signed TLS → Secret zyvor-axiom-tls" \
    "Build zyvor-axiom:${AXIOM_IMAGE_TAG} (unless --skip-images)" \
    "kubectl apply deploy/k8s/*" \
    "Verify https://${HOST}:${NODE_PORT}/api/v1/health (curl -k)"
  exit 0
fi

# ── Uninstall ──
if $UNINSTALL_MODE; then
  step 1 2 "Remove Axiom from cluster"
  _ssh "${REMOTE_SH_PREFIX}; kubectl delete -f deploy/k8s/nodeport.yaml --ignore-not-found; kubectl delete -f deploy/k8s/deployment.yaml --ignore-not-found; kubectl delete -f deploy/k8s/rbac.yaml --ignore-not-found; kubectl -n axiom-system delete secret zyvor-axiom-tls zyvor-axiom-auth --ignore-not-found; kubectl delete ns axiom-system --ignore-not-found" || true
  info "Namespace/resources deleted"
  step 2 2 "Remote checkout kept"
  warn "Leaving ${REMOTE_DIR} in place (remove: ssh ${USER}@${HOST} rm -rf ${REMOTE_RM_TARGET})"
  axiom_ui_success "$HOST" "$USER"
  exit 0
fi

# ── Preflight ──
step 1 7 "SSH preflight"
REMOTE_UNAME=$(_ssh 'uname -sr 2>/dev/null || echo unknown' | tr -d '\r' | head -1)
info "Connected: ${REMOTE_UNAME:-ok}"
if ! _ssh 'command -v kubectl >/dev/null'; then
  error "kubectl not found on ${HOST}"
fi
info "kubectl present"

# ── Sync ──
step 2 7 "Rsync source → remote"
if $SKIP_SYNC; then
  warn "Skipping rsync (--skip-sync)"
else
  _ssh "mkdir -p \"\$HOME/${DEPLOYMENTS_SUBDIR}/${AXIOM_CHECKOUT}\""
  _rsync "${REPO_DIR}/" "${RSYNC_DEST}"
  info "Synced to ${REMOTE_DIR}"
fi

# ── TLS (self-signed, reused across redeploys) ──
step 3 7 "Self-signed TLS certificate"
_ssh "export AXIOM_TLS_HOST='${HOST}'; ${REMOTE_SH_PREFIX}; \
mkdir -p .run/tls
if [ ! -f .run/tls/tls.crt ] || [ ! -f .run/tls/tls.key ]; then
  openssl req -x509 -newkey rsa:2048 -sha256 -days 3650 -nodes \
    -keyout .run/tls/tls.key -out .run/tls/tls.crt \
    -subj \"/CN=\${AXIOM_TLS_HOST}/O=Zyvor Axiom\" \
    -addext \"subjectAltName=DNS:\${AXIOM_TLS_HOST},IP:\${AXIOM_TLS_HOST}\" \
    2>&1 | grep -v '^writing\|^Generating' || true
  echo 'Generated self-signed TLS cert (.run/tls/tls.crt, 3650d)'
else
  echo 'Reusing existing TLS cert (.run/tls/tls.crt)'
fi
kubectl create namespace axiom-system --dry-run=client -o yaml | kubectl apply -f -
kubectl -n axiom-system create secret tls zyvor-axiom-tls \
  --cert=.run/tls/tls.crt --key=.run/tls/tls.key \
  --dry-run=client -o yaml | kubectl apply -f -"
info "TLS secret zyvor-axiom-tls applied"

# ── Auth secret ──
step 4 7 "Configure auth secret"
_ssh "${REMOTE_SH_PREFIX}; cat > deploy/k8s/secret.yaml <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: zyvor-axiom-auth
  namespace: axiom-system
type: Opaque
stringData:
  username: ${AXIOM_USERNAME}
  password: ${AXIOM_PASSWORD}
EOF
kubectl apply -f deploy/k8s/secret.yaml"
info "Secret zyvor-axiom-auth applied (user=${AXIOM_USERNAME})"

# ── Images ──
step 5 7 "Build + import container image"
if $SKIP_IMAGES; then
  warn "Skipping image builds (--quick / --skip-images)"
else
  _ssh "export AXIOM_IMAGE_TAG='${AXIOM_IMAGE_TAG}'; ${REMOTE_SH_PREFIX}; . scripts/lib/deploy-remote-images.sh; axiom_build_all_images"
  info "Image imported into k3s"
fi

# ── Apply manifests ──
step 6 7 "Apply Kubernetes manifests"
_ssh "export AXIOM_IMAGE_TAG='${AXIOM_IMAGE_TAG}' AXIOM_NODE_PORT='${NODE_PORT}'; ${REMOTE_SH_PREFIX}; \
python3 - <<'PY'
from pathlib import Path
import os, re

tag = os.environ['AXIOM_IMAGE_TAG']
node_port = os.environ['AXIOM_NODE_PORT']

dep = Path('deploy/k8s/deployment.yaml')
text = dep.read_text()
text = re.sub(r'image: zyvor-axiom:[^\s]+', f'image: zyvor-axiom:{tag}', text)
dep.write_text(text)

np = Path('deploy/k8s/nodeport.yaml')
np_text = np.read_text()
np_text = re.sub(r'nodePort: [0-9]+', f'nodePort: {node_port}', np_text)
np.write_text(np_text)
print(f'deployment image -> zyvor-axiom:{tag}')
print(f'nodeport -> {node_port}')
PY
kubectl apply -f deploy/k8s/rbac.yaml
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/secret.yaml
if ! kubectl apply -f deploy/k8s/nodeport.yaml; then
  echo \"ERROR: nodePort \$AXIOM_NODE_PORT is already allocated to another Service on this cluster.\" >&2
  echo \"Find it with: kubectl get svc -A -o wide | grep \$AXIOM_NODE_PORT\" >&2
  echo \"Re-run with a free port: AXIOM_NODE_PORT=<port> $0 ${HOST} ${USER} --quick\" >&2
  exit 1
fi
LIVE_PORT=\$(kubectl -n axiom-system get svc zyvor-axiom-nodeport -o jsonpath='{.spec.ports[0].nodePort}')
if [ \"\$LIVE_PORT\" != \"\$AXIOM_NODE_PORT\" ]; then
  echo \"ERROR: zyvor-axiom-nodeport is live on \$LIVE_PORT, not the requested \$AXIOM_NODE_PORT — the apply was rejected (nodePort already allocated elsewhere). Re-run with a free port: AXIOM_NODE_PORT=<port> $0 ${HOST} ${USER} --quick\" >&2
  exit 1
fi
# Optional env bootstrap only — prefer Settings → Integrations in the UX.
# (Atlas/OIDC URLs and tokens are persisted to zyvor-axiom-integrations.)
true
kubectl -n axiom-system rollout restart deploy/zyvor-axiom || true
kubectl -n axiom-system rollout status deploy/zyvor-axiom --timeout=300s"
info "Workloads rolled out"

# ── Firewall (best effort) ──
_ssh "sudo ufw allow ${NODE_PORT}/tcp comment zyvor-axiom 2>/dev/null || true; \
  if command -v firewall-cmd &>/dev/null; then sudo firewall-cmd --permanent --add-port=${NODE_PORT}/tcp 2>/dev/null && sudo firewall-cmd --reload 2>/dev/null || true; fi; \
  true" || true

# ── Verify ──
step 7 7 "Verify HTTPS health"
sleep 3
BASE_URL="https://${HOST}:${NODE_PORT}"
if curl -fsSk --connect-timeout 10 "${BASE_URL}/api/v1/health" >/dev/null; then
  info "Health OK: ${BASE_URL}/api/v1/health"
else
  warn "Health check failed at ${BASE_URL}/api/v1/health — check: kubectl -n axiom-system get pods,svc"
  _ssh 'kubectl -n axiom-system get pods,svc -o wide; kubectl -n axiom-system describe pods -l app=zyvor-axiom 2>/dev/null | tail -40' || true
fi

# ── Atlas (optional) ──
ATLAS_WIRED=0
if $WITH_ATLAS; then
  ATLAS_NODE_PORT="${ATLAS_NODE_PORT:-30510}"
  ATLAS_REPO="${ATLAS_REPO:-$(cd "$REPO_DIR/../atlas" 2>/dev/null && pwd || true)}"
  if [ -z "$ATLAS_REPO" ] || [ ! -f "$ATLAS_REPO/scripts/deploy-remote.sh" ]; then
    warn "--with-atlas given but no atlas checkout found at ../atlas (set ATLAS_REPO=/path/to/atlas) — skipping"
  else
    info "Deploying Atlas to ${HOST} (${ATLAS_REPO})"
    ( cd "$ATLAS_REPO" && ./scripts/deploy-remote.sh "$HOST" "$USER" )

    ATLAS_URL="http://${HOST}:${ATLAS_NODE_PORT}"
    info "Minting an Axiom service token from Atlas's bootstrap-admin-token"
    BOOT_TOKEN=$(_ssh "kubectl -n zyvor-system get secret atlas-gateway-auth -o jsonpath='{.data.bootstrap-admin-token}' 2>/dev/null | base64 -d" || true)
    if [ -z "$BOOT_TOKEN" ]; then
      warn "No bootstrap-admin-token on atlas-gateway-auth (already rotated out?) — wire Atlas manually in Settings → Integrations: ${ATLAS_URL}"
    else
      ATLAS_JWT=$(curl -fsS --connect-timeout 10 \
        -H "Authorization: Bearer ${BOOT_TOKEN}" -H 'Content-Type: application/json' \
        -d '{"subject":"axiom","role":"admin","ttl_secs":7776000}' \
        "${ATLAS_URL}/api/atlas/v1/auth/tokens" 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))' 2>/dev/null || true)
      if [ -z "$ATLAS_JWT" ]; then
        warn "Could not mint an Atlas token — wire Atlas manually in Settings → Integrations: ${ATLAS_URL}"
      else
        AXIOM_AUTH_B64=$(printf '%s:%s' "$AXIOM_USERNAME" "$AXIOM_PASSWORD" | base64 | tr -d '\n')
        if curl -fsSk --connect-timeout 10 -X PUT \
            -H "Authorization: Basic ${AXIOM_AUTH_B64}" -H 'Content-Type: application/json' \
            -d "{\"atlasUrl\":\"${ATLAS_URL}\",\"atlasToken\":\"${ATLAS_JWT}\"}" \
            "${BASE_URL}/api/v1/integrations" >/dev/null 2>&1; then
          info "Axiom ↔ Atlas wired: ${ATLAS_URL} (token expires in 90d, mints a fresh one on redeploy)"
          ATLAS_WIRED=1
        else
          warn "Atlas token minted but PUT ${BASE_URL}/api/v1/integrations failed — wire manually in Settings → Integrations: ${ATLAS_URL}"
        fi
      fi
    fi
  fi
fi

MODE_LABEL=$($SKIP_IMAGES && echo quick || echo full)
axiom_save_deploy_last "$REPO_DIR" "$HOST" "$USER" "$MODE_LABEL" "$AXIOM_IMAGE_TAG"

axiom_ui_success "$HOST" "$USER"
PANEL_LINES=(
  "UI:      ${BASE_URL}"
  "Health:  ${BASE_URL}/api/v1/health"
  "Login:   ${AXIOM_USERNAME} / ${AXIOM_PASSWORD}"
  "curl:    curl -k ${BASE_URL}/api/v1/health"
)
if [ "${ATLAS_WIRED:-0}" = "1" ]; then
  PANEL_LINES+=("Atlas:   ${ATLAS_URL} (wired into Settings → Integrations)")
fi
PANEL_LINES+=(
  ""
  "Accept the browser certificate warning once."
  "Redeploy fast: ./scripts/deploy-remote.sh ${HOST} ${USER} --quick"
)
axiom_ui_panel "Access (HTTPS only, self-signed)" "${PANEL_LINES[@]}"
