import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { Metrics } from '../../src/lib/types';
import {
  barColor,
  fetchMetrics,
  formatCpu,
  formatMem,
  isUsable,
  useMetrics,
} from '../../src/lib/metrics';

const sample: Metrics = {
  pods: { 'prod/web': { cpuMilli: 150, memoryMi: 192, cpuPercent: 0, memPercent: 0 } },
  nodes: { n1: { cpuMilli: 1500, memoryMi: 2048, cpuPercent: 37, memPercent: 25 } },
};

function stubOk(data: Metrics): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(data) });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('formatCpu', () => {
  it('formats millicores and hides zero', () => {
    expect(formatCpu(150)).toBe('150m');
    expect(formatCpu(0)).toBe('');
  });
});

describe('formatMem', () => {
  it('formats mebibytes, gibibytes and hides zero', () => {
    expect(formatMem(192)).toBe('192Mi');
    expect(formatMem(2048)).toBe('2.0Gi');
    expect(formatMem(0)).toBe('');
  });
});

describe('barColor', () => {
  it('escalates green to yellow to red', () => {
    expect(barColor(10)).toBe('bg-green-500');
    expect(barColor(75)).toBe('bg-amber-500');
    expect(barColor(95)).toBe('bg-red-500');
  });
});

describe('fetchMetrics', () => {
  it('requests /api/metrics and returns the parsed metrics', async () => {
    const fetchMock = stubOk(sample);
    const result = await fetchMetrics();
    expect(fetchMock).toHaveBeenCalledWith('/api/metrics');
    expect(result).toEqual(sample);
  });

  it('throws when the response is not ok', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 503, json: () => Promise.resolve({}) }),
    );
    await expect(fetchMetrics()).rejects.toThrow('metrics request failed with status 503');
  });
});

describe('useMetrics', () => {
  it('returns null and does not fetch when disabled', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() => useMetrics(false));
    expect(result.current).toBeNull();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('loads metrics when enabled', async () => {
    stubOk(sample);
    const { result } = renderHook(() => useMetrics(true));
    await waitFor(() => {
      expect(result.current).toEqual(sample);
    });
  });

  it('stays null when the fetch fails', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error('down'));
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() => useMetrics(true));
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled();
    });
    expect(result.current).toBeNull();
  });

  it('re-fetches on the poll interval', async () => {
    vi.useFakeTimers();
    const fetchMock = stubOk(sample);
    renderHook(() => useMetrics(true));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe('a failed metrics poll', () => {
  it('keeps the values already on screen', async () => {
    vi.useFakeTimers();
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        if (call === 1) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve(sample) });
        }
        return Promise.reject(new Error('metrics-server is down'));
      }),
    );

    const { result } = renderHook(() => useMetrics(true));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(result.current).toEqual(sample);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(11000);
    });

    expect(call).toBeGreaterThan(1);
    expect(result.current).toEqual(sample);
    vi.useRealTimers();
  });
});

describe('a metrics payload that reports a list failure', () => {
  it('keeps the last good values instead of blanking the columns', async () => {
    vi.useFakeTimers();
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        if (call === 1) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve(sample) });
        }
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              pods: {},
              nodes: {},
              error: '2 of 3 resource types could not be listed',
            }),
        });
      }),
    );

    const { result } = renderHook(() => useMetrics(true));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });

    expect(result.current).toEqual(sample);
  });

  it('accepts a partial payload that still carries data', async () => {
    const partial: Metrics = {
      pods: sample.pods,
      nodes: {},
      error: '1 of 3 resource types could not be listed',
    };
    stubOk(partial);

    const { result } = renderHook(() => useMetrics(true));

    await waitFor(() => {
      expect(result.current).toEqual(partial);
    });
  });

  it('accepts a payload whose error field is empty', () => {
    expect(isUsable({ pods: {}, nodes: {}, error: '' })).toBe(true);
  });

  it('accepts a node-only payload after a pod list failure', () => {
    expect(isUsable({ pods: {}, nodes: sample.nodes, error: 'pods failed' })).toBe(true);
  });
});

describe('a slow metrics poll', () => {
  it('does not stack a second request on top of the first', async () => {
    vi.useFakeTimers();
    let resolveFirst: (value: unknown) => void = () => undefined;
    const fetchMock = vi.fn(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
    );
    vi.stubGlobal('fetch', fetchMock);

    renderHook(() => useMetrics(true));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveFirst({ ok: true, json: () => Promise.resolve(sample) });
      await vi.advanceTimersByTimeAsync(10000);
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe('a metrics response that lands after unmount', () => {
  it('is dropped instead of setting state on a dead hook', async () => {
    let resolveFetch: (value: unknown) => void = () => undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            resolveFetch = resolve;
          }),
      ),
    );

    const { unmount } = renderHook(() => useMetrics(true));
    unmount();

    await act(async () => {
      resolveFetch({ ok: true, json: () => Promise.resolve(sample) });
      await Promise.resolve();
    });
  });
});
