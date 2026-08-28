import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchTopology, topologyParams } from '../../src/lib/topology';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('topologyParams', () => {
  it('asks for the whole cluster when nothing is chosen', () => {
    expect(topologyParams({ namespace: '', expanded: [], root: null })).toBe('');
  });

  it('carries the namespace, the open nodes and the root', () => {
    const params = topologyParams({
      namespace: 'prod',
      expanded: ['a', 'b'],
      root: {
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        namespace: 'prod',
        name: 'api',
      },
    });

    expect(params).toContain('namespace=prod');
    expect(params).toContain('expand=a%2Cb');
    expect(params).toContain('rootResource=deployments');
    expect(params).toContain('rootName=api');
  });
});

describe('fetchTopology', () => {
  it('returns the parsed graph', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ nodes: [], edges: [] }),
      }),
    );

    await expect(fetchTopology({ namespace: '', expanded: [], root: null })).resolves.toEqual({
      nodes: [],
      edges: [],
      error: undefined,
    });
  });

  it('says so when the backend refuses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }));

    await expect(fetchTopology({ namespace: '', expanded: [], root: null })).rejects.toThrow(
      'topology request failed with status 503',
    );
  });
});
