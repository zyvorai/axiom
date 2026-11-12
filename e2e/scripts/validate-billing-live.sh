#!/usr/bin/env bash
# Live-account check for experimental cloud invoice clients.
# Does NOT print credentials. Exit 0 when the provider answers; 2 when skipped.
#
# AWS:
#   AXIOM_BILLING_LIVE=1 AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... \
#   [AWS_SESSION_TOKEN=...] [AWS_REGION=us-east-1] [BILLING_TAG_KEY=...] [BILLING_TAG_VALUE=...] \
#   go test -tags=kubernetes ./internal/platform/ -run TestAWSBillingLive -count=1 -v
#
# Or against a running Axiom (Settings already configured):
#   AXIOM_BASE=https://… AXIOM_USER=… AXIOM_PASS=… ./e2e/scripts/validate-billing-live.sh
set -u

if [ "${AXIOM_BILLING_LIVE:-}" = "1" ] && [ -n "${AWS_ACCESS_KEY_ID:-}" ]; then
  echo "== direct AWS Cost Explorer live call via Go test =="
  cd "$(dirname "$0")/../.."
  AXIOM_BILLING_LIVE=1 go test ./internal/platform/ -run TestAWSBillingLive -count=1 -v
  exit $?
fi

BASE="${AXIOM_BASE:-}"
if [ -z "$BASE" ]; then
  echo "SKIP  set AXIOM_BILLING_LIVE=1 + AWS_* or AXIOM_BASE + credentials to run"
  exit 2
fi

AUTH="${AXIOM_USER:-}:${AXIOM_PASS:-}"
echo "== POST /api/v1/integrations/billing/test against $BASE =="
code=$(curl -sk ${AUTH:+-u "$AUTH"} -X POST -o /tmp/axiom-billing-test.json -w '%{http_code}' "$BASE/api/v1/integrations/billing/test")
body=$(cat /tmp/axiom-billing-test.json 2>/dev/null || true)
rm -f /tmp/axiom-billing-test.json
echo "HTTP $code  $body"
if [ "$code" = "200" ] && echo "$body" | grep -q '"ok":true'; then
  echo "PASS  live billing test"
  exit 0
fi
if [ "$code" = "409" ] || echo "$body" | grep -qi 'not configured'; then
  echo "SKIP  billing not configured in Settings"
  exit 2
fi
echo "FAIL  live billing test"
exit 1
