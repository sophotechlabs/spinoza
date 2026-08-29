import { create } from 'zustand';
import type { ObjectRef } from '../lib/types';
import { activeClusterNow, useActiveCluster } from './clusters';
import type { ByCluster } from './perCluster';
import { drop, held, put } from './perCluster';

export const MAX_RECENTS = 15;

const NONE: ObjectRef[] = [];

interface RecentsState {
  byCluster: ByCluster<ObjectRef[]>;
  remember: (ref: ObjectRef) => void;
  forget: (cluster: string) => void;
  clear: () => void;
}

function keyOf(ref: ObjectRef): string {
  return `${ref.group}/${ref.version}/${ref.resource}/${ref.namespace}/${ref.name}`;
}

export const useRecentsStore = create<RecentsState>((set) => ({
  byCluster: {},
  remember: (ref) => {
    const on = activeClusterNow();
    set((state) => {
      const key = keyOf(ref);
      const rest = held(state.byCluster, on, NONE).filter((one) => keyOf(one) !== key);
      return { byCluster: put(state.byCluster, on, [ref, ...rest].slice(0, MAX_RECENTS)) };
    });
  },
  forget: (cluster) => {
    set((state) => ({ byCluster: drop(state.byCluster, cluster) }));
  },
  clear: () => {
    set({ byCluster: {} });
  },
}));

export function useRecents(): ObjectRef[] {
  const on = useActiveCluster();
  return useRecentsStore((state) => held(state.byCluster, on, NONE));
}

export function rememberObject(ref: ObjectRef): void {
  useRecentsStore.getState().remember(ref);
}

export function clearRecents(): void {
  useRecentsStore.getState().clear();
}

export function forgetRecents(cluster: string): void {
  useRecentsStore.getState().forget(cluster);
}
