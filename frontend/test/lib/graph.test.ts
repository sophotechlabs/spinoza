import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Graph } from '../../src/lib/types';
import { fetchGraph } from '../../src/lib/graph';
import { anySignal, makeGraphEdge, makeGraphNode } from '../helpers';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchGraph', () => {
  it('requests /api/gitops/graph and returns the parsed graph', async () => {
    const graph: Graph = {
      nodes: [makeGraphNode({ id: 'a' }), makeGraphNode({ id: 'b' })],
      edges: [makeGraphEdge({ from: 'a', to: 'b' })],
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(graph),
    });
    vi.stubGlobal('fetch', fetchMock);
    const result = await fetchGraph();
    expect(fetchMock).toHaveBeenCalledWith('/api/gitops/graph', {
      signal: anySignal(),
    });
    expect(result).toEqual(graph);
  });

  it('throws when the response is not ok', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 502,
      json: () => Promise.resolve({ nodes: [], edges: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);
    await expect(fetchGraph()).rejects.toThrow('gitops graph request failed with status 502');
  });
});
