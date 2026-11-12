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
#   ./scripts/deploy-remote.sh <host> zyvorai
#   ./scripts/deploy-remote.sh <host> zyvorai --build        # build the image on the host from source (instead of pulling GHCR)
#   ./scripts/deploy-remote.sh <host> zyvorai --quick        # same as the default (no image build); kept for compatibility
#   ./scripts/deploy-remote.sh <host> zyvorai --skip-images  # manifests only
#   ./scripts/deploy-remote.sh <host> zyvorai --with-atlas   # also deploy + wire ../atlas
#   ./scripts/deploy-remote.sh <host> zyvorai --uninstall
#
# Environment:
#   DEPLOY_HOST / DEPLOY_USER / DEPLOY_PASS
#   DEPLOY_DIR                 absolute remote checkout (overrides layout)
#   DEPLOYMENTS_SUBDIR         default: .deployment
#   AXIOM_CHECKOUT             default: axiom
#   AXIOM_IMAGE                image to run (default: the manifest's ghcr.io/zyvorai/axiom:<version>)
#   AXIOM_IMAGE_TAG            tag for --build local images (default: dev)
#   AXIOM_NODE_PORT            default: 30590
#   ATLAS_REPO                 path to the atlas checkout (default: ../atlas next to this repo)
#   ATLAS_NODE_PORT            default: 30510 (must match ../atlas/deploy/k8s/atlas-gateway.yaml)
#   AXIOM_USERNAME             default: existing Secret if present, else admin
#   AXIOM_PASSWORD             default: existing Secret if present, else Admin@321
#   AXIOM_TLS_MTLS             require|request|off (default require; client CA always minted)
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
SKIP_IMAGES=true   # default: pull the published GHCR image; --build builds locally
BUILD_LOCAL=false
SKIP_SYNC=false
DRY_RUN=false
WITH_ATLAS=false
POSITIONAL=()
for arg in "$@"; do
  case "$arg" in
    --quick)       QUICK_MODE=true; SKIP_IMAGES=true ;;
    --build)       BUILD_LOCAL=true; SKIP_IMAGES=false ;;
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
# Explicit credentials always win; otherwise an existing zyvor-axiom-auth Secret
# on the target is reused (so a redeploy never resets a rotated password);
# only a fresh install falls back to admin/Admin@321.
AUTH_EXPLICIT=false
if [ -n "${AXIOM_USERNAME:-}" ] || [ -n "${AXIOM_PASSWORD:-}" ]; then AUTH_EXPLICIT=true; fi
AUTH_REUSED=false
AXIOM_USERNAME="${AXIOM_USERNAME:-admin}"
AXIOM_PASSWORD="${AXIOM_PASSWORD:-Admin@321}"
AXIOM_TLS_MTLS="${AXIOM_TLS_MTLS:-require}"

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
axiom_ui_kv "Image" "$($BUILD_LOCAL && echo "zyvor-axiom:${AXIOM_IMAGE_TAG} (local build)" || echo "${AXIOM_IMAGE:-GHCR image from deploy/k8s/deployment.yaml}")"
axiom_ui_kv "HTTPS" "https://${HOST}:${NODE_PORT} (self-signed)"
axiom_ui_kv "Mode" "$($UNINSTALL_MODE && echo uninstall || ($SKIP_IMAGES && echo skip-images || echo full))"

if $DRY_RUN; then
  warn "Dry run — no changes will be made"
  axiom_ui_panel "Would run" \
    "Rsync → ${REMOTE_DIR}" \
    "Generate/reuse self-signed TLS → Secret zyvor-axiom-tls" \
    "Pull the GHCR image (or build zyvor-axiom:${AXIOM_IMAGE_TAG} with --build)" \
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

# ── TLS + mTLS client CA (self-signed, reused across redeploys) ──
step 3 7 "Self-signed TLS + mTLS client CA"
_ssh "export AXIOM_TLS_HOST='${HOST}'; ${REMOTE_SH_PREFIX}; \
mkdir -p .run/tls .run/mtls
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
if [ ! -f .run/mtls/ca.crt ] || [ ! -f .run/mtls/ca.key ]; then
  openssl req -x509 -newkey rsa:2048 -sha256 -days 3650 -nodes \
    -keyout .run/mtls/ca.key -out .run/mtls/ca.crt \
    -subj '/CN=Axiom mTLS CA/O=Zyvor Axiom' \
    2>&1 | grep -v '^writing\|^Generating' || true
  echo 'Generated mTLS client CA (.run/mtls/ca.crt)'
