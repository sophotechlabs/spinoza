import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import {
  fetchHelmHistory,
  fetchHelmRelease,
  fetchHelmReleases,
  fetchHelmSupport,
  fetchChartValues,
  fetchHelmVersions,
  installRelease,
  refOf,
  searchCharts,
  rollbackRelease,
  uninstallRelease,
  upgradeRelease,
  useHelmRelease,
  useHelmSupport,
  latestColor,
  latestLabel,
  latestNote,
  statusDot,
  statusLabel,
  statusText,
  statusTone,
  useHelmReleases,
} from '../../src/lib/helm';
import { bumpHelmEpoch } from '../../src/store/helm';
import { bumpClusterEpoch, useClusterStore } from '../../src/store/cluster';
import { anySignal, capabilities } from '../helpers';

const payload = {
  releases: [
    {
      name: 'podinfo',
      namespace: 'demo',
      chart: 'podinfo',
      chartVersion: '6.9.2',
      appVersion: '6.9.2',
      revision: 3,
      status: 'deployed',
      updated: '2026-08-11T09:30:00Z',
      description: 'Upgrade complete',
    },
  ],
};

afterEach(() => {
  vi.unstubAllGlobals();
  act(() => {
    useClusterStore.getState().reset();
  });
});

describe('fetchHelmReleases', () => {
  it('requests /api/helm and parses what comes back', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) });
    vi.stubGlobal('fetch', fetchMock);

    const got = await fetchHelmReleases();

    expect(fetchMock).toHaveBeenCalledWith('/api/helm', { signal: anySignal() });
    expect(got.releases).toHaveLength(1);
    expect(got.releases[0].chartVersion).toBe('6.9.2');
    expect(got.releases[0].revision).toBe(3);
  });

  it('reads a payload with no releases at all', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) }),
    );

    const got = await fetchHelmReleases();

    expect(got.releases).toEqual([]);
    expect(got.error).toBeUndefined();
  });

  it('carries the server message when the request is refused', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: () => Promise.resolve({ message: 'secrets is forbidden' }),
      }),
    );

    await expect(fetchHelmReleases()).rejects.toThrow('secrets is forbidden');
  });

  it('falls back to the status when the body says nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 500, json: () => Promise.reject(new Error()) }),
    );

    await expect(fetchHelmReleases()).rejects.toThrow('helm request failed with status 500');
  });
});

