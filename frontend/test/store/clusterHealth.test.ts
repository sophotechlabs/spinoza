import { beforeEach, describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import {
  forgetHealth,
  reportHealth,
  useClusterHealthStore,
  useClusterHealth,
  useClusterReachable,
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
    reportHealth(MK1, {
      reachable: false,
      wobbling: false,
      reason: 'connection refused',
      cause: '',
    });

    const { result } = renderHook(() => useClusterHealth());

    expect(result.current.reason).toBe('connection refused');
    expect(renderHook(() => useClusterReachable()).result.current).toBe(false);
  });

  it('clears the reason when the cluster answers again', () => {
    reportHealth(MK1, {
      reachable: false,
      wobbling: false,
      reason: 'connection refused',
      cause: '',
    });

    reportHealth(MK1, { reachable: true, wobbling: false, reason: '', cause: '' });

    expect(renderHook(() => useClusterHealth()).result.current.reason).toBe('');
    expect(renderHook(() => useClusterReachable()).result.current).toBe(true);
  });

  it('reports the tab in front, not another tab', () => {
    reportHealth(MK2, { reachable: false, wobbling: false, reason: 'gone', cause: '' });

    expect(renderHook(() => useClusterReachable()).result.current).toBe(true);
    expect(renderHook(() => useReachable(MK2)).result.current).toBe(false);
  });

  it('lets go of a closed tab', () => {
    reportHealth(MK2, { reachable: false, wobbling: false, reason: 'gone', cause: '' });

    forgetHealth(MK2);

    expect(renderHook(() => useReachable(MK2)).result.current).toBe(true);
  });

  it('forgets everything on reset', () => {
    reportHealth(MK1, { reachable: false, wobbling: false, reason: 'gone', cause: '' });

    useClusterHealthStore.getState().reset();

    expect(useClusterHealthStore.getState().byCluster).toEqual({});
  });
});

describe('what the health of a cluster carries beyond its words', () => {
  beforeEach(() => {
    useClusterHealthStore.getState().reset();
    showing(MK1);
  });

  it('keeps the cause the server named, so the view need not read the raw text', () => {
    reportHealth(MK1, {
      reachable: false,
      wobbling: false,
      reason: 'net/http: TLS handshake timeout',
      cause: 'timeout',
    });

    expect(useClusterHealthStore.getState().byCluster[MK1]?.cause).toBe('timeout');
  });

  it('holds the moment the cluster stopped answering across later reports', () => {
    reportHealth(MK1, { reachable: false, wobbling: false, reason: 'first', cause: 'timeout' });
    const since = useClusterHealthStore.getState().byCluster[MK1]?.since;

    reportHealth(MK1, { reachable: false, wobbling: false, reason: 'second', cause: 'timeout' });

    expect(useClusterHealthStore.getState().byCluster[MK1]?.since).toBe(since);
    expect(useClusterHealthStore.getState().byCluster[MK1]?.reason).toBe('second');
  });

  it('takes a new moment when the cluster changes its mind about answering', () => {
    reportHealth(MK1, { reachable: false, wobbling: false, reason: 'gone', cause: 'refused' });
    const since = useClusterHealthStore.getState().byCluster[MK1]?.since ?? 0;

    reportHealth(MK1, { reachable: true, wobbling: false, reason: '', cause: '' });

    expect(useClusterHealthStore.getState().byCluster[MK1]?.since).toBeGreaterThanOrEqual(since);
    expect(useClusterHealthStore.getState().byCluster[MK1]?.since).not.toBe(0);
  });

  it('counts a cluster that came back, so views can refresh at once', () => {
    expect(useClusterHealthStore.getState().recoveries).toBe(0);

    reportHealth(MK1, { reachable: false, wobbling: false, reason: 'gone', cause: 'refused' });
    expect(useClusterHealthStore.getState().recoveries).toBe(0);

    reportHealth(MK1, { reachable: true, wobbling: false, reason: '', cause: '' });
    expect(useClusterHealthStore.getState().recoveries).toBe(1);
  });

  it('does not count a cluster that was answering all along', () => {
    reportHealth(MK1, { reachable: true, wobbling: false, reason: '', cause: '' });
    reportHealth(MK1, {
      reachable: true,
      wobbling: true,
      reason: 'a missed ping',
      cause: 'timeout',
    });

    expect(useClusterHealthStore.getState().recoveries).toBe(0);
  });
});