else
  echo 'Reusing existing mTLS client CA (.run/mtls/ca.crt)'
fi
if [ ! -f .run/mtls/admin.crt ] || [ ! -f .run/mtls/admin.key ]; then
  openssl req -newkey rsa:2048 -nodes -keyout .run/mtls/admin.key -out .run/mtls/admin.csr \
    -subj '/CN=axiom-admin/O=Zyvor Axiom' 2>/dev/null
  openssl x509 -req -in .run/mtls/admin.csr -CA .run/mtls/ca.crt -CAkey .run/mtls/ca.key \
    -CAcreateserial -out .run/mtls/admin.crt -days 3650 -sha256 2>/dev/null
  rm -f .run/mtls/admin.csr
  openssl pkcs12 -export -out .run/mtls/admin.p12 -inkey .run/mtls/admin.key -in .run/mtls/admin.crt \
    -certfile .run/mtls/ca.crt -passout pass:axiom 2>/dev/null || true
  chmod 600 .run/mtls/admin.key .run/mtls/ca.key .run/mtls/admin.p12 2>/dev/null || true
  echo 'Generated mTLS admin client cert (.run/mtls/admin.crt + admin.p12, PKCS12 password: axiom)'
else
  echo 'Reusing existing mTLS admin client cert (.run/mtls/admin.crt)'
fi
kubectl create namespace axiom-system --dry-run=client -o yaml | kubectl apply -f -
kubectl -n axiom-system create secret tls zyvor-axiom-tls \
  --cert=.run/tls/tls.crt --key=.run/tls/tls.key \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n axiom-system create secret generic zyvor-axiom-mtls \
  --from-file=ca.crt=.run/mtls/ca.crt \
  --from-file=tls.crt=.run/mtls/admin.crt \
  --from-file=tls.key=.run/mtls/admin.key \
  --dry-run=client -o yaml | kubectl apply -f -"
info "TLS secret zyvor-axiom-tls + mTLS secret zyvor-axiom-mtls applied"

# ── Auth secret ──
step 4 7 "Configure auth secret"
if ! $AUTH_EXPLICIT; then
  _EXISTING=$(_ssh "kubectl -n axiom-system get secret zyvor-axiom-auth -o go-template='{{index .data \"username\"}}:{{index .data \"password\"}}' 2>/dev/null" 2>/dev/null || true)
  if [[ "$_EXISTING" == *:* ]]; then
    _U=$(printf '%s' "${_EXISTING%%:*}" | base64 -d 2>/dev/null || true)
    _P=$(printf '%s' "${_EXISTING#*:}" | base64 -d 2>/dev/null || true)
    if [ -n "$_U" ] && [ -n "$_P" ]; then
      AXIOM_USERNAME="$_U"; AXIOM_PASSWORD="$_P"; AUTH_REUSED=true
    fi
  fi
fi
$AUTH_REUSED && info "Secret zyvor-axiom-auth already exists — reusing it (user=${AXIOM_USERNAME}); set AXIOM_USERNAME/AXIOM_PASSWORD to change"
# secret.yaml is rewritten every run: the apply in step 6 re-applies it, so it
# must always carry the resolved (possibly reused) credentials.
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
  warn "Not building an image: using the published GHCR image (pass --build to build locally)"
else
  _ssh "export AXIOM_IMAGE_TAG='${AXIOM_IMAGE_TAG}'; ${REMOTE_SH_PREFIX}; . scripts/lib/deploy-remote-images.sh; axiom_build_all_images"
  info "Image imported into k3s"
fi

# ── Apply manifests ──
step 6 7 "Apply Kubernetes manifests"
_ssh "export AXIOM_IMAGE_TAG='${AXIOM_IMAGE_TAG}' AXIOM_NODE_PORT='${NODE_PORT}' AXIOM_BUILD_LOCAL='${BUILD_LOCAL}' AXIOM_IMAGE='${AXIOM_IMAGE:-}'; ${REMOTE_SH_PREFIX}; \
python3 - <<'PY'
from pathlib import Path
import os, re

tag = os.environ['AXIOM_IMAGE_TAG']
node_port = os.environ['AXIOM_NODE_PORT']

dep = Path('deploy/k8s/deployment.yaml')
text = dep.read_text()
pat = r'image: (?:zyvor-axiom|ghcr\.io/zyvorai/axiom):[^\s]+'
if os.environ.get('AXIOM_BUILD_LOCAL') == 'true':
    text = re.sub(pat, f'image: zyvor-axiom:{tag}', text)