describe('useHelmReleases', () => {
  it('polls once the view is mounted', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) }),
    );

    const { result } = renderHook(() => useHelmReleases());

    await waitFor(() => {
      expect(result.current.data?.releases).toHaveLength(1);
    });
  });

  it('refetches as soon as a helm action bumps the epoch', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) });
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() => useHelmReleases());
    await waitFor(() => {
      expect(result.current.data?.releases).toHaveLength(1);
    });
    const before = fetchMock.mock.calls.length;

    act(() => {
      bumpHelmEpoch();
    });

    await waitFor(() => {
      expect(fetchMock.mock.calls.length).toBe(before + 1);
    });
  });

  it('stays quiet while the view is hidden', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    renderHook(() => useHelmReleases(false));

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('what a release status looks like', () => {
  it('reads deployed as healthy', () => {
    expect(statusTone('deployed')).toBe('ok');
    expect(statusDot('deployed')).toBe('bg-ok-solid');
    expect(statusText('deployed')).toBe('text-ok');
  });

  it('reads failed and unknown as failures', () => {
    expect(statusTone('failed')).toBe('error');
    expect(statusTone('unknown')).toBe('error');
    expect(statusDot('failed')).toBe('bg-error-solid');
    expect(statusText('failed')).toBe('text-error');
  });

  it('reads anything pending as busy', () => {
    expect(statusTone('pending-upgrade')).toBe('warn');
    expect(statusTone('uninstalling')).toBe('warn');
    expect(statusDot('pending-install')).toBe('bg-warn-solid');
    expect(statusText('pending-rollback')).toBe('text-warn');
  });

  it('reads a superseded or unset status as idle', () => {
    expect(statusTone('superseded')).toBe('idle');
    expect(statusTone('')).toBe('idle');
    expect(statusDot('superseded')).toBe('bg-idle-solid');
    expect(statusText('superseded')).toBe('text-fg-muted');
  });

  it('names an empty status rather than showing a gap', () => {
    expect(statusLabel('')).toBe('unknown');
    expect(statusLabel('deployed')).toBe('deployed');
  });
});

describe('the newest chart version a repository offers', () => {
  it('names the version when there is one', () => {
    expect(latestLabel({ latest: '7.1.0' })).toBe('7.1.0');
  });

  it('shows a dash when no repository knows the chart', () => {
    expect(latestLabel({})).toBe('-');
    expect(latestLabel({ latest: '' })).toBe('-');
  });

  it('colours an outdated release apart from a current one', () => {
    expect(latestColor({ outdated: true })).toBe('text-warn');
    expect(latestColor({ outdated: false })).toBe('text-fg-muted');
    expect(latestColor({})).toBe('text-fg-muted');
  });

  it('spells the state out for a screen reader', () => {
    expect(latestNote({ latest: '7.1.0', outdated: true })).toBe(
      'a newer chart version is available',
    );
    expect(latestNote({ latest: '6.9.2', outdated: false })).toBe('up to date');
    expect(latestNote({})).toBe('no chart repository knows this chart');
  });
});

describe('reading one release', () => {
  it('asks for the release by namespace and name', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          release: payload.releases[0],
          driver: 'secret',
          values: 'replicaCount: 2\n',
          notes: 'hello',
          manifest: 'kind: ConfigMap\n',
          resources: [{ apiVersion: 'v1', kind: 'ConfigMap', name: 'cm', resource: 'configmaps' }],
          history: [
            {
              revision: 3,
              status: 'deployed',
              chartVersion: '6.9.2',
              appVersion: '6.9.2',
              updated: '',
            },
          ],
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await fetchHelmRelease('demo', 'podinfo');

    expect(fetchMock.mock.calls[0][0]).toBe('/api/helm/release?namespace=demo&name=podinfo');
    expect(got.driver).toBe('secret');
    expect(got.resources).toHaveLength(1);
    expect(got.history[0].revision).toBe(3);
  });

  it('carries the server message when the release is gone', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'no such helm release: demo/ghost' }),
      }),
    );

    await expect(fetchHelmRelease('demo', 'ghost')).rejects.toThrow('no such helm release');
  });

  it('reads a payload with nothing in it', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) }),
    );

    const got = await fetchHelmRelease('demo', 'podinfo');

    expect(got.values).toBe('');
    expect(got.resources).toEqual([]);
    expect(got.history).toEqual([]);
  });
});

describe('reading release history', () => {
  it('surfaces a failed history request', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      json: () => Promise.resolve({ message: 'history is unavailable' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchHelmHistory('demo', 'podinfo', 3)).rejects.toThrow('history is unavailable');
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/api/helm/history?namespace=demo&name=podinfo&through=3',
    );
  });
});

describe('whether helm can act', () => {
  it('reports what the server says', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve(
            capabilities({ helm: { available: false, reason: 'not on PATH', binary: 'helm' } }),
          ),
      }),
    );

    const got = await fetchHelmSupport();

    expect(got.available).toBe(false);
    expect(got.reason).toBe('not on PATH');
  });

  it('reports a failed check', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }));

    await expect(fetchHelmSupport()).rejects.toThrow('capabilities request failed with status 503');
  });
});

