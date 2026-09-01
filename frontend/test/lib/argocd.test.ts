import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import {
  fetchArgo,
  graphOf,
  idOf,
  readyOf,
  refOf,
  statusOf,
  tree,
  useArgo,
} from '../../src/lib/argocd';
import type { ArgoApp, ArgoDashboard } from '../../src/lib/types';
import { bumpClusterEpoch, useClusterStore } from '../../src/store/cluster';

function dashboard(extra: Partial<ArgoDashboard> = {}): ArgoDashboard {
  return { apps: [], applicationSets: [], projects: [], ...extra };
}

function makeApp(name: string, extra: Partial<ArgoApp> = {}): ArgoApp {
  return {
    kind: 'Application',
    group: 'argoproj.io',
    version: 'v1alpha1',
    resource: 'applications',
    name,
    namespace: 'argocd',
    project: 'default',
    sync: 'Synced',
    health: 'Healthy',
    revision: 'abc123',
    repo: 'https://git/apps',
    path: `apps/${name}`,
    destination: 'in-cluster shop',
    message: '',
    createdAt: '2026-08-17T09:00:00Z',
    ...extra,
  };
}

function stub(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn(() => Promise.resolve({ ok, status, json: () => Promise.resolve(body) }));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  useClusterStore.getState().reset();
});

describe('refOf', () => {
  it('turns an app into something the inspector can open', () => {
    expect(refOf(makeApp('web'))).toEqual({
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'applications',
      namespace: 'argocd',
      name: 'web',
    });
  });
});

describe('fetchArgo', () => {
  it('reads the dashboard', async () => {
    stub({ apps: [makeApp('web')], applicationSets: [] });

    const found = await fetchArgo();

    expect(found.apps).toHaveLength(1);
    expect(found.apps[0].name).toBe('web');
  });

  it('fills in what the backend left out', async () => {
    stub({ apps: [{ name: 'bare' }] });

    const found = await fetchArgo();

    expect(found.apps[0]).toMatchObject({ name: 'bare', sync: '', health: '', project: '' });
    expect(found.applicationSets).toEqual([]);
    expect(found.projects).toEqual([]);
  });

  it('reads the projects alongside the applications', async () => {
    stub({
      apps: [],
      applicationSets: [],
      projects: [{ name: 'default', kind: 'AppProject', resource: 'appprojects' }],
    });

    expect((await fetchArgo()).projects[0]).toMatchObject({
      name: 'default',
      kind: 'AppProject',
    });
  });

  it('carries a partial failure', async () => {
    stub({ apps: [], applicationSets: [], error: 'applications is forbidden' });

    expect((await fetchArgo()).error).toBe('applications is forbidden');
  });

  it('reports a request the backend refused', async () => {
    stub({ message: 'spinoza has no cluster' }, false, 503);

    await expect(fetchArgo()).rejects.toThrow('no cluster');
  });
});

describe('tree', () => {
  it('keeps standalone apps flat', () => {
    const rows = tree([makeApp('a'), makeApp('b')]);

    expect(rows.map((row) => [row.app.name, row.depth])).toEqual([
      ['a', 0],
      ['b', 0],
    ]);
  });

  it('nests an app of apps under its parent', () => {
    const rows = tree([
      makeApp('root'),
      makeApp('web', { owner: 'root' }),
      makeApp('api', { owner: 'root' }),
    ]);

    expect(rows.map((row) => [row.app.name, row.depth])).toEqual([
      ['root', 0],
      ['web', 1],
      ['api', 1],
    ]);
  });

  it('nests deeper than one level', () => {
    const rows = tree([
      makeApp('root'),
      makeApp('team', { owner: 'root' }),
      makeApp('web', { owner: 'team' }),
    ]);

    expect(rows.map((row) => row.depth)).toEqual([0, 1, 2]);
  });

  it('still lists an app whose parent is missing', () => {
    const rows = tree([makeApp('orphan', { owner: 'gone' })]);

    expect(rows.map((row) => [row.app.name, row.depth])).toEqual([['orphan', 0]]);
  });

  it('does not loop when two apps own each other', () => {
    const rows = tree([makeApp('a', { owner: 'b' }), makeApp('b', { owner: 'a' })]);

    expect(rows).toHaveLength(2);
  });

  it('has nothing to show for no apps', () => {
    expect(tree([])).toEqual([]);
  });
});

describe('idOf', () => {
  it('names a node by its resource, namespace and name', () => {
    expect(idOf(makeApp('web'))).toBe('applications/argocd/web');
  });
});

