import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { fetchOverview, percentOf, useOverview } from '../../src/lib/overview';
import { anySignal } from '../helpers';

const payload = {
  version: 'v1.36.1',
  nodes: {
    total: 3,
    ready: 3,
    unschedulable: 0,
    cpuAllocatableMilli: 12000,
    cpuUsedMilli: 3000,
    memAllocatableMi: 32768,
    memUsedMi: 8192,
    usageKnown: true,
  },
  pods: { total: 40, running: 38, pending: 1, failed: 1, succeeded: 0, known: true },
  warnings: [
    {
      namespace: 'flux-system',
      object: 'Pod/web-1',
      reason: 'BackOff',
      message: 'back-off restarting',
      count: 4,
      lastSeen: '2026-08-11T11:00:00Z',
    },
  ],
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchOverview', () => {
  it('requests /api/overview and parses what comes back', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) });
    vi.stubGlobal('fetch', fetchMock);

    const got = await fetchOverview();

    expect(fetchMock).toHaveBeenCalledWith('/api/overview', { signal: anySignal() });
    expect(got.version).toBe('v1.36.1');
    expect(got.nodes.total).toBe(3);
    expect(got.pods.running).toBe(38);
    expect(got.warnings[0].object).toBe('Pod/web-1');
  });

  it('reads a payload missing every field without throwing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) }),
    );

    const got = await fetchOverview();

    expect(got.version).toBe('');
    expect(got.nodes.total).toBe(0);
    expect(got.nodes.usageKnown).toBe(false);
    expect(got.pods.known).toBe(false);
    expect(got.warnings).toEqual([]);
    expect(got.error).toBeUndefined();
  });

  it('reports a failed status', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }));

    await expect(fetchOverview()).rejects.toThrow('overview request failed with status 503');
  });
});

describe('useOverview', () => {
  it('polls once the view is mounted', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) }),
    );

    const { result } = renderHook(() => useOverview());

    await waitFor(() => {
      expect(result.current.data?.version).toBe('v1.36.1');
    });
  });

  it('stays quiet while the view is hidden', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    renderHook(() => useOverview(false));

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('the share of a total something uses', () => {
  it('rounds to whole percent', () => {
    expect(percentOf(3000, 12000)).toBe(25);
    expect(percentOf(1, 3)).toBe(33);
  });

  it('is zero when there is no total to divide by', () => {
    expect(percentOf(500, 0)).toBe(0);
    expect(percentOf(500, -1)).toBe(0);
  });
});
