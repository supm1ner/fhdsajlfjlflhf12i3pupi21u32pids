// Browser-side OpenID Connect login (Authorization Code + PKCE) against the SSO
// provider (Ory Hydra fronted by cotton-id). The resulting ID token is passed to
// the messenger backend via login('oidc', id_token) — see lib/tinode.js.
//
// This is a public client: there is no client secret in the browser; security comes
// from PKCE and the registered redirect URI.

// --- Configuration (override via Vite env: VITE_OIDC_*; see .env.example) -----
// Issuer = Hydra public URL. Must end with a slash and match the backend's oidc.issuer.
const env = import.meta.env ?? {};
const ISSUER = env.VITE_OIDC_ISSUER || 'http://localhost:4444/';
const CLIENT_ID = env.VITE_OIDC_CLIENT_ID || 'sunrise-messenger';
const SCOPES = env.VITE_OIDC_SCOPES || 'openid profile email';
// Where Hydra redirects back to after login. Must be registered with the OAuth client.
const REDIRECT_URI = env.VITE_OIDC_REDIRECT || (window.location.origin + '/');

const STORAGE_KEY = 'sunrise_oidc_pkce';

// --- PKCE helpers ------------------------------------------------------------

function base64UrlEncode(bytes) {
  let str = '';
  for (const b of bytes) str += String.fromCharCode(b);
  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function randomString(byteLen = 32) {
  const arr = new Uint8Array(byteLen);
  crypto.getRandomValues(arr);
  return base64UrlEncode(arr);
}

async function sha256Challenge(verifier) {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier));
  return base64UrlEncode(new Uint8Array(digest));
}

// --- Public API --------------------------------------------------------------

// beginLogin redirects the browser to the IdP's authorization endpoint.
export async function beginLogin() {
  const verifier = randomString(32);
  const state = randomString(16);
  const nonce = randomString(16);
  const challenge = await sha256Challenge(verifier);

  sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ verifier, state, nonce }));

  const params = new URLSearchParams({
    response_type: 'code',
    client_id: CLIENT_ID,
    redirect_uri: REDIRECT_URI,
    scope: SCOPES,
    state,
    nonce,
    code_challenge: challenge,
    code_challenge_method: 'S256',
  });

  window.location.assign(ISSUER + 'oauth2/auth?' + params.toString());
}

// isRedirectCallback reports whether the current URL looks like an OIDC redirect back.
export function isRedirectCallback() {
  const q = new URLSearchParams(window.location.search);
  return q.has('code') || q.has('error');
}

// completeLogin handles the redirect back from the IdP: it validates state, exchanges
// the authorization code for tokens, clears the URL, and returns the ID token.
// Returns null when the current URL is not a redirect callback.
export async function completeLogin() {
  const q = new URLSearchParams(window.location.search);

  if (q.has('error')) {
    clearCallbackUrl();
    throw new Error(q.get('error_description') || q.get('error'));
  }
  const code = q.get('code');
  if (!code) return null;

  const saved = sessionStorage.getItem(STORAGE_KEY);
  sessionStorage.removeItem(STORAGE_KEY);
  if (!saved) throw new Error('Missing PKCE state (start the login again)');
  const { verifier, state } = JSON.parse(saved);

  if (q.get('state') !== state) throw new Error('OIDC state mismatch');

  const body = new URLSearchParams({
    grant_type: 'authorization_code',
    code,
    redirect_uri: REDIRECT_URI,
    client_id: CLIENT_ID,
    code_verifier: verifier,
  });

  const resp = await fetch(ISSUER + 'oauth2/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: body.toString(),
  });

  clearCallbackUrl();

  if (!resp.ok) {
    const text = await resp.text().catch(() => '');
    throw new Error('Token exchange failed: ' + resp.status + ' ' + text);
  }
  const tokens = await resp.json();
  if (!tokens.id_token) throw new Error('No id_token in token response');
  return tokens.id_token;
}

// clearCallbackUrl removes OAuth query params so a reload doesn't re-trigger the exchange.
function clearCallbackUrl() {
  window.history.replaceState({}, document.title, REDIRECT_URI);
}
