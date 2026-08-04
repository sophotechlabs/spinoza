import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import {
  forwardKind,
  listForwards,
  refreshForwards,
  startForward,
  stopForward,
  useForwardPolling,
} from '../../src/lib/portForward';
import { useForwardsStore } from '../../src/store/forwards';
import type { ObjectRef, PortForward } from '../../src/lib/types';
import { anySignal } from '../helpers';

const ref: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'flux-system',
  name: 'web',
};

function forward(overrides: Partial<PortForward> = {}): PortForward {
  return {
    id: 'pf-1',
    kind: 'Pod',
    namespace: 'flux-system',
    name: 'web',
    remotePort: 8080,
    localPort: 45123,
    state: 'running',
    startedAt: '2026-07-27T18:00:00Z',
    ...overrides,
  };
}

function ok(payload: unknown) {
  return { ok: true, json: () => Promise.resolve(payload) };
}

describe('portForward', () => {
  beforeEach(() => {
    useForwardsStore.setState({ forwards: [] });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    useForwardsStore.setState({ forwards: [] });
  });

  it('recognises forwardable kinds', () => {
    expect(forwardKind('v1', 'Pod')).toBe('Pod');
    expect(forwardKind('v1', 'Service')).toBe('Service');
  });

  it('rejects everything else', () => {
    expect(forwardKind('v1', 'ConfigMap')).toBeNull();
    expect(forwardKind('apps/v1', 'Deployment')).toBeNull();
    expect(forwardKind('v1', 'Node')).toBeNull();
  });

  it('lists forwards', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(ok([forward()])));

    await expect(listForwards()).resolves.toHaveLength(1);
  });

  it('reports a list failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 500, json: () => Promise.reject(new Error('x')) }),
    );

    await expect(listForwards()).rejects.toThrow('forward list failed with status 500');
  });

  it('starts a forward with the target in the query', async () => {
    const mock = vi.fn().mockResolvedValue(ok(forward()));
    vi.stubGlobal('fetch', mock);

    await expect(startForward('Pod', ref, 8080)).resolves.toEqual(forward());
    expect(mock).toHaveBeenCalledWith(
      '/api/portforward?kind=Pod&namespace=flux-system&name=web&port=8080',
      { method: 'POST', signal: anySignal() },
    );
  });

  it('surfaces the server message when starting fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'no ready pod backs the service' }),
      }),
    );

    await expect(startForward('Service', ref, 9090)).rejects.toThrow(
      'no ready pod backs the service',
    );
  });

  it('stops a forward by id', async () => {
    const mock = vi.fn().mockResolvedValue(ok({}));
    vi.stubGlobal('fetch', mock);

    await expect(stopForward('pf-1')).resolves.toBeUndefined();
    expect(mock).toHaveBeenCalledWith('/api/portforward?id=pf-1', {
      method: 'DELETE',
      signal: anySignal(),
    });
  });

  it('reports a stop failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue({ ok: false, status: 404, json: () => Promise.reject(new Error('x')) }),
    );

    await expect(stopForward('pf-9')).rejects.toThrow(
      'stopping the forward failed with status 404',
    );
  });

  it('refreshes the store', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(ok([forward()])));

    await refreshForwards();

    expect(useForwardsStore.getState().forwards).toHaveLength(1);
  });

  it('leaves the store alone when the refresh fails', async () => {
    useForwardsStore.setState({ forwards: [forward()] });
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    await refreshForwards();

    expect(useForwardsStore.getState().forwards).toHaveLength(1);
  });

  it('polls while enabled and stops on unmount', async () => {
    vi.useFakeTimers();
    const mock = vi.fn().mockResolvedValue(ok([]));
    vi.stubGlobal('fetch', mock);

    const view = renderHook(() => {
      useForwardPolling(true);
    });
    expect(mock).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(5000);
    expect(mock).toHaveBeenCalledTimes(2);

    view.unmount();
    await vi.advanceTimersByTimeAsync(15000);
    expect(mock).toHaveBeenCalledTimes(2);
  });

  it('does not poll while disabled', async () => {
    vi.useFakeTimers();
    const mock = vi.fn().mockResolvedValue(ok([]));
    vi.stubGlobal('fetch', mock);

    renderHook(() => {
      useForwardPolling(false);
    });
    await vi.advanceTimersByTimeAsync(20000);

    expect(mock).not.toHaveBeenCalled();
  });
});
