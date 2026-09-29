import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { fetchNamespaces, useNamespaces } from '../../src/lib/namespaces';
import { ALL, settle, useNamespaceStore } from '../../src/store/namespace';
import { useClustersStore } from '../../src/store/clusters';
import { useToastsStore } from '../../src/store/toasts';
import { MK1, MK2, showing } from '../helpers-clusters';

function stub(body: unknown, ok = true, status = 200) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve({ ok, status, json: () => Promise.resolve(body) })),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  useNamespaceStore.getState().reset();
  useClustersStore.getState().reset();
  useToastsStore.getState().clear();
});

describe('fetchNamespaces', () => {
  it('reads the names', async () => {
    stub({ names: ['default', 'shop'] });

    await expect(fetchNamespaces()).resolves.toEqual({
      names: ['default', 'shop'],
      narrowed: false,
      error: undefined,
    });
  });

  it('says when the list was narrowed to what the account can read', async () => {
    stub({ names: ['payments'], narrowed: true });

    expect((await fetchNamespaces()).narrowed).toBe(true);
  });

  it('has no names when the backend sent none', async () => {
    stub({});

    expect((await fetchNamespaces()).names).toEqual([]);
  });

  it('carries a partial failure', async () => {
    stub({ names: [], error: 'namespaces is forbidden' });

    expect((await fetchNamespaces()).error).toBe('namespaces is forbidden');
  });

  it('reports a request the backend refused', async () => {
    stub({ message: 'spinoza has no cluster' }, false, 503);

    await expect(fetchNamespaces()).rejects.toThrow('no cluster');
  });
});

describe('settle', () => {
  it('keeps a namespace the cluster has', () => {
    expect(settle('shop', ['default', 'shop'])).toBe('shop');
  });

  it('keeps the all-namespaces choice', () => {
    expect(settle(ALL, ['default'])).toBe(ALL);
  });

  it('falls back to every namespace when the kept one is gone', () => {
    expect(settle('shop', ['default', 'kube-system'])).toBe(ALL);
  });

  it('waits rather than guessing before the names arrive', () => {
    expect(settle('shop', [])).toBe('shop');
  });

  it('moves off every namespace when the account can read only some', () => {
    expect(settle(ALL, ['payments', 'storefront'], true)).toBe('payments');
  });

  it('keeps a readable namespace when the account can read only some', () => {
    expect(settle('storefront', ['payments', 'storefront'], true)).toBe('storefront');
  });

  it('moves to a readable namespace when the kept one is not readable', () => {
    expect(settle('shop', ['payments'], true)).toBe('payments');
  });

  it('waits on a narrowed list that has not arrived', () => {
    expect(settle(ALL, [], true)).toBe(ALL);
  });
});

describe('useNamespaces', () => {
  it('keeps the last namespace list and reports a failed refresh', async () => {
    showing(MK1);
    useNamespaceStore.getState().offer(MK1, ['default', 'shop']);
    stub({ names: [], error: 'namespaces is forbidden' });

    renderHook(() => {
      useNamespaces();
    });

    await waitFor(() => {
      expect(useToastsStore.getState().toasts.at(-1)?.message).toBe(
        'Listing namespaces: namespaces is forbidden',
      );
    });
    expect(useNamespaceStore.getState().byCluster[MK1]?.names).toEqual(['default', 'shop']);
  });

  it('opens a narrowed account on a namespace it can read', async () => {
    showing(MK1);
    stub({ names: ['payments'], narrowed: true });

    renderHook(() => {
      useNamespaces();
    });

    await waitFor(() => {
      expect(useNamespaceStore.getState().byCluster[MK1]?.namespace).toBe('payments');
    });
    expect(useNamespaceStore.getState().byCluster[MK1]?.narrowed).toBe(true);
  });

  it('points an account that reads nothing at the setting', async () => {
    showing(MK1);
    stub({
      names: [],
      narrowed: true,
      error:
        'your account reads no namespace spinoza knows of; add the ones it can read under Settings, Cluster',
    });

    renderHook(() => {
      useNamespaces();
    });

    await waitFor(() => {
      expect(useToastsStore.getState().toasts.at(-1)?.message).toBe(
        'Listing namespaces: your account reads no namespace spinoza knows of; add the ones it can read under Settings, Cluster',
      );
    });
  });

  it('asks again when told the readable namespaces changed', async () => {
    showing(MK1);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ names: [], narrowed: true, error: 'reads nothing' }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ names: ['payments'], narrowed: true }),
      });
    vi.stubGlobal('fetch', fetchMock);
    renderHook(() => {
      useNamespaces();
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    act(() => {
      useNamespaceStore.getState().askAgain();
    });

    await waitFor(() => {
      expect(useNamespaceStore.getState().byCluster[MK1]?.names).toEqual(['payments']);
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('reports a namespace request that failed outright', async () => {
    showing(MK1);
    stub({ message: 'no cluster' }, false, 503);

    renderHook(() => {
      useNamespaces();
    });

    await waitFor(() => {
      expect(useToastsStore.getState().toasts.at(-1)?.message).toBe(
        'Listing namespaces: no cluster',
      );
    });
  });

  it('waits for a cluster before asking for its namespaces', async () => {
    useClustersStore.getState().reset();
    stub({ names: ['default', 'e2e'] });

    renderHook(() => {
      useNamespaces();
    });

    expect(fetch).not.toHaveBeenCalled();

    act(() => {
      showing(MK1);
    });

    await waitFor(() => {
      expect(useNamespaceStore.getState().byCluster[MK1]?.names).toEqual(['default', 'e2e']);
    });
  });

  it('does not give a slow response to the cluster selected after it', async () => {
    let answerFirst: (value: unknown) => void = () => undefined;
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            answerFirst = resolve;
          }),
      )
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ names: ['second'] }),
      });
    vi.stubGlobal('fetch', fetchMock);
    showing(MK1);
    renderHook(() => {
      useNamespaces();
    });
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    act(() => {
      showing(MK2);
    });

    await waitFor(() => {
      expect(useNamespaceStore.getState().byCluster[MK2]?.names).toEqual(['second']);
    });

    await act(async () => {
      answerFirst({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ names: ['first'] }),
      });
      await Promise.resolve();
    });

    expect(useNamespaceStore.getState().byCluster[MK1]?.names ?? []).toEqual([]);
    expect(useNamespaceStore.getState().byCluster[MK2]?.names).toEqual(['second']);
  });
});