elif os.environ.get('AXIOM_IMAGE'):
    text = re.sub(pat, 'image: ' + os.environ['AXIOM_IMAGE'], text)
dep.write_text(text)

np = Path('deploy/k8s/nodeport.yaml')
np_text = np.read_text()
np_text = re.sub(r'nodePort: [0-9]+', f'nodePort: {node_port}', np_text)
np.write_text(np_text)
print('deployment image -> ' + re.search(r'image: (\S+)', text).group(1))
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
# Prefer hostNetwork :5090 on lab hosts where Cilium/NodePort is down.
ALT_URL="https://${HOST}:5090"
_MTLS_CURL=()
if _ssh "test -f .run/mtls/admin.crt -a -f .run/mtls/admin.key"; then
  # Pull client certs for the verify curl (mTLS require rejects bare HTTPS).
  mkdir -p "${REPO_DIR}/.run/mtls"
  _ssh "cat .run/mtls/admin.crt" > "${REPO_DIR}/.run/mtls/admin.crt"
  _ssh "cat .run/mtls/admin.key" > "${REPO_DIR}/.run/mtls/admin.key"
  chmod 600 "${REPO_DIR}/.run/mtls/admin.key" 2>/dev/null || true
  _MTLS_CURL=(--cert "${REPO_DIR}/.run/mtls/admin.crt" --key "${REPO_DIR}/.run/mtls/admin.key")
fi
HEALTH_OK=false
for u in "$BASE_URL" "$ALT_URL"; do
  if curl -fsSk --connect-timeout 10 "${_MTLS_CURL[@]}" "${u}/api/v1/health" >/dev/null; then
    info "Health OK: ${u}/api/v1/health"
    BASE_URL="$u"
    HEALTH_OK=true
    break
  fi
done
if ! $HEALTH_OK; then
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

# ── Auto-wire an already-deployed Atlas / PacketWolf (no --with-atlas needed) ──
# --with-atlas above only knows how to deploy+wire Atlas's own fake/demo image
# path (namespace zyvor-system, port 30510, via ../atlas/scripts/deploy-remote.sh).
# A *real*-Ceph Atlas gateway — the one ../hypercluster's `storage apply` deploys
# via ../atlas/scripts/deploy-ceph-gateway-remote.sh, which is what actually backs
# production Ceph clusters — lives in namespace rook-ceph on port 30511 instead,
# and --with-atlas has no idea it exists. Found live (2026-08-31) redeploying to a
# host that already had exactly this: Atlas and PacketWolf were both fully healthy
# on the cluster, yet Axiom's own Settings → Integrations showed neither wired,
# because the only wiring path assumed a namespace/port this deployment never
# used. Auto-detect both on every deploy, unconditionally and idempotently —
# harmless to re-check when already wired, and this is the only way an axiom
# instance actually finds sibling services that ../hypercluster (not this script)
# deployed. --with-atlas above still takes priority when explicitly requested.
if [ "$ATLAS_WIRED" != "1" ]; then
  AXIOM_AUTH_B64=$(printf '%s:%s' "$AXIOM_USERNAME" "$AXIOM_PASSWORD" | base64 | tr -d '\n')
  _ALREADY_CONFIGURED=$(curl -sk --connect-timeout 5 -H "Authorization: Basic ${AXIOM_AUTH_B64}" \
    "${BASE_URL}/api/v1/integrations" 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin).get("atlasConfigured", False))' 2>/dev/null || echo False)
  if [ "$_ALREADY_CONFIGURED" != "True" ]; then
    _REAL_CEPH_BOOT_TOKEN=$(_ssh "kubectl -n rook-ceph get secret atlas-gateway-auth -o jsonpath='{.data.bootstrap-admin-token}' 2>/dev/null | base64 -d" 2>/dev/null || true)
    if [ -n "$_REAL_CEPH_BOOT_TOKEN" ]; then
      _REAL_CEPH_ATLAS_URL="http://${HOST}:30511"
      info "Detected a real-Ceph Atlas gateway (rook-ceph ns, not the --with-atlas path) — wiring ${_REAL_CEPH_ATLAS_URL}"
      _REAL_CEPH_JWT=$(curl -fsS --connect-timeout 10 \
        -H "Authorization: Bearer ${_REAL_CEPH_BOOT_TOKEN}" -H 'Content-Type: application/json' \
        -d '{"subject":"axiom","role":"admin","ttl_secs":7776000}' \
        "${_REAL_CEPH_ATLAS_URL}/api/atlas/v1/auth/tokens" 2>/dev/null \
        | python3 -c 'import json,sys; print(json.load(sys.stdin).get("token",""))' 2>/dev/null || true)
      if [ -n "$_REAL_CEPH_JWT" ]; then
        if curl -fsSk --connect-timeout 10 -X PUT \
            -H "Authorization: Basic ${AXIOM_AUTH_B64}" -H 'Content-Type: application/json' \
            -d "{\"atlasUrl\":\"${_REAL_CEPH_ATLAS_URL}\",\"atlasToken\":\"${_REAL_CEPH_JWT}\"}" \
            "${BASE_URL}/api/v1/integrations" >/dev/null 2>&1; then
          info "Axiom ↔ Atlas (real-Ceph) wired: ${_REAL_CEPH_ATLAS_URL}"
        else
          warn "Real-Ceph Atlas token minted but PUT ${BASE_URL}/api/v1/integrations failed — wire manually: ${_REAL_CEPH_ATLAS_URL}"
        fi
      fi
    fi
  fi
