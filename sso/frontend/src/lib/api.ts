/* ============================================================================
 * api.ts — typed fetch client for the cotton-id backend
 *
 * Contract (build-contract §3, §7):
 *  - All requests are same-origin and send cookies: `credentials: 'include'`.
 *  - State-changing requests carry `X-CSRF-Token` whose value equals the token
 *    returned by `GET /api/v1/csrf` (which also sets the `cid_csrf` cookie —
 *    double-submit). The token is cached and lazily (re)fetched.
 *  - Errors are RFC 7807 `application/problem+json`; we parse them into a typed
 *    `ApiError` so callers can branch on `status` / `detail` without string-matching.
 *  - JSON bodies are camelCase.
 * ==========================================================================*/

import type {
  CredentialCreationOptionsJSON,
  CredentialRequestOptionsJSON,
  PublicKeyCredentialWithAssertionJSON,
  PublicKeyCredentialWithAttestationJSON,
} from '@github/webauthn-json';

/**
 * The base64url-encoded `PublicKeyCredential{Creation,Request}Options` JSON the
 * backend returns under `publicKey`. `@github/webauthn-json` only names the outer
 * `Credential*OptionsJSON` wrappers publicly, so we index out the `publicKey` shape
 * — it is exactly what `create({ publicKey })` / `get({ publicKey })` consume.
 */
export type PublicKeyCredentialCreationOptionsJSON = CredentialCreationOptionsJSON['publicKey'];
export type PublicKeyCredentialRequestOptionsJSON = NonNullable<
  CredentialRequestOptionsJSON['publicKey']
>;

const BASE = '/api/v1';

/** Public user shape returned by the auth endpoints (camelCase per contract). */
export interface User {
  id: string;
  email: string;
  emailVerified: boolean;
  username: string;
  displayName: string;
  role: string;
  about?: string;
  location?: string;
  // The backend's PublicUser projection (internal/auth.PublicUser) does not emit
  // status/createdAt/updatedAt on the auth/consent responses, so these are
  // optional here to match the wire shape rather than imply a value that is
  // never sent. Later changes (account self-service) may surface them.
  status?: string;
  createdAt?: string;
  updatedAt?: string;
}

/** RFC 7807 problem document. */
export interface ProblemJson {
  type?: string;
  title?: string;
  status?: number;
  detail?: string;
  instance?: string;
  /** Optional field-level validation errors some handlers may include. */
  errors?: Record<string, string>;
  /** Optional field name for field-level validation errors. */
  field?: string;
}

/** Typed error thrown for any non-2xx response (or transport failure). */
export class ApiError extends Error {
  readonly status: number;
  readonly problem: ProblemJson;

  constructor(status: number, problem: ProblemJson) {
    super(problem.detail || problem.title || `Request failed (${status})`);
    this.name = 'ApiError';
    this.status = status;
    this.problem = problem;
  }

  /** Field-level errors, when the backend supplied them. */
  get fieldErrors(): Record<string, string> {
    return this.problem.errors ?? {};
  }
}

const MUTATING = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

/** Cached CSRF token from the most recent `GET /api/v1/csrf`. */
let csrfToken: string | null = null;

/** Clear the cached CSRF token. Exported for tests; harmless in production. */
export function __resetCsrfCache(): void {
  csrfToken = null;
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  /** Skip CSRF token injection (used by the csrf fetch itself). */
  skipCsrf?: boolean;
  signal?: AbortSignal;
}

/**
 * Fetch (and cache) the CSRF token. The backend also sets the `cid_csrf` cookie;
 * the header value must equal that cookie (double-submit). Safe to call repeatedly.
 */
export async function fetchCsrfToken(force = false): Promise<string> {
  if (csrfToken && !force) return csrfToken;
  const res = await fetch(`${BASE}/csrf`, {
    method: 'GET',
    credentials: 'include',
    headers: { Accept: 'application/json' },
  });
  if (!res.ok) {
    throw await toApiError(res);
  }
  const data = (await res.json()) as { token: string };
  csrfToken = data.token;
  return csrfToken;
}

/** Parse a non-OK response into an ApiError, tolerating non-JSON bodies. */
async function toApiError(res: Response): Promise<ApiError> {
  let problem: ProblemJson = { status: res.status, title: res.statusText };
  const contentType = res.headers.get('content-type') ?? '';
  if (contentType.includes('json')) {
    try {
      problem = { ...problem, ...((await res.json()) as ProblemJson) };
    } catch {
      /* malformed JSON — keep the status/title fallback */
    }
  }
  return new ApiError(res.status, problem);
}

/** Core request helper: credentials, CSRF, problem+json handling, typed result. */
async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = (options.method ?? 'GET').toUpperCase();
  const headers: Record<string, string> = { Accept: 'application/json' };

  if (options.body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }

  // Inject the CSRF token on mutating requests (contract §3). The token is
  // (re)fetched if we don't have it cached yet.
  if (MUTATING.has(method) && !options.skipCsrf) {
    headers['X-CSRF-Token'] = await fetchCsrfToken();
  }

  const doFetch = (): Promise<Response> =>
    fetch(`${BASE}${path}`, {
      method,
      credentials: 'include',
      headers,
      ...(options.body !== undefined ? { body: JSON.stringify(options.body) } : {}),
      ...(options.signal ? { signal: options.signal } : {}),
    });

  let res = await doFetch();

  // If a mutation is rejected for CSRF reasons (403), the cached token may be
  // stale (e.g. cookie rotated). Refresh once and retry transparently.
  if (res.status === 403 && MUTATING.has(method) && !options.skipCsrf) {
    headers['X-CSRF-Token'] = await fetchCsrfToken(true);
    res = await doFetch();
  }

  if (!res.ok) {
    throw await toApiError(res);
  }

  // 204 No Content (logout, reset) — nothing to parse.
  if (res.status === 204 || res.headers.get('content-length') === '0') {
    return undefined as T;
  }
  const contentType = res.headers.get('content-type') ?? '';
  if (!contentType.includes('json')) {
    return undefined as T;
  }
  return (await res.json()) as T;
}

