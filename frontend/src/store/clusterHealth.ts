import { create } from 'zustand';

interface ClusterHealthState {
  // Whether spinoza can reach the cluster's apiserver. A window connected to
  // spinoza is not the same as a cluster that answers, and only the server
  // knows the difference.
  reachable: boolean;
  reason: string;
  report: (reachable: boolean, reason: string) => void;
  reset: () => void;
}

export const useClusterHealthStore = create<ClusterHealthState>((set) => ({
  reachable: true,
  reason: '',
  report: (reachable, reason) => {
    set({ reachable, reason });
  },
  reset: () => {
    set({ reachable: true, reason: '' });
  },
}));

export function useClusterReachable(): boolean {
  return useClusterHealthStore((state) => state.reachable);
}

export function useClusterUnreachableReason(): string {
  return useClusterHealthStore((state) => state.reason);
}
