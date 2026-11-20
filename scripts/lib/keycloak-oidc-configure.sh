#!/bin/sh
# Runs inside a curl pod in the keycloak namespace.
set -eu
TOK=$(curl -sS -X POST http://platform-service:8080/realms/master/protocol/openid-connect/token \
  -d client_id=admin-cli \
  -d username="$ADMIN_USER" \
  -d password="$ADMIN_PASS" \
  -d grant_type=password)
ACCESS=$(printf '%s' "$TOK" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
echo "token_len=${#ACCESS}"
test -n "$ACCESS"
AUTH="Authorization: Bearer $ACCESS"

REALM_JSON=$(curl -sS -H "$AUTH" "http://platform-service:8080/admin/realms/${REALM}")
REALM_JSON=$(printf '%s' "$REALM_JSON" | sed 's/"sslRequired":"[^"]*"/"sslRequired":"none"/')
curl -sS -o /dev/null -w "realm_ssl=%{http_code}\n" -X PUT -H "$AUTH" -H 'Content-Type: application/json' \
  "http://platform-service:8080/admin/realms/${REALM}" --data-binary @- <<EOF
$REALM_JSON
EOF
curl -sS -H "$AUTH" "http://platform-service:8080/admin/realms/${REALM}" | sed -n 's/.*"sslRequired":"\([^"]*\)".*/sslRequired=\1/p'

CLIENTS=$(curl -sS -H "$AUTH" "http://platform-service:8080/admin/realms/${REALM}/clients?clientId=${CLIENT_ID}&max=1")
CID=$(printf '%s' "$CLIENTS" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -1)
echo "client_uuid=$CID"
if [ -n "$CID" ]; then
  BODY=$(curl -sS -H "$AUTH" "http://platform-service:8080/admin/realms/${REALM}/clients/$CID")
  BODY=$(printf '%s' "$BODY" | sed "s#\"redirectUris\":\[[^]]*\]#\"redirectUris\":[\"${AXIOM_PUBLIC_URL}/*\",\"${AXIOM_PUBLIC_URL}/api/v1/auth/oidc/callback\"]#")
  curl -sS -o /dev/null -w "client_update=%{http_code}\n" -X PUT -H "$AUTH" -H 'Content-Type: application/json' \
    "http://platform-service:8080/admin/realms/${REALM}/clients/$CID" -d "$BODY"
fi

USERS=$(curl -sS -H "$AUTH" "http://platform-service:8080/admin/realms/${REALM}/users?username=demo&exact=true")
UID=$(printf '%s' "$USERS" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -1)
if [ -z "$UID" ]; then
  curl -sS -o /dev/null -w "create_user=%{http_code}\n" -X POST -H "$AUTH" -H 'Content-Type: application/json' \
    "http://platform-service:8080/admin/realms/${REALM}/users" \
    -d '{"username":"demo","enabled":true,"email":"demo@zyvor.dev","emailVerified":true,"firstName":"Demo","lastName":"Operator"}'
  USERS=$(curl -sS -H "$AUTH" "http://platform-service:8080/admin/realms/${REALM}/users?username=demo&exact=true")
  UID=$(printf '%s' "$USERS" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -1)
fi
echo "user_id=$UID"
if [ -n "$UID" ]; then
  curl -sS -o /dev/null -w "set_password=%{http_code}\n" -X PUT -H "$AUTH" -H 'Content-Type: application/json' \
    "http://platform-service:8080/admin/realms/${REALM}/users/$UID/reset-password" \
    -d '{"type":"password","value":"demo","temporary":false}'
fi

# Patch Keycloak CR hostname URL via kubectl is done by the wrapper; print discovery issuer.
curl -sS "http://platform-service:8080/realms/${REALM}/.well-known/openid-configuration" \
  | sed -n 's/.*"issuer":"\([^"]*\)".*/issuer=\1/p'
