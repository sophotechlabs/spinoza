import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import type { ServerMsg } from '../../src/lib/types';
import {
  useResourcesStore,
  useSubColumns,
  useSubNamespaced,
  useSubRows,
} from '../../src/store/resources';
import { makeColumns, makeRow } from '../helpers';

function resetStore(): void {
  useResourcesStore.setState({ subs: new Map() });
}

describe('useResourcesStore', () => {
  beforeEach(() => {
    resetStore();
  });

  afterEach(() => {
    resetStore();
  });

  it('starts with no subscriptions', () => {
    expect(useResourcesStore.getState().subs.size).toBe(0);
  });

  it('applySnapshot builds a sub keyed by subId with rows keyed by uid', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns(['Ready', 'Status']), true, [
        makeRow({ uid: 'a', name: 'alpha' }),
        makeRow({ uid: 'b', name: 'bravo' }),
      ]);
    const sub = useResourcesStore.getState().subs.get('s1');
    if (sub === undefined) {
      throw new Error('sub not found');
    }
    expect(sub.columns).toEqual([{ name: 'Ready' }, { name: 'Status' }]);
    expect(sub.namespaced).toBe(true);
    expect(sub.rows.size).toBe(2);
    expect(sub.rows.get('a')?.name).toBe('alpha');
  });

  it('applySnapshot replaces a prior snapshot for the same subId', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns(['A']), true, [makeRow({ uid: 'old' })]);
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns(['B']), false, [makeRow({ uid: 'new' })]);
    const sub = useResourcesStore.getState().subs.get('s1');
    expect(sub?.columns).toEqual([{ name: 'B' }]);
    expect(sub?.namespaced).toBe(false);
    expect(sub?.rows.has('old')).toBe(false);
    expect(sub?.rows.has('new')).toBe(true);
  });

  it('keeps subscriptions isolated by subId', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns([]), true, [makeRow({ uid: 'a' })]);
    useResourcesStore
      .getState()
      .applySnapshot('s2', makeColumns([]), false, [makeRow({ uid: 'b' })]);
    expect(useResourcesStore.getState().subs.get('s1')?.rows.has('a')).toBe(true);
    expect(useResourcesStore.getState().subs.get('s2')?.rows.has('b')).toBe(true);
  });

  it('applyDelta added inserts a row into an existing sub', () => {
    useResourcesStore.getState().applySnapshot('s1', makeColumns([]), true, []);
    const msg: ServerMsg = {
      type: 'added',
      subId: 's1',
      row: makeRow({ uid: 'x', name: 'x-row' }),
    };
    useResourcesStore.getState().applyDeltas('s1', [msg]);
    expect(useResourcesStore.getState().subs.get('s1')?.rows.get('x')?.name).toBe('x-row');
  });

  it('applyDelta modified overwrites a row with the same uid', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns([]), true, [makeRow({ uid: 'x', name: 'before' })]);
    const msg: ServerMsg = {
      type: 'modified',
      subId: 's1',
      row: makeRow({ uid: 'x', name: 'after' }),
    };
    useResourcesStore.getState().applyDeltas('s1', [msg]);
    const sub = useResourcesStore.getState().subs.get('s1');
    expect(sub?.rows.get('x')?.name).toBe('after');
    expect(sub?.rows.size).toBe(1);
  });

  it('applyDelta deleted removes a row by uid', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns([]), true, [makeRow({ uid: 'x' }), makeRow({ uid: 'y' })]);
    const msg: ServerMsg = { type: 'deleted', subId: 's1', uid: 'x' };
    useResourcesStore.getState().applyDeltas('s1', [msg]);
    const sub = useResourcesStore.getState().subs.get('s1');
    expect(sub?.rows.has('x')).toBe(false);
    expect(sub?.rows.has('y')).toBe(true);
  });

  it('applyDelta on an unknown subId leaves state unchanged', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns([]), true, [makeRow({ uid: 'x' })]);
    const before = useResourcesStore.getState();
    const msg: ServerMsg = { type: 'added', subId: 'missing', row: makeRow({ uid: 'z' }) };
    before.applyDeltas('missing', [msg]);
    expect(useResourcesStore.getState().subs).toBe(before.subs);
  });

  it('applyDelta with a snapshot message leaves the sub unchanged', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns([]), true, [makeRow({ uid: 'x' })]);
    const before = useResourcesStore.getState();
    const msg: ServerMsg = {
      type: 'snapshot',
      subId: 's1',
      columns: [],
      namespaced: true,
      rows: [],
    };
    before.applyDeltas('s1', [msg]);
    expect(useResourcesStore.getState().subs).toBe(before.subs);
  });

  it('applyDelta with an error message leaves the sub unchanged', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns([]), true, [makeRow({ uid: 'x' })]);
    const before = useResourcesStore.getState();
    const msg: ServerMsg = { type: 'error', subId: 's1', message: 'boom' };
    before.applyDeltas('s1', [msg]);
    expect(useResourcesStore.getState().subs).toBe(before.subs);
  });

  it('clearSub removes a subscription', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns([]), true, [makeRow({ uid: 'x' })]);
    useResourcesStore.getState().clearSub('s1');
    expect(useResourcesStore.getState().subs.has('s1')).toBe(false);
  });

  it('clearSub on an unknown subId leaves state unchanged', () => {
    useResourcesStore.getState().applySnapshot('s1', makeColumns([]), true, []);
    const before = useResourcesStore.getState();
    before.clearSub('missing');
    expect(useResourcesStore.getState().subs).toBe(before.subs);
  });
});

