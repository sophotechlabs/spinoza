import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  REQUEST_TIMEOUT_MS,
  SLOW_REQUEST_TIMEOUT_MS,
  TIMEOUT_MESSAGE,
  TOKEN_HEADER,
  authToken,
  request,
} from '../../src/lib/http';
import { anySignal, rejectsWith } from '../helpers';

interface TokenWindow {
  __SPINOZA_TOKEN__?: string;
}

function injectToken(token: string): void {
  (window as unknown as TokenWindow).__SPINOZA_TOKEN__ = token;
}

function clearToken(): void {
  delete (window as unknown as TokenWindow).__SPINOZA_TOKEN__;
}

function headersOf(mock: ReturnType<typeof stubFetch>): Headers {
  return new Headers(mock.mock.calls[0][1].headers);
}

function reasonOf(signal: AbortSignal | null | undefined): Error {
  if (signal === null || signal === undefined) {
    return new Error('aborted');
  }
  return signal.reason as Error;
}

function stubFetch(impl: (url: string, init: RequestInit) => Promise<unknown>) {
  const mock = vi.fn(impl);
  vi.stubGlobal('fetch', mock);
  return mock;
}

function signalOf(mock: ReturnType<typeof stubFetch>): AbortSignal {
  const signal = mock.mock.calls[0][1].signal;
  if (!(signal instanceof AbortSignal)) {
    throw new Error('expected the request to carry an abort signal');
  }
  return signal;
}

describe('authToken', () => {
  afterEach(() => {
    clearToken();
    window.history.replaceState(null, '', '/');
  });

  it('prefers the token the server injected into the page', () => {
    injectToken('from-index');
    window.history.replaceState(null, '', '/?token=from-url');

    expect(authToken()).toBe('from-index');
  });

  it('falls back to the token in the page url', () => {
    window.history.replaceState(null, '', '/?token=from-url');

    expect(authToken()).toBe('from-url');
  });

  it('ignores an empty injected token', () => {
    injectToken('');
    window.history.replaceState(null, '', '/?token=from-url');

    expect(authToken()).toBe('from-url');
  });

  it('ignores an empty token in the url', () => {
    window.history.replaceState(null, '', '/?token=');

    expect(authToken()).toBeNull();
  });

  it('has no token on a page that was never handed one', () => {
    expect(authToken()).toBeNull();
  });
});

describe('request', () => {
  afterEach(() => {
    clearToken();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('carries the token on every call', async () => {
    injectToken('s3cret');
    const mock = stubFetch(() => Promise.resolve({ ok: true }));

    await request('/api/flux');

    expect(headersOf(mock).get(TOKEN_HEADER)).toBe('s3cret');
  });

  it('keeps the headers the caller passed alongside the token', async () => {
    injectToken('s3cret');
    const mock = stubFetch(() => Promise.resolve({ ok: true }));

    await request('/api/object', { headers: { 'Content-Type': 'application/yaml' } });

    const headers = headersOf(mock);
    expect(headers.get('Content-Type')).toBe('application/yaml');
    expect(headers.get(TOKEN_HEADER)).toBe('s3cret');
  });

  it('sends no token header when the page has none', async () => {
    const mock = stubFetch(() => Promise.resolve({ ok: true }));

    await request('/api/flux');

    expect(headersOf(mock).get(TOKEN_HEADER)).toBeNull();
  });

  it('attaches an abort signal every caller gets for free', async () => {
    const mock = stubFetch(() => Promise.resolve({ ok: true }));

    await request('/api/flux');

    expect(mock).toHaveBeenCalledWith('/api/flux', { signal: anySignal() });
    expect(signalOf(mock).aborted).toBe(false);
  });

  it('keeps the init options the caller passed', async () => {
    const mock = stubFetch(() => Promise.resolve({ ok: true }));

    await request('/api/object', { method: 'DELETE' });

    expect(mock).toHaveBeenCalledWith('/api/object', {
      method: 'DELETE',
      signal: anySignal(),
    });
  });

  it('never forwards timeoutMs to fetch', async () => {
    const mock = stubFetch(() => Promise.resolve({ ok: true }));

    await request('/api/debug', { method: 'POST', timeoutMs: SLOW_REQUEST_TIMEOUT_MS });

    const init = mock.mock.calls[0][1] as Record<string, unknown>;
    expect(init.timeoutMs).toBeUndefined();
    expect(init.method).toBe('POST');
  });

  it('aborts a backend that never answers', async () => {
    const mock = stubFetch(
      (_url, init) =>
        new Promise((_resolve, reject) => {
          init.signal?.addEventListener('abort', () => {
            reject(reasonOf(init.signal));
          });
        }),
    );

    await expect(request('/api/contexts', { timeoutMs: 10 })).rejects.toThrow(TIMEOUT_MESSAGE);
    expect(signalOf(mock).aborted).toBe(true);
  });

  it('lets a slow answer through while it is still inside the budget', async () => {
    stubFetch(
      (_url, init) =>
        new Promise((resolve, reject) => {
          init.signal?.addEventListener('abort', () => {
            reject(reasonOf(init.signal));
          });
          setTimeout(() => {
            resolve({ ok: true });
          }, 20);
        }),
    );

    await expect(request('/api/debug', { timeoutMs: 2000 })).resolves.toEqual({ ok: true });
  });

  it('gives slow endpoints a much longer budget than ordinary reads', () => {
    expect(SLOW_REQUEST_TIMEOUT_MS).toBeGreaterThan(REQUEST_TIMEOUT_MS);
  });

  it('passes any other failure through untouched', async () => {
    stubFetch(() => Promise.reject(new Error('network down')));

    await expect(request('/api/flux')).rejects.toThrow('network down');
  });

  it('passes a non-Error rejection through untouched', async () => {
    stubFetch(rejectsWith('nope'));

    await expect(request('/api/flux')).rejects.toBe('nope');
  });
});
