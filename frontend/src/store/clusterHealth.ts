import { create } from 'zustand';
import { useActiveCluster } from './clusters';
import type { ByCluster } from './perCluster';
import { drop, held, put } from './perCluster';

interface Health {
  reachable: boolean;
  reason: string;
}

const ANSWERING: Health = { reachable: true, reason: '' };

interface ClusterHealthState {
  byCluster: ByCluster<Health>;
  report: (cluster: string, reachable: boolean, reason: string) => void;
  forget: (cluster: string) => void;
  reset: () => void;
}

export const useClusterHealthStore = create<ClusterHealthState>((set) => ({
  byCluster: {},
  report: (cluster, reachable, reason) => {
    set((state) => ({ byCluster: put(state.byCluster, cluster, { reachable, reason }) }));
  },
  forget: (cluster) => {
    set((state) => ({ byCluster: drop(state.byCluster, cluster) }));
  },
  reset: () => {
    set({ byCluster: {} });
  },
}));

export function useClusterReachable(): boolean {
  const on = useActiveCluster();
  return useClusterHealthStore((state) => held(state.byCluster, on, ANSWERING).reachable);
}

export function useClusterUnreachableReason(): string {
  const on = useActiveCluster();
  return useClusterHealthStore((state) => held(state.byCluster, on, ANSWERING).reason);
}

export function useReachable(cluster: string): boolean {
  return useClusterHealthStore((state) => held(state.byCluster, cluster, ANSWERING).reachable);
}

export function reportHealth(cluster: string, reachable: boolean, reason: string): void {
  useClusterHealthStore.getState().report(cluster, reachable, reason);
}

export function forgetHealth(cluster: string): void {
  useClusterHealthStore.getState().forget(cluster);
}
