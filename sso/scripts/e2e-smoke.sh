#!/bin/sh
# ─────────────────────────────────────────────────────────────────────────────
# cotton-id — end-to-end smoke test (harden-and-observe, task 6.1)
#
# Exercises every top-level capability of a RUNNING stack over HTTP with curl and
# a cookie jar — no Go, no jq, no build. It is the production-readiness gate: a
# green run proves the whole IdP answers, from CSRF through the OIDC RP flow to
# admin RBAC and the audit Journal.
#
# Run it against an up stack (see docs/RUNBOOK.md §11):
#
#     make up                      # bring the stack up (incl. the dev seed)
#     make e2e                     # or: sh scripts/e2e-smoke.sh
#
# Override the endpoints when the stack is not on the default localhost ports:
#
#     BASE_URL=http://localhost:8080 \
#     HYDRA_PUBLIC_URL=http://localhost:4444 \
#     FRONTEND_BASE_URL=http://localhost:3000 \
#     ADMIN_API_KEY=... \
#     sh scripts/e2e-smoke.sh
#
# Each step prints "PASS <step>" or "FAIL <step> — <why>". The script keeps going
# so you see every failure in one run, then exits non-zero if ANY step failed.
#
# Requirements on the host: a POSIX sh, curl, sed, grep. The dev seed must have
# run (it registers the demo admin used by the RBAC steps). Intended for a dev
# stack (COTTON_ENV=development, COOKIE_SECURE=false) — it uses http and the seed.
#
# POSIX note: written to be shellcheck-clean. No bashisms, no process
# substitution, no arrays; temp files in a per-run dir cleaned up on exit.
# ─────────────────────────────────────────────────────────────────────────────

set -eu

# ---- Configuration (env-overridable) ----------------------------------------
BASE_URL="${BASE_URL:-http://localhost:8080}"
HYDRA_PUBLIC_URL="${HYDRA_PUBLIC_URL:-http://localhost:4444}"
FRONTEND_BASE_URL="${FRONTEND_BASE_URL:-http://localhost:3000}"

# Seed operator (dev profile). Override if you seeded different credentials.
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@cotton.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-DemoAdmin!2026}"

# Strip trailing slashes so URL building is predictable.
BASE_URL=$(printf '%s' "$BASE_URL" | sed 's#/*$##')
HYDRA_PUBLIC_URL=$(printf '%s' "$HYDRA_PUBLIC_URL" | sed 's#/*$##')
FRONTEND_BASE_URL=$(printf '%s' "$FRONTEND_BASE_URL" | sed 's#/*$##')

# ADMIN_API_KEY: prefer the env, else read it from deploy/.env (dev convenience).
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
ENV_FILE="$REPO_ROOT/deploy/.env"
ADMIN_API_KEY="${ADMIN_API_KEY:-}"
if [ -z "$ADMIN_API_KEY" ] && [ -f "$ENV_FILE" ]; then
	ADMIN_API_KEY=$(sed -n 's/^ADMIN_API_KEY=//p' "$ENV_FILE" | head -n1)
fi

# A unique-enough suffix so reruns do not collide on the unique email/username.
SUFFIX=$(date +%s)
USER_EMAIL="smoke+${SUFFIX}@example.com"
USER_NAME="smoke${SUFFIX}"
USER_PASS="correct horse battery staple ${SUFFIX}"

# ---- Workspace + cleanup ----------------------------------------------------
WORKDIR=$(mktemp -d 2>/dev/null || mktemp -d -t cottonsmoke)
USER_JAR="$WORKDIR/user.cookies"
ADMIN_JAR="$WORKDIR/admin.cookies"
ANON_JAR="$WORKDIR/anon.cookies"
BODY="$WORKDIR/body"      # last response body
HDRS="$WORKDIR/headers"   # last response headers

cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT INT TERM

FAILS=0

