import { beforeEach, describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import {
  useClusterHealthStore,
  useClusterReachable,
  useClusterUnreachableReason,
} from '../../src/store/clusterHealth';

describe('what is known about the cluster', () => {
  beforeEach(() => {
    useClusterHealthStore.getState().reset();
  });

  it('assumes the cluster answers until told otherwise', () => {
    const { result } = renderHook(() => useClusterReachable());

    expect(result.current).toBe(true);
  });

  it('takes the reason the server gives', () => {
    useClusterHealthStore.getState().report(false, 'connection refused');

    const { result } = renderHook(() => useClusterUnreachableReason());

    expect(result.current).toBe('connection refused');
    expect(useClusterHealthStore.getState().reachable).toBe(false);
  });

  it('clears the reason when the cluster answers again', () => {
    useClusterHealthStore.getState().report(false, 'connection refused');

    useClusterHealthStore.getState().report(true, '');

    expect(useClusterHealthStore.getState().reachable).toBe(true);
    expect(useClusterHealthStore.getState().reason).toBe('');
  });

  it('forgets everything on reset', () => {
    useClusterHealthStore.getState().report(false, 'gone');

    useClusterHealthStore.getState().reset();

    expect(useClusterHealthStore.getState()).toMatchObject({ reachable: true, reason: '' });
  });
});
