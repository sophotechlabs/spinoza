import { create } from 'zustand';
import type { ObjectRef } from '../lib/types';

export const MAX_RECENTS = 15;

interface RecentsState {
  recents: ObjectRef[];
  remember: (ref: ObjectRef) => void;
  clear: () => void;
}

function keyOf(ref: ObjectRef): string {
  return `${ref.group}/${ref.version}/${ref.resource}/${ref.namespace}/${ref.name}`;
}

export const useRecentsStore = create<RecentsState>((set) => ({
  recents: [],
  remember: (ref) => {
    set((state) => {
      const key = keyOf(ref);
      const rest = state.recents.filter((one) => keyOf(one) !== key);
      return { recents: [ref, ...rest].slice(0, MAX_RECENTS) };
    });
  },
  clear: () => {
    set({ recents: [] });
  },
}));

export function useRecents(): ObjectRef[] {
  return useRecentsStore((state) => state.recents);
}

export function rememberObject(ref: ObjectRef): void {
  useRecentsStore.getState().remember(ref);
}

export function clearRecents(): void {
  useRecentsStore.getState().clear();
}
