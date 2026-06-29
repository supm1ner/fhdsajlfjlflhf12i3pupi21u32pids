# SSO ↔ Messenger integration (M1)

This document describes the **single sign-on** path implemented in milestone **M1** of
[`ROADMAP.md`](../ROADMAP.md): logging into the Sunrise messenger with an identity from
**cotton-id** (the OpenID Connect provider in [`sso/`](../sso), fronted by Ory Hydra).

cotton-id owns *identity*; Sunrise owns *messaging*. The bridge is a native OIDC auth
scheme on the Sunrise backend that validates the IdP's ID token — the messenger never
sees a password and the IdP never sees a Sunrise session.

## What was implemented

| Component | Change |
|---|---|
| Backend (`chat/server/auth/oidc/`) | New pluggable auth scheme **`oidc`** that validates an ID token (JWT) against the IdP's JWKS, checks `iss`/`aud`/`exp`/signature, maps the stable `sub` claim to a local user (auto-provisioning on first login), and lets the existing `token` scheme mint the Sunrise session token. Registered via blank import in `chat/server/main.go`. |
| Backend config (`chat/server/sunrise.conf`) | Documented `auth_config.oidc` block. |
| Frontend (`webapp-svelte/src/lib/oidc.js`) | Browser-side Authorization Code + PKCE flow (public client): `beginLogin`, `completeLogin`, `isRedirectCallback`. |
| Frontend (`webapp-svelte/src/lib/tinode.js`) | `loginWithToken(idToken)` → `login('oidc', idToken)`. |
| Frontend (`webapp-svelte/src/views/Login.svelte`) | "Sign in with SSO" button + redirect-callback handling on mount. |

## End-to-end flow

1. User clicks **Sign in with SSO**. The SPA generates a PKCE verifier/challenge + state,
   and redirects to Hydra: `GET {ISSUER}oauth2/auth?response_type=code&client_id=…&redirect_uri=…&scope=openid profile email&code_challenge=…&code_challenge_method=S256`.
2. Hydra delegates login/consent to cotton-id; the user authenticates there.
3. Hydra redirects back to the SPA with `?code=…&state=…`. The SPA validates `state` and
   exchanges the code at `{ISSUER}oauth2/token` (PKCE, no client secret) for an **ID token**.
4. The SPA calls `login('oidc', id_token)` over the messenger's WebSocket/gRPC transport.
5. The Sunrise `oidc` scheme verifies the token against the IdP's JWKS, maps `sub` → local
   user (creating the account on first login), and the `token` scheme issues the Sunrise
   session token. The user is in.

## Configuration

Three values must agree across the SPA, the backend, and the registered OAuth client:

| Value | SPA (`oidc.js`) | Backend (`sunrise.conf` → `oidc`) | Hydra OAuth client |
|---|---|---|---|
| Issuer | `ISSUER` | `issuer` | Hydra public URL |
| Client id | `CLIENT_ID` | `client_id` (default audience) | client `client_id` |
| Redirect URI | `REDIRECT_URI` | — | client `redirect_uris` |

Enable the scheme by adding `oidc` to `auth_config.logical_names`, e.g.
`["basic:basic", "token:token", "oidc:oidc"]`.

### Registering the messenger as a relying party

Register an OAuth client with Hydra (see `sso/docs/RUNBOOK.md`). For the SPA (public client,
PKCE, no secret):

```bash
# Against Hydra's admin API (default :4445)
hydra create oauth2-client \
  --name "Sunrise Messenger" \
  --grant-type authorization_code,refresh_token \
  --response-type code \
  --scope openid,profile,email \
  --redirect-uri http://localhost:5173/ \
  --token-endpoint-auth-method none \
  --endpoint http://localhost:4445
```

Use the returned `client_id` as `CLIENT_ID` / `client_id` above. The browser token exchange
requires Hydra to allow CORS from the SPA origin for the public OAuth2 endpoints.

## Notes & limits (M1)

- **Account mapping** uses the IdP `sub` (stable, opaque). Email is stored as a searchable
  tag when `add_to_tags` is on.
- **Signature algorithms**: RS/ES/PS 256/384/512. Symmetric (`HS*`) and `none` are rejected.
- **JWKS** is fetched from the issuer's discovery document and cached (`jwks_refresh`, default
  1h), with an automatic refresh on an unknown `kid`.
- The native `oidc` scheme is Variant B from the roadmap. Variant A (the `rest` scheme + a thin
  adapter) remains available without backend changes if a deployment prefers it.
