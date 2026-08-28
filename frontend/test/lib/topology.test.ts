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

describe('topologyParams edge cases', () => {
  it('leaves the expand list out when nothing is open', () => {
    const params = topologyParams({ namespace: 'prod', expanded: [], root: null });

    expect(params).toBe('?namespace=prod');
  });

  it('sends every open node in one parameter', () => {
    const params = topologyParams({ namespace: '', expanded: ['a', 'b', 'c'], root: null });

    expect(params).toBe('?expand=a%2Cb%2Cc');
  });

  it('sends a root that is in no namespace', () => {
    const params = topologyParams({
      namespace: '',
      expanded: [],
      root: { group: '', version: 'v1', resource: 'nodes', namespace: '', name: 'worker-1' },
    });

    expect(params).toContain('rootResource=nodes');
    expect(params).toContain('rootNamespace=');
    expect(params).toContain('rootName=worker-1');
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

  it('carries the fold counts off the wire', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            nodes: [{ id: 'a', name: 'api', contains: 40, unhealthy: 2, category: 'workload' }],
            edges: [{ from: 'a', to: 'b', kind: 'owns' }],
          }),
      }),
    );

    const result = await fetchTopology({ namespace: '', expanded: [], root: null });

    expect(result.nodes[0].contains).toBe(40);
    expect(result.nodes[0].unhealthy).toBe(2);
    expect(result.nodes[0].category).toBe('workload');
    expect(result.edges[0].kind).toBe('owns');
  });

  it('falls back rather than trusting counts the backend did not send', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ nodes: [{ id: 'a', category: 'nonsense' }], edges: [] }),
      }),
    );

    const result = await fetchTopology({ namespace: '', expanded: [], root: null });

    expect(result.nodes[0].contains).toBe(0);
    expect(result.nodes[0].unhealthy).toBe(0);
    expect(result.nodes[0].category).toBe('managed');
  });
});
