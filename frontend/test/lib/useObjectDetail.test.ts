import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useObjectDetail } from '../../src/lib/useObjectDetail';
import type { ObjectRef, Row } from '../../src/lib/types';
import { makeRow } from '../helpers';

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

interface Props {
  target: ObjectRef | null;
  live: Row | null;
}

function stubOk(): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(detail) });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function renderDetail(props: Props) {
  return renderHook((current: Props) => useObjectDetail(current.target, current.live), {
    initialProps: props,
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('useObjectDetail', () => {
  it('reports nothing without a target', () => {
    vi.stubGlobal('fetch', vi.fn());
    const { result } = renderHook(() => useObjectDetail(null, null));

    expect(result.current.detail).toBeNull();
    expect(result.current.error).toBeNull();
    expect(result.current.gone).toBe(false);
  });

  it('loads the object it was pointed at', async () => {
    stubOk();
    const { result } = renderHook(() => useObjectDetail(ref, null));

    await waitFor(() => {
      expect(result.current.detail).toEqual(detail);
    });
  });

  it('refetches when reload is called', async () => {
    const fetchMock = stubOk();
    const { result } = renderHook(() => useObjectDetail(ref, null));
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
    const { result } = renderHook(() => useObjectDetail(ref, null));

    await waitFor(() => {
      expect(result.current.error).toBe('pods "web" not found');
    });
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    const { result } = renderHook(() => useObjectDetail(ref, null));

    await waitFor(() => {
      expect(result.current.error).toBe('object request failed');
    });
  });

  it('drops the old object as soon as the target changes', async () => {
    stubOk();
    const { result, rerender } = renderDetail({ target: ref, live: null });
    await waitFor(() => {
      expect(result.current.detail).not.toBeNull();
    });

    rerender({ target: { ...ref, name: 'other' }, live: null });

    expect(result.current.detail).toBeNull();
  });
});

describe('an object the watch says has changed', () => {
  it('refetches when the live row is replaced', async () => {
    const fetchMock = stubOk();
    const first = makeRow({ uid: 'uid-web', name: 'web', namespace: 'prod' });
    const { result, rerender } = renderDetail({ target: ref, live: first });
    await waitFor(() => {
      expect(result.current.detail).not.toBeNull();
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);

    rerender({ target: ref, live: { ...first, cells: ['Running'] } });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
    expect(result.current.gone).toBe(false);
  });

  it('does not refetch while the same row object stays put', async () => {
    const fetchMock = stubOk();
    const row = makeRow({ uid: 'uid-web', name: 'web', namespace: 'prod' });
    const { result, rerender } = renderDetail({ target: ref, live: row });
    await waitFor(() => {
      expect(result.current.detail).not.toBeNull();
    });

    rerender({ target: ref, live: row });
    rerender({ target: ref, live: row });

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe('an object the watch says is deleted', () => {
  it('clears the detail and says the object is gone', async () => {
    stubOk();
    const row = makeRow({ uid: 'uid-web', name: 'web', namespace: 'prod' });
    const { result, rerender } = renderDetail({ target: ref, live: row });
    await waitFor(() => {
      expect(result.current.detail).not.toBeNull();
    });

    rerender({ target: ref, live: null });

    expect(result.current.detail).toBeNull();
    expect(result.current.error).toBeNull();
    expect(result.current.gone).toBe(true);
  });

  it('stops saying gone once the target moves to another object', async () => {
    stubOk();
    const row = makeRow({ uid: 'uid-web', name: 'web', namespace: 'prod' });
    const { result, rerender } = renderDetail({ target: ref, live: row });
    await waitFor(() => {
      expect(result.current.detail).not.toBeNull();
    });
    rerender({ target: ref, live: null });
    expect(result.current.gone).toBe(true);

    rerender({ target: { ...ref, name: 'other' }, live: null });

    expect(result.current.gone).toBe(false);
  });

  it('says gone when a row that arrived late disappears again', async () => {
    stubOk();
    const row = makeRow({ uid: 'uid-web', name: 'web', namespace: 'prod' });
    const { result, rerender } = renderDetail({ target: ref, live: null });
    await waitFor(() => {
      expect(result.current.detail).not.toBeNull();
    });

    rerender({ target: ref, live: row });
    expect(result.current.gone).toBe(false);

    rerender({ target: ref, live: null });
    expect(result.current.gone).toBe(true);
  });
});
