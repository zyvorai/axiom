#!/usr/bin/env bash
# Configure Haven Keycloak for Axiom OIDC on the lab, then enable Settings integrations.
# Usage:
#   ./scripts/keycloak-oidc-lab.sh <host> zyvorai
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HOST="${1:-}"
USER="${2:-zyvorai}"
AXIOM_PUBLIC_URL="${AXIOM_PUBLIC_URL:-https://${HOST:-127.0.0.1}:${AXIOM_NODE_PORT:-30590}}"
KC_PUBLIC_URL="${KC_PUBLIC_URL:-http://auth.${HOST:-127.0.0.1}.nip.io:${AXIOM_NODE_PORT:-30591}}"
REALM="${KC_REALM:-platform}"
CLIENT_ID="${OIDC_CLIENT_ID:-axiom}"

if [[ -z "$HOST" ]]; then
  echo "Usage: $0 <ssh-host> [ssh-user]" >&2
  exit 2
fi

echo "=== Keycloak OIDC lab setup ==="
echo "Axiom:    $AXIOM_PUBLIC_URL"
echo "Keycloak: $KC_PUBLIC_URL"

# Do not patch Keycloak CR hostname fields here — operator rejects unknown
# hostname.port / hostname-url additionalOptions on this version. Use admin API.
scp -o BatchMode=yes "$SCRIPT_DIR/lib/keycloak-oidc-configure.sh" "${USER}@${HOST}:/tmp/keycloak-oidc-configure.sh"

ssh -o BatchMode=yes "${USER}@${HOST}" bash -s <<REMOTE
set -euo pipefail
ADMIN_USER=\$(sudo k3s kubectl -n keycloak get secret platform-initial-admin -o jsonpath='{.data.username}' | base64 -d)
ADMIN_PASS=\$(sudo k3s kubectl -n keycloak get secret platform-initial-admin -o jsonpath='{.data.password}' | base64 -d)
sudo k3s kubectl -n keycloak delete pod kc-oidc-job --ignore-not-found >/dev/null 2>&1 || true
sudo k3s kubectl -n keycloak delete cm kc-oidc-script --ignore-not-found >/dev/null 2>&1 || true
sudo k3s kubectl -n keycloak create configmap kc-oidc-script --from-file=run.sh=/tmp/keycloak-oidc-configure.sh
sudo k3s kubectl -n keycloak run kc-oidc-job --restart=Never --image=curlimages/curl:8.5.0 --env=ADMIN_USER="\$ADMIN_USER" --env=ADMIN_PASS="\$ADMIN_PASS" \
  --env=AXIOM_PUBLIC_URL='${AXIOM_PUBLIC_URL}' --env=REALM='${REALM}' --env=CLIENT_ID='${CLIENT_ID}' \
  --overrides='{"spec":{"containers":[{"name":"kc-oidc-job","image":"curlimages/curl:8.5.0","command":["sh","/scripts/run.sh"],"env":[{"name":"ADMIN_USER","value":"'"\$ADMIN_USER"'"},{"name":"ADMIN_PASS","value":"'"\$ADMIN_PASS"'"},{"name":"AXIOM_PUBLIC_URL","value":"${AXIOM_PUBLIC_URL}"},{"name":"REALM","value":"${REALM}"},{"name":"CLIENT_ID","value":"${CLIENT_ID}"}],"volumeMounts":[{"name":"s","mountPath":"/scripts"}]}],"volumes":[{"name":"s","configMap":{"name":"kc-oidc-script","defaultMode":365}}]}}'
for i in \$(seq 1 40); do
  PHASE=\$(sudo k3s kubectl -n keycloak get pod kc-oidc-job -o jsonpath='{.status.phase}' 2>/dev/null || echo Missing)
  echo "phase=\$PHASE"
  [ "\$PHASE" = Succeeded ] && break
  [ "\$PHASE" = Failed ] && break
  sleep 2
done
sudo k3s kubectl -n keycloak logs kc-oidc-job || true
sudo k3s kubectl -n keycloak delete pod kc-oidc-job --ignore-not-found >/dev/null 2>&1 || true
REMOTE

echo "=== Enable OIDC in Axiom Settings ==="
curl -fsSk -u demo:demo -X PUT "${AXIOM_PUBLIC_URL}/api/v1/integrations" \
  -H 'Content-Type: application/json' \
  -d "{\"oidcIssuer\":\"${KC_PUBLIC_URL}/realms/${REALM}\",\"oidcLoginUrl\":\"${KC_PUBLIC_URL}/realms/${REALM}/protocol/openid-connect/auth\",\"oidcClientId\":\"${CLIENT_ID}\"}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);print("oidcConfigured",d.get("oidcConfigured"),d.get("oidcIssuer"))'

curl -fsSk "${AXIOM_PUBLIC_URL}/api/v1/auth/providers" | python3 -c 'import sys,json;d=json.load(sys.stdin);print("oidc.enabled",d["oidc"]["enabled"],"issuer",d["oidc"].get("issuer"))'

echo
echo "Sign in: ${AXIOM_PUBLIC_URL} → More sign-in options → Sign in with OIDC"
echo "Keycloak user: demo / demo"
