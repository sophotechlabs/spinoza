import { afterEach, describe, expect, it, vi } from 'vitest';
import type { TrafficGraph, TrafficSupport } from '../../src/lib/types';
import { fetchTrafficGraph, fetchTrafficSupport } from '../../src/lib/traffic';
import { anySignal } from '../helpers';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchTrafficGraph', () => {
  it('requests /api/traffic and returns the parsed graph', async () => {
    const graph: TrafficGraph = {
      source: 'Cilium Hubble',
      nodes: [{ id: 'apps/web', namespace: 'apps', workload: 'web' }],
      edges: [{ from: 'apps/web', to: 'apps/api', rate: 3, dropped: 0 }],
    };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(graph) });
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchTrafficGraph();

    expect(fetchMock).toHaveBeenCalledWith('/api/traffic', { signal: anySignal() });
    expect(result).toEqual({ ...graph, folded: false, workloads: 0, error: undefined });
  });

  it('reads the fold the server applied', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            source: 'Cilium Hubble',
            nodes: [{ id: 'apps', namespace: 'apps', workload: '' }],
            edges: [],
            folded: true,
            workloads: 612,
          }),
      }),
    );

    const result = await fetchTrafficGraph();

    expect(result.folded).toBe(true);
    expect(result.workloads).toBe(612);
  });

  it('treats a graph with no fold as unfolded', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ source: 'Cilium Hubble', nodes: [], edges: [] }),
      }),
    );

    const result = await fetchTrafficGraph();

    expect(result.folded).toBe(false);
    expect(result.workloads).toBe(0);
  });

  it('throws when the response is not ok', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 503, json: () => Promise.resolve({}) }),
    );
    await expect(fetchTrafficGraph()).rejects.toThrow(
      'traffic graph request failed with status 503',
    );
  });
});

describe('fetchTrafficSupport', () => {
  it('requests /api/traffic/support and returns the parsed answer', async () => {
    const support: TrafficSupport = { available: true, source: 'Cilium Hubble' };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(support) });
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchTrafficSupport();

    expect(fetchMock).toHaveBeenCalledWith('/api/traffic/support', { signal: anySignal() });
    expect(result).toEqual(support);
  });

  it('reads a refusal with its reason', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ available: false, reason: 'add flow:labelsContext' }),
      }),
    );

    const result = await fetchTrafficSupport();

    expect(result).toEqual({ available: false, reason: 'add flow:labelsContext' });
  });

  it('throws when the response is not ok', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 500, json: () => Promise.resolve({}) }),
    );
    await expect(fetchTrafficSupport()).rejects.toThrow(
      'traffic support request failed with status 500',
    );
  });
});
