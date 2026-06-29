# Passkeys (WebAuthn)

cotton-id supports **passkeys** — phishing-resistant, passwordless sign-in backed by
[WebAuthn / FIDO2](https://www.w3.org/TR/webauthn-2/). A passkey is a public-key credential bound
to a device's authenticator (Touch ID / Face ID, Windows Hello, a security key, or a synced
platform passkey). The user proves possession of the private key with a biometric or PIN; the
private key never leaves the authenticator and the credential is **scoped to cotton-id's domain**,
so it cannot be replayed against a look-alike phishing site. This is the strong-authentication
story that offsets password-only accounts (TOTP is deliberately deferred).

Passkeys are part of the `add-passkey-auth` change. The protocol details (the two ceremonies, the
`webauthn_credentials` model, the signed `cid_wa` ceremony cookie, the Hydra `login_challenge`
continuation) live in
[`openspec/changes/add-passkey-auth/`](../openspec/changes/add-passkey-auth/); this doc covers the
**operator's view**: what passkeys do here, the relying-party configuration, the lost-passkey
fallback, and the clone-detection behavior.

---

## What passkeys do in cotton-id

- **Register** (signed-in users): from the **Passkeys** page a signed-in user adds a credential
  for the current device. cotton-id stores the credential's public key, id, sign counter,
  transports, and a user-supplied nickname — bound to that account. The private key stays on the
  authenticator.
- **Passwordless sign-in:** on the login screen the passkey button runs the WebAuthn
  authentication ceremony and, on a valid assertion, establishes a normal cotton-id session — no
  password. Both **username-first** (type your email, then authenticate) and **discoverable /
  usernameless** (just pick a passkey) flows are supported.
- **Continue an OIDC login:** exactly like password and social login, a passkey sign-in that
  carries a Hydra `login_challenge` accepts the challenge and returns the browser to the relying
  party — the same `sub` (account UUID) the user always has.
- **Manage:** users list and remove their own passkeys; a credential removed here can no longer
  sign in.

The passkey button is **hidden automatically** on browsers that don't expose
`window.PublicKeyCredential`, so it never appears where it can't work.

---

## Relying-party (RP) configuration

WebAuthn binds every credential to a **relying party** — cotton-id. The RP is described by three
environment variables (see [dev/build-contract.md](dev/build-contract.md) §5 for the var registry):

| Variable | What it is | Dev default |
|---|---|---|
| `WEBAUTHN_RP_ID` | The **registrable domain** that owns the passkeys. | `localhost` |
| `WEBAUTHN_RP_DISPLAY_NAME` | Human-readable RP name some authenticators show at registration. | `cotton-id` |
| `WEBAUTHN_RP_ORIGINS` | Comma-separated **allowed origins** (scheme + host + port) the ceremony may run from. | `http://localhost:3000` |

**The one rule that matters:** the **RP ID must be a registrable suffix of every origin** in
`WEBAUTHN_RP_ORIGINS`. The browser computes the origin's effective domain and refuses the ceremony
if the RP ID isn't a suffix of it. Concretely:

| `WEBAUTHN_RP_ID` | A valid origin | An **invalid** origin |
|---|---|---|
| `localhost` | `http://localhost:3000` | `http://127.0.0.1:3000` (not a suffix of `localhost`) |
| `id.example.com` | `https://id.example.com` | `https://example.com` (RP ID is *narrower* than the origin) |
| `example.com` | `https://id.example.com`, `https://example.com` | `https://example.org` |

Notes:

- The RP ID is a **bare host**, never a URL — no scheme, no port, no path (`id.example.com`, not
  `https://id.example.com:443`).
- Origins **do** include scheme and port and must match the SPA origin **exactly** (the browser
  compares the full origin). List every origin the SPA is served from.
- A credential is **only usable under the RP ID it was registered with.** Changing `WEBAUTHN_RP_ID`
  later invalidates all existing passkeys (users must re-register). Pick the production RP ID once.

### Dev vs. production

The defaults ship for local development against the SPA at `http://localhost:3000`:

```
WEBAUTHN_RP_ID=localhost
WEBAUTHN_RP_DISPLAY_NAME=cotton-id
WEBAUTHN_RP_ORIGINS=http://localhost:3000
```

**For production you MUST set the real values:**

