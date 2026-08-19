import { create } from 'zustand';

interface HelmState {
  epoch: number;
  bump: () => void;
  reset: () => void;
}

export const useHelmStore = create<HelmState>((set) => ({
  epoch: 0,
  bump: () => {
    set((state) => ({ epoch: state.epoch + 1 }));
  },
  reset: () => {
    set({ epoch: 0 });
  },
}));

export function useHelmEpoch(): number {
  return useHelmStore((state) => state.epoch);
}

export function bumpHelmEpoch(): void {
  useHelmStore.getState().bump();
}
