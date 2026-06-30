#!/usr/bin/env bash
# Registers (or updates) the Sunrise messenger as an OAuth2 public client in Hydra,
# so "Sign in with SSO" works. Run AFTER the stack is up (hydra healthy).
#
#   ./register-rp.sh
#
# Reads ports/ids from .env if present; override via env vars.
set -euo pipefail
cd "$(dirname "$0")"
[ -f .env ] && set -a && . ./.env && set +a

ADMIN="http://localhost:${HYDRA_ADMIN_PORT:-4445}"
CLIENT_ID="${OIDC_CLIENT_ID:-sunrise-messenger}"
REDIRECT="${VITE_OIDC_REDIRECT:-http://localhost:8088/}"

read -r -d '' BODY <<JSON || true
{
  "client_id": "${CLIENT_ID}",
  "client_name": "Sunrise Messenger",
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "scope": "openid profile email",
  "redirect_uris": ["${REDIRECT}"],
  "post_logout_redirect_uris": ["${REDIRECT}"],
  "token_endpoint_auth_method": "none",
  "skip_consent": false
}
JSON

echo "Registering client '${CLIENT_ID}' (redirect ${REDIRECT}) at ${ADMIN} ..."
code=$(curl -s -o /tmp/lk_rp.json -w '%{http_code}' -X POST "${ADMIN}/admin/clients" \
  -H 'Content-Type: application/json' -d "${BODY}")

if [ "${code}" = "201" ]; then
  echo "Created."
elif [ "${code}" = "409" ]; then
  echo "Already exists — updating."
  curl -s -o /dev/null -X PUT "${ADMIN}/admin/clients/${CLIENT_ID}" \
    -H 'Content-Type: application/json' -d "${BODY}"
  echo "Updated."
else
  echo "Unexpected response (${code}):"; cat /tmp/lk_rp.json; exit 1
fi
echo "Done. Use 'Sign in with SSO' in the web client at ${REDIRECT}"
