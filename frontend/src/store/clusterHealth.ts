import { create } from 'zustand';
import { useActiveCluster } from './clusters';
import type { ByCluster } from './perCluster';
import { drop, held, put } from './perCluster';

export interface Health {
  reachable: boolean;
  wobbling: boolean;
  reason: string;
  cause: string;
  since: number;
}

export interface HealthReport {
  reachable: boolean;
  wobbling: boolean;
  reason: string;
  cause: string;
}

const ANSWERING: Health = { reachable: true, wobbling: false, reason: '', cause: '', since: 0 };

interface ClusterHealthState {
  byCluster: ByCluster<Health>;
  recoveries: number;
  recovered: string;
  report: (cluster: string, next: HealthReport) => void;
  forget: (cluster: string) => void;
  reset: () => void;
}

function settled(was: Health | undefined, next: HealthReport, now: number): Health {
  if (was === undefined) {
    return { ...next, since: now };
  }
  if (was.reachable === next.reachable && was.wobbling === next.wobbling) {
    return { ...next, since: was.since };
  }
  return { ...next, since: now };
}

function recovered(was: Health | undefined, next: HealthReport): boolean {
  if (was === undefined) {
    return false;
  }
  if (was.reachable) {
    return false;
  }
  return next.reachable;
}

export const useClusterHealthStore = create<ClusterHealthState>((set) => ({
  byCluster: {},
  recoveries: 0,
  recovered: '',
  report: (cluster, next) => {
    set((state) => {
      const was = state.byCluster[cluster];
      const now = Date.now();
      const byCluster = put(state.byCluster, cluster, settled(was, next, now));
      if (recovered(was, next)) {
        return { byCluster, recoveries: state.recoveries + 1, recovered: cluster };
      }
      return { byCluster };
    });
  },
  forget: (cluster) => {
    set((state) => ({ byCluster: drop(state.byCluster, cluster) }));
  },
  reset: () => {
    set({ byCluster: {}, recoveries: 0, recovered: '' });
  },
}));

export function useClusterHealth(): Health {
  const on = useActiveCluster();
  return useClusterHealthStore((state) => held(state.byCluster, on, ANSWERING));
}

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

export function useRecoveries(): number {
  return useClusterHealthStore((state) => state.recoveries);
}

export function whatCameBack(): string {
  return useClusterHealthStore.getState().recovered;
}

export function reportHealth(cluster: string, next: HealthReport): void {
  useClusterHealthStore.getState().report(cluster, next);
}

export function forgetHealth(cluster: string): void {
  useClusterHealthStore.getState().forget(cluster);
}

export function useClusterWobbling(): boolean {
  const on = useActiveCluster();
  return useClusterHealthStore((state) => held(state.byCluster, on, ANSWERING).wobbling);
}