# ---- Output helpers ---------------------------------------------------------
pass() { printf 'PASS  %s\n' "$1"; }
fail() {
	# $1 = step, $2 = reason
	printf 'FAIL  %s — %s\n' "$1" "$2"
	FAILS=$((FAILS + 1))
}
info() { printf '      %s\n' "$1"; }

# request METHOD URL [curl args...]
#   Performs the request, writing the body to $BODY and headers to $HDRS, and
#   echoes the numeric HTTP status to stdout. Never sends -f, so a 4xx/5xx still
#   returns a status (each step decides what is acceptable).
request() {
	_m=$1
	_u=$2
	shift 2
	curl -sS -o "$BODY" -D "$HDRS" -w '%{http_code}' -X "$_m" "$@" "$_u"
}

# json_field KEY  — extract a flat string JSON value (no jq). Best-effort: works
# for the simple, flat top-level fields this smoke needs (token, id, ...).
json_field() {
	sed -n 's/.*"'"$1"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$BODY" | head -n1
}

# fresh_csrf JAR  — GET /csrf with the given jar and echo the token (also sets
# the cid_csrf cookie in the jar). State-changing requests echo it back.
fresh_csrf() {
	curl -sS -c "$1" -b "$1" -o "$BODY" "$BASE_URL/api/v1/csrf" >/dev/null
	json_field token
}

# b64url DATA  — URL-safe base64 with no padding (for the PKCE challenge bytes).
b64url() {
	openssl base64 -A | tr '+/' '-_' | tr -d '='
}

printf '== cotton-id e2e smoke ==\n'
info "backend       $BASE_URL"
info "hydra public  $HYDRA_PUBLIC_URL"
info "frontend      $FRONTEND_BASE_URL"
info "test user     $USER_EMAIL"
printf '\n'

# ─────────────────────────────────────────────────────────────────────────────
# 0. Reachability — the backend must answer /healthz before anything else.
# ─────────────────────────────────────────────────────────────────────────────
code=$(request GET "$BASE_URL/healthz" || true)
if [ "$code" = "200" ]; then
	pass "healthz 200 (stack reachable)"
else
	fail "healthz" "expected 200, got ${code:-no-response} — is the stack up? (make up)"
	# Without the backend nothing else can pass; stop early with a non-zero exit.
	printf '\nAborting: backend not reachable.\n'
	exit 1
fi

# ─────────────────────────────────────────────────────────────────────────────
# 1. CSRF — GET /api/v1/csrf returns a token and sets the cid_csrf cookie.
# ─────────────────────────────────────────────────────────────────────────────
CSRF=$(fresh_csrf "$USER_JAR")
if [ -n "$CSRF" ] && grep -q 'cid_csrf' "$USER_JAR"; then
	pass "csrf token issued + cid_csrf cookie set"
else
	fail "csrf" "no token in body or no cid_csrf cookie in jar"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 2. Signup — POST /api/v1/auth/signup → 201, sets the session cookie.
