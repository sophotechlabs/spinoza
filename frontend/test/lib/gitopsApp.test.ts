import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import {
  fetchGitopsApp,
  fetchGitopsAppGraph,
  isGitopsApp,
  parseGitopsApp,
  useGitopsApp,
} from '../../src/lib/gitopsApp';
import type { ObjectRef } from '../../src/lib/types';
import { bumpClusterEpoch, useClusterStore } from '../../src/store/cluster';

const ref: ObjectRef = {
  group: 'argoproj.io',
  version: 'v1alpha1',
  resource: 'applications',
  namespace: 'argocd',
  name: 'podinfo',
};

const full = {
  ref,
  controller: 'argocd',
  kind: 'Application',
  name: 'podinfo',
  namespace: 'argocd',
  source: {
    repo: 'https://example.test/apps',
    path: 'podinfo',
    target: 'main',
    destination: 'web',
    project: 'default',
    syncMode: 'auto',
    policy: 'prune',
  },
  state: { sync: 'Synced', health: 'Healthy', revision: 'abc', syncedAt: 'now', message: 'ok' },
  issues: [{ severity: 'warning', title: 'Drifting', detail: 'why', subject: 'drift' }],
  resources: [
    {
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      kind: 'Deployment',
      name: 'podinfo',
      namespace: 'web',
      sync: 'OutOfSync',
      health: 'Healthy',
      terminating: true,
      finalizers: ['foregroundDeletion'],
      drift: [{ path: 'spec.replicas', declared: '1', live: '3' }],
      driftOwners: true,
      driftNote: 'more',
      events: [{ type: 'Warning', reason: 'Failed', message: 'nope', lastSeen: 'now' }],
      eventsTruncated: true,
    },
  ],
  history: [
    { id: 2, revision: 'abc', source: 'podinfo', deployedAt: 'then', initiatedBy: 'someone' },
  ],
  operation: { phase: 'Running', running: true, message: 'syncing', cause: 'because' },
};

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  useClusterStore.getState().reset();
});

describe('recognising a gitops applier', () => {
  it('takes an argo application', () => {
    expect(isGitopsApp('argoproj.io/v1alpha1', 'Application')).toBe(true);
  });

  it('takes the two flux appliers', () => {
    expect(isGitopsApp('kustomize.toolkit.fluxcd.io/v1', 'Kustomization')).toBe(true);
    expect(isGitopsApp('helm.toolkit.fluxcd.io/v2', 'HelmRelease')).toBe(true);
  });

  it('refuses a source and a plain workload', () => {
    expect(isGitopsApp('source.toolkit.fluxcd.io/v1', 'GitRepository')).toBe(false);
    expect(isGitopsApp('apps/v1', 'Deployment')).toBe(false);
  });
});

describe('reading one application', () => {
  it('keeps every field the backend sent', () => {
    const app = parseGitopsApp(full, ref);

    expect(app.controller).toBe('argocd');
    expect(app.source.repo).toBe('https://example.test/apps');
    expect(app.state.sync).toBe('Synced');
    expect(app.issues?.[0].title).toBe('Drifting');
    expect(app.resources?.[0].drift?.[0]).toEqual({
      path: 'spec.replicas',
      declared: '1',
      live: '3',
    });
    expect(app.resources?.[0].finalizers).toEqual(['foregroundDeletion']);
    expect(app.resources?.[0].driftOwners).toBe(true);
    expect(app.resources?.[0].events?.[0].reason).toBe('Failed');
    expect(app.resources?.[0].eventsTruncated).toBe(true);
    expect(app.history?.[0].id).toBe(2);
    expect(app.operation?.running).toBe(true);
  });

  it('fills in what an empty answer leaves out', () => {
    const app = parseGitopsApp({}, ref);

    expect(app.ref).toEqual(ref);
    expect(app.controller).toBe('');
    expect(app.source.syncMode).toBe('');
    expect(app.issues).toEqual([]);
    expect(app.resources).toEqual([]);
    expect(app.history).toEqual([]);
    expect(app.operation).toBeUndefined();
  });

  it('fills in half-written entries', () => {
    const app = parseGitopsApp(
      { issues: [{}], resources: [{ drift: [{}] }], history: [{}], operation: {} },
      ref,
    );

    expect(app.issues?.[0].severity).toBe('info');
    expect(app.resources?.[0].kind).toBe('');
    expect(app.resources?.[0].drift?.[0].path).toBe('');
    expect(app.history?.[0].id).toBe(0);
    expect(app.operation?.phase).toBe('');
  });

  it('reads an operation that is not there as nothing', () => {
    expect(parseGitopsApp({ operation: null }, ref).operation).toBeUndefined();
  });
});

