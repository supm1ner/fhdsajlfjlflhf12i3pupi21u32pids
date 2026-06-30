import * as TinodeModule from 'tinode-sdk';

// --- Configuration (override via Vite env: VITE_*; see .env.example) ----------
const env = import.meta.env ?? {};
const API_KEY = env.VITE_API_KEY || 'AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K';
const HOST = env.VITE_HOST || 'localhost:6060';
const APP_NAME = env.VITE_APP_NAME || 'SunriseWeb';
const SECURE = String(env.VITE_TLS ?? 'false') === 'true';
const TOKEN_KEY = 'sunrise_auth_token';

const Tinode = TinodeModule.Tinode ?? TinodeModule.default?.Tinode;
export const Drafty = TinodeModule.Drafty ?? TinodeModule.default?.Drafty;

let client = null;

export function getClient() {
  if (!client) {
    client = new Tinode({ appName: APP_NAME, host: HOST, apiKey: API_KEY, transport: 'ws', secure: SECURE });
    client.enableLogging(false);
  }
  return client;
}

// baseUrl returns the absolute origin of the messenger server.
export function baseUrl() {
  return (SECURE ? 'https://' : 'http://') + HOST;
}

// b64 encodes a (possibly unicode) string to base64. The login secret is decoded by the
// server as a []byte (standard base64), so credentials must be base64-encoded before sending.
function b64(str) {
  return btoa(unescape(encodeURIComponent(str)));
}

export function connect() {
  return new Promise((resolve, reject) => {
    const c = getClient();
    if (c.isConnected()) { resolve(c); return; }
    const to = setTimeout(() => reject(new Error('Connection timeout')), 10000);
    c.onConnect = () => { clearTimeout(to); resolve(c); };
    c.onDisconnect = (err) => { clearTimeout(to); reject(err); };
    c.connect().catch(reject);
  });
}

// --- Authentication ----------------------------------------------------------

function persistToken(c) {
  try {
    const tok = c.getAuthToken();
    if (tok) localStorage.setItem(TOKEN_KEY, JSON.stringify(tok));
  } catch { /* ignore */ }
}

export function login(login, password) {
  // Secret must be base64-encoded "login:password" (server decodes it as []byte).
  return getClient().login('basic', b64(`${login}:${password}`), true).then((ctrl) => {
    persistToken(getClient());
    return ctrl;
  });
}

// loginWithToken authenticates using an external OIDC ID token (from the SSO provider).
// The JWT is base64-encoded so the server decodes the secret []byte back to the raw token.
export function loginWithToken(idToken) {
  return getClient().login('oidc', b64(idToken), true).then((ctrl) => {
    persistToken(getClient());
    return ctrl;
  });
}

// loginWithSavedToken attempts to resume a session from a previously stored token.
export async function loginWithSavedToken() {
  const raw = localStorage.getItem(TOKEN_KEY);
  if (!raw) return false;
  let tok;
  try { tok = JSON.parse(raw); } catch { return false; }
  if (!tok || (tok.expires && new Date(tok.expires) < new Date())) {
    localStorage.removeItem(TOKEN_KEY);
    return false;
  }
  const c = getClient();
  c.setAuthToken(tok);
  await connect();
  await c.login('token', tok.token);
  persistToken(c);
  return true;
}

export function createAccount(login, password, name, email) {
  const c = getClient();
  const secret = b64(`${login}:${password}`);
  const desc = { public: { fn: name } };
  const cred = email ? [{ meth: 'email', val: email }] : undefined;
  return c.createAccount('basic', secret, true, desc, undefined, cred);
}

export function logout() {
  localStorage.removeItem(TOKEN_KEY);
  if (client) {
    client.disconnect();
    client = null;
  }
}

export function myUID() {
  return getClient().getCurrentUserID();
}

// --- Connection resilience ---------------------------------------------------

let connListener = null;
// setConnectionListener registers a callback receiving 'online' | 'connecting' | 'offline'.
export function setConnectionListener(cb) { connListener = cb; }
function notifyConn(state) { try { connListener?.(state); } catch { /* */ } }

// installSession wires persistent reconnect handlers: the SDK auto-reconnects the socket,
// and on each (re)connect we transparently re-login with the saved token and resume 'me'.
export function installSession() {
  const c = getClient();
  c.onConnect = async () => {
    try {
      const raw = localStorage.getItem(TOKEN_KEY);
      const tok = raw ? JSON.parse(raw) : null;
      if (tok?.token) {
        c.setAuthToken(tok);
        await c.login('token', tok.token);
        persistToken(c);
      }
      await subscribeMe().catch(() => {});
      notifyConn('online');
    } catch {
      notifyConn('offline');
    }
  };
  c.onDisconnect = () => notifyConn('connecting');
}

// --- 'me' topic & contacts ---------------------------------------------------

export function getMe() {
  return getClient().getMeTopic();
}

