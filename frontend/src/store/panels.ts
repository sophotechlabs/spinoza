import { create } from 'zustand';
import type { DockSide, Layout, PanelId, Placement } from '../lib/panels';
import {
  DEFAULT_LAYOUT,
  DEFAULT_PLACEMENT,
  readLayout,
  readPlacement,
  writeLayout,
  writePlacement,
} from '../lib/panels';

interface PanelsState {
  placement: Placement;
  sizes: Record<DockSide, number | null>;
  collapsed: Record<DockSide, boolean>;
  active: Record<DockSide, PanelId | null>;
  sidebar: number | null;
  move: (id: PanelId, side: DockSide) => void;
  resize: (side: DockSide, size: number) => void;
  resizeSidebar: (size: number) => void;
  collapse: (side: DockSide, collapsed: boolean) => void;
  activate: (side: DockSide, id: PanelId) => void;
  reset: () => void;
}

const stored = readLayout();

function layoutOf(state: PanelsState): Layout {
  return {
    sizes: state.sizes,
    collapsed: state.collapsed,
    active: state.active,
    sidebar: state.sidebar,
  };
}

export const usePanelsStore = create<PanelsState>((set, get) => ({
  placement: readPlacement(),
  sizes: stored.sizes,
  collapsed: stored.collapsed,
  active: stored.active,
  sidebar: stored.sidebar,
  move: (id, side) => {
    if (get().placement[id] === side) {
      return;
    }
    const placement = { ...get().placement, [id]: side };
    writePlacement(placement);
    set({ placement });
  },
  resize: (side, size) => {
    const sizes = { ...get().sizes, [side]: size };
    writeLayout({ ...layoutOf(get()), sizes });
    set({ sizes });
  },
  resizeSidebar: (size) => {
    writeLayout({ ...layoutOf(get()), sidebar: size });
    set({ sidebar: size });
  },
  collapse: (side, collapsed) => {
    const next = { ...get().collapsed, [side]: collapsed };
    writeLayout({ ...layoutOf(get()), collapsed: next });
    set({ collapsed: next });
  },
  activate: (side, id) => {
    const active = { ...get().active, [side]: id };
    writeLayout({ ...layoutOf(get()), active });
    set({ active });
  },
  reset: () => {
    const placement = { ...DEFAULT_PLACEMENT };
    const layout: Layout = {
      sizes: { ...DEFAULT_LAYOUT.sizes },
      collapsed: { ...DEFAULT_LAYOUT.collapsed },
      active: { ...DEFAULT_LAYOUT.active },
      sidebar: null,
    };
    writePlacement(placement);
    writeLayout(layout);
    set({ placement, ...layout });
  },
}));
