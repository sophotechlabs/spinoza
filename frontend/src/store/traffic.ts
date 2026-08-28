import { create } from 'zustand';
import type { TrafficSupport } from '../lib/types';

interface TrafficState {
  support: TrafficSupport;
  remember: (support: TrafficSupport) => void;
}

const UNKNOWN: TrafficSupport = { available: false };

export const useTrafficStore = create<TrafficState>((set) => ({
  support: UNKNOWN,
  remember: (support) => {
    set({ support });
  },
}));

export function useTrafficSupport(): TrafficSupport {
  return useTrafficStore((state) => state.support);
}

export function rememberTrafficSupport(support: TrafficSupport): void {
  useTrafficStore.getState().remember(support);
}
