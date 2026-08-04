import { create } from 'zustand';
import type { DockSide, PanelId, Placement } from '../lib/panels';
import { DEFAULT_PLACEMENT, readPlacement, writePlacement } from '../lib/panels';

interface PanelsState {
  placement: Placement;
  move: (id: PanelId, side: DockSide) => void;
  reset: () => void;
}

export const usePanelsStore = create<PanelsState>((set, get) => ({
  placement: readPlacement(),
  move: (id, side) => {
    if (get().placement[id] === side) {
      return;
    }
    const placement = { ...get().placement, [id]: side };
    writePlacement(placement);
    set({ placement });
  },
  reset: () => {
    const placement = { ...DEFAULT_PLACEMENT };
    writePlacement(placement);
    set({ placement });
  },
}));
