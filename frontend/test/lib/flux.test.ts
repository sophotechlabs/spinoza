import { afterEach, describe, expect, it, vi } from 'vitest';
import type { FluxDashboard } from '../../src/lib/types';
import { fetchFlux } from '../../src/lib/flux';

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
    expect(fetchMock).toHaveBeenCalledWith('/api/flux');
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