describe('fetching one application', () => {
  it('asks the endpoint for the object it was given', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(full) });
    vi.stubGlobal('fetch', fetchMock);

    const app = await fetchGitopsApp(ref);

    expect(fetchMock.mock.calls[0][0]).toContain('/api/gitops/app?');
    expect(fetchMock.mock.calls[0][0]).toContain('name=podinfo');
    expect(app.name).toBe('podinfo');
  });

  it('reports what the server said', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 409,
        json: () => Promise.resolve({ message: 'not an applier' }),
      }),
    );

    await expect(fetchGitopsApp(ref)).rejects.toThrow('not an applier');
  });

  it('falls back to the status when the body says nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 500, json: () => Promise.reject(new Error()) }),
    );

    await expect(fetchGitopsApp(ref)).rejects.toThrow('failed with status 500');
  });
});

describe('fetching the managed-resource graph', () => {
  it('asks the graph endpoint', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue({ ok: true, json: () => Promise.resolve({ nodes: [], edges: [] }) });
    vi.stubGlobal('fetch', fetchMock);

    const graph = await fetchGitopsAppGraph(ref);

    expect(fetchMock.mock.calls[0][0]).toContain('/api/gitops/app/graph?');
    expect(graph.nodes).toEqual([]);
  });

  it('reports a failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 404, json: () => Promise.reject(new Error()) }),
    );

    await expect(fetchGitopsAppGraph(ref)).rejects.toThrow('failed with status 404');
  });
});

