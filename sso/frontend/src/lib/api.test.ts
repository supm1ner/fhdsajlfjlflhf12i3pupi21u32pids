import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, api, __resetCsrfCache } from './api';

/** Build a JSON Response with the given status/body. */
function jsonResponse(status: number, body: unknown, contentType = 'application/json'): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { 'content-type': contentType },
  });
}

/** A single recorded fetch call: [url, init]. */
type FetchCall = [string, RequestInit & { headers: Record<string, string> }];

/** Read the n-th recorded call off a mocked fetch, asserting it exists. */
function call(fetchMock: ReturnType<typeof vi.fn>, n: number): FetchCall {
  const c = fetchMock.mock.calls[n];
  expect(c, `expected fetch call #${n}`).toBeDefined();
  return c as FetchCall;
}

describe('api client', () => {
  beforeEach(() => {
    // Reset the module-level CSRF cache so each test controls its own fetch
    // sequence (the cache otherwise persists across tests in the same module).
    __resetCsrfCache();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    __resetCsrfCache();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('includes credentials and injects X-CSRF-Token on mutating requests', async () => {
    const fetchMock = vi
      .fn()
      // GET /csrf
      .mockResolvedValueOnce(jsonResponse(200, { token: 'csrf-123' }))
      // POST /auth/login
      .mockResolvedValueOnce(jsonResponse(200, { user: { id: 'u1' } }));
    vi.stubGlobal('fetch', fetchMock);

    await api.login({ email: 'a@b.c', password: 'pw', remember: true });

    // First call: CSRF fetch with credentials.
    const [csrfUrl, csrfInit] = call(fetchMock, 0);
    expect(csrfUrl).toBe('/api/v1/csrf');
    expect(csrfInit.credentials).toBe('include');

    // Second call: login carries the token header + credentials + JSON body.
    const [loginUrl, loginInit] = call(fetchMock, 1);
    expect(loginUrl).toBe('/api/v1/auth/login');
    expect(loginInit.method).toBe('POST');
    expect(loginInit.credentials).toBe('include');
    expect(loginInit.headers['X-CSRF-Token']).toBe('csrf-123');
    expect(JSON.parse(loginInit.body as string)).toEqual({
      email: 'a@b.c',
      password: 'pw',
      remember: true,
    });
  });

  it('does not send a CSRF header on GET requests', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(200, { user: { id: 'u1' } }));
    vi.stubGlobal('fetch', fetchMock);

    await api.session();

    const [url, init] = call(fetchMock, 0);
    expect(url).toBe('/api/v1/auth/session');
    expect(init.method).toBe('GET');
    expect(init.headers['X-CSRF-Token']).toBeUndefined();
  });

  it('parses problem+json into a typed ApiError', async () => {
    const problem = {
      type: 'about:blank',
      title: 'Unauthorized',
      status: 401,
      detail: 'invalid credentials',
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(200, { token: 'csrf-1' }))
      .mockResolvedValueOnce(jsonResponse(401, problem, 'application/problem+json'));
    vi.stubGlobal('fetch', fetchMock);

    let caught: unknown;
    try {
      await api.login({ email: 'a@b.c', password: 'x', remember: false });
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(ApiError);
    const apiErr = caught as ApiError;
    expect(apiErr.status).toBe(401);
    expect(apiErr.message).toBe('invalid credentials');
    expect(apiErr.problem.title).toBe('Unauthorized');
  });

  it('returns undefined for 204 responses (logout)', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(200, { token: 'csrf-1' }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(api.logout()).resolves.toBeUndefined();
  });

  it('retries once with a fresh token when a mutation 403s', async () => {
    const fetchMock = vi
      .fn()
      // initial csrf
      .mockResolvedValueOnce(jsonResponse(200, { token: 'stale' }))
      // first login attempt rejected (stale token)
      .mockResolvedValueOnce(jsonResponse(403, { title: 'Forbidden', status: 403 }))
      // refreshed csrf
      .mockResolvedValueOnce(jsonResponse(200, { token: 'fresh' }))
      // retried login succeeds
      .mockResolvedValueOnce(jsonResponse(200, { user: { id: 'u1' } }));
    vi.stubGlobal('fetch', fetchMock);

    await api.login({ email: 'a@b.c', password: 'pw', remember: true });

    // Final login attempt (call #3) used the refreshed token.
    const [, lastInit] = call(fetchMock, 3);
    expect(lastInit.headers['X-CSRF-Token']).toBe('fresh');
  });
});