// subscribeMe subscribes to the 'me' topic so the contact list populates and stays live.
export async function subscribeMe() {
  const me = getMe();
  if (!me.isSubscribed()) {
    await me.subscribe(me.startMetaQuery().withLaterSub().withLaterDesc().build());
  }
  return me;
}

// searchUsers queries the 'fnd' (find) topic for users/groups matching the query
// (a name, or "email:..."/"tel:..." tag). Returns [{ topic, name, online }].
export async function searchUsers(query) {
  const q = (query || '').trim();
  if (!q) return [];
  const fnd = getClient().getFndTopic();
  if (!fnd.isSubscribed()) {
    await fnd.subscribe(fnd.startMetaQuery().withSub().build());
  }
  await fnd.setMeta({ desc: { public: q } });
  await fnd.getMeta(fnd.startMetaQuery().withSub().build());
  const out = [];
  fnd.contacts((s) => {
    const topic = s.user || s.topic;
    if (topic) out.push({ topic, name: s.public?.fn || topic, online: !!s.online });
  });
  return out;
}

// contactFromTopic builds a plain descriptor from an SDK Topic object.
export function contactFromTopic(topic) {
  const last = topic.latestMessage?.();
  return {
    topic: topic.name,
    name: topic.public?.fn || topic.name,
    avatar: topic.public?.photo ? avatarFromPhoto(topic.public.photo) : '',
    online: !!topic.online,
    unread: topic.unread || 0,
    touched: topic.touched ? new Date(topic.touched).getTime() : 0,
    lastMsg: last ? messagePreview(last) : '',
    lastTs: last?.ts ? new Date(last.ts).getTime() : 0,
  };
}

// mapContacts returns the current list of conversation contacts, most-recent first.
export function mapContacts() {
  const c = getClient();
  const out = [];
  c.mapTopics((topic) => {
    if (topic.isCommType && topic.isCommType()) {
      out.push(contactFromTopic(topic));
    }
  });
  out.sort((a, b) => (b.touched || b.lastTs) - (a.touched || a.lastTs));
  return out;
}

// messagePreview produces a short text preview for a message in the contact list.
export function messagePreview(msg) {
  const c = msg?.content;
  if (c == null) return '';
  if (typeof c === 'string') return c;
  if (msg?.head?.webrtc) return '📞 Call';
  if (Drafty.isValid?.(c)) {
    if (Drafty.hasEntities?.(c)) {
      let label = '';
      Drafty.entities?.(c, (ent) => {
        if (label) return;
        switch (ent.tp) {
          case 'IM': label = '🖼 Photo'; break;
          case 'VD': label = ent.data?.width === ent.data?.height ? '⭕ Video note' : '🎬 Video'; break;
          case 'AU': label = '🎤 Voice message'; break;
          case 'EX': label = '📎 ' + (ent.data?.name || 'File'); break;
          case 'VC': label = '📞 Call'; break;
        }
      });
      if (label) return label;
    }
    return Drafty.toPlainText?.(c) || '';
  }
  return c.txt || '';
}

// --- Files -------------------------------------------------------------------

export function getUploader() {
  return getClient().getLargeFileHelper();
}

// fileUrl turns a server file ref ("/v0/file/s/..") into an authorized absolute URL.
export function fileUrl(ref) {
  if (!ref) return '';
  if (/^https?:/i.test(ref)) return ref;
  const url = new URL(baseUrl() + ref);
  url.searchParams.set('apikey', API_KEY);
  try {
    const tok = getClient().getAuthToken();
    if (tok?.token) {
      url.searchParams.set('auth', 'token');
      url.searchParams.set('secret', tok.token);
    }
  } catch { /* ignore */ }
  return url.toString();
}

// avatarFromPhoto resolves an avatar photo descriptor (inline data or ref) to a usable src.
function avatarFromPhoto(photo) {
  if (!photo) return '';
  if (photo.ref) return fileUrl(photo.ref);
  if (photo.data && photo.type) return `data:${photo.type === 'jpg' ? 'image/jpeg' : 'image/' + photo.type};base64,${photo.data}`;
  return '';
}

export { avatarFromPhoto };

// --- Server params -----------------------------------------------------------

export function iceServers() {
  return getClient().getServerParam ? getClient().getServerParam('iceServers', null) : null;
}

// fetchLiveKitToken requests a LiveKit access token for the given room from the backend.
// Returns { url, token, room, identity }. Throws with code 501 if LiveKit is not configured.
export async function fetchLiveKitToken(room) {
  const u = new URL(baseUrl() + '/v0/livekit/token');
  u.searchParams.set('room', room);
  u.searchParams.set('apikey', API_KEY);
  const tok = getClient().getAuthToken();
  if (tok?.token) {
    u.searchParams.set('auth', 'token');
    u.searchParams.set('secret', tok.token);
  }
  const resp = await fetch(u.toString());
  if (resp.status === 501) {
    const err = new Error('LiveKit is not configured');
    err.code = 501;
    throw err;
  }
  if (!resp.ok) throw new Error('LiveKit token request failed: ' + resp.status);
  return resp.json();
}