describe('acting on a release', () => {
  it('posts a rollback with its revision', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'rollback', message: 'done', revision: 2 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await rollbackRelease('demo', 'podinfo', 2);

    const call = fetchMock.mock.calls[0] as [string, { method: string }];
    expect(call[0]).toContain('action=rollback');
    expect(call[0]).toContain('revision=2');
    expect(call[1].method).toBe('POST');
    expect(got.revision).toBe(2);
  });

  it('carries the typed confirmation into a rollback', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'rollback', message: 'done', revision: 2 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await rollbackRelease('demo', 'podinfo', 2, 'podinfo');

    expect(String(fetchMock.mock.calls[0][0])).toContain('confirm=podinfo');
  });

  it('carries the typed confirmation into an uninstall', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'uninstall', message: 'gone' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await uninstallRelease('demo', 'podinfo', 'podinfo');

    expect(String(fetchMock.mock.calls[0][0])).toContain('confirm=podinfo');
  });

  it('posts an uninstall', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'uninstall', message: 'gone' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await uninstallRelease('demo', 'podinfo');

    expect(fetchMock.mock.calls[0][0]).toContain('action=uninstall');
    expect(got.message).toBe('gone');
    expect(got.revision).toBeUndefined();
  });

  it('carries the failure the server reports', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'release: not found' }),
      }),
    );

    await expect(uninstallRelease('demo', 'podinfo')).rejects.toThrow('release: not found');
  });

  it('falls back to the status when the body says nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 502, json: () => Promise.reject(new Error()) }),
    );

    await expect(rollbackRelease('demo', 'podinfo', 1)).rejects.toThrow(
      'the release action failed with status 502',
    );
  });
});

describe('looking up chart versions', () => {
  it('asks for the chart and parses the grouped versions', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          chart: 'podinfo',
          repos: [{ name: 'podinfo', url: 'https://example.com', versions: ['6.15.1', '6.14.0'] }],
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await fetchHelmVersions('podinfo');

    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/helm/versions?chart=podinfo');
    expect(got.repos[0].versions).toEqual(['6.15.1', '6.14.0']);
  });

  it('carries the failure the server reports', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'chart is required' }),
      }),
    );

    await expect(fetchHelmVersions('podinfo')).rejects.toThrow('chart is required');
  });
});

describe('upgrading a release', () => {
  const args = {
    namespace: 'demo',
    name: 'podinfo',
    chart: 'podinfo',
    repo: 'https://example.com',
    version: '6.15.1',
    values: 'replicaCount: 2\n',
  };

  it('posts the upgrade as a json body', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'upgrade', message: 'upgraded' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await upgradeRelease(args, false);

    const call = fetchMock.mock.calls[0] as [string, { method: string; body: string }];
    expect(call[0]).toBe('/api/helm/upgrade');
    expect(call[1].method).toBe('POST');
    expect(JSON.parse(call[1].body)).toEqual(args);
    expect(got.message).toBe('upgraded');
  });

  it('asks for a dry run in the query', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({ action: 'upgrade', dryRun: true, manifest: 'kind: ConfigMap\n' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await upgradeRelease(args, true);

    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/helm/upgrade?dryRun=true');
    expect(got.dryRun).toBe(true);
    expect(got.manifest).toBe('kind: ConfigMap\n');
  });

  it('carries the typed confirmation', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'upgrade', message: 'upgraded' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await upgradeRelease(args, false, 'podinfo');

    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/helm/upgrade?confirm=podinfo');
  });

  it('carries the failure the server reports', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 409,
        json: () => Promise.resolve({ message: 'managed by flux' }),
      }),
    );

    await expect(upgradeRelease(args, false)).rejects.toThrow('managed by flux');
  });

  it('falls back to the status when the body says nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 502, json: () => Promise.reject(new Error()) }),
    );

    await expect(upgradeRelease(args, false)).rejects.toThrow('the upgrade failed with status 502');
  });
});

