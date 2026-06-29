# Social login setup

cotton-id can let users sign in with an external identity provider — **Google, GitHub, VK
(vk.com), and Yandex (ya.ru)** — in addition to email/password. This guide is for operators:
how to register an OAuth application with each provider and which environment variables to set.

Social login is part of the `add-social-login` change. The protocol details (find-or-link by
verified email, the start/callback flow, the `social_identities` model) live in
[`openspec/changes/add-social-login/`](../openspec/changes/add-social-login/); this doc covers
**only the external setup**.

> **Apple is intentionally not supported.** Apple Sign-In requires a paid Apple Developer
> account and a client-secret-as-JWT flow; it is deferred. There is no Apple button and no Apple
> configuration.

---

## How it works (operator's view)

Each provider is **independently configurable** and **disabled by default**. A provider becomes
**enabled** only when *both* of its credentials are set:

| Provider | Client id var | Client secret var |
|---|---|---|
| Google | `SOCIAL_GOOGLE_CLIENT_ID` | `SOCIAL_GOOGLE_CLIENT_SECRET` |
| GitHub | `SOCIAL_GITHUB_CLIENT_ID` | `SOCIAL_GITHUB_CLIENT_SECRET` |
| VK | `SOCIAL_VK_CLIENT_ID` | `SOCIAL_VK_CLIENT_SECRET` |
| Yandex | `SOCIAL_YANDEX_CLIENT_ID` | `SOCIAL_YANDEX_CLIENT_SECRET` |

When a provider's vars are empty:

- the SPA **hides** that provider's button (only enabled providers are advertised), and
- its `start` / `callback` endpoints return a `400` "provider not enabled" error.

So you can turn providers on one at a time — set the two vars, restart the backend, done.

### The redirect URI (the same shape for every provider)

Every provider asks you to register an **authorized redirect URI** (also called callback URL,
return URL, or redirect URL). For cotton-id it is always:

```
<PUBLIC_BASE_URL>/api/v1/auth/social/<provider>/callback
```

where `<PUBLIC_BASE_URL>` is the backend's externally-reachable base URL (the `PUBLIC_BASE_URL`
env var — see [dev/build-contract.md](dev/build-contract.md) §5) and `<provider>` is the lowercase
provider slug: `google`, `github`, `vk`, or `yandex`.

Concrete examples:

| Environment | `PUBLIC_BASE_URL` | Google redirect URI |
|---|---|---|
| Local dev | `http://localhost:8080` | `http://localhost:8080/api/v1/auth/social/google/callback` |
| Production | `https://id.example.com` | `https://id.example.com/api/v1/auth/social/google/callback` |

The full set for a host at `https://id.example.com`:

```
https://id.example.com/api/v1/auth/social/google/callback
https://id.example.com/api/v1/auth/social/github/callback
https://id.example.com/api/v1/auth/social/vk/callback
https://id.example.com/api/v1/auth/social/yandex/callback
```

The redirect URI you register at the provider **must match exactly** (scheme, host, port, path,
no trailing slash) — providers reject mismatches. Most providers require **https** for the
redirect URI in production; `http://localhost` is accepted for local development.

---

## Where to put the credentials

- **Docker Compose deployment:** put the values in `deploy/.env` (copied from
  `deploy/.env.example`). Compose passes them into the backend service. `deploy/.env` is
  gitignored — never commit real secrets. After editing, recreate the backend:

  ```bash
  docker compose -f deploy/docker-compose.yml up -d --force-recreate backend
  ```

- **Local backend (running the Go binary directly):** put them in `backend/.env` (copied from
  `backend/.env.example`) or export them in your shell before starting the backend.

Client **secrets** are sensitive credentials. Treat them like any other secret (see
[SECURITY.md](SECURITY.md)): keep them out of version control, rotate them at the provider if
leaked.

---

## Provider setup

For each provider, register an OAuth app, copy its credentials into the two env vars, and add the
redirect URI above. Set the scopes listed so cotton-id can read the user's identity (email is what
account linking keys on).

### Google (OIDC)

