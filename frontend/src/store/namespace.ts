import { create } from 'zustand';
import { namespaceStart } from './settings';
import { activeClusterNow, useActiveCluster } from './clusters';
import type { ByCluster } from './perCluster';
import { drop, held, put } from './perCluster';

export const ALL = '';

export const DEFAULT_NAMESPACE = 'default';

interface Scope {
  namespace: string;
  names: string[];
  narrowed: boolean;
  touched: boolean;
}

interface NamespaceState {
  byCluster: ByCluster<Scope>;
  asked: number;
  askAgain: () => void;
  choose: (namespace: string) => void;
  offer: (cluster: string, names: string[], narrowed?: boolean) => void;
  openOn: (context: string) => void;
  applyStart: (context: string) => void;
  forget: (cluster: string) => void;
  reset: () => void;
}

const NO_NAMES: string[] = [];

export function opensOn(cluster: string, context = ''): string {
  if (namespaceStart(cluster, context) === 'default') {
    return DEFAULT_NAMESPACE;
  }
  return ALL;
}

function fresh(): Scope {
  return { namespace: opensOn(''), names: NO_NAMES, narrowed: false, touched: false };
}

export function settle(wanted: string, names: string[], narrowed = false): string {
  if (names.length === 0) {
    return wanted;
  }
  if (names.includes(wanted)) {
    return wanted;
  }
  if (narrowed) {
    return names[0];
  }
  return ALL;
}

function change(state: NamespaceState, on: string, next: (scope: Scope) => Scope): NamespaceState {
  return {
    ...state,
    byCluster: put(state.byCluster, on, next(held(state.byCluster, on, fresh()))),
  };
}

export const useNamespaceStore = create<NamespaceState>((set) => ({
  byCluster: {},
  asked: 0,
  askAgain: () => {
    set((state) => ({ asked: state.asked + 1 }));
  },
  choose: (namespace) => {
    const on = activeClusterNow();
    set((state) => change(state, on, (scope) => ({ ...scope, namespace, touched: true })));
  },
  openOn: (context) => {
    const on = activeClusterNow();
    set((state) =>
      change(state, on, (scope) => {
        if (scope.touched) {
          return scope;
        }
        return { ...scope, namespace: opensOn(on, context) };
      }),
    );
  },
  applyStart: (context) => {
    const on = activeClusterNow();
    set((state) =>
      change(state, on, (scope) => ({
        ...scope,
        namespace: settle(opensOn(on, context), scope.names, scope.narrowed),
        touched: false,
      })),
    );
  },
  offer: (cluster, names, narrowed = false) => {
    set((state) =>
      change(state, cluster, (scope) => ({
        ...scope,
        names,
        narrowed,
        namespace: settle(scope.namespace, names, narrowed),
      })),
    );
  },
  forget: (cluster) => {
    set((state) => ({ byCluster: drop(state.byCluster, cluster) }));
  },
  reset: () => {
    set({ byCluster: {}, asked: 0 });
  },
}));

export function useNamespace(): string {
  const on = useActiveCluster();
  return useNamespaceStore((state) => held(state.byCluster, on, fresh()).namespace);
}

export function useNamespaceNames(): string[] {
  const on = useActiveCluster();
  return useNamespaceStore((state) => state.byCluster[on]?.names ?? NO_NAMES);
}

export function useNamespaceNarrowed(): boolean {
  const on = useActiveCluster();
  return useNamespaceStore((state) => state.byCluster[on]?.narrowed ?? false);
}

export function namespaceNow(): string {
  return held(useNamespaceStore.getState().byCluster, activeClusterNow(), fresh()).namespace;
}

export function forgetNamespace(cluster: string): void {
  useNamespaceStore.getState().forget(cluster);
}
