import { beforeEach, describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import {
  forgetHealth,
  reportHealth,
  useClusterHealthStore,
  useClusterReachable,
  useClusterUnreachableReason,
  useReachable,
} from '../../src/store/clusterHealth';
import { MK1, MK2, showing } from '../helpers-clusters';

describe('what is known about the cluster', () => {
  beforeEach(() => {
    useClusterHealthStore.getState().reset();
    showing(MK1);
  });

  it('assumes the cluster answers until told otherwise', () => {
    const { result } = renderHook(() => useClusterReachable());

    expect(result.current).toBe(true);
  });

  it('takes the reason the server gives', () => {
    reportHealth(MK1, false, 'connection refused');

    const { result } = renderHook(() => useClusterUnreachableReason());

    expect(result.current).toBe('connection refused');
    expect(renderHook(() => useClusterReachable()).result.current).toBe(false);
  });

  it('clears the reason when the cluster answers again', () => {
    reportHealth(MK1, false, 'connection refused');

    reportHealth(MK1, true, '');

    expect(renderHook(() => useClusterUnreachableReason()).result.current).toBe('');
    expect(renderHook(() => useClusterReachable()).result.current).toBe(true);
  });

  it('reports the tab in front, not another tab', () => {
    reportHealth(MK2, false, 'gone');

    expect(renderHook(() => useClusterReachable()).result.current).toBe(true);
    expect(renderHook(() => useReachable(MK2)).result.current).toBe(false);
  });

  it('lets go of a closed tab', () => {
    reportHealth(MK2, false, 'gone');

    forgetHealth(MK2);

    expect(renderHook(() => useReachable(MK2)).result.current).toBe(true);
  });

  it('forgets everything on reset', () => {
    reportHealth(MK1, false, 'gone');

    useClusterHealthStore.getState().reset();

    expect(useClusterHealthStore.getState().byCluster).toEqual({});
  });
});