describe('turning a rendered resource into an object reference', () => {
  it('builds a reference discovery could resolve', () => {
    expect(
      refOf({
        apiVersion: 'apps/v1',
        kind: 'Deployment',
        name: 'web',
        namespace: 'demo',
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
      }),
    ).toEqual({
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: 'demo',
      name: 'web',
    });
  });

  it('refuses one discovery could not', () => {
    expect(refOf({ apiVersion: 'acme.io/v1', kind: 'Widget', name: 'thing' })).toBeNull();
    expect(
      refOf({ apiVersion: 'acme.io/v1', kind: 'Widget', name: 'thing', resource: '' }),
    ).toBeNull();
  });

  it('reads a cluster-scoped resource with no namespace', () => {
    expect(
      refOf({
        apiVersion: 'v1',
        kind: 'Namespace',
        name: 'demo',
        version: 'v1',
        resource: 'namespaces',
      }),
    ).toEqual({ group: '', version: 'v1', resource: 'namespaces', namespace: '', name: 'demo' });
  });
});

describe('the release detail hook', () => {
  it('waits for both coordinates before asking', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    const { result } = renderHook(() => useHelmRelease('', ''));

    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.current.data).toBeNull();
    expect(result.current.loading).toBe(false);
  });

  it('reloads on demand', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ release: payload.releases[0], driver: 'secret' }),
    });
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() => useHelmRelease('demo', 'podinfo'));
    await waitFor(() => {
      expect(result.current.data).not.toBeNull();
    });

    act(() => {
      result.current.reload();
    });

    await waitFor(() => {
      expect(fetchMock.mock.calls.length).toBe(2);
    });
  });

  it('hides the previous release while another target loads', async () => {
    const pending = new Promise<never>(() => undefined);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ release: payload.releases[0], driver: 'secret' }),
      })
      .mockImplementationOnce(() => pending);
    vi.stubGlobal('fetch', fetchMock);
    const { result, rerender } = renderHook(
      ({ name }: { name: string }) => useHelmRelease('demo', name),
      { initialProps: { name: 'podinfo' } },
    );
    await waitFor(() => {
      expect(result.current.data).not.toBeNull();
    });

    rerender({ name: 'other' });

    expect(result.current.data).toBeNull();
    expect(result.current.loading).toBe(true);
  });

  it('hides the previous clusters release while the replacement loads', async () => {
    const pending = new Promise<never>(() => undefined);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ release: payload.releases[0], driver: 'secret' }),
      })
      .mockImplementationOnce(() => pending);
    vi.stubGlobal('fetch', fetchMock);
    const { result, unmount } = renderHook(() => useHelmRelease('demo', 'podinfo'));
    await waitFor(() => {
      expect(result.current.data).not.toBeNull();
    });

    await act(async () => {
      bumpClusterEpoch();
      await Promise.resolve();
    });

    expect(result.current.data).toBeNull();
    expect(result.current.loading).toBe(true);
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
    unmount();
  });

  it('reports a failure and drops any stale detail', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('helm is down')));
    const { result } = renderHook(() => useHelmRelease('demo', 'podinfo'));

    await waitFor(() => {
      expect(result.current.error).toBe('helm is down');
    });
    expect(result.current.data).toBeNull();
  });

  it('reports a non-Error rejection plainly', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    const { result } = renderHook(() => useHelmRelease('demo', 'podinfo'));

    await waitFor(() => {
      expect(result.current.error).toBe('the request failed');
    });
  });
});

describe('the helm support hook', () => {
  it('reports what the server says', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(capabilities()),
      }),
    );

    const { result } = renderHook(() => useHelmSupport());

    await waitFor(() => {
      expect(result.current?.available).toBe(true);
    });
  });

  it('treats a failed check as helm being unusable', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    const { result } = renderHook(() => useHelmSupport());

    await waitFor(() => {
      expect(result.current?.available).toBe(false);
    });
    expect(result.current?.reason).toBe('offline');
  });

  it('rechecks support after a cluster switch and drops the old answer', async () => {
    let answerFirst: (body: unknown) => void = () => undefined;
    const first = new Promise((resolve) => {
      answerFirst = resolve;
    });
    let asked = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        asked += 1;
        if (asked === 1) {
          return first;
        }
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(
              capabilities({
                helm: { available: false, reason: 'helm is not installed', binary: 'helm' },
              }),
            ),
        });
      }),
    );

    const { result } = renderHook(() => useHelmSupport());
    await waitFor(() => {
      expect(asked).toBe(1);
    });

    act(() => {
      bumpClusterEpoch();
    });

    expect(result.current).toBeNull();
    await waitFor(() => {
      expect(result.current?.available).toBe(false);
    });

    answerFirst({ ok: true, json: () => Promise.resolve(capabilities()) });
    await waitFor(() => {
      expect(result.current?.available).toBe(false);
    });
  });
});

