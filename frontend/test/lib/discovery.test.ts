import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchResources } from '../../src/lib/discovery';
import { makeCategory, makeDescriptor } from '../helpers';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchResources', () => {
  it('requests /api/resources and returns the parsed categories', async () => {
    const categories = [makeCategory('Workloads', [makeDescriptor({ resource: 'pods' })])];
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(categories),
    });
    vi.stubGlobal('fetch', fetchMock);
    const result = await fetchResources();
    expect(fetchMock).toHaveBeenCalledWith('/api/resources');
    expect(result).toEqual(categories);
  });

  it('throws when the response is not ok', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      json: () => Promise.resolve([]),
    });
    vi.stubGlobal('fetch', fetchMock);
    await expect(fetchResources()).rejects.toThrow('discovery request failed with status 503');
  });
});
