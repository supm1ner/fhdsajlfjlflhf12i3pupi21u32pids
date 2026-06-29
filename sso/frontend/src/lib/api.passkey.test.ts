import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { passkeyApi, __resetCsrfCache } from './api';

/** Build a JSON Response with the given status/body. */
function jsonResponse(status: number, body: unknown, contentType = 'application/json'): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { 'content-type': contentType },
  });
}

/** A single recorded fetch call: [url, init]. */
type FetchCall = [string, RequestInit & { headers: Record<string, string> }];

function call(fetchMock: ReturnType<typeof vi.fn>, n: number): FetchCall {
  const c = fetchMock.mock.calls[n];
  expect(c, `expected fetch call #${n}`).toBeDefined();
  return c as FetchCall;
}

/**
 * Most passkey endpoints are mutating, so the api client first fetches a CSRF
 * token. Seed that first response, then the endpoint's own response.
 */
function mockWithCsrf(endpointResponse: Response): ReturnType<typeof vi.fn> {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(jsonResponse(200, { token: 'csrf-pk' }))
    .mockResolvedValueOnce(endpointResponse);
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

describe('passkey api client', () => {
  beforeEach(() => {
    __resetCsrfCache();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    __resetCsrfCache();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('registerBegin POSTs to /passkeys/register/begin with a CSRF header', async () => {
    const options = { publicKey: { challenge: 'abc', rp: { id: 'localhost' } } };
    const fetchMock = mockWithCsrf(jsonResponse(200, options));

    const res = await passkeyApi.registerBegin();

    const [url, init] = call(fetchMock, 1);
    expect(url).toBe('/api/v1/passkeys/register/begin');
    expect(init.method).toBe('POST');
    expect(init.credentials).toBe('include');
    expect(init.headers['X-CSRF-Token']).toBe('csrf-pk');
    expect(res.publicKey).toEqual(options.publicKey);
  });

  it('registerFinish POSTs the credential + nickname', async () => {
    const credential = { id: 'cred-1', type: 'public-key', rawId: 'r', response: {} };
    const fetchMock = mockWithCsrf(
      jsonResponse(200, { passkey: { id: 'pk-1', name: 'Laptop', createdAt: '2026-01-01' } }),
    );

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const res = await passkeyApi.registerFinish(credential as any, 'Laptop');

    const [url, init] = call(fetchMock, 1);
    expect(url).toBe('/api/v1/passkeys/register/finish');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body as string)).toEqual({ credential, name: 'Laptop' });
    expect(res.passkey.name).toBe('Laptop');
  });

  it('list GETs /passkeys without a CSRF header', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(200, { passkeys: [{ id: 'pk-1', name: 'Phone' }] }));
    vi.stubGlobal('fetch', fetchMock);

    const res = await passkeyApi.list();

    const [url, init] = call(fetchMock, 0);
    expect(url).toBe('/api/v1/passkeys');
    expect(init.method ?? 'GET').toBe('GET');
    expect(init.headers['X-CSRF-Token']).toBeUndefined();
    expect(res.passkeys).toHaveLength(1);
  });

  it('delete DELETEs /passkeys/{id} (id URL-encoded) with a CSRF header', async () => {
    const fetchMock = mockWithCsrf(new Response(null, { status: 204 }));

    await expect(passkeyApi.delete('pk 1/2')).resolves.toBeUndefined();

    const [url, init] = call(fetchMock, 1);
    expect(url).toBe('/api/v1/passkeys/pk%201%2F2');
    expect(init.method).toBe('DELETE');
    expect(init.headers['X-CSRF-Token']).toBe('csrf-pk');
  });

  it('loginBegin with an email + loginChallenge posts both', async () => {
    const fetchMock = mockWithCsrf(jsonResponse(200, { publicKey: { challenge: 'xyz' } }));

    await passkeyApi.loginBegin('user@example.com', 'lc-123');

    const [url, init] = call(fetchMock, 1);
    expect(url).toBe('/api/v1/auth/passkey/login/begin');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body as string)).toEqual({
      email: 'user@example.com',
      loginChallenge: 'lc-123',
    });
  });

  it('loginBegin without an email omits email (discoverable ceremony)', async () => {
    const fetchMock = mockWithCsrf(jsonResponse(200, { publicKey: { challenge: 'xyz' } }));

    await passkeyApi.loginBegin();

    const [, init] = call(fetchMock, 1);
    // No email and no loginChallenge → empty body (usernameless, plain sign-in).
    expect(JSON.parse(init.body as string)).toEqual({});
  });

  it('loginFinish posts only the credential and returns redirectTo', async () => {
    const credential = { id: 'c', type: 'public-key', rawId: 'r', response: {} };
    const fetchMock = mockWithCsrf(jsonResponse(200, { redirectTo: 'https://rp.example/cb' }));

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const res = await passkeyApi.loginFinish(credential as any);

    const [url, init] = call(fetchMock, 1);
    expect(url).toBe('/api/v1/auth/passkey/login/finish');
    // The loginChallenge is carried in the cid_wa cookie (set by loginBegin), NOT
    // in the finish body — the backend's strict decoder rejects unknown fields.
    expect(JSON.parse(init.body as string)).toEqual({ credential });
    expect(res.redirectTo).toBe('https://rp.example/cb');
  });
});
