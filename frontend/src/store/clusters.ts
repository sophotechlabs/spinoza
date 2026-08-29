import { create } from 'zustand';
import type { ClusterList, OpenCluster, RememberedCluster } from '../lib/types';
import type { Route } from '../lib/router';
import { setActiveCluster } from '../lib/cluster';
import type { ByCluster } from './perCluster';
import { drop, put } from './perCluster';

export interface Tab {
  id: string;
  context: string;
  kubeconfig: string;
  protection: string;
}

interface ClustersState {
  tabs: Tab[];
  remembered: RememberedCluster[];
  active: string;
  routes: ByCluster<Route>;
  adopt: (list: ClusterList) => void;
  focus: (id: string) => void;
  rememberRoute: (id: string, route: Route) => void;
  reset: () => void;
}

const NO_TABS: Tab[] = [];

const NOTHING_REMEMBERED: RememberedCluster[] = [];

function tabOf(cluster: OpenCluster): Tab {
  return {
    id: cluster.id,
    context: cluster.context,
    kubeconfig: cluster.kubeconfig ?? '',
    protection: cluster.protection,
  };
}

export function activeOf(clusters: OpenCluster[]): string {
  for (const one of clusters) {
    if (one.active) {
      return one.id;
    }
  }
  return '';
}

function onlyOpen(routes: ByCluster<Route>, tabs: Tab[]): ByCluster<Route> {
  let kept = routes;
  for (const id of Object.keys(routes)) {
    if (!tabs.some((tab) => tab.id === id)) {
      kept = drop(kept, id);
    }
  }
  return kept;
}

export const useClustersStore = create<ClustersState>((set, get) => ({
  tabs: NO_TABS,
  remembered: NOTHING_REMEMBERED,
  active: '',
  routes: {},
  adopt: (list) => {
    const tabs = list.clusters.map(tabOf);
    const active = activeOf(list.clusters);
    setActiveCluster(active);
    set({
      tabs,
      remembered: list.remembered,
      active,
      routes: onlyOpen(get().routes, tabs),
    });
  },
  focus: (id) => {
    setActiveCluster(id);
    set({ active: id });
  },
  rememberRoute: (id, route) => {
    set((state) => ({ routes: put(state.routes, id, route) }));
  },
  reset: () => {
    setActiveCluster('');
    set({ tabs: NO_TABS, remembered: NOTHING_REMEMBERED, active: '', routes: {} });
  },
}));

export function useTabs(): Tab[] {
  return useClustersStore((state) => state.tabs);
}

export function useActiveCluster(): string {
  return useClustersStore((state) => state.active);
}

export function useTabStrip(): boolean {
  return useClustersStore((state) => state.tabs.length > 1);
}

export function activeClusterNow(): string {
  return useClustersStore.getState().active;
}

export function routeOf(id: string): Route | null {
  return useClustersStore.getState().routes[id] ?? null;
}

export function rememberRoute(id: string, route: Route): void {
  useClustersStore.getState().rememberRoute(id, route);
}

export function adoptClusters(list: ClusterList): void {
  useClustersStore.getState().adopt(list);
}
