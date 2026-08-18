import { create } from 'zustand';
import { namespaceStart } from './settings';

export const ALL = '';

export const DEFAULT_NAMESPACE = 'default';

interface NamespaceState {
  namespace: string;
  names: string[];
  choose: (namespace: string) => void;
  offer: (names: string[]) => void;
  reset: () => void;
}

export function opensOn(): string {
  if (namespaceStart() === 'default') {
    return DEFAULT_NAMESPACE;
  }
  return ALL;
}

export function settle(wanted: string, names: string[]): string {
  if (names.length === 0) {
    return wanted;
  }
  if (wanted === ALL || names.includes(wanted)) {
    return wanted;
  }
  return ALL;
}

export const useNamespaceStore = create<NamespaceState>((set, get) => ({
  namespace: opensOn(),
  names: [],
  choose: (namespace) => {
    set({ namespace });
  },
  offer: (names) => {
    const namespace = settle(get().namespace, names);
    set({ names, namespace });
  },
  reset: () => {
    set({ namespace: opensOn(), names: [] });
  },
}));

export function useNamespace(): string {
  return useNamespaceStore((state) => state.namespace);
}
