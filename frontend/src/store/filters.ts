import { create } from 'zustand';
import type { Chip } from '../lib/filterChips';
import { chipKey } from '../lib/filterChips';

const NONE: Chip[] = [];

type Held = Partial<Record<string, Chip[]>>;

interface FiltersState {
  chips: Held;
  add: (key: string, chip: Chip) => void;
  removeAt: (key: string, index: number) => void;
  impose: (key: string, chips: Chip[]) => void;
  clearKind: (key: string) => void;
  clear: () => void;
}

function without(chips: Held, key: string): Held {
  const next: Held = {};
  for (const [held, list] of Object.entries(chips)) {
    if (held !== key) {
      next[held] = list;
    }
  }
  return next;
}

export const useFiltersStore = create<FiltersState>((set) => ({
  chips: {},
  add: (key, chip) => {
    set((state) => {
      const current = state.chips[key] ?? NONE;
      if (current.some((one) => chipKey(one) === chipKey(chip))) {
        return {};
      }
      return { chips: { ...state.chips, [key]: [...current, chip] } };
    });
  },
  removeAt: (key, index) => {
    set((state) => {
      const current = state.chips[key] ?? NONE;
      const next = current.filter((_one, at) => at !== index);
      if (next.length === 0) {
        return { chips: without(state.chips, key) };
      }
      return { chips: { ...state.chips, [key]: next } };
    });
  },
  impose: (key, chips) => {
    set((state) => {
      if (chips.length === 0) {
        return { chips: without(state.chips, key) };
      }
      return { chips: { ...state.chips, [key]: chips } };
    });
  },
  clearKind: (key) => {
    set((state) => ({ chips: without(state.chips, key) }));
  },
  clear: () => {
    set({ chips: {} });
  },
}));

export function useChips(key: string): Chip[] {
  return useFiltersStore((state) => state.chips[key] ?? NONE);
}

export function imposeChips(key: string, chips: Chip[]): void {
  useFiltersStore.getState().impose(key, chips);
}

export function clearFilters(): void {
  useFiltersStore.getState().clear();
}

export function chipsOf(key: string): Chip[] {
  return useFiltersStore.getState().chips[key] ?? NONE;
}
