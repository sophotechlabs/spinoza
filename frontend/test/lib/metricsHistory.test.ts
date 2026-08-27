import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  DEFAULT_RANGE,
  RANGES,
  collectedFor,
  fetchMetricHistory,
  peak,
  rangesFor,
} from '../../src/lib/metricsHistory';
import { anySignal } from '../helpers';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchMetricHistory', () => {
  it('asks for the pod over the chosen range', async () => {
    const history = { namespace: 'monitoring', pod: 'loki-0', cpu: [], memory: [] };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(history) });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchMetricHistory('monitoring', 'loki-0', '6h')).resolves.toEqual(history);
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/metrics/history?namespace=monitoring&pod=loki-0&range=6h',
      { signal: anySignal() },
    );
  });

  it('surfaces the server message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'prometheus is unavailable' }),
      }),
    );

    await expect(fetchMetricHistory('a', 'b', '1h')).rejects.toThrow('prometheus is unavailable');
  });

  it('falls back to a status message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.reject(new Error('not json')),
      }),
    );

    await expect(fetchMetricHistory('a', 'b', '1h')).rejects.toThrow(
      'metric history failed with status 500',
    );
  });
});

describe('formatting', () => {
  it('finds the peak, and returns zero for no samples', () => {
    expect(peak([{ value: 1 }, { value: 7 }, { value: 3 }])).toBe(7);
    expect(peak([])).toBe(0);
  });

  it('defaults to an hour and offers the shorter and longer spans', () => {
    expect(DEFAULT_RANGE).toBe('1h');
    expect(RANGES).toContain('15m');
    expect(RANGES).toContain('24h');
  });
});

describe('what a sampled chart may be asked for', () => {
  it('offers every span when a metrics database answered', () => {
    expect(rangesFor(false)).toEqual(RANGES);
  });

  it('drops the spans spinoza cannot reach back to on its own', () => {
    expect(rangesFor(true)).toEqual(['15m', '1h']);
  });

  it('keeps the default usable in both', () => {
    expect(rangesFor(true)).toContain(DEFAULT_RANGE);
    expect(rangesFor(false)).toContain(DEFAULT_RANGE);
  });
});

describe('how far back the readings reach', () => {
  const now = Date.UTC(2026, 7, 27, 12, 0, 0);

  it('says nothing when nothing has been collected', () => {
    expect(collectedFor(undefined, now)).toBe('');
    expect(collectedFor(0, now)).toBe('');
  });

  it('counts the first seconds as less than a minute', () => {
    expect(collectedFor(now - 20_000, now)).toBe('less than a minute');
  });

  it('counts minutes, singular and plural', () => {
    expect(collectedFor(now - 60_000, now)).toBe('1 minute');
    expect(collectedFor(now - 12 * 60_000, now)).toBe('12 minutes');
  });

  it('counts hours once there are any', () => {
    expect(collectedFor(now - 60 * 60_000, now)).toBe('1 hour');
    expect(collectedFor(now - 200 * 60_000, now)).toBe('3 hours');
  });
});
