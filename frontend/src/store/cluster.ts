import { create } from 'zustand';

interface ClusterState {
  epoch: number;
  bump: () => void;
  reset: () => void;
}

export const useClusterStore = create<ClusterState>((set) => ({
  epoch: 0,
  bump: () => {
    set((state) => ({ epoch: state.epoch + 1 }));
  },
  reset: () => {
    set({ epoch: 0 });
  },
}));

export function useClusterEpoch(): number {
  return useClusterStore((state) => state.epoch);
}

export function bumpClusterEpoch(): void {
  useClusterStore.getState().bump();
}
