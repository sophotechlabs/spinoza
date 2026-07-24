import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { Metrics } from '../../src/lib/types';
import { barColor, fetchMetrics, formatCpu, formatMem, useMetrics } from '../../src/lib/metrics';

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
    expect(barColor(75)).toBe('bg-yellow-500');
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
