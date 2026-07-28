import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchResources, refreshResources } from '../../src/lib/discovery';
import { makeCategory, makeDescriptor } from '../helpers';

afterEach(() => {
  vi.unstubAllGlobals();
});

function catalog() {
  return { categories: [makeCategory('Workloads', [makeDescriptor({ resource: 'pods' })])] };
}

describe('fetchResources', () => {
  it('requests /api/resources and returns the catalog', async () => {
    const body = catalog();
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(body) });
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchResources();

    expect(fetchMock).toHaveBeenCalledWith('/api/resources', { method: 'GET' });
    expect(result).toEqual(body);
  });

  it('carries the discovery error through', async () => {
    const body = { categories: [], error: 'the server could not find the requested resource' };
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(body) }),
    );

    await expect(fetchResources()).resolves.toEqual(body);
  });

  it('throws when the response is not ok', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      json: () => Promise.resolve({}),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchResources()).rejects.toThrow('discovery request failed with status 503');
  });
});

describe('refreshResources', () => {
  it('posts to re-run discovery', async () => {
    const body = catalog();
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(body) });
    vi.stubGlobal('fetch', fetchMock);

    const result = await refreshResources();

    expect(fetchMock).toHaveBeenCalledWith('/api/resources', { method: 'POST' });
    expect(result).toEqual(body);
  });

  it('throws when the refresh fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 500, json: () => Promise.resolve({}) }),
    );

    await expect(refreshResources()).rejects.toThrow('discovery request failed with status 500');
  });
});