1. Go to the [Google Cloud Console](https://console.cloud.google.com/) and select or create a
   project.
2. **APIs & Services → OAuth consent screen:** configure the consent screen (user type
   *External* for public sign-in, app name, support email). Add the `email`, `profile`, and
   `openid` scopes. Publish the app (or add test users while it's in testing).
3. **APIs & Services → Credentials → Create Credentials → OAuth client ID.**
   - Application type: **Web application**.
   - **Authorized redirect URIs:** add
     `<PUBLIC_BASE_URL>/api/v1/auth/social/google/callback`.
4. Copy the generated **Client ID** and **Client secret**.

   - `SOCIAL_GOOGLE_CLIENT_ID` = Client ID
   - `SOCIAL_GOOGLE_CLIENT_SECRET` = Client secret

**Scopes:** `openid email profile`.

### GitHub

1. Go to **GitHub → Settings → Developer settings → OAuth Apps →
   [New OAuth App](https://github.com/settings/developers)** (for an organization, use the org's
   Developer settings instead).
2. Fill in:
   - **Application name:** e.g. `cotton-id`.
   - **Homepage URL:** your `PUBLIC_BASE_URL` (or the product site).
   - **Authorization callback URL:** `<PUBLIC_BASE_URL>/api/v1/auth/social/github/callback`.
3. Register the app, then **Generate a new client secret**.
4. Copy the **Client ID** and the generated **client secret**.

   - `SOCIAL_GITHUB_CLIENT_ID` = Client ID
   - `SOCIAL_GITHUB_CLIENT_SECRET` = client secret

**Scopes:** `read:user user:email` — `user:email` is required because GitHub returns the user's
email addresses from a separate endpoint, and cotton-id uses the **primary, verified** address for
account linking.

### VK (vk.com / VK ID)

1. Create an application at **[VK ID / VK for Developers](https://id.vk.com/)** (the
   [dev.vk.com](https://dev.vk.com/) developer portal). Create a **Website** / web application.
2. In the app settings, set the site/base address to your `PUBLIC_BASE_URL` and add the
   **authorized redirect URI** (Trusted redirect URI):
   `<PUBLIC_BASE_URL>/api/v1/auth/social/vk/callback`.
3. Note the app's **Application ID** (client id) and the **secure key** (client secret /
   protected key).

   - `SOCIAL_VK_CLIENT_ID` = Application ID
   - `SOCIAL_VK_CLIENT_SECRET` = secure key

**Scopes:** request the **`email`** scope so VK returns the user's email (VK returns the email in
the token response, and only when the user grants the email permission). Without a granted,
verified email, VK sign-in creates a separate account rather than linking to an existing one.

> VK's OAuth/VK ID endpoints have changed over time (legacy VK OAuth vs. VK ID). Register against
> the **current** VK ID developer portal; the backend connector targets the documented current
> endpoints.

### Yandex (ya.ru)

1. Go to **[Yandex OAuth](https://oauth.yandex.com/)** (`oauth.yandex.com`, or `oauth.yandex.ru`)
   and **Create a new app**.
2. Platform: **Web services**. Set the **Redirect URI / Callback URL** to
   `<PUBLIC_BASE_URL>/api/v1/auth/social/yandex/callback`.
3. Under **Permissions / Data access**, grant access to the login, name, and **email address**
   (e.g. "Access to email address" and "Access to user's name and surname").
4. Save, then copy the app's **ID** and **password**.

   - `SOCIAL_YANDEX_CLIENT_ID` = app ID
   - `SOCIAL_YANDEX_CLIENT_SECRET` = app password

**Scopes:** `login:email login:info` (access to the user's email and basic profile via
`login.yandex.ru/info`).

---

## Verify

After setting a provider's two vars and restarting the backend:

1. The provider should appear in the enabled-providers list the SPA reads:

   ```bash
   curl -s http://localhost:8080/api/v1/auth/social/providers
   ```

   Only configured providers are returned.

2. Visiting the provider's start endpoint should **302-redirect** to the provider's authorization
   page:

   ```bash
   curl -si http://localhost:8080/api/v1/auth/social/google/start | grep -i '^location'
   ```

3. A provider whose vars are empty returns a `400` "provider not enabled" problem+json response
   from `start`, and its button is absent from the SPA.

If a provider rejects the redirect, the most common cause is a **redirect-URI mismatch** — the URI
registered at the provider must equal `<PUBLIC_BASE_URL>/api/v1/auth/social/<provider>/callback`
character-for-character.
