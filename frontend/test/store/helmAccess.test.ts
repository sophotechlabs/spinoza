import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import {
  helmAccessKey,
  useHelmAccessStore,
  useHelmRefusal,
  useHelmRefusals,
} from '../../src/store/helmAccess';
import { contextScope, useContextsStore, EMPTY_CONTEXTS } from '../../src/store/contexts';
import { useHelmAccess } from '../../src/lib/useHelmAccess';
import { bumpClusterEpoch, useClusterStore } from '../../src/store/cluster';

const clusterKey = contextScope({ kubeconfig: '', name: 'p-mk1' }, 0);
const releaseKey = helmAccessKey(clusterKey, 'demo', 'podinfo');
const installKey = helmAccessKey(clusterKey, 'demo', '');

function onCluster(name: string, kubeconfig = ''): void {
  useContextsStore.getState().setList({ ...EMPTY_CONTEXTS, current: { kubeconfig, name } });
}

function reset(): void {
  useHelmAccessStore.setState({ answers: {} });
  useClusterStore.getState().reset();
  onCluster('p-mk1');
}

function replies(refused: { capability: string; reason: string }[]) {
  const fetchMock = vi
    .fn()
    .mockResolvedValue({ ok: true, json: () => Promise.resolve({ refused }) });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

describe('what the cluster refuses a helm action', () => {
  beforeEach(reset);

  it('says nothing before anything has been asked', () => {
    const { result } = renderHook(() => useHelmRefusal('demo', 'podinfo', 'upgrade'));

    expect(result.current).toBeNull();
  });

  it('gives the reason once it is known', () => {
    useHelmAccessStore.getState().setRefused(releaseKey, { upgrade: 'no creating secrets' });

    const { result } = renderHook(() => useHelmRefusal('demo', 'podinfo', 'upgrade'));

    expect(result.current).toBe('no creating secrets');
  });

  it('keeps the answers for one release off another', () => {
    useHelmAccessStore.getState().setRefused(releaseKey, { upgrade: 'no creating secrets' });

    const { result } = renderHook(() => useHelmRefusal('demo', 'other', 'upgrade'));

    expect(result.current).toBeNull();
  });

  it('keeps the answers for one namespace off another', () => {
    useHelmAccessStore.getState().setRefused(releaseKey, { upgrade: 'no creating secrets' });

    const { result } = renderHook(() => useHelmRefusal('prod', 'podinfo', 'upgrade'));

    expect(result.current).toBeNull();
  });

  it('keeps the answers for one cluster off another', () => {
    useHelmAccessStore.getState().setRefused(releaseKey, { upgrade: 'no creating secrets' });
    onCluster('p-mk2');

    const { result } = renderHook(() => useHelmRefusal('demo', 'podinfo', 'upgrade'));

    expect(result.current).toBeNull();
  });

  it('keeps same-named contexts from different kubeconfigs apart', () => {
    useHelmAccessStore.getState().setRefused(releaseKey, { upgrade: 'no creating secrets' });
    onCluster('p-mk1', '/other/config');

    const { result } = renderHook(() => useHelmRefusal('demo', 'podinfo', 'upgrade'));

    expect(result.current).toBeNull();
  });

  it('leaves the actions it was not told about alone', () => {
    useHelmAccessStore.getState().setRefused(releaseKey, { uninstall: 'no deleting secrets' });

    const { result } = renderHook(() => useHelmRefusal('demo', 'podinfo', 'upgrade'));

    expect(result.current).toBeNull();
  });

  it('says nothing without a namespace', () => {
    const { result } = renderHook(() => useHelmRefusals('', 'podinfo'));

    expect(result.current).toEqual({});
  });

  it('holds an answer for a release and one for an install at once', () => {
    useHelmAccessStore.getState().setRefused(releaseKey, { uninstall: 'no deleting secrets' });
    useHelmAccessStore.getState().setRefused(installKey, { install: 'no creating secrets' });

    const release = renderHook(() => useHelmRefusal('demo', 'podinfo', 'uninstall'));
    const install = renderHook(() => useHelmRefusal('demo', '', 'install'));

    expect(release.result.current).toBe('no deleting secrets');
    expect(install.result.current).toBe('no creating secrets');
  });
});

describe('asking when a release is opened', () => {
  beforeEach(() => {
    reset();
    vi.unstubAllGlobals();
  });

  it('records what the server says', async () => {
    replies([{ capability: 'upgrade', reason: 'no creating secrets' }]);

    renderHook(() => {
      useHelmAccess('demo', 'podinfo');
    });

    await waitFor(() => {
      expect(useHelmAccessStore.getState().answers[releaseKey]).toEqual({
        upgrade: 'no creating secrets',
      });
    });
  });

  it('asks nothing without a namespace', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    renderHook(() => {
      useHelmAccess('', 'podinfo');
    });

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('forgets its answer when the question cannot be put', async () => {
    useHelmAccessStore.getState().setRefused(releaseKey, { upgrade: 'no creating secrets' });
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    renderHook(() => {
      useHelmAccess('demo', 'podinfo');
    });

    await waitFor(() => {
      expect(useHelmAccessStore.getState().answers[releaseKey]).toBeUndefined();
    });
  });

  it('leaves the other answers alone when one question fails', async () => {
    useHelmAccessStore.getState().setRefused(installKey, { install: 'no creating secrets' });
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    renderHook(() => {
      useHelmAccess('demo', 'podinfo');
    });

    await waitFor(() => {
      expect(useHelmAccessStore.getState().answers[installKey]).toEqual({
        install: 'no creating secrets',
      });
    });
  });

  it('asks again when the release changes', async () => {
    const fetchMock = replies([]);

    const { rerender } = renderHook(
      ({ name }: { name: string }) => {
        useHelmAccess('demo', name);
      },
      { initialProps: { name: 'podinfo' } },
    );
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    rerender({ name: 'other' });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
  });

  it('asks again when the namespace changes', async () => {
    const fetchMock = replies([]);

    const { rerender } = renderHook(
      ({ namespace }: { namespace: string }) => {
        useHelmAccess(namespace, '');
      },
      { initialProps: { namespace: 'demo' } },
    );
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    rerender({ namespace: 'prod' });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
  });

  it('does not ask twice for the same release', async () => {
    const fetchMock = replies([]);

    const { rerender } = renderHook(() => {
      useHelmAccess('demo', 'podinfo');
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    rerender();

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('asks again when the cluster epoch changes under the release', async () => {
    const fetchMock = replies([]);
    renderHook(() => {
      useHelmAccess('demo', 'podinfo');
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    act(() => {
      bumpClusterEpoch();
    });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
  });
});
