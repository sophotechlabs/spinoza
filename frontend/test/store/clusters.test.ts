import { beforeEach, describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import {
  activeClusterNow,
  adoptClusters,
  rememberRoute,
  routeOf,
  useActiveCluster,
  useClustersStore,
  useTabStrip,
  useTabs,
} from '../../src/store/clusters';
import { activeCluster } from '../../src/lib/cluster';
import { EMPTY_ROUTE } from '../../src/lib/router';
import { MK1, MK2, listOf } from '../helpers-clusters';

const somewhere = { ...EMPTY_ROUTE, context: 'p-mk1', view: 'issues' as const };

describe('the clusters this window has open', () => {
  beforeEach(() => {
    useClustersStore.getState().reset();
  });

  it('starts with none', () => {
    expect(renderHook(() => useTabs()).result.current).toEqual([]);
    expect(activeClusterNow()).toBe('');
  });

  it('takes the tabs the server reports', () => {
    adoptClusters(listOf(MK1));

    const tabs = renderHook(() => useTabs()).result.current;
    expect(tabs.map((one) => one.context)).toEqual(['p-mk1', 'p-mk2']);
    expect(tabs[0].kubeconfig).toBe('');
  });

  it('follows the cluster the server calls active', () => {
    adoptClusters(listOf(MK2));

    expect(renderHook(() => useActiveCluster()).result.current).toBe(MK2);
  });

  it('tells every request which cluster it is for', () => {
    adoptClusters(listOf(MK2));

    expect(activeCluster()).toBe(MK2);
  });

  it('names nothing active when the server has no active cluster', () => {
    adoptClusters({ clusters: [], remembered: [] });

    expect(activeClusterNow()).toBe('');
  });

  it('keeps the strip hidden while only one cluster is open', () => {
    adoptClusters({ clusters: listOf(MK1).clusters.slice(0, 1), remembered: [] });

    expect(renderHook(() => useTabStrip()).result.current).toBe(false);
  });

  it('shows the strip once a second cluster is open', () => {
    adoptClusters(listOf(MK1));

    expect(renderHook(() => useTabStrip()).result.current).toBe(true);
  });

  it('remembers where each tab was', () => {
    adoptClusters(listOf(MK1));

    rememberRoute(MK1, somewhere);

    expect(routeOf(MK1)).toEqual(somewhere);
    expect(routeOf(MK2)).toBeNull();
  });

  it('forgets where a closed tab was', () => {
    adoptClusters(listOf(MK1));
    rememberRoute(MK1, somewhere);

    adoptClusters({ clusters: listOf(MK2).clusters.slice(1), remembered: [] });

    expect(routeOf(MK1)).toBeNull();
  });

  it('brings a tab forward without waiting for the server', () => {
    adoptClusters(listOf(MK1));

    useClustersStore.getState().focus(MK2);

    expect(activeClusterNow()).toBe(MK2);
    expect(activeCluster()).toBe(MK2);
  });

  it('holds on to what the server said was open last time', () => {
    adoptClusters({
      clusters: [],
      remembered: [{ id: MK2, context: 'p-mk2', kubeconfig: '/work.yaml' }],
    });

    expect(useClustersStore.getState().remembered).toHaveLength(1);
  });

  it('names no cluster once the window lets go of them all', () => {
    adoptClusters(listOf(MK1));

    useClustersStore.getState().reset();

    expect(activeCluster()).toBe('');
    expect(useClustersStore.getState().routes).toEqual({});
  });
});