# ─────────────────────────────────────────────────────────────────────────────
code=$(request POST "$BASE_URL/api/v1/auth/signup" \
	-c "$USER_JAR" -b "$USER_JAR" \
	-H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" \
	--data "{\"displayName\":\"Smoke User\",\"username\":\"$USER_NAME\",\"email\":\"$USER_EMAIL\",\"password\":\"$USER_PASS\"}" || true)
if [ "$code" = "201" ] && grep -q 'cid_session' "$USER_JAR"; then
	pass "signup 201 + cid_session cookie set"
else
	fail "signup" "expected 201 + cid_session, got $code ($(json_field detail))"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 3. Session check + logout + login — the password auth round-trip.
#    a) GET /auth/session with the new cookie → 200.
#    b) logout → 204, then the session cookie no longer authenticates (401).
#    c) login → 200, re-establishing a session for the rest of the run.
# ─────────────────────────────────────────────────────────────────────────────
code=$(request GET "$BASE_URL/api/v1/auth/session" -b "$USER_JAR" || true)
if [ "$code" = "200" ]; then
	pass "session check 200 (authenticated after signup)"
else
	fail "session-check" "expected 200, got $code"
fi

CSRF=$(fresh_csrf "$USER_JAR")
code=$(request POST "$BASE_URL/api/v1/auth/logout" \
	-b "$USER_JAR" -c "$USER_JAR" -H "X-CSRF-Token: $CSRF" || true)
if [ "$code" = "204" ]; then
	pass "logout 204"
else
	fail "logout" "expected 204, got $code"
fi

CSRF=$(fresh_csrf "$USER_JAR")
code=$(request POST "$BASE_URL/api/v1/auth/login" \
	-c "$USER_JAR" -b "$USER_JAR" \
	-H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" \
	--data "{\"email\":\"$USER_EMAIL\",\"password\":\"$USER_PASS\"}" || true)
if [ "$code" = "200" ] && grep -q 'cid_session' "$USER_JAR"; then
	pass "login 200 + session re-established"
else
	fail "login" "expected 200 + cid_session, got $code ($(json_field detail))"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 4. Password forgot — POST /auth/password/forgot → 202, NON-enumerating
#    (the same 202 whether or not the email exists). We assert the contract's
#    success status only; the token is delivered by the mailer, never returned.
# ─────────────────────────────────────────────────────────────────────────────
CSRF=$(fresh_csrf "$USER_JAR")
code=$(request POST "$BASE_URL/api/v1/auth/password/forgot" \
	-b "$USER_JAR" \
	-H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" \
	--data "{\"email\":\"$USER_EMAIL\"}" || true)
if [ "$code" = "202" ]; then
	pass "password forgot 202 (non-enumerating)"
else
	fail "password-forgot" "expected 202, got $code"
fi
# And an unknown address must get the SAME 202 (enumeration resistance).
CSRF=$(fresh_csrf "$USER_JAR")
code=$(request POST "$BASE_URL/api/v1/auth/password/forgot" \
	-b "$USER_JAR" \
	-H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" \
	--data '{"email":"nobody-here@example.com"}' || true)
if [ "$code" = "202" ]; then
	pass "password forgot 202 for unknown email (no enumeration)"
else
	fail "password-forgot-unknown" "expected 202, got $code"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 5. Social providers list — GET /auth/social/providers → 200 with a providers
#    array (empty in a stack with no social creds configured — that is valid).
# ─────────────────────────────────────────────────────────────────────────────
code=$(request GET "$BASE_URL/api/v1/auth/social/providers" || true)
if [ "$code" = "200" ] && grep -q '"providers"' "$BODY"; then
	pass "social providers list 200 (providers array present)"
else
	fail "social-providers" "expected 200 with a providers array, got $code"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 6. Passkey login begin — POST /auth/passkey/login/begin → 200 with a publicKey
#    options object and a challenge, plus the short-lived cid_wa state cookie.
# ─────────────────────────────────────────────────────────────────────────────
CSRF=$(fresh_csrf "$ANON_JAR")
code=$(request POST "$BASE_URL/api/v1/auth/passkey/login/begin" \
	-c "$ANON_JAR" -b "$ANON_JAR" \
	-H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" \
	--data '{}' || true)
if [ "$code" = "200" ] && grep -q '"challenge"' "$BODY"; then
	pass "passkey login/begin 200 (request options returned)"
	if grep -q 'cid_wa' "$ANON_JAR"; then
		info "cid_wa ceremony-state cookie set"
	fi
else
	fail "passkey-login-begin" "expected 200 with a challenge, got $code"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 7. OIDC RP flow — register an OAuth client via the machine API, then drive an
#    authorization_code + PKCE /oauth2/auth and assert the 302 chain reaches the
#    cotton-id login page (anonymous browser, so Hydra → /oauth/login → SPA).
# ─────────────────────────────────────────────────────────────────────────────
if [ -z "$ADMIN_API_KEY" ]; then
	fail "oidc-register-client" "no ADMIN_API_KEY (set it or create deploy/.env)"
else
	RP_REDIRECT="$FRONTEND_BASE_URL/callback"
	code=$(request POST "$BASE_URL/api/v1/admin/clients" \
		-H "Content-Type: application/json" -H "X-Admin-Key: $ADMIN_API_KEY" \
		--data "{\"name\":\"Smoke RP $SUFFIX\",\"redirectUris\":[\"$RP_REDIRECT\"],\"scopes\":[\"openid\",\"profile\",\"email\"],\"grantTypes\":[\"authorization_code\",\"refresh_token\"],\"responseTypes\":[\"code\"],\"clientType\":\"public\"}" || true)
	CLIENT_ID=$(json_field clientId)
	if [ "$code" = "201" ] && [ -n "$CLIENT_ID" ]; then
		pass "oidc client registered via machine API (clientId=$CLIENT_ID)"
	else
		fail "oidc-register-client" "expected 201 with clientId, got $code ($(json_field detail))"
	fi

	if [ -n "$CLIENT_ID" ]; then
		# PKCE: verifier (random) → S256 challenge = base64url(sha256(verifier)).
		VERIFIER=$(openssl rand -hex 32)
		CHALLENGE=$(printf '%s' "$VERIFIER" | openssl dgst -binary -sha256 | b64url)
		STATE=$(openssl rand -hex 8)
		AUTH_URL="$HYDRA_PUBLIC_URL/oauth2/auth?response_type=code&client_id=$CLIENT_ID&redirect_uri=$RP_REDIRECT&scope=openid%20profile%20email&state=$STATE&code_challenge=$CHALLENGE&code_challenge_method=S256"

		# Follow the redirect chain with an ANONYMOUS jar (no cotton-id session) so
		# it must land on the SPA login page. --max-redirs bounds the chain; -w
		# %{url_effective} reports where we ended up after following 302s.
		FINAL_URL=$(curl -sS -L --max-redirs 10 \
			-c "$ANON_JAR" -b "$ANON_JAR" \
			-o "$BODY" -w '%{url_effective}' \
			"$AUTH_URL" 2>/dev/null || true)
		# Expect the chain to terminate at FRONTEND_BASE_URL/login?login_challenge=…
		case "$FINAL_URL" in
		"$FRONTEND_BASE_URL/login?login_challenge="*)
			pass "authorize 302 chain reaches the cotton-id login (login_challenge present)"
			info "landed: $FINAL_URL"
			;;
		*"/login?login_challenge="*)
			# Tolerate a differing front-end host (e.g. proxied) as long as the SPA
			# login route + challenge are present.
			pass "authorize 302 chain reaches a cotton-id login page with login_challenge"
			info "landed: $FINAL_URL"
			;;
		*)
			fail "oidc-authorize-chain" "did not land on /login?login_challenge=… (ended at: ${FINAL_URL:-none})"
			;;
		esac

		# Clean up the throwaway RP so reruns do not accumulate clients.
		request DELETE "$BASE_URL/api/v1/admin/clients/$CLIENT_ID" \
			-H "X-Admin-Key: $ADMIN_API_KEY" >/dev/null 2>&1 || true
	fi
fi

# ─────────────────────────────────────────────────────────────────────────────
# 8. Account gate — GET /account is 401 without a session and 200 with one.
# ─────────────────────────────────────────────────────────────────────────────
code=$(request GET "$BASE_URL/api/v1/account" || true)
if [ "$code" = "401" ]; then
	pass "GET /account 401 without a session"
else
	fail "account-anon" "expected 401, got $code"
fi
code=$(request GET "$BASE_URL/api/v1/account" -b "$USER_JAR" || true)
if [ "$code" = "200" ]; then
	pass "GET /account 200 with a session"
else
	fail "account-auth" "expected 200, got $code"
fi
# Sessions surface should answer for the signed-in user (last-seen lands here).
code=$(request GET "$BASE_URL/api/v1/account/sessions" -b "$USER_JAR" || true)
if [ "$code" = "200" ]; then
	pass "GET /account/sessions 200 (active sessions surfaced)"
else
	fail "account-sessions" "expected 200, got $code"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 9. Admin RBAC — the seed admin reaches /admin/overview (200); the un-privileged
#    signed-in smoke user and an anonymous caller are refused (403 / 401).
# ─────────────────────────────────────────────────────────────────────────────
ACSRF=$(fresh_csrf "$ADMIN_JAR")
code=$(request POST "$BASE_URL/api/v1/auth/login" \
	-c "$ADMIN_JAR" -b "$ADMIN_JAR" \
	-H "Content-Type: application/json" -H "X-CSRF-Token: $ACSRF" \
	--data "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" || true)
if [ "$code" = "200" ]; then
	pass "admin login 200 (seed operator)"
	ADMIN_OK=1
else
	fail "admin-login" "expected 200 (is the dev seed loaded?), got $code"
	ADMIN_OK=0
fi

if [ "$ADMIN_OK" = "1" ]; then
	code=$(request GET "$BASE_URL/api/v1/admin/overview" -b "$ADMIN_JAR" || true)
	if [ "$code" = "200" ]; then
		pass "admin /admin/overview 200 (operator authorized)"
	else
		fail "admin-overview" "expected 200, got $code"
	fi
fi

# Un-privileged signed-in user → 403.
code=$(request GET "$BASE_URL/api/v1/admin/overview" -b "$USER_JAR" || true)
if [ "$code" = "403" ]; then
	pass "non-admin /admin/overview 403 (RBAC denies the ordinary user)"
else
	fail "rbac-403" "expected 403 for the non-admin user, got $code"
fi

# Anonymous caller → 401.
code=$(request GET "$BASE_URL/api/v1/admin/overview" || true)
if [ "$code" = "401" ]; then
	pass "anonymous /admin/overview 401"
else
	fail "rbac-401" "expected 401 anonymous, got $code"
fi

# ─────────────────────────────────────────────────────────────────────────────
# 10. Audit Journal — a lifecycle action appears in /admin/audit. We run a
#     suspend on the smoke user (an admin can act on a non-owner) and then assert
#     the action surfaces in the Journal filtered by action=user.suspended.
# ─────────────────────────────────────────────────────────────────────────────
if [ "$ADMIN_OK" = "1" ]; then
	# Find the smoke user's id via the admin user search.
	code=$(request GET "$BASE_URL/api/v1/admin/users?query=$USER_NAME&page=1&pageSize=20" -b "$ADMIN_JAR" || true)
	TID=$(json_field id)
	if [ "$code" = "200" ] && [ -n "$TID" ]; then
		ACSRF=$(fresh_csrf "$ADMIN_JAR")
		code=$(request POST "$BASE_URL/api/v1/admin/users/$TID/suspend" \
			-b "$ADMIN_JAR" -H "X-CSRF-Token: $ACSRF" || true)
		if [ "$code" = "200" ] || [ "$code" = "204" ]; then
			pass "admin lifecycle action: suspend the smoke user ($code)"
		else
			fail "admin-suspend" "expected 200/204, got $code ($(json_field detail))"
		fi

		# The suspend must be visible in the Journal (audit action: admin.user.suspend).
		code=$(request GET "$BASE_URL/api/v1/admin/audit?action=admin.user.suspend&page=1&pageSize=10" -b "$ADMIN_JAR" || true)
		if [ "$code" = "200" ] && grep -q 'admin.user.suspend' "$BODY"; then
			pass "audit Journal shows the admin.user.suspend lifecycle action"
		else
			fail "audit-journal" "admin.user.suspend not found in /admin/audit (status $code)"
		fi
	else
		fail "admin-find-user" "could not resolve the smoke user id (status $code)"
	fi
fi

# ─────────────────────────────────────────────────────────────────────────────
# Summary + exit code.
# ─────────────────────────────────────────────────────────────────────────────
printf '\n'
if [ "$FAILS" -eq 0 ]; then
	printf '== e2e smoke: ALL STEPS PASSED ==\n'
	exit 0
fi
printf '== e2e smoke: %d STEP(S) FAILED ==\n' "$FAILS"
exit 1
