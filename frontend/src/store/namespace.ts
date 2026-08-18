import { create } from 'zustand';
import { namespaceStart } from './settings';

export const ALL = '';

export const DEFAULT_NAMESPACE = 'default';

interface NamespaceState {
  namespace: string;
  names: string[];
  touched: boolean;
  choose: (namespace: string) => void;
  offer: (names: string[]) => void;
  openOn: (context: string) => void;
  reset: () => void;
}

export function opensOn(context: string): string {
  if (namespaceStart(context) === 'default') {
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
  namespace: opensOn(''),
  names: [],
  touched: false,
  choose: (namespace) => {
    set({ namespace, touched: true });
  },
  openOn: (context) => {
    if (get().touched) {
      return;
    }
    set({ namespace: opensOn(context) });
  },
  offer: (names) => {
    const namespace = settle(get().namespace, names);
    set({ names, namespace });
  },
  reset: () => {
    set({ namespace: opensOn(''), names: [], touched: false });
  },
}));

export function useNamespace(): string {
  return useNamespaceStore((state) => state.namespace);
}