describe('watching one application', () => {
  it('loads it and reloads on demand', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(full) });
    vi.stubGlobal('fetch', fetchMock);

    const { result } = renderHook(() => useGitopsApp(ref));

    await waitFor(() => {
      expect(result.current.data?.name).toBe('podinfo');
    });
    result.current.reload();
    await waitFor(() => {
      expect(fetchMock.mock.calls.length).toBeGreaterThan(1);
    });
  });

  it('asks for nothing while the panel is not the one showing', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(full) });
    vi.stubGlobal('fetch', fetchMock);

    const { result } = renderHook(() => useGitopsApp(ref, false));

    await waitFor(() => {
      expect(result.current.data).toBeNull();
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('starts asking the moment the panel is shown', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(full) });
    vi.stubGlobal('fetch', fetchMock);
    const { result, rerender } = renderHook(
      ({ active }: { active: boolean }) => useGitopsApp(ref, active),
      { initialProps: { active: false } },
    );
    expect(fetchMock).not.toHaveBeenCalled();

    rerender({ active: true });

    await waitFor(() => {
      expect(result.current.data?.name).toBe('podinfo');
    });
  });

  it('hides the previous application while another target loads', async () => {
    const other: ObjectRef = { ...ref, name: 'other' };
    let finishNext!: (response: { ok: boolean; json: () => Promise<typeof full> }) => void;
    const next = new Promise<{ ok: boolean; json: () => Promise<typeof full> }>((resolve) => {
      finishNext = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(full) })
      .mockImplementationOnce(() => next);
    vi.stubGlobal('fetch', fetchMock);
    const { result, rerender } = renderHook(
      ({ target }: { target: ObjectRef }) => useGitopsApp(target),
      { initialProps: { target: ref } },
    );
    await waitFor(() => {
      expect(result.current.data?.name).toBe('podinfo');
    });

    rerender({ target: other });

    expect(result.current.data).toBeNull();
    await act(async () => {
      finishNext({
        ok: true,
        json: () => Promise.resolve({ ...full, ref: other, name: 'other' }),
      });
      await next;
    });
    await waitFor(() => {
      expect(result.current.data?.name).toBe('other');
    });
  });

  it('hides the previous clusters application while the replacement loads', async () => {
    const pending = new Promise<never>(() => undefined);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(full) })
      .mockImplementationOnce(() => pending);
    vi.stubGlobal('fetch', fetchMock);
    const { result, unmount } = renderHook(() => useGitopsApp(ref));
    await waitFor(() => {
      expect(result.current.data?.name).toBe('podinfo');
    });

    await act(async () => {
      bumpClusterEpoch();
      await Promise.resolve();
    });

    expect(result.current.data).toBeNull();
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
    unmount();
  });

  it('holds nothing when there is no object to watch', () => {
    const { result } = renderHook(() => useGitopsApp(null));

    expect(result.current.data).toBeNull();
  });

  it('reports a failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'gone' }),
      }),
    );

    const { result } = renderHook(() => useGitopsApp(ref));

    await waitFor(() => {
      expect(result.current.error).toBe('gone');
    });
  });

  it('drops the previous application error while a different application loads', async () => {
    let finishNext!: (response: { ok: boolean; json: () => Promise<typeof full> }) => void;
    const next = new Promise<{ ok: boolean; json: () => Promise<typeof full> }>((resolve) => {
      finishNext = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'the old application is gone' }),
      })
      .mockImplementationOnce(() => next);
    vi.stubGlobal('fetch', fetchMock);
    const replacement = { ...ref, name: 'checkout' };
    const { result, rerender } = renderHook(
      ({ target }: { target: ObjectRef }) => useGitopsApp(target),
      { initialProps: { target: ref } },
    );
    await waitFor(() => {
      expect(result.current.error).toBe('the old application is gone');
    });

    rerender({ target: replacement });

    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeNull();
    finishNext({
      ok: true,
      json: () => Promise.resolve({ ...full, ref: replacement, name: 'checkout' }),
    });
    await waitFor(() => {
      expect(result.current.data?.name).toBe('checkout');
    });
  });

  it('drops the previous application when the cluster changes', async () => {
    let finishNext!: (response: { ok: boolean; json: () => Promise<typeof full> }) => void;
    const next = new Promise<{ ok: boolean; json: () => Promise<typeof full> }>((resolve) => {
      finishNext = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(full) })
      .mockImplementationOnce(() => next);
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() => useGitopsApp(ref));
    await waitFor(() => {
      expect(result.current.data?.name).toBe('podinfo');
    });

    act(() => {
      bumpClusterEpoch();
    });

    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeNull();
    finishNext({ ok: true, json: () => Promise.resolve(full) });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
  });

  it('falls back to a plain message for a rejection that is not an Error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));

    const { result } = renderHook(() => useGitopsApp(ref));

    await waitFor(() => {
      expect(result.current.error).toBe('the application request failed');
    });
  });

  it('does not overlap scheduled refreshes', async () => {
    vi.useFakeTimers();
    let finishFirst!: (response: { ok: boolean; json: () => Promise<typeof full> }) => void;
    const first = new Promise<{ ok: boolean; json: () => Promise<typeof full> }>((resolve) => {
      finishFirst = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(() => first)
      .mockResolvedValue({ ok: true, json: () => Promise.resolve(full) });
    vi.stubGlobal('fetch', fetchMock);
    renderHook(() => useGitopsApp(ref));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    await act(async () => {
      finishFirst({ ok: true, json: () => Promise.resolve(full) });
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