fi
# PacketWolf integration is env-var only (AXIOM_PACKETWOLF_URL/_API_KEY, read at
# request time — see internal/platform/packetwolf.go), not persisted via
# Settings → Integrations like Atlas is. That means it must be re-applied on
# every deploy (a bare `kubectl apply -f deployment.yaml`, which this script
# always runs, resets the pod template's env to whatever's literally in the
# manifest and would silently drop an imperative `kubectl set env` from a prior
# run) — so this block is intentionally unconditional, not gated on "already
# configured" the way the Atlas block above is.
_PW_NS="${PACKETWOLF_K8S_NAMESPACE:-packetwolf}"
if _ssh "kubectl -n ${_PW_NS} get svc packetwolf-api >/dev/null 2>&1"; then
  _PW_KEY=$(_ssh "kubectl -n ${_PW_NS} get secret packetwolf-secret -o jsonpath='{.data.PACKETWOLF_ADMIN_API_KEY}' 2>/dev/null | base64 -d" 2>/dev/null || true)
  if [ -n "$_PW_KEY" ]; then
    info "Detected PacketWolf (ns ${_PW_NS}) — wiring AXIOM_PACKETWOLF_URL"
    if _ssh "kubectl -n axiom-system set env deployment/zyvor-axiom AXIOM_PACKETWOLF_URL='http://packetwolf-api.${_PW_NS}.svc.cluster.local:9191' AXIOM_PACKETWOLF_API_KEY='${_PW_KEY}' >/dev/null 2>&1 && kubectl -n axiom-system rollout status deployment/zyvor-axiom --timeout=120s >/dev/null 2>&1"; then
      info "Axiom ↔ PacketWolf wired: http://packetwolf-api.${_PW_NS}.svc.cluster.local:9191"
    else
      warn "Could not wire PacketWolf — wire manually: kubectl -n axiom-system set env deployment/zyvor-axiom AXIOM_PACKETWOLF_URL=http://packetwolf-api.${_PW_NS}.svc.cluster.local:9191"
    fi
  fi
fi

MODE_LABEL=$($SKIP_IMAGES && echo quick || echo full)
axiom_save_deploy_last "$REPO_DIR" "$HOST" "$USER" "$MODE_LABEL" "$AXIOM_IMAGE_TAG"

axiom_ui_success "$HOST" "$USER"
PANEL_LINES=(
  "UI:      ${BASE_URL}"
  "Health:  ${BASE_URL}/api/v1/health"
  "Login:   ${AXIOM_USERNAME} / $($AUTH_REUSED && echo '(existing secret)' || echo "${AXIOM_PASSWORD}")"
  "mTLS:    ${AXIOM_TLS_MTLS} — import ~/.deployment/axiom/.run/mtls/admin.p12 (password: axiom)"
  "curl:    curl -k --cert .run/mtls/admin.crt --key .run/mtls/admin.key ${BASE_URL}/api/v1/health"
)
if [ "${ATLAS_WIRED:-0}" = "1" ]; then
  PANEL_LINES+=("Atlas:   ${ATLAS_URL} (wired into Settings → Integrations)")
fi
PANEL_LINES+=(
  ""
  "Accept the browser certificate warning once; install the mTLS client cert when mode is require."
  "Redeploy fast: ./scripts/deploy-remote.sh ${HOST} ${USER} --quick"
)
axiom_ui_panel "Access (HTTPS + mTLS)" "${PANEL_LINES[@]}"
