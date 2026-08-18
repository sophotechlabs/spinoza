import { create } from 'zustand';
import type { Category } from '../lib/types';

interface CatalogState {
  categories: Category[];
  remember: (categories: Category[]) => void;
  clear: () => void;
}

const NONE: Category[] = [];

export const useCatalogStore = create<CatalogState>((set) => ({
  categories: NONE,
  remember: (categories) => {
    set({ categories });
  },
  clear: () => {
    set({ categories: NONE });
  },
}));

export function useCategories(): Category[] {
  return useCatalogStore((state) => state.categories);
}

export function rememberCatalog(categories: Category[]): void {
  useCatalogStore.getState().remember(categories);
}

export function clearCatalog(): void {
  useCatalogStore.getState().clear();
}