/* ----------------------------- AUTH ----------------------------- */

export interface SignupInput {
  displayName: string;
  username: string;
  email: string;
  password: string;
}
export interface LoginInput {
  email: string;
  password: string;
  remember: boolean;
}

export const api = {
  /** Eagerly establish the CSRF cookie/token (call once on app start). */
  csrf: (): Promise<string> => fetchCsrfToken(true),

  signup: (input: SignupInput): Promise<{ user: User }> =>
    request('/auth/signup', { method: 'POST', body: input }),

  sendSignupCode: (email: string): Promise<{ message: string; code?: string }> =>
    request('/auth/signup/send-code', { method: 'POST', body: { email } }),

  verifySignupCode: (email: string, code: string): Promise<{ valid: boolean }> =>
    request('/auth/signup/verify-code', { method: 'POST', body: { email, code } }),

  login: (input: LoginInput): Promise<{ user: User }> =>
    request('/auth/login', { method: 'POST', body: input }),

  logout: (): Promise<void> => request('/auth/logout', { method: 'POST' }),

  session: (signal?: AbortSignal): Promise<{ user: User }> =>
    request('/auth/session', signal ? { signal } : {}),

  forgotPassword: (email: string): Promise<{ message: string }> =>
    request('/auth/password/forgot', { method: 'POST', body: { email } }),

  resetPassword: (token: string, password: string): Promise<void> =>
    request('/auth/password/reset', { method: 'POST', body: { token, password } }),

  /* ----------------------------- OIDC handshake ----------------------------- */

  /** Step 3 of the handshake: accept the Hydra login challenge. */
  acceptLogin: (loginChallenge: string): Promise<{ redirectTo: string }> =>
    request('/oauth/login/accept', { method: 'POST', body: { loginChallenge } }),

  /**
   * Cancel an in-progress sign-in: rejects the Hydra login challenge so the
   * relying party receives `access_denied` instead of the flow hanging open.
   */
  rejectLogin: (loginChallenge: string): Promise<{ redirectTo: string }> =>
    request('/oauth/login/reject', { method: 'POST', body: { loginChallenge } }),

  /** Step 5: fetch the consent request (client + requested scopes). */
  getConsent: (consentChallenge: string, signal?: AbortSignal): Promise<ConsentInfo> =>
    request(`/oauth/consent?consent_challenge=${encodeURIComponent(consentChallenge)}`, {
      ...(signal ? { signal } : {}),
    }),

  acceptConsent: (
    consentChallenge: string,
    grantScopes: string[],
    remember: boolean,
  ): Promise<{ redirectTo: string }> =>
    request('/oauth/consent/accept', {
      method: 'POST',
      body: { consentChallenge, grantScopes, remember },
    }),

  rejectConsent: (consentChallenge: string): Promise<{ redirectTo: string }> =>
    request('/oauth/consent/reject', { method: 'POST', body: { consentChallenge } }),
};

/** Shape of `GET /api/v1/oauth/consent` (contract §3). */
export interface ConsentInfo {
  client: { id: string; name: string };
  requestedScopes: string[];
  user: User;
}

/* ----------------------------- SOCIAL LOGIN ----------------------------- */

/** Supported external identity providers (Apple removed — add-social-login). */
export type SocialProviderId = 'google' | 'github' | 'vk' | 'yandex';

/** One enabled social provider as advertised by the backend. */
export interface SocialProvider {
  id: SocialProviderId;
  name: string;
}

/** Shape of `GET /api/v1/auth/social/providers` — only configured providers. */
export interface SocialProvidersResponse {
  providers: SocialProvider[];
}

/**
 * List the social providers the backend has credentials for. The SPA renders a
 * button only for each returned provider (graceful degradation: an unconfigured
 * provider is simply absent). Optionally abortable for unmount-safety.
 */
export function getSocialProviders(signal?: AbortSignal): Promise<SocialProvidersResponse> {
  return request<SocialProvidersResponse>('/auth/social/providers', signal ? { signal } : {});
}

/**
 * Kick off a social-login flow. The caller navigates the browser to this URL and
 * the backend drives the OAuth redirect (state/PKCE → provider authorization).
 */
export function socialStartUrl(provider: SocialProviderId): string {
  return `${BASE}/auth/social/${provider}/start`;
}

/* ----------------------------- PASSKEYS (WebAuthn) ----------------------------- */

/*
 * WebAuthn JSON contract (add-passkey-auth design §D4/§D6):
 *  - The `begin` endpoints return the WebAuthn *options* (a base64url-encoded JSON
 *    `PublicKeyCredentialCreationOptions` / `...RequestOptions`) under `publicKey`
 *    and set the short-lived signed `cid_wa` ceremony cookie server-side.
 *  - The browser hands those options to `@github/webauthn-json` `create()` / `get()`
 *    (navigator.credentials.{create,get} with ArrayBuffer↔base64url handled for us)
 *    and posts the resulting credential JSON to the matching `finish` endpoint.
 *
 * We re-use the library's `PublicKeyCredential*OptionsJSON` and
 * `PublicKeyCredentialWith{Attestation,Assertion}JSON` types so the wire shape is
 * exactly what `create()` / `get()` consume and produce — no hand-rolled encoding.
 * (The library types are imported at the top of this module.)
 */

/** One stored passkey as listed by `GET /api/v1/passkeys` (camelCase per contract). */
export interface Passkey {
  id: string;
  name: string;
  createdAt: string;
  /** ISO timestamp of the last successful authentication; absent if never used. */
  lastUsedAt?: string;
  /** Authenticator transports advertised at registration (e.g. `internal`, `hybrid`). */
  transports?: string[];
}

/** `GET /api/v1/passkeys` response. */
export interface PasskeysResponse {
  passkeys: Passkey[];
}

/**
 * `POST /api/v1/passkeys/register/begin` response: the creation options to hand to
 * `@github/webauthn-json` `create({ publicKey })`. The server also sets `cid_wa`.
 */
export interface PasskeyRegisterBeginResponse {
  publicKey: PublicKeyCredentialCreationOptionsJSON;
}

