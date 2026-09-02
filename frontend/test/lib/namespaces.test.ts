import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { fetchNamespaces, useNamespaces } from '../../src/lib/namespaces';
import { ALL, settle, useNamespaceStore } from '../../src/store/namespace';
import { useClustersStore } from '../../src/store/clusters';
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
});

describe('fetchNamespaces', () => {
  it('reads the names', async () => {
    stub({ names: ['default', 'shop'] });

    await expect(fetchNamespaces()).resolves.toEqual({
      names: ['default', 'shop'],
      error: undefined,
    });
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
});

describe('useNamespaces', () => {
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
