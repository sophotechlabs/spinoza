import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useObjectDetail } from '../../src/lib/useObjectDetail';
import type { ObjectRef } from '../../src/lib/types';

const ref: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'prod',
  name: 'web',
};

const detail = {
  apiVersion: 'v1',
  kind: 'Pod',
  name: 'web',
  namespace: 'prod',
  uid: 'uid-web',
  createdAt: '2026-08-03T09:00:00Z',
  yaml: 'kind: Pod\n',
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('useObjectDetail', () => {
  it('reports nothing without a target', () => {
    vi.stubGlobal('fetch', vi.fn());
    const { result } = renderHook(() => useObjectDetail(null));

    expect(result.current.detail).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it('loads the object it was pointed at', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(detail) }),
    );
    const { result } = renderHook(() => useObjectDetail(ref));

    await waitFor(() => {
      expect(result.current.detail).toEqual(detail);
    });
  });

  it('refetches when reload is called', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(detail) });
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() => useObjectDetail(ref));
    await waitFor(() => {
      expect(result.current.detail).not.toBeNull();
    });

    act(() => {
      result.current.reload();
    });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
  });

  it('reports a failed fetch', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'pods "web" not found' }),
      }),
    );
    const { result } = renderHook(() => useObjectDetail(ref));

    await waitFor(() => {
      expect(result.current.error).toBe('pods "web" not found');
    });
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    const { result } = renderHook(() => useObjectDetail(ref));

    await waitFor(() => {
      expect(result.current.error).toBe('object request failed');
    });
  });

  it('drops the old object as soon as the target changes', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(detail) }),
    );
    const { result, rerender } = renderHook((target: ObjectRef) => useObjectDetail(target), {
      initialProps: ref,
    });
    await waitFor(() => {
      expect(result.current.detail).not.toBeNull();
    });

    rerender({ ...ref, name: 'other' });

    expect(result.current.detail).toBeNull();
  });
});