/**
 * `POST /api/v1/auth/passkey/login/begin` response: the request options to hand to
 * `@github/webauthn-json` `get({ publicKey })`. The server also sets `cid_wa`.
 */
export interface PasskeyLoginBeginResponse {
  publicKey: PublicKeyCredentialRequestOptionsJSON;
}

/**
 * `POST /api/v1/auth/passkey/login/finish` result. When the login was continuing an
 * in-progress OIDC flow (a `loginChallenge` was carried into begin), the backend
 * accepts the Hydra challenge and returns `redirectTo`; otherwise it returns the
 * signed-in `user` (a plain passwordless sign-in).
 */
export interface PasskeyLoginFinishResponse {
  redirectTo?: string;
  user?: User;
}

export const passkeyApi = {
  /**
   * Begin registration for the signed-in user. Returns creation options whose
   * `excludeCredentials` already lists the account's existing passkeys.
   */
  registerBegin: (): Promise<PasskeyRegisterBeginResponse> =>
    request('/passkeys/register/begin', { method: 'POST', body: {} }),

  /**
   * Finish registration: post the attestation credential produced by `create()`
   * plus the user-chosen nickname. The backend verifies + stores the credential.
   */
  registerFinish: (
    credential: PublicKeyCredentialWithAttestationJSON,
    name: string,
  ): Promise<{ passkey: Passkey }> =>
    request('/passkeys/register/finish', { method: 'POST', body: { credential, name } }),

  /** List the signed-in user's passkeys (never another account's). */
  list: (signal?: AbortSignal): Promise<PasskeysResponse> =>
    request('/passkeys', signal ? { signal } : {}),

  /** Remove one of the signed-in user's passkeys by id. */
  delete: (id: string): Promise<void> =>
    request(`/passkeys/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  /**
   * Begin passwordless login. With `email` the backend issues an allow-list (the
   * account's credentials); omit it for a discoverable (usernameless) ceremony.
   * `loginChallenge` carries an in-progress OIDC flow so `finish` can continue it.
   */
  loginBegin: (email?: string, loginChallenge?: string): Promise<PasskeyLoginBeginResponse> =>
    request('/auth/passkey/login/begin', {
      method: 'POST',
      body: {
        ...(email ? { email } : {}),
        ...(loginChallenge ? { loginChallenge } : {}),
      },
    }),

  /**
   * Finish passwordless login: post the assertion produced by `get()`. On success a
   * session is established; `redirectTo` (OIDC continuation) or `user` is returned.
   *
   * The in-progress `loginChallenge` is NOT sent here — it was captured into the
   * signed `cid_wa` ceremony cookie by `loginBegin` and the backend reads it from
   * there (add-passkey-auth §D3/§D4). The finish body carries only the credential;
   * the backend's strict JSON decoder rejects any unknown field.
   */
  loginFinish: (
    credential: PublicKeyCredentialWithAssertionJSON,
  ): Promise<PasskeyLoginFinishResponse> =>
    request('/auth/passkey/login/finish', {
      method: 'POST',
      body: { credential },
    }),
};

/** True when the browser exposes the WebAuthn API (passkeys are usable). */
export function passkeysSupported(): boolean {
  return typeof window !== 'undefined' && typeof window.PublicKeyCredential !== 'undefined';
}

/* ----------------------------- ACCOUNT (self-service) ----------------------------- */

/*
 * Account self-service contract (add-account-self-service proposal §Impact, spec).
 * All endpoints are session-gated and live under `/api/v1/account`; mutating ones
 * are in the CSRF group (the `request` helper injects the token automatically).
 * JSON is camelCase; errors are problem+json parsed into `ApiError`. A 401 from any
 * of these means "no session" — callers redirect to /login.
 */

/** A user's stored UI preferences (D6). */
export interface AccountPreferences {
  /** Persisted theme; `system` defers to the OS (the SPA maps it to dark/light). */
  theme: 'dark' | 'light' | 'system';
  /** Persisted UI language (matches the i18n `Lang` union). */
  lang: 'ru' | 'en';
  /** Email me when a new device signs in (delivery lands with the mailer). */
  loginNotifications: boolean;
}

/** Aggregate counts shown on the profile header (sessions/passkeys/connections). */
export interface AccountCounts {
  sessions: number;
  passkeys: number;
  connections: number;
}

/** Full account payload, flattened for the UI from the backend's `{user,counts}`. */
export interface Account {
  id: string;
  email: string;
  emailVerified: boolean;
  username: string;
  displayName: string;
  role: string;
  status: string;
  about: string;
  location: string;
  /** Served avatar URL (account image route); absent until one is uploaded. */
  avatarUrl?: string;
  /** Served banner URL; absent until one is uploaded. */
  bannerUrl?: string;
  hasPassword: boolean;
  createdAt: string;
  updatedAt: string;
  preferences: AccountPreferences;
  counts: AccountCounts;
}

/*
 * Backend wire shapes (internal/account handlers). The UI consumes the flattened
 * `Account` above; these mirror the actual JSON so we map at the client boundary
 * rather than leak the envelope into the screen. The user view carries the prefs
 * *flat* (`theme`/`lang`/`loginNotifications`); we re-nest them under `preferences`.
 */

/** `userView` — the backend's per-user projection (handlers.go). */
interface UserViewWire {
  id: string;
  email: string;
  emailVerified: boolean;
  username: string;
  displayName: string;
  role: string;
  status: string;
  about: string;
  location: string;
  avatarUrl?: string;
  bannerUrl?: string;
  theme: 'dark' | 'light' | 'system';
  lang: 'ru' | 'en';
  loginNotifications: boolean;
  hasPassword: boolean;
  createdAt: string;
  updatedAt: string;
}

/** `GET /account` body: `{ user, counts }`. */
interface ProfileResponseWire {
  user: UserViewWire;
  counts: AccountCounts;
}

/** `PATCH /account`, `PATCH /account/preferences`, `PUT /account/images/{kind}` body: `{ user }`. */
interface UserEnvelopeWire {
  user: UserViewWire;
}

/** Flatten a backend user view into the UI's profile fields (no counts). */
function flattenUser(u: UserViewWire): Omit<Account, 'counts'> {
  return {
    id: u.id,
    email: u.email,
    emailVerified: u.emailVerified,
    username: u.username,
    displayName: u.displayName,
    role: u.role,
    status: u.status,
    about: u.about,
    location: u.location,
    ...(u.avatarUrl !== undefined ? { avatarUrl: u.avatarUrl } : {}),
    ...(u.bannerUrl !== undefined ? { bannerUrl: u.bannerUrl } : {}),
    hasPassword: u.hasPassword,
    createdAt: u.createdAt,
    updatedAt: u.updatedAt,
    preferences: {
      theme: u.theme,
      lang: u.lang,
      loginNotifications: u.loginNotifications,
    },
  };
}

/**
 * What the profile/preferences PATCH endpoints return: the full account fields
 * minus `counts` (the backend's `{ user }` envelope does not re-send counts, since
 * an edit cannot change session/passkey/connection tallies). Callers merge this
 * with the prior `counts` when replacing account state.
 */
export type UpdatedAccount = Omit<Account, 'counts'>;

/** Editable profile fields (`PATCH /api/v1/account`). */
export interface ProfileUpdate {
  displayName: string;
  about: string;
  location: string;
}

/** One active session (`GET /api/v1/account/sessions`). */
export interface AccountSession {
  id: string;
  /** Raw user-agent string; the UI derives a friendly device label from it. */
  userAgent: string;
  ip: string;
  createdAt: string;
  /** Last time this session authenticated a request (may equal createdAt). */
  lastSeenAt?: string;
  expiresAt: string;
  /** True for the session making this request (cannot be silently revoked). */
  current: boolean;
}

/** `GET /api/v1/account/sessions` response. */
export interface AccountSessionsResponse {
  sessions: AccountSession[];
}

/** One connected app = a Hydra consent grant (`GET /api/v1/account/connections`). */
export interface AccountConnection {
  /** OAuth client id — used as the revoke key. */
  client: string;
  /** Human-readable client name when Hydra supplies one; else falls back to id. */
  clientName: string;
  grantedScopes: string[];
  grantedAt: string;
}

/** `GET /api/v1/account/connections` response (UI-facing, flattened). */
export interface AccountConnectionsResponse {
  connections: AccountConnection[];
}

/*
 * Backend connections wire shape: the client is a nested object `{id,name}` and
 * there is no top-level `clientName`/flat `client`. We flatten it for the UI.
 */
interface ConnectionWire {
  client: { id: string; name?: string };
  grantedScopes?: string[];
  grantedAt?: string;
}
interface ConnectionsResponseWire {
  connections: ConnectionWire[];
}

/** Flatten a backend connection record into the UI's `AccountConnection`. */
function flattenConnection(c: ConnectionWire): AccountConnection {
  const id = c.client?.id ?? '';
  return {
    client: id,
    clientName: c.client?.name || id,
    grantedScopes: c.grantedScopes ?? [],
    grantedAt: c.grantedAt ?? '',
  };
}

/** The two uploadable profile image slots. */
export type ImageKind = 'avatar' | 'banner';

/** `PUT /api/v1/account/images/{kind}` result: the new served URL for that slot. */
export interface ImageUploadResponse {
  url: string;
}

/**
 * Upload an avatar/banner image as `multipart/form-data`. This bypasses the JSON
 * `request` helper (it must NOT set `Content-Type` — the browser sets the multipart
 * boundary) but still injects the CSRF token + credentials and parses problem+json.
 * The field name is `file` (single-file overwrite per D2).
 *
 * The backend responds with the refreshed `{ user }` (handlers.go `userEnvelope`),
 * so we read the kind's served URL back off the updated user rather than expecting
 * a bare `{ url }` (which the backend does not send).
 */
export async function uploadAccountImage(
  kind: ImageKind,
  file: File,
): Promise<ImageUploadResponse> {
  const form = new FormData();
  form.append('file', file);

  const send = async (token: string): Promise<Response> =>
    fetch(`${BASE}/account/images/${kind}`, {
      method: 'PUT',
      credentials: 'include',
      headers: { Accept: 'application/json', 'X-CSRF-Token': token },
      body: form,
    });

  let res = await send(await fetchCsrfToken());
  if (res.status === 403) {
    res = await send(await fetchCsrfToken(true));
  }
  if (!res.ok) {
    throw await toApiError(res);
  }
  const { user } = (await res.json()) as UserEnvelopeWire;
  const url = (kind === 'avatar' ? user.avatarUrl : user.bannerUrl) ?? '';
  return { url };
}

export const accountApi = {
  /** Load the full account (profile + prefs + counts). 401 ⇒ not signed in. */
  get: async (signal?: AbortSignal): Promise<Account> => {
    const res = await request<ProfileResponseWire>('/account', signal ? { signal } : {});
    return { ...flattenUser(res.user), counts: res.counts };
  },

  /**
   * Save editable profile fields; returns the updated account. The backend's
   * `{ user }` response omits counts (an edit cannot change them), so the caller
   * should merge with the prior counts (or rely on the next `get`).
   */
  updateProfile: async (input: ProfileUpdate): Promise<UpdatedAccount> => {
    const res = await request<UserEnvelopeWire>('/account', { method: 'PATCH', body: input });
    return flattenUser(res.user);
  },

  /** Persist theme/lang/login-notification preferences; returns the updated user. */
  updatePreferences: async (input: AccountPreferences): Promise<UpdatedAccount> => {
    const res = await request<UserEnvelopeWire>('/account/preferences', {
      method: 'PATCH',
      body: input,
    });
    return flattenUser(res.user);
  },

  /**
   * Change password: verify `currentPassword`, enforce the policy on `newPassword`,
   * rehash, and revoke other sessions (D3). 204 on success.
   */
  changePassword: (currentPassword: string, newPassword: string): Promise<void> =>
    request('/account/password', { method: 'PUT', body: { currentPassword, newPassword } }),

  /** List the user's active sessions with the current one flagged (D4). */
  sessions: (signal?: AbortSignal): Promise<AccountSessionsResponse> =>
    request('/account/sessions', signal ? { signal } : {}),

  /** Revoke one session by id (scoped to the user). */
  revokeSession: (id: string): Promise<void> =>
    request(`/account/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  /** Revoke every session except the current one ("sign out other sessions"). */
  revokeOtherSessions: (): Promise<void> => request('/account/sessions', { method: 'DELETE' }),

  /** List the user's connected apps (Hydra consent grants, D5). */
  connections: async (signal?: AbortSignal): Promise<AccountConnectionsResponse> => {
    const res = await request<ConnectionsResponseWire>(
      '/account/connections',
      signal ? { signal } : {},
    );
    return { connections: (res.connections ?? []).map(flattenConnection) };
  },

  /** Revoke a client's consent for the user's subject (D5). */
  revokeConnection: (client: string): Promise<void> =>
    request(`/account/connections/${encodeURIComponent(client)}`, { method: 'DELETE' }),

  /** Upload an avatar/banner image (multipart). Returns the new served URL. */
  uploadImage: uploadAccountImage,

  /**
   * Permanently delete the account after re-auth (D7). Password accounts must pass
   * their current `password`; social-only accounts pass `confirm: true` instead.
   * Best-effort revokes the subject's Hydra sessions server-side. 204 on success.
   *
   * The backend's `deleteAccountRequest` names the field `currentPassword` (and the
   * decoder rejects unknown fields), so the ergonomic `password` is mapped to the
   * wire field here rather than sent verbatim.
   */
  remove: (reauth: { password?: string; confirm?: boolean }): Promise<void> => {
    const body: { currentPassword?: string; confirm?: boolean } = {};
    if (reauth.password !== undefined) body.currentPassword = reauth.password;
    if (reauth.confirm !== undefined) body.confirm = reauth.confirm;
    return request('/account', { method: 'DELETE', body });
  },
};

/* ----------------------------- ADMIN CONSOLE ----------------------------- */

/*
 * Admin console contract (add-admin-console proposal §Impact, design D3–D5, spec).
 * Every endpoint lives under `/api/v1/admin`, is session-gated AND role-gated
 * (admin/owner) server-side; the SPA additionally gates client-side for UX. JSON is
 * camelCase; errors are problem+json parsed into `ApiError`. A 401 means "no
 * session" (→ /login); a 403 means "not authorized" (→ home / not-authorized note).
 *
 * These wire shapes mirror the backend's projections described in the change docs.
 * The backend `internal/admin` package is built by a sibling slice; the names below
 * pin the contract the UI consumes.
 */

/** Account role rank (`user` < `admin` < `owner`). */
export type Role = 'user' | 'admin' | 'owner';
/** Account lifecycle status. */
export type UserStatus = 'active' | 'invited' | 'suspended';

/** One stat card value for the overview header (D3). */
export interface OverviewStats {
  /** Total accounts. */
  totalUsers: number;
  /** Accounts active today (audit login events / session last-seen fallback). */
  activeToday: number;
  /** Accounts created in the last 7 days. */
  newThisWeek: number;
  /** Registered OAuth services (Hydra client count). */
  services: number;
}

/** One day of the 30-day sign-up series (D3). */
export interface SignupPoint {
  /** ISO date (`YYYY-MM-DD`) for the day bucket. */
  date: string;
  /** Number of sign-ups that day. */
  count: number;
}

/** A compact user row for the recent-sign-ups list (D3). */
export interface RecentUser {
  id: string;
  displayName: string;
  username: string;
  email: string;
  status: UserStatus;
  role: Role;
  createdAt: string;
}

/** One audit entry as surfaced to the Journal + activity feed (D2). */
export interface AuditEntry {
  id: string;
  /** Event timestamp (ISO). */
  ts: string;
  /** Acting account id, when there is one (system events may be null). */
  actorId?: string | null;
  /** Human-readable actor label (username/email or `system`). */
  actorLabel?: string;
  /** Event action key, e.g. `login.ok`, `user.suspend`, `signup`. */
  action: string;
  /** Type of the affected entity, e.g. `user`, `client`, `session`. */
  targetType?: string;
  /** Affected entity id, when applicable. */
  targetId?: string;
  /** Originating IP, when captured. */
  ip?: string;
  /** Correlating request id. */
  requestId?: string;
  /** Optional structured detail. */
  metadata?: Record<string, unknown>;
}

/** `GET /api/v1/admin/overview` response (D3). */
export interface AdminOverview {
  stats: OverviewStats;
  /** 30-day daily sign-up series, oldest-first. */
  signups: SignupPoint[];
  /** Latest N sign-ups (newest-first). */
  recentSignups: RecentUser[];
  /** Latest N audit rows (newest-first). */
  recentActivity: AuditEntry[];
}

/** One row in the users table (D4 projection: incl. connected-services count). */
export interface AdminUserRow {
  id: string;
  displayName: string;
  username: string;
  email: string;
  status: UserStatus;
  role: Role;
  /** Per-user connected-services (consent grants) count. */
  services: number;
  /** Account creation date (ISO). */
  createdAt: string;
}

/** `GET /api/v1/admin/users` response: a page of rows + the total for paging. */
export interface AdminUsersResponse {
  users: AdminUserRow[];
  /** Total matching the filters (for page-count). */
  total: number;
  page: number;
  pageSize: number;
}

/** Filters for the users listing (all optional; omitted ones are not constrained). */
export interface AdminUsersQuery {
  /** Case-insensitive search over username/displayName/email. */
  query?: string;
  status?: UserStatus;
  role?: Role;
  page?: number;
  pageSize?: number;
}

/** One of the target user's active sessions (admin view; D4). */
export interface AdminUserSession {
  id: string;
  userAgent: string;
  ip: string;
  createdAt: string;
  lastSeenAt?: string;
  expiresAt: string;
}

/** One of the target user's connected services (admin view; D4). */
export interface AdminUserConnection {
  client: string;
  clientName: string;
  grantedScopes: string[];
  grantedAt?: string;
}

/** Aggregate per-user counts shown on the detail header. */
export interface AdminUserCounts {
  sessions: number;
  services: number;
  /** Total recorded logins (from the audit log), when available. */
  logins?: number;
}

/** `GET /api/v1/admin/users/{id}` detail (D4). */
export interface AdminUserDetail {
  id: string;
  displayName: string;
  username: string;
  email: string;
  emailVerified: boolean;
  status: UserStatus;
  role: Role;
  about: string;
  location: string;
  avatarUrl?: string;
  createdAt: string;
  updatedAt: string;
  /** Last recorded activity timestamp (ISO), when known. */
  lastActiveAt?: string;
  counts: AdminUserCounts;
  sessions: AdminUserSession[];
  connections: AdminUserConnection[];
  /** Recent audit rows scoped to this user (newest-first). */
  recentActivity: AuditEntry[];
}

/** Filters for the Journal (`GET /api/v1/admin/audit`). */
export interface AdminAuditQuery {
  /** Filter to a single action key. */
  action?: string;
  /** Filter to an actor (id or label substring). */
  actor?: string;
  /** Filter to a target entity type (e.g. `user`, `client`, `session`). */
  targetType?: string;
  /** Filter to a target entity id. */
  targetId?: string;
  /** Inclusive lower bound (ISO date/time). */
  from?: string;
  /** Exclusive upper bound (ISO date/time). */
  to?: string;
  page?: number;
  pageSize?: number;
}

/** `GET /api/v1/admin/audit` response: a page of entries + total. */
export interface AdminAuditResponse {
  entries: AuditEntry[];
  total: number;
  page: number;
  pageSize: number;
}

/** Result of a force-password-reset action (the issued link, when surfaced). */
export interface ResetPasswordResult {
  /** A single-use reset link, when the backend returns one (dev/stub). */
  resetLink?: string;
}

/** Build a query string from a flat record, skipping empty/undefined values. */
function toQuery(params: Record<string, string | number | undefined>): string {
  const usp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '') continue;
    usp.set(k, String(v));
  }
  const s = usp.toString();
  return s ? `?${s}` : '';
}

