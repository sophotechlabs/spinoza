import { create } from 'zustand';
import type { ContextList, Kubeconfig } from '../lib/types';

export const EMPTY_CONTEXTS: ContextList = {
  current: { kubeconfig: '', name: '' },
  kubeconfigs: [],
};

interface ContextsState {
  list: ContextList;
  setList: (list: ContextList) => void;
  reset: () => void;
}

export const useContextsStore = create<ContextsState>((set) => ({
  list: EMPTY_CONTEXTS,
  setList: (list) => {
    set({ list });
  },
  reset: () => {
    set({ list: EMPTY_CONTEXTS });
  },
}));

export function useContextList(): ContextList {
  return useContextsStore((state) => state.list);
}

export function unreadableCurrent(list: ContextList): Kubeconfig | null {
  if (list.current.name === '') {
    return null;
  }
  for (const entry of list.kubeconfigs) {
    if (entry.path === list.current.kubeconfig && entry.error !== undefined) {
      return entry;
    }
  }
  return null;
}

export function useUnreadableCurrent(): Kubeconfig | null {
  return useContextsStore((state) => unreadableCurrent(state.list));
}