describe('readyOf', () => {
  it('is ready only when the app is both synced and healthy', () => {
    expect(readyOf(makeApp('web'))).toBe('True');
    expect(readyOf(makeApp('web', { sync: 'OutOfSync' }))).toBe('Unknown');
  });

  it('is not ready when the health is degraded or missing', () => {
    expect(readyOf(makeApp('web', { health: 'Degraded' }))).toBe('False');
    expect(readyOf(makeApp('web', { health: 'Missing' }))).toBe('False');
  });

  it('knows nothing about a resource that reports no health', () => {
    expect(readyOf(makeApp('web', { health: '', sync: '' }))).toBe('Unknown');
  });
});

describe('statusOf', () => {
  it('joins the sync and health words', () => {
    expect(statusOf(makeApp('web'))).toBe('Synced Healthy');
  });

  it('leaves out the half a resource does not report', () => {
    expect(statusOf(makeApp('web', { sync: '' }))).toBe('Healthy');
    expect(statusOf(makeApp('web', { sync: '', health: '' }))).toBe('');
  });
});

describe('graphOf', () => {
  it('draws an edge from a parent app to its child', () => {
    const graph = graphOf(
      dashboard({ apps: [makeApp('root'), makeApp('web', { owner: 'root' })] }),
    );

    expect(graph.nodes.map((node) => node.id)).toEqual([
      'applications/argocd/root',
      'applications/argocd/web',
    ]);
    expect(graph.edges).toEqual([
      {
        from: 'applications/argocd/root',
        to: 'applications/argocd/web',
        kind: 'manages',
      },
    ]);
  });

  it('puts an application set before the apps it generates', () => {
    const set = makeApp('shops', { kind: 'ApplicationSet', resource: 'applicationsets' });
    const graph = graphOf(
      dashboard({ apps: [makeApp('web', { owner: 'shops' })], applicationSets: [set] }),
    );

    expect(graph.nodes[0]).toMatchObject({ name: 'shops', category: 'applier' });
    expect(graph.nodes[1]).toMatchObject({ name: 'web', category: 'app' });
    expect(graph.edges[0].from).toBe('applicationsets/argocd/shops');
  });

  it('drops an edge to a parent that is not in the dashboard', () => {
    const graph = graphOf(dashboard({ apps: [makeApp('web', { owner: 'gone' })] }));

    expect(graph.edges).toEqual([]);
  });

  it('leaves an app with no parent alone', () => {
    const graph = graphOf(dashboard({ apps: [makeApp('web')] }));

    expect(graph.edges).toEqual([]);
  });

  it('carries the partial failure through to the canvas', () => {
    const graph = graphOf(dashboard({ error: 'applications is forbidden' }));

    expect(graph.error).toBe('applications is forbidden');
  });
});

describe('useArgo', () => {
  it('asks again when the caller reloads it', async () => {
    const asked = stub(dashboard());
    const { result } = renderHook(() => useArgo());
    await waitFor(() => {
      expect(result.current.data).not.toBeNull();
    });

    act(() => {
      result.current.reload();
    });

    await waitFor(() => {
      expect(asked).toHaveBeenCalledTimes(2);
    });
  });

  it('reports a sweep that failed', async () => {
    stub({ message: 'argo is unreachable' }, false, 503);
    const { result } = renderHook(() => useArgo());

    await waitFor(() => {
      expect(result.current.error).toContain('argo is unreachable');
    });
  });

  it('drops an error from the previous cluster while loading the next one', async () => {
    let holdNext!: () => void;
    const next = new Promise<{ ok: boolean; json: () => Promise<ArgoDashboard> }>((resolve) => {
      holdNext = () => {
        resolve({ ok: true, json: () => Promise.resolve(dashboard()) });
      };
    });
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce({
          ok: false,
          status: 503,
          json: () => Promise.resolve({ message: 'old cluster refused applications' }),
        })
        .mockImplementationOnce(() => next),
    );
    const { result } = renderHook(() => useArgo());
    await waitFor(() => {
      expect(result.current.error).toContain('old cluster refused applications');
    });

    act(() => {
      bumpClusterEpoch();
    });

    expect(result.current.error).toBeNull();
    holdNext();
  });

  it('does not overlap scheduled refreshes', async () => {
    vi.useFakeTimers();
    let finishFirst!: (response: { ok: boolean; json: () => Promise<ArgoDashboard> }) => void;
    const first = new Promise<{ ok: boolean; json: () => Promise<ArgoDashboard> }>((resolve) => {
      finishFirst = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(() => first)
      .mockResolvedValue({ ok: true, json: () => Promise.resolve(dashboard()) });
    vi.stubGlobal('fetch', fetchMock);
    renderHook(() => useArgo());

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    await act(async () => {
      finishFirst({ ok: true, json: () => Promise.resolve(dashboard()) });
      await first;
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
