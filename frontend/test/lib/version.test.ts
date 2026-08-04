import { afterEach, describe, expect, it, vi } from 'vitest';
import { FRONTEND_VERSION, fetchBackendVersion } from '../../src/lib/version';
import { anySignal } from '../helpers';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('FRONTEND_VERSION', () => {
  it('is stamped in at build time', () => {
    expect(FRONTEND_VERSION).toBe('test');
  });
});

describe('fetchBackendVersion', () => {
  it('asks the server what it is running', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue({ ok: true, json: () => Promise.resolve({ version: 'v1.4.0' }) });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchBackendVersion()).resolves.toBe('v1.4.0');
    expect(fetchMock).toHaveBeenCalledWith('/api/version', { signal: anySignal() });
  });

  it('is empty when the server sends no version', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) }),
    );

    await expect(fetchBackendVersion()).resolves.toBe('');
  });

  it('throws when the endpoint is not there', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 404, json: () => Promise.resolve({}) }),
    );

    await expect(fetchBackendVersion()).rejects.toThrow('version request failed with status 404');
  });
});
