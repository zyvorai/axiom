#!/usr/bin/env bash
# Smoke Axiom integrations API (Atlas, OIDC, Postgres, probe) against a running instance.
# No lab IP/NodePort defaults — pass URLs via env (or a local .env you source).
#
# Required:
#   AXIOM_URL   e.g. https://axiom.example.com  or  https://<node>:<nodeport>
#
# Optional (only set what you want the smoke to configure/test):
#   AXIOM_USER / AXIOM_PASS
#   ATLAS_URL / ATLAS_CONSOLE_URL / ATLAS_TOKEN / PROBE_URL
#   OIDC_ISSUER / OIDC_LOGIN_URL / OIDC_CLIENT_ID
#   POSTGRES_HOST / POSTGRES_PORT / POSTGRES_DB / POSTGRES_USER / POSTGRES_PASS
#   LAB_HOST / LAB_USER — only if you want secrets fetched via SSH kubectl
set -euo pipefail

AXIOM_URL="${AXIOM_URL:-}"
AXIOM_USER="${AXIOM_USER:-demo}"
AXIOM_PASS="${AXIOM_PASS:-demo}"
LAB_HOST="${LAB_HOST:-}"
LAB_USER="${LAB_USER:-sus}"

POSTGRES_HOST="${POSTGRES_HOST:-postgres-lab.axiom-system.svc}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_DB="${POSTGRES_DB:-axiom}"
POSTGRES_USER="${POSTGRES_USER:-axiom}"
POSTGRES_PASS="${POSTGRES_PASS:-}"

ATLAS_URL="${ATLAS_URL:-}"
ATLAS_CONSOLE_URL="${ATLAS_CONSOLE_URL:-}"
ATLAS_TOKEN="${ATLAS_TOKEN:-}"
OIDC_ISSUER="${OIDC_ISSUER:-}"
OIDC_LOGIN_URL="${OIDC_LOGIN_URL:-}"
OIDC_CLIENT_ID="${OIDC_CLIENT_ID:-axiom}"
PROBE_URL="${PROBE_URL:-}"

if [[ -z "$AXIOM_URL" ]]; then
  echo "AXIOM_URL is required (e.g. export AXIOM_URL=https://axiom.example.com)" >&2
  exit 2
fi

export AXIOM_URL AXIOM_USER AXIOM_PASS POSTGRES_HOST POSTGRES_PORT POSTGRES_DB POSTGRES_USER POSTGRES_PASS
export ATLAS_URL ATLAS_CONSOLE_URL ATLAS_TOKEN OIDC_ISSUER OIDC_LOGIN_URL OIDC_CLIENT_ID PROBE_URL

ok() { echo "  OK  $*"; }
fail() { echo "  FAIL $*" >&2; exit 1; }

echo "=== Axiom integration smoke ==="
echo "URL: $AXIOM_URL"

curl -fsSk -u "${AXIOM_USER}:${AXIOM_PASS}" "$AXIOM_URL/api/v1/health" | python3 -c \
  'import sys,json; d=json.load(sys.stdin); assert d.get("status")=="ok"' \
  && ok "health" || fail "health"

EX=$(curl -fsSk -u "${AXIOM_USER}:${AXIOM_PASS}" "$AXIOM_URL/api/v1/integrations/examples")
EX_COUNT=$(echo "$EX" | python3 -c \
  'import sys,json; d=json.load(sys.stdin); assert len(d.get("examples",[]))>=1; print(len(d["examples"]))')
ok "integrations/examples ($EX_COUNT templates)"

if [[ -z "$POSTGRES_PASS" && -n "$LAB_HOST" ]]; then
  POSTGRES_PASS=$(ssh -o BatchMode=yes "${LAB_USER}@${LAB_HOST}" \
    'sudo k3s kubectl -n axiom-system get secret postgres-lab-auth -o jsonpath="{.data.password}" 2>/dev/null | base64 -d' || true)
  export POSTGRES_PASS
fi

