import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import type { ServerMsg } from '../../src/lib/types';
import { usePodRows, usePodStore } from '../../src/store/pods';
import { makePod } from '../helpers';

function resetStore(): void {
  usePodStore.setState({ rows: new Map(), sorted: [] });
}

describe('usePodStore', () => {
  beforeEach(() => {
    resetStore();
  });

  afterEach(() => {
    resetStore();
  });

  it('starts with an empty map and empty sorted array', () => {
    const state = usePodStore.getState();
    expect(state.rows.size).toBe(0);
    expect(state.sorted).toEqual([]);
  });

  it('applySnapshot rebuilds the map keyed by uid', () => {
    const items = [
      makePod({ uid: 'a', name: 'alpha', namespace: 'ns1' }),
      makePod({ uid: 'b', name: 'bravo', namespace: 'ns1' }),
    ];
    usePodStore.getState().applySnapshot(items);
    const state = usePodStore.getState();
    expect(state.rows.size).toBe(2);
    expect(state.rows.get('a')?.name).toBe('alpha');
    expect(state.rows.get('b')?.name).toBe('bravo');
  });

  it('applySnapshot replaces any prior contents', () => {
    usePodStore.getState().applySnapshot([makePod({ uid: 'old', name: 'old-pod' })]);
    usePodStore.getState().applySnapshot([makePod({ uid: 'new', name: 'new-pod' })]);
    const state = usePodStore.getState();
    expect(state.rows.has('old')).toBe(false);
    expect(state.rows.has('new')).toBe(true);
    expect(state.rows.size).toBe(1);
  });

  it('applyDelta added inserts a row keyed by uid', () => {
    const msg: ServerMsg = {
      type: 'added',
      resource: 'pods',
      item: makePod({ uid: 'x', name: 'x-pod' }),
    };
    usePodStore.getState().applyDelta(msg);
    const state = usePodStore.getState();
    expect(state.rows.get('x')?.name).toBe('x-pod');
    expect(state.sorted).toHaveLength(1);
  });

  it('applyDelta modified overwrites the row with the same uid', () => {
    usePodStore.getState().applySnapshot([makePod({ uid: 'x', phase: 'Pending' })]);
    const msg: ServerMsg = {
      type: 'modified',
      resource: 'pods',
      item: makePod({ uid: 'x', phase: 'Running' }),
    };
    usePodStore.getState().applyDelta(msg);
    const state = usePodStore.getState();
    expect(state.rows.get('x')?.phase).toBe('Running');
    expect(state.rows.size).toBe(1);
  });

  it('applyDelta deleted removes the row keyed by uid', () => {
    usePodStore
      .getState()
      .applySnapshot([makePod({ uid: 'x', name: 'x-pod' }), makePod({ uid: 'y', name: 'y-pod' })]);
    const msg: ServerMsg = { type: 'deleted', resource: 'pods', uid: 'x' };
    usePodStore.getState().applyDelta(msg);
    const state = usePodStore.getState();
    expect(state.rows.has('x')).toBe(false);
    expect(state.rows.has('y')).toBe(true);
  });

  it('applyDelta with a snapshot message leaves state unchanged', () => {
    usePodStore.getState().applySnapshot([makePod({ uid: 'x' })]);
    const before = usePodStore.getState();
    const msg: ServerMsg = { type: 'snapshot', resource: 'pods', items: [], rv: '1' };
    before.applyDelta(msg);
    const after = usePodStore.getState();
    expect(after.rows).toBe(before.rows);
    expect(after.sorted).toBe(before.sorted);
  });

  it('applyDelta with an error message leaves state unchanged', () => {
    usePodStore.getState().applySnapshot([makePod({ uid: 'x' })]);
    const before = usePodStore.getState();
    const msg: ServerMsg = { type: 'error', message: 'boom' };
    before.applyDelta(msg);
    const after = usePodStore.getState();
    expect(after.rows).toBe(before.rows);
    expect(after.sorted).toBe(before.sorted);
  });

  it('sortRows orders by namespace then by name', () => {
    const items = [
      makePod({ uid: '1', name: 'zeta', namespace: 'kube-system' }),
      makePod({ uid: '2', name: 'alpha', namespace: 'kube-system' }),
      makePod({ uid: '3', name: 'beta', namespace: 'default' }),
      makePod({ uid: '4', name: 'alpha', namespace: 'default' }),
    ];
    usePodStore.getState().applySnapshot(items);
    const order = usePodStore.getState().sorted.map((row) => `${row.namespace}/${row.name}`);
    const expected = ['default/alpha', 'default/beta', 'kube-system/alpha', 'kube-system/zeta'];
    expect(order).toEqual(expected);
  });
});

describe('usePodRows', () => {
  beforeEach(() => {
    resetStore();
  });

  afterEach(() => {
    resetStore();
  });

  it('returns the store sorted array', () => {
    usePodStore.getState().applySnapshot([makePod({ uid: 'x', name: 'x-pod' })]);
    const { result } = renderHook(() => usePodRows());
    expect(result.current).toBe(usePodStore.getState().sorted);
    expect(result.current.map((row) => row.name)).toEqual(['x-pod']);
  });

  it('keeps a stable reference across re-renders when state does not change', () => {
    usePodStore.getState().applySnapshot([makePod({ uid: 'x' })]);
    const { result, rerender } = renderHook(() => usePodRows());
    const first = result.current;
    rerender();
    expect(result.current).toBe(first);
  });
});
