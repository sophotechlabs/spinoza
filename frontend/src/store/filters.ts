import { create } from 'zustand';
import type { Chip } from '../lib/filterChips';
import { chipKey } from '../lib/filterChips';
import { activeClusterNow, useActiveCluster } from './clusters';
import type { ByCluster } from './perCluster';
import { drop, held, put } from './perCluster';

const NONE: Chip[] = [];

type Held = Partial<Record<string, Chip[]>>;

const NO_CHIPS: Held = {};

interface FiltersState {
  byCluster: ByCluster<Held>;
  add: (key: string, chip: Chip) => void;
  removeAt: (key: string, index: number) => void;
  impose: (key: string, chips: Chip[]) => void;
  clearKind: (key: string) => void;
  forget: (cluster: string) => void;
  clear: () => void;
}

function without(chips: Held, key: string): Held {
  const next: Held = {};
  for (const [held_, list] of Object.entries(chips)) {
    if (held_ !== key) {
      next[held_] = list;
    }
  }
  return next;
}

function change(state: FiltersState, next: (chips: Held) => Held): Partial<FiltersState> {
  const on = activeClusterNow();
  return { byCluster: put(state.byCluster, on, next(held(state.byCluster, on, NO_CHIPS))) };
}

export const useFiltersStore = create<FiltersState>((set) => ({
  byCluster: {},
  add: (key, chip) => {
    set((state) =>
      change(state, (chips) => {
        const current = chips[key] ?? NONE;
        if (current.some((one) => chipKey(one) === chipKey(chip))) {
          return chips;
        }
        return { ...chips, [key]: [...current, chip] };
      }),
    );
  },
  removeAt: (key, index) => {
    set((state) =>
      change(state, (chips) => {
        const next = (chips[key] ?? NONE).filter((_one, at) => at !== index);
        if (next.length === 0) {
          return without(chips, key);
        }
        return { ...chips, [key]: next };
      }),
    );
  },
  impose: (key, chips) => {
    set((state) =>
      change(state, (current) => {
        if (chips.length === 0) {
          return without(current, key);
        }
        return { ...current, [key]: chips };
      }),
    );
  },
  clearKind: (key) => {
    set((state) => change(state, (chips) => without(chips, key)));
  },
  forget: (cluster) => {
    set((state) => ({ byCluster: drop(state.byCluster, cluster) }));
  },
  clear: () => {
    set({ byCluster: {} });
  },
}));

export function useChips(key: string): Chip[] {
  const on = useActiveCluster();
  return useFiltersStore((state) => held(state.byCluster, on, NO_CHIPS)[key] ?? NONE);
}

export function imposeChips(key: string, chips: Chip[]): void {
  useFiltersStore.getState().impose(key, chips);
}

export function clearFilters(): void {
  useFiltersStore.getState().clear();
}

export function forgetFilters(cluster: string): void {
  useFiltersStore.getState().forget(cluster);
}

export function chipsOf(key: string): Chip[] {
  const state = useFiltersStore.getState();
  return held(state.byCluster, activeClusterNow(), NO_CHIPS)[key] ?? NONE;
}
