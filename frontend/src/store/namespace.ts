import { create } from 'zustand';
import { readStored, writeStored } from '../lib/persist';

export const NAMESPACE_KEY = 'spinoza.namespace.v1';

export const ALL = '';

interface NamespaceState {
  namespace: string;
  names: string[];
  choose: (namespace: string) => void;
  offer: (names: string[]) => void;
}

function stored(): string {
  const kept = readStored(NAMESPACE_KEY);
  if (kept === null) {
    return ALL;
  }
  return kept;
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
  namespace: stored(),
  names: [],
  choose: (namespace) => {
    writeStored(NAMESPACE_KEY, namespace);
    set({ namespace });
  },
  offer: (names) => {
    const namespace = settle(get().namespace, names);
    set({ names, namespace });
  },
}));

export function useNamespace(): string {
  return useNamespaceStore((state) => state.namespace);
}
