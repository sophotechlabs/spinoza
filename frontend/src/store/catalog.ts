import { create } from 'zustand';
import type { Category } from '../lib/types';

interface CatalogState {
  categories: Category[];
  counts: Partial<Record<string, number>>;
  remember: (categories: Category[]) => void;
  rememberCounts: (counts: Record<string, number>) => void;
  clear: () => void;
}

const NONE: Category[] = [];

const NO_COUNTS: Partial<Record<string, number>> = {};

export const useCatalogStore = create<CatalogState>((set) => ({
  categories: NONE,
  counts: NO_COUNTS,
  remember: (categories) => {
    set({ categories });
  },
  rememberCounts: (counts) => {
    set({ counts });
  },
  clear: () => {
    set({ categories: NONE, counts: NO_COUNTS });
  },
}));

export function useCategories(): Category[] {
  return useCatalogStore((state) => state.categories);
}

export function useCounts(): Partial<Record<string, number>> {
  return useCatalogStore((state) => state.counts);
}

export function rememberCatalog(categories: Category[]): void {
  useCatalogStore.getState().remember(categories);
}

export function rememberCounts(counts: Record<string, number>): void {
  useCatalogStore.getState().rememberCounts(counts);
}

export function clearCatalog(): void {
  useCatalogStore.getState().clear();
}