/*
 * Backend wire shapes (internal/admin dto.go). The UI consumes the flattened/
 * nested shapes declared above (AdminOverview, AdminUserRow, AdminUserDetail);
 * the backend emits a different projection, so we map at the client boundary —
 * the same pattern used for the account API above (flattenUser/flattenConnection)
 * — rather than leak the wire envelope into the screens. Keeping these in sync
 * with dto.go is the integration contract.
 */

/** `adminUserSummary` — the per-user row the backend returns (joinedAt/servicesCount). */
interface AdminUserSummaryWire {
  id: string;
  displayName: string;
  username: string;
  email: string;
  status: UserStatus;
  role: Role;
  joinedAt: string;
  servicesCount: number;
}

/** `adminUserDetail` — the backend's per-user profile (joinedAt, no avatar/counts). */
interface AdminUserDetailWire {
  id: string;
  displayName: string;
  username: string;
  email: string;
  emailVerified: boolean;
  status: UserStatus;
  role: Role;
  about: string;
  location: string;
  joinedAt: string;
  updatedAt: string;
}

/** `sessionView` — the backend's per-session projection (no id / lastSeenAt). */
interface AdminSessionWire {
  userAgent: string;
  ip: string;
  createdAt: string;
  expiresAt: string;
}

/** `overviewResponse` — the backend's flat overview envelope. */
interface OverviewResponseWire {
  totalUsers: number;
  activeToday: number;
  newThisWeek: number;
  services: number;
  signups?: SignupPoint[];
  recentSignups?: AdminUserSummaryWire[];
  recentActivity?: AuditEntry[];
}

