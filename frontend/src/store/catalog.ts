import { create } from 'zustand';
import type { Category } from '../lib/types';
import { activeClusterNow, useActiveCluster } from './clusters';
import type { ByCluster } from './perCluster';
import { drop, held, put } from './perCluster';

interface CatalogState {
  categories: ByCluster<Category[]>;
  counts: ByCluster<Partial<Record<string, number>>>;
  remember: (categories: Category[]) => void;
  rememberCounts: (counts: Record<string, number>) => void;
  forget: (cluster: string) => void;
  clear: () => void;
}

const NONE: Category[] = [];

const NO_COUNTS: Partial<Record<string, number>> = {};

export const useCatalogStore = create<CatalogState>((set) => ({
  categories: {},
  counts: {},
  remember: (categories) => {
    const on = activeClusterNow();
    set((state) => ({ categories: put(state.categories, on, categories) }));
  },
  rememberCounts: (counts) => {
    const on = activeClusterNow();
    set((state) => ({ counts: put(state.counts, on, counts) }));
  },
  forget: (cluster) => {
    set((state) => ({
      categories: drop(state.categories, cluster),
      counts: drop(state.counts, cluster),
    }));
  },
  clear: () => {
    set({ categories: {}, counts: {} });
  },
}));

export function useCategories(): Category[] {
  const on = useActiveCluster();
  return useCatalogStore((state) => held(state.categories, on, NONE));
}

export function useCounts(): Partial<Record<string, number>> {
  const on = useActiveCluster();
  return useCatalogStore((state) => held(state.counts, on, NO_COUNTS));
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

export function forgetCatalog(cluster: string): void {
  useCatalogStore.getState().forget(cluster);
}
