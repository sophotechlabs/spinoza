import { create } from 'zustand';
import type { PortForward } from '../lib/types';

interface ForwardsState {
  forwards: PortForward[];
  setForwards: (forwards: PortForward[]) => void;
}

export const useForwardsStore = create<ForwardsState>((set) => ({
  forwards: [],
  setForwards: (forwards) => {
    set({ forwards });
  },
}));