/** `usersResponse` — a page of `adminUserSummary` rows. */
interface UsersResponseWire {
  users?: AdminUserSummaryWire[];
  total: number;
  page: number;
  pageSize: number;
}

/** `userDetailResponse` — `{user, sessions, recentActivity, connections:int}`. */
interface UserDetailResponseWire {
  user: AdminUserDetailWire;
  sessions?: AdminSessionWire[];
  recentActivity?: AuditEntry[];
  connections: number;
}

/** Map a backend user summary to the UI's recent-signup row. */
function toRecentUser(u: AdminUserSummaryWire): RecentUser {
  return {
    id: u.id,
    displayName: u.displayName,
    username: u.username,
    email: u.email,
    status: u.status,
    role: u.role,
    createdAt: u.joinedAt,
  };
}

/** Map a backend user summary to the UI's users-table row. */
function toUserRow(u: AdminUserSummaryWire): AdminUserRow {
  return {
    id: u.id,
    displayName: u.displayName,
    username: u.username,
    email: u.email,
    status: u.status,
    role: u.role,
    services: u.servicesCount,
    createdAt: u.joinedAt,
  };
}

export const adminApi = {
  /** Aggregate overview metrics + series + recent lists. 403 ⇒ not authorized. */
  overview: async (signal?: AbortSignal): Promise<AdminOverview> => {
    const res = await request<OverviewResponseWire>('/admin/overview', signal ? { signal } : {});
    return {
      stats: {
        totalUsers: res.totalUsers,
        activeToday: res.activeToday,
        newThisWeek: res.newThisWeek,
        services: res.services,
      },
      signups: res.signups ?? [],
      recentSignups: (res.recentSignups ?? []).map(toRecentUser),
      recentActivity: res.recentActivity ?? [],
    };
  },

  /** List users with search/status/role filters + pagination (D4). */
  users: async (q: AdminUsersQuery = {}, signal?: AbortSignal): Promise<AdminUsersResponse> => {
    const res = await request<UsersResponseWire>(
      `/admin/users${toQuery({
        query: q.query,
        status: q.status,
        role: q.role,
        page: q.page,
        pageSize: q.pageSize,
      })}`,
      signal ? { signal } : {},
    );
    return {
      users: (res.users ?? []).map(toUserRow),
      total: res.total,
      page: res.page,
      pageSize: res.pageSize,
    };
  },

  /** Fetch one user's full detail (profile, sessions, activity, connections). */
  user: async (id: string, signal?: AbortSignal): Promise<AdminUserDetail> => {
    const res = await request<UserDetailResponseWire>(
      `/admin/users/${encodeURIComponent(id)}`,
      signal ? { signal } : {},
    );
    const u = res.user;
    const sessions = (res.sessions ?? []).map((s, i) => ({
      // The backend does not expose a session id (no raw token/id leaks); synthesise
      // a stable per-render key from the index so the UI list can key on it.
      id: `s${i}`,
      userAgent: s.userAgent,
      ip: s.ip,
      createdAt: s.createdAt,
      expiresAt: s.expiresAt,
    }));
    return {
      id: u.id,
      displayName: u.displayName,
      username: u.username,
      email: u.email,
      emailVerified: u.emailVerified,
      status: u.status,
      role: u.role,
      about: u.about,
      location: u.location,
      createdAt: u.joinedAt,
      updatedAt: u.updatedAt,
      counts: {
        sessions: sessions.length,
        // The backend's detail returns a connected-services *count*, not a list
        // (the per-user connections listing lands with the Services tab, Change 6).
        services: res.connections,
      },
      sessions,
      // No connection list on the wire yet — the count is surfaced via counts.services.
      connections: [],
      recentActivity: res.recentActivity ?? [],
    };
  },

  /** Suspend a user (revokes their sessions server-side). Audited. */
  suspendUser: (id: string): Promise<void> =>
    request(`/admin/users/${encodeURIComponent(id)}/suspend`, { method: 'POST', body: {} }),

  /** Reactivate a suspended user. Audited. */
  reactivateUser: (id: string): Promise<void> =>
    request(`/admin/users/${encodeURIComponent(id)}/reactivate`, { method: 'POST', body: {} }),

  /** Change a user's role (owner-only on the server; escalation-guarded). Audited. */
  changeUserRole: (id: string, role: Role): Promise<void> =>
    request(`/admin/users/${encodeURIComponent(id)}/role`, { method: 'PATCH', body: { role } }),

  /** Force a password reset: issues a single-use reset token/link (stub email). */
  resetUserPassword: (id: string): Promise<ResetPasswordResult> =>
    request(`/admin/users/${encodeURIComponent(id)}/reset-password`, {
      method: 'POST',
      body: {},
    }),

  /**
   * Send an admin-composed email to a user (`POST /admin/users/{id}/message`).
   * Delivery is best-effort server-side (a mailer failure is logged but the
   * action still succeeds and is audited). `subject` is optional; `body` is
   * required. Returns the server's confirmation `{ message }`.
   */
  messageUser: (id: string, body: string, subject?: string): Promise<{ message: string }> =>
    request(`/admin/users/${encodeURIComponent(id)}/message`, {
      method: 'POST',
      body: subject ? { subject, body } : { body },
    }),

  /** Delete a user (owner-only on the server; cascade + Hydra revoke). Audited. */
  deleteUser: (id: string): Promise<void> =>
    request(`/admin/users/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  /** Query the audit log for the Journal (filters + pagination). */
  audit: (q: AdminAuditQuery = {}, signal?: AbortSignal): Promise<AdminAuditResponse> =>
    request(
      `/admin/audit${toQuery({
        action: q.action,
        actor: q.actor,
        targetType: q.targetType,
        targetId: q.targetId,
        from: q.from,
        to: q.to,
        page: q.page,
        pageSize: q.pageSize,
      })}`,
      signal ? { signal } : {},
    ),
};

/* ----------------------------- ADMIN SERVICES (clients) ----------------------------- */

/*
 * Client (relying-party) management contract (add-client-consent-management
 * proposal §Impact, design D1–D5, spec). Per design D1 the human console manages
 * clients under `/api/v1/admin/services*` (session + RequireRole(admin) + CSRF) —
 * a DISTINCT path from the machine `/api/v1/admin/clients` (X-Admin-Key) route so
 * chi does not collide two registrations on the same method+path. Both call the
 * same `oidc.HydraClient` CRUD, so behaviour is consistent.
 *
 * The console handlers (internal/admin services.go) reuse the `internal/adminapi`
 * client projection (`adminapi.ClientSummary`) — `clientId`/`name`/`clientType`/
 * `redirectUris`/`scopes`/`grantTypes`/`responseTypes`/`createdAt` — but wrap it in
 * console-specific envelopes: the list is `{services:[...]}` and a single record is
 * `{service}` (the create reply is the bare `{clientId,clientSecret?}`). The UI
 * consumes the flattened `Client` shape below (`{id,name,type,...,consentCount}`);
 * we map at the client boundary, the same pattern as the account/admin APIs above.
 *
 * Secret handling (design D4): the generated `clientSecret` is returned exactly
 * once on create (confidential clients only) and never re-served on subsequent
 * reads — the UI shows it in a copy-once panel and then discards it.
 */

/** A client's auth profile: `public` (PKCE, no secret) or `confidential` (secret). */
export type ClientType = 'public' | 'confidential';

/** One registered relying-party client as surfaced to the Services tab (flattened). */
export interface Client {
  /** OAuth `client_id` (the stable key for get/update/delete). */
  id: string;
  /** Human-readable client name. */
  name: string;
  /** Auth profile (drives the secret / PKCE behaviour). */
  type: ClientType;
  /** Allowed redirect URIs (absolute, no fragment — validated server-side). */
  redirectUris: string[];
  /** Allowed scopes. */
  scopes: string[];
  /** Allowed OAuth grant types (e.g. `authorization_code`, `refresh_token`). */
  grantTypes: string[];
  /** Allowed response types (e.g. `code`). */
  responseTypes: string[];
  /** Registration timestamp (ISO), when Hydra supplies one. */
  createdAt?: string;
  /**
   * Best-effort count of users who have an active consent grant for this client
   * (design D3). `undefined` until lazily fetched per row; the backend returns a
   * best-effort number per Hydra's capability.
   */
  consentCount?: number;
}

/** Body for creating a client (`POST /api/v1/admin/services`). */
export interface CreateClientInput {
  name: string;
  type: ClientType;
  redirectUris: string[];
  scopes: string[];
  grantTypes: string[];
  responseTypes: string[];
}

/**
 * Body for editing a client (`PATCH /api/v1/admin/services/{id}`). The client type
 * is intentionally NOT editable here — flipping public↔confidential changes the
 * auth method + secret and must be an explicit, separate action (design D2/§Risks).
 */
export interface UpdateClientInput {
  name: string;
  redirectUris: string[];
  scopes: string[];
  grantTypes: string[];
  responseTypes: string[];
}

/**
 * Result of creating a client. `secret` is present exactly once for confidential
 * clients (design D4); public (PKCE) clients have no secret so it is absent.
 */
export interface CreatedClient {
  /** The full client record (without a secret). */
  client: Client;
  /** The generated secret — shown once, then discarded. Confidential clients only. */
  secret?: string;
}

/*
 * Backend wire shapes (internal/adminapi clients.go). The UI consumes the flattened
 * `Client`/`CreatedClient` above; the backend uses Hydra's `clientId`/`clientType`
 * naming, so we map at the boundary rather than leak it into the screen.
 */

/** `clientSummary` — the client-safe projection from the list/get endpoints. */
interface ClientSummaryWire {
  clientId: string;
  name: string;
  redirectUris?: string[];
  scopes?: string[];
  grantTypes?: string[];
  responseTypes?: string[];
  clientType: ClientType;
  createdAt?: string;
}

/**
 * `servicesListResponse` — the backend's `{services:[...]}` envelope for the
 * console Services list (internal/admin services.go). NOTE: the console route is
 * `/admin/services` (design D1) and wraps its rows under `services`, distinct from
 * the machine `/admin/clients` route's `{clients:[...]}` envelope.
 */
interface ServicesListResponseWire {
  services?: ClientSummaryWire[];
}

/** `serviceDetailResponse` — the backend's `{service}` envelope (get/update). */
interface ServiceDetailResponseWire {
  service: ClientSummaryWire;
}

/**
 * `registerClientResponse` — the 201 create body. `clientSecret` is present once
 * for confidential clients only. The backend returns the id + (optional) secret;
 * the UI merges them with the request to render the new row without a refetch.
 */
interface RegisterClientResponseWire {
  clientId: string;
  clientSecret?: string;
}

/** `consentCountResponse` — best-effort per-client consent usage (design D3). */
interface ConsentCountResponseWire {
  count: number;
}

/** Map a backend client summary to the UI's flattened `Client`. */
function toClient(c: ClientSummaryWire): Client {
  return {
    id: c.clientId,
    name: c.name,
    type: c.clientType,
    redirectUris: c.redirectUris ?? [],
    scopes: c.scopes ?? [],
    grantTypes: c.grantTypes ?? [],
    responseTypes: c.responseTypes ?? [],
    ...(c.createdAt !== undefined ? { createdAt: c.createdAt } : {}),
  };
}

export const adminServicesApi = {
  /** List registered clients (services). 403 ⇒ not authorized. */
  list: async (signal?: AbortSignal): Promise<Client[]> => {
    const res = await request<ServicesListResponseWire>(
      '/admin/services',
      signal ? { signal } : {},
    );
    return (res.services ?? []).map(toClient);
  },

  /** Fetch one client's detail by id (backend wraps it in a `{service}` envelope). */
  get: async (id: string, signal?: AbortSignal): Promise<Client> => {
    const res = await request<ServiceDetailResponseWire>(
      `/admin/services/${encodeURIComponent(id)}`,
      signal ? { signal } : {},
    );
    return toClient(res.service);
  },

  /**
   * Register a client. The backend issues the `clientId` and (for confidential
   * clients) a one-time `clientSecret`. We reconstruct the full `Client` from the
   * request + issued id so the caller can render it without a refetch; the secret
   * is returned once and must not be persisted.
   */
  create: async (input: CreateClientInput): Promise<CreatedClient> => {
    const res = await request<RegisterClientResponseWire>('/admin/services', {
      method: 'POST',
      body: {
        name: input.name,
        clientType: input.type,
        redirectUris: input.redirectUris,
        scopes: input.scopes,
        grantTypes: input.grantTypes,
        responseTypes: input.responseTypes,
      },
    });
    const client: Client = {
      id: res.clientId,
      name: input.name,
      type: input.type,
      redirectUris: input.redirectUris,
      scopes: input.scopes,
      grantTypes: input.grantTypes,
      responseTypes: input.responseTypes,
    };
    return res.clientSecret !== undefined && res.clientSecret !== ''
      ? { client, secret: res.clientSecret }
      : { client };
  },

  /** Edit a client (name/redirect URIs/scopes/grant+response types). Audited. */
  update: async (id: string, input: UpdateClientInput): Promise<Client> => {
    const res = await request<ServiceDetailResponseWire>(
      `/admin/services/${encodeURIComponent(id)}`,
      {
        method: 'PATCH',
        body: {
          name: input.name,
          redirectUris: input.redirectUris,
          scopes: input.scopes,
          grantTypes: input.grantTypes,
          responseTypes: input.responseTypes,
        },
      },
    );
    return toClient(res.service);
  },

  /** Delete a client. Audited; subsequent auth requests with its id are rejected. */
  delete: (id: string): Promise<void> =>
    request(`/admin/services/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  /** Best-effort count of users who have granted this client consent (design D3). */
  consentCount: async (id: string, signal?: AbortSignal): Promise<number> => {
    const res = await request<ConsentCountResponseWire>(
      `/admin/services/${encodeURIComponent(id)}/consents`,
      signal ? { signal } : {},
    );
    return res.count;
  },

  /** Revoke all of this client's consent grants (users must re-consent). Audited. */
  revokeConsents: (id: string): Promise<void> =>
    request(`/admin/services/${encodeURIComponent(id)}/consents`, { method: 'DELETE' }),
};