describe('searching for a chart', () => {
  it('asks the backend with the query', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          query: 'podinfo',
          hits: [{ chart: 'podinfo', version: '6.15.1', url: 'https://example.com' }],
          truncated: true,
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await searchCharts('podinfo');

    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/helm/charts?query=podinfo');
    expect(got.hits[0].chart).toBe('podinfo');
    expect(got.truncated).toBe(true);
  });

  it('carries the failure the server reports', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'the index is unreachable' }),
      }),
    );

    await expect(searchCharts('podinfo')).rejects.toThrow('the index is unreachable');
  });

  it('falls back to the status when the body says nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 502, json: () => Promise.reject(new Error()) }),
    );

    await expect(searchCharts('podinfo')).rejects.toThrow(
      'the chart search failed with status 502',
    );
  });
});

describe('reading the chart defaults', () => {
  it('names the chart, the repository and the version', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({ chart: 'podinfo', version: '6.15.1', values: 'replicaCount: 1\n' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await fetchChartValues('podinfo', 'https://example.com', '6.15.1');

    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain('chart=podinfo');
    expect(url).toContain('repo=https%3A%2F%2Fexample.com');
    expect(url).toContain('version=6.15.1');
    expect(got.values).toBe('replicaCount: 1\n');
  });

  it('carries the failure the server reports', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'chart not found' }),
      }),
    );

    await expect(fetchChartValues('podinfo', 'https://example.com', '6.15.1')).rejects.toThrow(
      'chart not found',
    );
  });

  it('falls back to the status when the body says nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 502, json: () => Promise.reject(new Error()) }),
    );

    await expect(fetchChartValues('podinfo', 'https://example.com', '6.15.1')).rejects.toThrow(
      'the chart values request failed with status 502',
    );
  });
});

describe('installing a chart', () => {
  const args = {
    namespace: 'demo',
    name: 'podinfo',
    chart: 'podinfo',
    repo: 'https://example.com',
    version: '6.15.1',
    values: 'replicaCount: 2\n',
    createNamespace: true,
  };

  it('posts the install as a json body', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'install', message: 'installed' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await installRelease(args, false);

    const call = fetchMock.mock.calls[0] as [string, { method: string; body: string }];
    expect(call[0]).toBe('/api/helm/install');
    expect(call[1].method).toBe('POST');
    expect(JSON.parse(call[1].body)).toEqual(args);
    expect(got.message).toBe('installed');
  });

  it('asks for a dry run in the query', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'install', dryRun: true, manifest: 'kind: Service\n' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await installRelease(args, true);

    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/helm/install?dryRun=true');
    expect(got.manifest).toBe('kind: Service\n');
  });

  it('carries the typed confirmation', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ action: 'install', message: 'installed' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await installRelease(args, false, 'podinfo');

    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/helm/install?confirm=podinfo');
  });

  it('carries the failure the server reports', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'cannot re-use a name' }),
      }),
    );

    await expect(installRelease(args, false)).rejects.toThrow('cannot re-use a name');
  });

  it('falls back to the status when the body says nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 502, json: () => Promise.reject(new Error()) }),
    );

    await expect(installRelease(args, false)).rejects.toThrow('the install failed with status 502');
  });
});