if [[ -z "$ATLAS_TOKEN" && -n "$LAB_HOST" ]]; then
  ATLAS_TOKEN=$(ssh -o BatchMode=yes "${LAB_USER}@${LAB_HOST}" \
    'sudo k3s kubectl -n zyvor-system get secret atlas-gateway-auth -o jsonpath="{.data.bootstrap-admin-token}" 2>/dev/null | base64 -d' || true)
  export ATLAS_TOKEN
fi

PATCH=$(python3 <<PY
import json, os
p = {}
for key, env in [
  ("atlasUrl", "ATLAS_URL"),
  ("atlasConsoleUrl", "ATLAS_CONSOLE_URL"),
  ("postgresHost", "POSTGRES_HOST"),
  ("postgresDatabase", "POSTGRES_DB"),
  ("postgresUser", "POSTGRES_USER"),
  ("probeUrl", "PROBE_URL"),
  ("oidcIssuer", "OIDC_ISSUER"),
  ("oidcLoginUrl", "OIDC_LOGIN_URL"),
  ("oidcClientId", "OIDC_CLIENT_ID"),
]:
  v = os.environ.get(env, "")
  if v:
    p[key] = v
if os.environ.get("POSTGRES_PORT"):
  p["postgresPort"] = int(os.environ["POSTGRES_PORT"])
p["postgresSslMode"] = "disable"
if os.environ.get("ATLAS_TOKEN"):
  p["atlasToken"] = os.environ["ATLAS_TOKEN"]
if os.environ.get("POSTGRES_PASS"):
  p["postgresPassword"] = os.environ["POSTGRES_PASS"]
print(json.dumps(p))
PY
)

if [[ "$PATCH" != "{}" && "$PATCH" != '{"postgresSslMode": "disable"}' && "$PATCH" != '{"postgresPort": 5432, "postgresSslMode": "disable"}' ]]; then
  curl -fsSk -u "${AXIOM_USER}:${AXIOM_PASS}" -X PUT "$AXIOM_URL/api/v1/integrations" \
    -H 'Content-Type: application/json' -d "$PATCH" >/dev/null \
    && ok "PUT integrations" || fail "PUT integrations"
else
  echo "  SKIP PUT integrations (no integration env set)"
fi

test_ep() {
  local name="$1" path="$2" required="${3:-0}"
  local body
  body=$(curl -fsSk -u "${AXIOM_USER}:${AXIOM_PASS}" -X POST "$AXIOM_URL/api/v1/integrations/$path/test" \
    -H 'Content-Type: application/json' -d '{}' 2>/dev/null || echo '{"ok":false,"error":"request failed"}')
  if echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); sys.exit(0 if d.get('ok') else 1)"; then
    local ms
    ms=$(echo "$body" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("latencyMs",""))' 2>/dev/null || echo "")
    ok "$name ${ms:+$ms ms}"
    return 0
  fi
  if [[ "$required" == "1" ]]; then
    echo "  FAIL $name: $body" >&2
    return 1
  fi
  echo "  WARN $name: $body"
  return 0
}

# Require only endpoints that have been configured via env (or already saved in Axiom).
[[ -n "$ATLAS_URL" || -n "$ATLAS_TOKEN" ]] && test_ep atlas atlas 1 || test_ep atlas atlas 0
[[ -n "$PROBE_URL" ]] && test_ep probe probe 1 || test_ep probe probe 0
[[ -n "$POSTGRES_PASS" || -n "$POSTGRES_HOST" ]] && test_ep postgres postgres 0
[[ -n "$OIDC_ISSUER" ]] && test_ep oidc oidc 0 || test_ep oidc oidc 0

ATLAS_SUM=$(curl -fsSk -u "${AXIOM_USER}:${AXIOM_PASS}" "$AXIOM_URL/api/v1/atlas" || echo '{}')
echo "$ATLAS_SUM" | python3 -c \
  'import sys,json; d=json.load(sys.stdin); print("  atlas summary: available=", d.get("available"), "backends=", len(d.get("backends") or []))' \
  2>/dev/null || true

echo "=== smoke complete ==="
