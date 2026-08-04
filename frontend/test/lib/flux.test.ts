import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import type { FluxDashboard } from '../../src/lib/types';
import { fetchFlux, useFlux } from '../../src/lib/flux';
import { anySignal } from '../helpers';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchFlux', () => {
  it('requests /api/flux and returns the parsed dashboard', async () => {
    const dashboard: FluxDashboard = {
      groups: [
        {
          name: 'Sources',
          ready: 1,
          reporting: 1,
          total: 1,
          resources: [
            {
              kind: 'GitRepository',
              group: 'source.toolkit.fluxcd.io',
              version: 'v1',
              resource: 'gitrepositories',
              name: 'app-repo',
              namespace: 'flux-system',
              ready: 'True',
              suspended: false,
              revision: 'main@sha1:abc',
              source: '',
              message: '',
              createdAt: '2026-07-24T00:00:00Z',
            },
          ],
        },
      ],
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(dashboard),
    });
    vi.stubGlobal('fetch', fetchMock);
    const result = await fetchFlux();
    expect(fetchMock).toHaveBeenCalledWith('/api/flux', { signal: anySignal() });
    expect(result).toEqual(dashboard);
  });

  it('throws when the response is not ok', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      json: () => Promise.resolve({ groups: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);
    await expect(fetchFlux()).rejects.toThrow('flux request failed with status 500');
  });
});

describe('polling while a request is still open', () => {
  it('does not stack a second dashboard fetch on top of a slow one', async () => {
    vi.useFakeTimers();
    let calls = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        calls += 1;
        return new Promise(() => {
          return undefined;
        });
      }),
    );

    renderHook(() => useFlux());
    await vi.advanceTimersByTimeAsync(30000);

    expect(calls).toBe(1);
    vi.useRealTimers();
  });

  it('resumes polling once the slow request settles', async () => {
    vi.useFakeTimers();
    let calls = 0;
    const gate: { release: () => void } = {
      release: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        calls += 1;
        return new Promise((resolve) => {
          gate.release = () => {
            resolve({ ok: true, json: () => Promise.resolve({ groups: [] }) });
          };
        });
      }),
    );

    renderHook(() => useFlux());
    await vi.advanceTimersByTimeAsync(20000);
    expect(calls).toBe(1);

    gate.release();
    await vi.advanceTimersByTimeAsync(20000);

    expect(calls).toBeGreaterThan(1);
    vi.useRealTimers();
  });
});
