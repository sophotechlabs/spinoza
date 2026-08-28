import { create } from 'zustand';

interface ClusterHealthState {
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