describe('selectors', () => {
  beforeEach(() => {
    resetStore();
  });

  afterEach(() => {
    resetStore();
  });

  it('useSubColumns returns a stable empty array when the sub is absent', () => {
    const { result, rerender } = renderHook(() => useSubColumns('none'));
    const first = result.current;
    expect(first).toEqual([]);
    rerender();
    expect(result.current).toBe(first);
  });

  it('useSubColumns returns the stored columns', () => {
    useResourcesStore.getState().applySnapshot('s1', makeColumns(['Ready']), true, []);
    const { result } = renderHook(() => useSubColumns('s1'));
    expect(result.current).toEqual([{ name: 'Ready' }]);
  });

  it('useSubNamespaced returns false when the sub is absent', () => {
    const { result } = renderHook(() => useSubNamespaced('none'));
    expect(result.current).toBe(false);
  });

  it('useSubNamespaced reflects the stored flag', () => {
    useResourcesStore.getState().applySnapshot('s1', makeColumns([]), true, []);
    const { result } = renderHook(() => useSubNamespaced('s1'));
    expect(result.current).toBe(true);
  });

  it('useSubRows returns a stable empty array when the sub is absent', () => {
    const { result, rerender } = renderHook(() => useSubRows('none'));
    const first = result.current;
    expect(first).toEqual([]);
    rerender();
    expect(result.current).toBe(first);
  });

  it('useSubRows returns rows sorted by namespace then name', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns([]), true, [
        makeRow({ uid: '1', name: 'zeta', namespace: 'kube-system' }),
        makeRow({ uid: '2', name: 'alpha', namespace: 'kube-system' }),
        makeRow({ uid: '3', name: 'beta', namespace: 'default' }),
        makeRow({ uid: '4', name: 'alpha', namespace: 'default' }),
      ]);
    const { result } = renderHook(() => useSubRows('s1'));
    const order = result.current.map((row) => `${row.namespace}/${row.name}`);
    expect(order).toEqual([
      'default/alpha',
      'default/beta',
      'kube-system/alpha',
      'kube-system/zeta',
    ]);
  });

  it('useSubRows keeps a stable reference across re-renders when rows do not change', () => {
    useResourcesStore
      .getState()
      .applySnapshot('s1', makeColumns([]), true, [makeRow({ uid: 'x' })]);
    const { result, rerender } = renderHook(() => useSubRows('s1'));
    const first = result.current;
    rerender();
    expect(result.current).toBe(first);
  });
});
