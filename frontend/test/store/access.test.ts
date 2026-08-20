import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useAccessStore, useRefusal, useRefusalsFor } from '../../src/store/access';
import { useAccess } from '../../src/lib/useAccess';
import type { ObjectRef } from '../../src/lib/types';

const pod: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'kube-system',
  name: 'calico-node-2cv49',
};

const otherPod: ObjectRef = { ...pod, name: 'calico-node-77xyz' };

const podKey = 'group=&version=v1&resource=pods&namespace=kube-system&name=calico-node-2cv49';

function reset(): void {
  useAccessStore.getState().forget();
}

describe('what the cluster refuses for the selected object', () => {
  beforeEach(reset);

  it('says nothing before anything has been asked', () => {
    const { result } = renderHook(() => useRefusal(pod, 'logs'));

    expect(result.current).toBeNull();
  });

  it('gives the reason once it is known', () => {
    useAccessStore.getState().setRefused(podKey, {
      logs: 'requires container.pods.getLogs',
    });

    const { result } = renderHook(() => useRefusal(pod, 'logs'));

    expect(result.current).toBe('requires container.pods.getLogs');
  });

  it('keeps the answers for one object off another object', () => {
    useAccessStore.getState().setRefused(podKey, {
      logs: 'requires container.pods.getLogs',
    });

    const { result } = renderHook(() => useRefusal(otherPod, 'logs'));

    expect(result.current).toBeNull();
  });

  it('says nothing when there is no object', () => {
    const { result } = renderHook(() => useRefusalsFor(null));

    expect(result.current).toEqual({});
  });

  it('leaves capabilities it was not told about alone', () => {
    useAccessStore.getState().setRefused(podKey, {
      logs: 'no logs for you',
    });

    const { result } = renderHook(() => useRefusal(pod, 'exec'));

    expect(result.current).toBeNull();
  });
});

describe('asking on selection', () => {
  beforeEach(() => {
    reset();
    vi.unstubAllGlobals();
  });

  it('records what the server says about the selected object', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ refused: [{ capability: 'exec', reason: 'no exec' }] }),
      }),
    );

    renderHook(() => {
      useAccess(pod);
    });

    await waitFor(() => {
      expect(useAccessStore.getState().refused).toEqual({ exec: 'no exec' });
    });
  });

  it('asks nothing with no selection', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    renderHook(() => {
      useAccess(null);
    });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(useAccessStore.getState().key).toBe('');
  });

  it('forgets the answers when the question cannot be put', async () => {
    useAccessStore.getState().setRefused('stale', { logs: 'no logs' });
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    renderHook(() => {
      useAccess(pod);
    });

    await waitFor(() => {
      expect(useAccessStore.getState().refused).toEqual({});
    });
  });

  it('asks again when the selection changes', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ refused: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const { rerender } = renderHook(
      ({ ref }: { ref: ObjectRef }) => {
        useAccess(ref);
      },
      {
        initialProps: { ref: pod },
      },
    );
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });
    rerender({ ref: otherPod });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
  });

  it('does not ask twice for the same object', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ refused: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const { rerender } = renderHook(
      ({ ref }: { ref: ObjectRef }) => {
        useAccess(ref);
      },
      {
        initialProps: { ref: pod },
      },
    );
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });
    rerender({ ref: { ...pod } });

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
