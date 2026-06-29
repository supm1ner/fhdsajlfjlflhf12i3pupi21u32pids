#!/bin/sh
# ─────────────────────────────────────────────────────────────────────────────
# cotton-id DEV SEED one-shot (dev profile only; NEVER run in production).
#
# Runs after the backend is healthy (it depends_on backend: service_healthy in
# compose) and performs two idempotent steps:
#
#   1. Seed a demo admin user into the cotton-id `users` table via psql
#      (deploy/seed/seed.sql), so a developer can log in immediately.
#   2. Register a demo relying-party OAuth2 client through the backend's admin
#      API (`POST /api/v1/admin/clients`, authorized by X-Admin-Key), which the
#      backend proxies to Hydra. This exercises the real registration path.
#
# Runs in alpine with postgresql-client + curl installed at start. All inputs
# come from env (wired in compose): DATABASE_URL, BACKEND_INTERNAL_URL,
# ADMIN_API_KEY, and the demo client parameters.
# ─────────────────────────────────────────────────────────────────────────────
set -eu

echo "[seed] installing tools (psql, curl)…"
apk add --no-cache postgresql-client curl >/dev/null

# ---- 1) Seed the demo admin user --------------------------------------------
echo "[seed] seeding demo admin user…"
# psql reads the connection from DATABASE_URL; seed.sql is mounted read-only.
psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -f /seed/seed.sql
echo "[seed] admin user ready: admin@cotton.local / DemoAdmin!2026"

# ---- 2) Register the demo relying-party client ------------------------------
# The backend admin endpoint is idempotent enough for dev: if a client with the
# same name already exists we tolerate a non-2xx and continue (compose seed is a
# one-shot that may re-run on `make seed`).
BACKEND="${BACKEND_INTERNAL_URL:-http://backend:8080}"

echo "[seed] registering demo relying-party client at ${BACKEND}…"
HTTP_CODE=$(curl -s -o /tmp/client.json -w '%{http_code}' \
    -X POST "${BACKEND}/api/v1/admin/clients" \
    -H "Content-Type: application/json" \
    -H "X-Admin-Key: ${ADMIN_API_KEY}" \
    --data @- <<JSON || echo "000"
{
  "name": "${DEMO_CLIENT_NAME:-Demo Relying Party}",
  "clientType": "${DEMO_CLIENT_TYPE:-public}",
  "redirectUris": ["${DEMO_CLIENT_REDIRECT_URI:-http://localhost:3000/callback}"],
  "scopes": ["openid", "profile", "email", "offline_access"],
  "grantTypes": ["authorization_code", "refresh_token"],
  "responseTypes": ["code"]
}
JSON
)

case "${HTTP_CODE}" in
    2*)
        echo "[seed] demo client registered:"
        cat /tmp/client.json
        echo ""
        ;;
    000)
        echo "[seed] WARN: could not reach backend admin API; skipping client registration."
        ;;
    *)
        echo "[seed] NOTE: client registration returned HTTP ${HTTP_CODE} (likely already exists); continuing."
        cat /tmp/client.json 2>/dev/null || true
        echo ""
        ;;
esac

echo "[seed] done."