```
WEBAUTHN_RP_ID=id.example.com
WEBAUTHN_RP_DISPLAY_NAME=cotton-id
WEBAUTHN_RP_ORIGINS=https://id.example.com
```

- Use the **registrable domain** the SPA is actually served from for `WEBAUTHN_RP_ID`.
- Use the **https** origin(s) for `WEBAUTHN_RP_ORIGINS` — WebAuthn requires a secure context in
  production (only `http://localhost` is exempt). This pairs with `COOKIE_SECURE=true`.
- If you serve the SPA from more than one origin (e.g. an apex and a `www`/sub-host), list them all,
  comma-separated, and choose an RP ID that is a suffix of every one of them.

> **A mismatched RP ID or origin breaks every passkey ceremony** — registration and login both fail
> in the browser before cotton-id is even asked to verify. If passkeys "don't work" in a new
> environment, check these three vars first.

### Where to set them

- **Docker Compose deployment:** set them in `deploy/.env` (copied from `deploy/.env.example`).
  Compose passes them into the backend (`WEBAUTHN_RP_ID`, `WEBAUTHN_RP_DISPLAY_NAME`,
  `WEBAUTHN_RP_ORIGINS`, each with the dev default above). After editing, recreate the backend:

  ```bash
  docker compose -f deploy/docker-compose.yml up -d --force-recreate backend
  ```

- **Local backend (running the Go binary directly):** set them in `backend/.env` (copied from
  `backend/.env.example`) or export them in your shell before starting the backend.

None of these are secrets — they describe public domain configuration — so they have safe dev
defaults and are committed in the `.env.example` templates.

---

## Lost-all-passkeys fallback (account recovery)

Passkeys are an **additional** sign-in method, not a replacement for the password. If a user loses
every passkey (lost or wiped device, no synced credential), they are **not locked out**:

1. They sign in with their **email and password** as before, or
2. if they've also forgotten the password, they use **password reset**
   (`POST /api/v1/auth/password/forgot` → emailed reset link), exactly as documented in
   [SECURITY.md](SECURITY.md) §2.4.

Once back in, they register a fresh passkey from the **Passkeys** page and (optionally) remove the
stale credentials. cotton-id deliberately does **not** implement a separate passkey-recovery flow —
password + reset already cover the lockout case (`add-passkey-auth` design, Non-Goals). Operators
should keep the password-reset (mailer) path working as the recovery backstop.

---

## Clone-detection behavior (sign counter)

Each authenticator maintains a **signature counter** that increments on every assertion. cotton-id
stores the last counter it saw for each credential and, on every passkey sign-in, compares the
counter the authenticator returns:

- A **strictly increasing** counter is normal → the new value is stored and sign-in proceeds.
- A counter that is **less than or equal** to the stored value (when the authenticator uses
  counters) is treated as a **possible cloned authenticator**: cotton-id **refuses the
  authentication** and records a structured **security event**, per
  [WebAuthn guidance](https://www.w3.org/TR/webauthn-2/#sctn-sign-counter).

This catches a private key that was extracted and replayed from a copy of the authenticator. Some
authenticators (notably many **synced** passkeys) report a counter of `0` and never increment it;
that is allowed — the regression check only fires for authenticators that actually use counters.

If a legitimate user hits a clone-detection refusal (rare — e.g. an authenticator firmware quirk),
the remedy is to **remove that passkey** from the Passkeys page (after signing in via password) and
**register a new one**.

---

## Verify

After setting the RP vars and restarting the backend:

1. Begin a registration ceremony (requires a signed-in session + CSRF token) and confirm the
   returned options carry your configured RP ID — e.g. the `relyingParty.id` (a.k.a. `rp.id`) field
   equals `WEBAUTHN_RP_ID`.
2. On the login screen, the **passkey button renders** in a WebAuthn-capable browser and is hidden
   in one without `window.PublicKeyCredential`.
3. A full register-then-login round-trip succeeds from an allowed origin; a ceremony attempted from
   an origin **not** in `WEBAUTHN_RP_ORIGINS` is rejected.

If a ceremony fails in the browser with a security/origin error, the cause is almost always an
**RP-ID ↔ origin mismatch** — re-check the three `WEBAUTHN_*` vars against the origin the SPA is
actually served from.
