import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  REQUEST_TIMEOUT_MS,
  SLOW_REQUEST_TIMEOUT_MS,
  TIMEOUT_MESSAGE,
  request,
} from '../../src/lib/http';
import { anySignal, rejectsWith } from '../helpers';

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

describe('request', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
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
